package control

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"aegis/capability"
	"github.com/z3r2ne/agentcore"
	"gorm.io/gorm"
)

// NativeDeliverySource provides the durable Aegis-side operations that a
// native AgentCore worker needs to finish an Issue. Workspace bytes are read
// from the Task container and streamed into attachment storage; there is no
// host workspace fallback.
type NativeDeliverySource struct{ Store *Store }

func (s NativeDeliverySource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Store == nil {
		return capability.Resolved{}, errors.New("native delivery: store is required")
	}
	issueID, _ := execution.Values[controlIssueIDValue].(string)
	runtimeID, _ := execution.Values["control.runtimeId"].(string)
	issueID, runtimeID = strings.TrimSpace(issueID), strings.TrimSpace(runtimeID)
	if issueID == "" || strings.TrimSpace(execution.ExecutionID) == "" || runtimeID == "" {
		return capability.Resolved{}, errors.New("native delivery: Issue, Execution and Task container identity are required")
	}
	issue, err := s.Store.GetIssue(issueID)
	if err != nil {
		return capability.Resolved{}, err
	}
	var durableExecution Execution
	if err = s.Store.db.First(&durableExecution, "id = ? AND issue_id = ?", execution.ExecutionID, issue.ID).Error; err != nil {
		return capability.Resolved{}, errors.New("native delivery: Execution does not belong to the Issue")
	}
	root := path.Clean(strings.TrimSpace(filepath.ToSlash(execution.Workspace)))
	if root == "." || !path.IsAbs(root) {
		return capability.Resolved{}, errors.New("native delivery: absolute container workspace is required")
	}
	binding := nativeDeliveryBinding{store: s.Store, issue: issue, execution: durableExecution, container: runtimeID, root: root}
	return capability.Resolved{
		Tools:        []agentcore.Tool{binding.publishAttachmentTool(), binding.reportProgressTool(), binding.submitFinalResultTool(), binding.submitBudgetSummaryTool()},
		Instructions: []capability.Instruction{{Source: "aegis-delivery", Content: "Publish every user-facing deliverable with aegis_publish_attachment before calling aegis_submit_final_result. Source files remain private in the Task container; only explicitly published attachments are copied into Aegis attachment storage. aegis_submit_budget_summary is reserved for the runtime-controlled budget summary phase and is rejected during normal work."}},
		Snapshot:     capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: ref.Version, Metadata: map[string]string{"workspace": root, "transport": "container-stream"}},
	}, nil
}

type nativeDeliveryBinding struct {
	store           *Store
	issue           Issue
	execution       Execution
	container, root string
}

func (b nativeDeliveryBinding) publishAttachmentTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_publish_attachment", Description: "Publish one regular file from the private Task container as a downloadable Aegis attachment. Archive a directory inside /workspace first, then publish the archive.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"name":{"type":"string"},"description":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		var input PublishAttachmentInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		target, err := b.target(input.Path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		sizeOutput, err := exec.CommandContext(ctx, "docker", "exec", b.container, "sh", "-c", `test -f "$1" && wc -c < "$1"`, "aegis-attachment-stat", target).CombinedOutput()
		if err != nil {
			return agentcore.ToolResult{}, fmt.Errorf("附件必须是任务 Workspace 中存在的普通文件: %s", truncate(strings.TrimSpace(string(sizeOutput)), 2048))
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(sizeOutput)), 10, 64)
		if err != nil || size < 0 || size > MaxAttachmentSize {
			return agentcore.ToolResult{}, fmt.Errorf("附件大小无效或超过 %d MB", MaxAttachmentSize>>20)
		}
		command := exec.CommandContext(ctx, "docker", "exec", b.container, "cat", "--", target)
		var content, stderr bytes.Buffer
		content.Grow(int(size))
		command.Stdout, command.Stderr = &content, &stderr
		if err = command.Run(); err != nil {
			return agentcore.ToolResult{}, fmt.Errorf("从任务容器读取附件失败: %s", truncate(strings.TrimSpace(stderr.String()), 2048))
		}
		if int64(content.Len()) != size {
			return agentcore.ToolResult{}, errors.New("从任务容器读取的附件内容不完整")
		}
		input.Path = target
		attachment, err := b.store.captureUploadedAttachment(b.issue, b.execution.ID, input, bytes.NewReader(content.Bytes()), size)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		b.store.addEvent(b.execution.ID, b.issue.ID, "attachment", "Agent 已发布附件", attachment.Name)
		b.store.notify()
		encoded, _ := json.Marshal(map[string]any{"attachmentId": attachment.ID, "name": attachment.Name, "size": attachment.Size, "published": true})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func (b nativeDeliveryBinding) reportProgressTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_report_progress", Description: "Persist a concise progress milestone for this execution.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"stage":{"type":"string","minLength":1,"maxLength":120},"summary":{"type":"string","minLength":1,"maxLength":4000},"currentActivity":{"type":"string","minLength":1,"maxLength":1000}},"required":["stage","summary","currentActivity"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input ReportExecutionProgressInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		input.Stage, input.Summary, input.CurrentActivity = strings.TrimSpace(input.Stage), strings.TrimSpace(input.Summary), strings.TrimSpace(input.CurrentActivity)
		if input.Stage == "" || input.Summary == "" || input.CurrentActivity == "" {
			return agentcore.ToolResult{}, errors.New("阶段、阶段总结和当前工作不能为空")
		}
		if utf8.RuneCountInString(input.Stage) > 120 || utf8.RuneCountInString(input.Summary) > 4000 || utf8.RuneCountInString(input.CurrentActivity) > 1000 {
			return agentcore.ToolResult{}, errors.New("进度字段超过长度限制")
		}
		now := time.Now()
		progress := ExecutionProgress{ID: nextID("progress"), ExecutionID: b.execution.ID, IssueID: b.issue.ID, Stage: input.Stage, Summary: input.Summary, CurrentActivity: input.CurrentActivity, CreatedAt: now}
		if err := b.store.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Create(&progress).Error; err != nil {
				return err
			}
			return tx.Model(&Execution{}).Where("id = ?", b.execution.ID).Updates(map[string]any{"checkpoint": "正在进行：" + truncate(input.CurrentActivity, 1900), "checkpoint_at": now, "updated_at": now}).Error
		}); err != nil {
			return agentcore.ToolResult{}, err
		}
		b.store.notify()
		return agentcore.TextToolResult("进度已记录。"), nil
	}}
}

func (b nativeDeliveryBinding) submitFinalResultTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_submit_final_result", Description: "Submit the standalone final result for this Issue and end the current Agent loop. Publish deliverable files first.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"body":{"type":"string","minLength":1,"maxLength":50000}},"required":["body"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		input.Body = strings.TrimSpace(input.Body)
		if input.Body == "" || utf8.RuneCountInString(input.Body) > 50000 {
			return agentcore.ToolResult{}, errors.New("最终结果正文为空或超过 50000 个字符")
		}
		var unfinished int64
		if err := b.store.db.Model(&Issue{}).Where("parent_id = ? AND status NOT IN ?", b.issue.ID, terminalIssueStatuses).Count(&unfinished).Error; err != nil {
			return agentcore.ToolResult{}, err
		}
		if unfinished > 0 {
			return agentcore.ToolResult{}, fmt.Errorf("仍有 %d 个直属子 Issue 未结束，不能提交最终结果", unfinished)
		}
		if err := b.store.db.Model(&Execution{}).Where("id = ? AND issue_id = ?", b.execution.ID, b.issue.ID).Updates(map[string]any{"final_result": input.Body, "final_result_submitted": true, "result": input.Body, "updated_at": time.Now()}).Error; err != nil {
			return agentcore.ToolResult{}, err
		}
		b.store.addEvent(b.execution.ID, b.issue.ID, "delivery", "Agent 已提交最终结果", "Native AgentCore 显式提交最终结果。")
		b.store.notify()
		result := agentcore.TextToolResult("最终结果已提交，当前 Agent loop 将结束并进入 Aegis 验收流程。")
		result.Terminate = true
		return result, nil
	}}
}

func (b nativeDeliveryBinding) submitBudgetSummaryTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "aegis_submit_budget_summary", Description: "Submit the structured partial-work summary after the runtime says this Execution exceeded its budget. This ends only the current over-budget Execution; it does not mark the Issue completed.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"body":{"type":"string","minLength":1,"maxLength":50000}},"required":["body"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input struct {
			Body string `json:"body"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		input.Body = strings.TrimSpace(input.Body)
		if input.Body == "" || utf8.RuneCountInString(input.Body) > 50000 {
			return agentcore.ToolResult{}, errors.New("预算总结正文为空或超过 50000 个字符")
		}
		var execution Execution
		if err := b.store.db.First(&execution, "id = ? AND issue_id = ?", b.execution.ID, b.issue.ID).Error; err != nil {
			return agentcore.ToolResult{}, err
		}
		if execution.BudgetPhase != "summarizing" {
			return agentcore.ToolResult{}, errors.New("当前 Execution 尚未进入预算总结阶段")
		}
		updated := b.store.db.Model(&Execution{}).Where("id = ? AND issue_id = ? AND budget_phase = ?", b.execution.ID, b.issue.ID, "summarizing").Updates(map[string]any{
			"result": input.Body, "checkpoint": "预算总结已提交", "checkpoint_at": time.Now(), "updated_at": time.Now(),
		})
		if updated.Error != nil {
			return agentcore.ToolResult{}, updated.Error
		}
		if updated.RowsAffected != 1 {
			return agentcore.ToolResult{}, errors.New("预算总结阶段已经结束")
		}
		b.store.addEvent(b.execution.ID, b.issue.ID, "budget", "Agent 已提交预算总结", "当前超预算 Execution 将结束；Issue 仍由预算结果状态表达。")
		b.store.notify()
		result := agentcore.TextToolResult("预算总结已提交，当前 Agent loop 将结束。")
		result.Terminate = true
		return result, nil
	}}
}

func (b nativeDeliveryBinding) target(value string) (string, error) {
	value = strings.TrimSpace(filepath.ToSlash(value))
	if value == "" {
		return "", errors.New("附件路径不能为空")
	}
	target := path.Clean(value)
	if !path.IsAbs(target) {
		target = path.Join(b.root, target)
	}
	if target != b.root && !strings.HasPrefix(target, b.root+"/") {
		return "", errors.New("附件路径不能离开任务 Workspace")
	}
	return target, nil
}

var _ capability.Source = NativeDeliverySource{}

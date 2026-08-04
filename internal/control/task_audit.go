package control

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aegis/agenthost"
	"aegis/capability"
	"github.com/z3r2ne/agentcore"
	"gorm.io/gorm"
)

const taskAuditFrameworkVersion = "agent-evaluation-framework/Draft-v1"

const taskAuditSystemPrompt = `你是 Aegis 的独立任务分析 Agent。你不参与任务执行，也不能修改任务；你只根据冻结的 aegis.task-evidence/v1 证据进行审计。

必须遵循 docs/architecture/agent-evaluation-framework.md（Draft v1）：先确定性事实、再规则、最后语义判断；不能让主观判断覆盖明确失败；所有问题必须引用 taskId/issueId/executionId、时间和证据文件；标记首次失效点并区分主根因与促成因素。

最终只输出一份独立、可下载的中文 Markdown 审计报告，至少包括：
1. 一页结论：任务是否成功、硬性门禁、任务执行质量/100、平台健康度/100、综合等级与置信度；
2. Requirement ledger：逐项 PASS/FAIL/UNPROVEN、依据与证据引用；
3. 关键时间线；
4. Issue 树：覆盖、重叠、粒度、依赖、角色和关键路径；
5. Agent 执行、工具、Phone/Board、父子信息流和最终整合；
6. 效率分解：work、coordination、validation、idle、retry、预算和恢复；
7. 首次失效点、硬门禁、缺陷清单（严重度、reason code、预期/实际、影响、主根因、促成因素）；
8. 分层整改：短期 Prompt/规则、中期契约/工具、长期架构，并为每项给出复测方法；
9. 证据局限与未能证明的事项。

两个分数必须独立。任务质量权重为 25/15/15/15/10/15/5；平台健康权重为 20/15/15/10/10/10/10/10；综合分仅为 0.65×任务质量+0.35×平台健康。任一安全授权、结果真实性、目标完整性、证据完整性、状态一致性、审计可追溯门禁失败，最高只能判“不合格”。不要编造缺失数据，也不要仅复述 Agent 自述。

先用 task_evidence_list 了解全部证据，再按需用 task_evidence_read 分页读取。优先读取 manifest、scope、evaluation_context、issues、executions、conversations、validations、事件/进度/评论/Relay、coordination、Phone、日志和附件清单。证据量大时应抽取关键路径并明确采样边界。`

type taskAuditConfigurer struct{}

func (taskAuditConfigurer) ConfigureAgent(_ context.Context, _ agenthost.ExecutionSpec, config *agentcore.Config) error {
	config.MaxTurns = 80
	config.MaxToolConcurrency = 2
	config.ContextPolicy.MaxToolResultBytes = 64 * 1024
	return nil
}

// TaskEvidenceSource exposes a frozen evidence ZIP through bounded read-only
// tools. The model can inspect the complete archive without placing it all in
// one context window.
type TaskEvidenceSource struct{ Store *Store }

func (s TaskEvidenceSource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Store == nil {
		return capability.Resolved{}, errors.New("task evidence: store is required")
	}
	auditID, _ := execution.Values["control.taskAuditId"].(string)
	var audit TaskAudit
	if err := s.Store.db.First(&audit, "id = ?", strings.TrimSpace(auditID)).Error; err != nil {
		return capability.Resolved{}, fmt.Errorf("task evidence: load audit: %w", err)
	}
	reader := taskAuditEvidenceReader{path: audit.EvidencePath}
	return capability.Resolved{
		Tools:        []agentcore.Tool{reader.listTool(), reader.readTool()},
		Instructions: []capability.Instruction{{Source: "task-evidence", Content: "The task evidence tools are read-only and access the immutable snapshot for this audit only."}},
		Snapshot:     capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: taskEvidenceSchemaVersion, Metadata: map[string]string{"auditId": audit.ID, "exportId": audit.EvidenceExportID}},
	}, nil
}

type taskAuditEvidenceReader struct{ path string }

func (r taskAuditEvidenceReader) listTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_evidence_list", Description: "List every file in the immutable task evidence archive with size and content type.",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, _ json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		archive, err := zip.OpenReader(r.path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer archive.Close()
		type item struct {
			Name string `json:"name"`
			Size uint64 `json:"size"`
		}
		items := make([]item, 0, len(archive.File))
		for _, file := range archive.File {
			items = append(items, item{Name: file.Name, Size: file.UncompressedSize64})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		encoded, _ := json.Marshal(items)
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func (r taskAuditEvidenceReader) readTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_evidence_read", Description: "Read a UTF-8 evidence file by byte offset. Use the returned nextOffset until eof=true. Maximum 48000 bytes per call.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":48000}},"required":["path"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input struct {
			Path   string `json:"path"`
			Offset int64  `json:"offset"`
			Limit  int64  `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if input.Limit <= 0 {
			input.Limit = 32000
		}
		if input.Limit > 48000 {
			input.Limit = 48000
		}
		archive, err := zip.OpenReader(r.path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer archive.Close()
		var target *zip.File
		for _, file := range archive.File {
			if file.Name == input.Path {
				target = file
				break
			}
		}
		if target == nil || strings.HasSuffix(target.Name, "/") {
			return agentcore.ToolResult{}, errors.New("evidence file not found")
		}
		if target.UncompressedSize64 > 0 && input.Offset >= int64(target.UncompressedSize64) {
			encoded, _ := json.Marshal(map[string]any{"path": input.Path, "offset": input.Offset, "nextOffset": input.Offset, "eof": true, "content": ""})
			return agentcore.TextToolResult(string(encoded)), nil
		}
		opened, err := target.Open()
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer opened.Close()
		if input.Offset > 0 {
			if _, err = io.CopyN(io.Discard, opened, input.Offset); err != nil {
				return agentcore.ToolResult{}, err
			}
		}
		data, err := io.ReadAll(io.LimitReader(opened, input.Limit))
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		if !utf8.Valid(data) {
			return agentcore.ToolResult{}, errors.New("evidence file is binary; use attachments.json metadata instead")
		}
		next := input.Offset + int64(len(data))
		encoded, _ := json.Marshal(map[string]any{"path": input.Path, "offset": input.Offset, "nextOffset": next, "eof": uint64(next) >= target.UncompressedSize64, "content": string(data)})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func (m *Manager) CreateTaskAudit(ctx context.Context, requestedID string) (TaskAudit, error) {
	if m == nil || m.store == nil {
		return TaskAudit{}, errors.New("task audit: manager is unavailable")
	}
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return TaskAudit{}, errors.New("task audit: task ID is required")
	}
	if _, _, err := m.taskAuditRuntime(); err != nil {
		return TaskAudit{}, err
	}
	var scope TaskEvidenceScope
	if err := m.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resolved, resolveErr := resolveTaskExportScope(tx, requestedID)
		scope = resolved
		return resolveErr
	}); err != nil {
		return TaskAudit{}, err
	}
	rootIDs := make([]string, len(scope.RootIssues))
	for index, root := range scope.RootIssues {
		rootIDs[index] = root.ID
	}
	taskID := ""
	if scope.Task != nil {
		taskID = scope.Task.ID
	}
	now := time.Now()
	audit := TaskAudit{ID: nextID("task-audit"), TaskID: taskID, RequestedID: requestedID, RootIssueIDs: rootIDs, Status: "preparing", FrameworkVersion: taskAuditFrameworkVersion, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(m.store.dataDir, "task-audits", audit.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TaskAudit{}, err
	}
	audit.EvidencePath = filepath.Join(dir, "evidence.zip")
	if err := m.store.db.Create(&audit).Error; err != nil {
		_ = os.RemoveAll(dir)
		return TaskAudit{}, err
	}
	m.store.notify()
	go m.prepareAndRunTaskAudit(audit.ID)
	return audit, nil
}

func (m *Manager) ListTaskAudits(requestedID string) ([]TaskAudit, error) {
	var audits []TaskAudit
	query := m.store.db.Where("requested_id = ? OR task_id = ?", requestedID, requestedID)
	var issue Issue
	if err := m.store.db.First(&issue, "id = ?", requestedID).Error; err == nil && issue.TaskSourceID != "" {
		query = m.store.db.Where("requested_id = ? OR task_id = ?", requestedID, issue.TaskSourceID)
	}
	err := query.Order("created_at desc").Find(&audits).Error
	return audits, err
}

func (m *Manager) GetTaskAudit(id string) (TaskAudit, error) {
	var audit TaskAudit
	return audit, m.store.db.First(&audit, "id = ?", id).Error
}

func (m *Manager) taskAuditRuntime() (*agenthost.Host, Config, error) {
	m.nativeMu.RLock()
	host := m.nativeHost
	m.nativeMu.RUnlock()
	if host == nil {
		return nil, Config{}, errors.New("task audit: AgentHost is unavailable")
	}
	cfg := m.store.Config()
	if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, Config{}, errors.New("task audit: model is not configured")
	}
	return host, cfg, nil
}

func (m *Manager) runTaskAudit(id string) {
	host, cfg, err := m.taskAuditRuntime()
	if err != nil {
		m.failTaskAudit(id, err)
		return
	}
	now := time.Now()
	_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "running", "started_at": now, "updated_at": now}).Error
	m.store.notify()
	var mu sync.Mutex
	current := ""
	sink := func(_ context.Context, event agentcore.Event) error {
		mu.Lock()
		defer mu.Unlock()
		switch event.Type {
		case agentcore.EventMessageStart:
			if event.Message != nil && event.Message.Role == agentcore.RoleAssistant {
				current = ""
			}
		case agentcore.EventMessageUpdate:
			if event.Delta != nil && event.Delta.TextDelta != "" {
				current += event.Delta.TextDelta
				_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"report_markdown": current, "updated_at": time.Now()}).Error
				m.store.notify()
			}
		case agentcore.EventMessageEnd:
			if event.Message != nil && event.Message.Role == agentcore.RoleAssistant && strings.TrimSpace(event.Message.Text()) != "" {
				current = event.Message.Text()
				_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"report_markdown": current, "updated_at": time.Now()}).Error
				m.store.notify()
			}
		}
		return nil
	}
	options := map[string]any{}
	if strings.TrimSpace(cfg.Thinking) != "" {
		options["thinking"] = cfg.Thinking
	}
	result, runErr := host.Run(context.Background(), agenthost.ExecutionSpec{ExecutionID: id, SessionID: id, AgentID: "task-analysis-agent", Model: agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options}, SystemPrompt: taskAuditSystemPrompt, Prompt: "审计这份冻结的任务证据。完整检查证据后，严格按评估框架生成最终 Markdown 报告。", Capabilities: []capability.Ref{{Kind: capability.KindTool, Name: "task-evidence"}}, Values: map[string]any{"control.taskAuditId": id}, Runtime: taskAuditConfigurer{}}, sink)
	if runErr != nil {
		m.failTaskAudit(id, runErr)
		return
	}
	report := strings.TrimSpace(current)
	if final := strings.TrimSpace(lastAssistantText(result.Core.State.Messages)); final != "" {
		report = final
	}
	if report == "" {
		m.failTaskAudit(id, errors.New("task audit: evaluator returned an empty report"))
		return
	}
	completed := time.Now()
	_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "completed", "report_markdown": report, "input_tokens": result.Core.Usage.InputTokens, "output_tokens": result.Core.Usage.OutputTokens, "completed_at": completed, "updated_at": completed, "error": ""}).Error
	var audit TaskAudit
	if m.store.db.First(&audit, "id = ?", id).Error == nil {
		_ = os.WriteFile(filepath.Join(filepath.Dir(audit.EvidencePath), "report.md"), []byte(report), 0o600)
	}
	m.store.notify()
}

func (m *Manager) prepareAndRunTaskAudit(id string) {
	var audit TaskAudit
	if err := m.store.db.First(&audit, "id = ?", id).Error; err != nil {
		return
	}
	file, err := os.OpenFile(audit.EvidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		m.failTaskAudit(id, err)
		return
	}
	manifest, exportErr := m.WriteTaskEvidence(context.Background(), audit.RequestedID, file, TaskExportOptions{IncludeArtifacts: true, RedactSecrets: true})
	closeErr := file.Close()
	if exportErr != nil || closeErr != nil {
		m.failTaskAudit(id, errors.Join(exportErr, closeErr))
		return
	}
	manifestJSON, _ := json.Marshal(manifest)
	rootIDsJSON, _ := json.Marshal(manifest.RootIssueIDs)
	now := time.Now()
	if err = m.store.db.Model(&TaskAudit{}).Where("id = ? AND status = ?", id, "preparing").Updates(map[string]any{
		"task_id": manifest.TaskID, "root_issue_ids": string(rootIDsJSON), "evidence_export_id": manifest.ExportID,
		"evidence_complete": manifest.Complete, "evidence_manifest": string(manifestJSON), "updated_at": now,
	}).Error; err != nil {
		m.failTaskAudit(id, err)
		return
	}
	m.store.notify()
	m.runTaskAudit(id)
}

func (m *Manager) failTaskAudit(id string, runErr error) {
	now := time.Now()
	_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "failed", "error": truncate(runErr.Error(), 8000), "completed_at": now, "updated_at": now}).Error
	m.store.notify()
}

var _ capability.Source = TaskEvidenceSource{}
var _ agenthost.AgentConfigurer = taskAuditConfigurer{}

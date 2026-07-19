package control

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

func encodeEventPayload(value any) string {
	if value == nil {
		return ""
	}
	data, err := json.Marshal(value)
	if err != nil {
		data, _ = json.Marshal(map[string]any{"text": fmt.Sprint(value)})
	}
	return string(data)
}

func toolEventTitle(tool string) string {
	switch tool {
	case "read":
		return "读取文件"
	case "write":
		return "写入文件"
	case "edit":
		return "编辑文件"
	case "bash":
		return "执行命令"
	case "aegis_create_subissues":
		return "创建子 Issues"
	case "aegis_report_progress":
		return "更新工作进度"
	case "aegis_broadcast":
		return "发送任务广播"
	case "aegis_list_broadcasts":
		return "读取广播历史"
	case "aegis_uncover_search":
		return "检索网络空间资产"
	default:
		if strings.TrimSpace(tool) == "" {
			return "工具调用"
		}
		return "调用 " + tool
	}
}

func (s *Store) startToolEvent(executionID, issueID, toolCallID, toolName string, input any) {
	now := time.Now()
	event := ExecutionEvent{
		ID: nextID("event"), ExecutionID: executionID, IssueID: issueID,
		Type: "tool", Title: toolEventTitle(toolName), ToolCallID: toolCallID,
		ToolName: toolName, Status: "running", InputJSON: encodeEventPayload(input),
		CreatedAt: now, UpdatedAt: now,
	}
	_ = s.db.Create(&event).Error
	s.notify()
}

func (s *Store) finishToolEvent(executionID, issueID, toolCallID, toolName string, output any, isError bool) {
	now := time.Now()
	status := "completed"
	if isError {
		status = "failed"
	}
	updates := map[string]any{
		"tool_name": toolName, "title": toolEventTitle(toolName), "status": status,
		"output_json": encodeEventPayload(output), "is_error": isError, "updated_at": now,
	}
	result := s.db.Model(&ExecutionEvent{}).
		Where("execution_id = ? AND tool_call_id = ? AND type = ?", executionID, toolCallID, "tool").
		Updates(updates)
	if result.Error == nil && result.RowsAffected == 0 {
		_ = s.db.Create(&ExecutionEvent{
			ID: nextID("event"), ExecutionID: executionID, IssueID: issueID,
			Type: "tool", Title: toolEventTitle(toolName), ToolCallID: toolCallID,
			ToolName: toolName, Status: status, OutputJSON: encodeEventPayload(output),
			IsError: isError, CreatedAt: now, UpdatedAt: now,
		}).Error
	}
	s.notify()
}

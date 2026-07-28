package control

import (
	"encoding/json"
	"strings"
)

func compactIssueForState(issue Issue) Issue {
	issue.Context = ""
	issue.Constraints = ""
	issue.Result = ""
	return issue
}

func compactExecution(execution Execution) Execution {
	execution.InitialPrompt = ""
	execution.SystemPrompt = ""
	execution.ToolsSnapshot = []ToolSnapshot{}
	execution.Result = ""
	return execution
}

func compactExecutionEvent(event ExecutionEvent) ExecutionEvent {
	event.Detail = truncate(strings.TrimSpace(event.Detail), 800)
	if event.Type != "tool" {
		return event
	}
	var input map[string]any
	_ = json.Unmarshal([]byte(event.InputJSON), &input)
	compact := map[string]any{}
	copyKeys := func(keys ...string) {
		for _, key := range keys {
			if value, exists := input[key]; exists {
				compact[key] = value
			}
		}
	}
	copyKeys("description")
	switch event.ToolName {
	case "read":
		copyKeys("path", "offset", "limit")
	case "write":
		copyKeys("path")
		compact["_lineCount"] = textLineCount(stringValue(input["content"]))
	case "edit":
		copyKeys("path")
		if edits, ok := input["edits"].([]any); ok {
			compact["_editCount"] = len(edits)
			lines := 0
			for _, raw := range edits {
				if edit, ok := raw.(map[string]any); ok {
					lines += textLineCount(stringValue(edit["newText"]))
				}
			}
			compact["_lineCount"] = lines
		}
	case "bash":
		compact["command"] = truncate(stringValue(input["command"]), 600)
		copyKeys("timeout")
	case "aegis_create_subissues":
		if children, ok := input["children"].([]any); ok {
			compact["_childCount"] = len(children)
		}
	case "aegis_report_progress":
		copyKeys("stage", "summary", "currentActivity")
	case "aegis_get_issue_progress":
		copyKeys("sessionId", "mode", "progressLimit", "messageLimit")
	case "aegis_uncover_search":
		copyKeys("engine", "query", "limit", "format", "field", "timeout")
	case "grep":
		copyKeys("pattern", "path", "glob", "ignoreCase", "literal", "context", "limit")
	case "find", "ls":
		copyKeys("pattern", "path", "limit")
	default:
		copyKeys("path", "query", "attachmentId", "offset", "limit")
	}
	event.InputJSON = encodeEventPayload(compact)
	event.OutputJSON = ""
	return event
}

func textLineCount(value string) int {
	if value == "" {
		return 0
	}
	return strings.Count(value, "\n") + 1
}

package control

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

type conciergeCaptureWriter struct {
	bytes.Buffer
}

func (w *conciergeCaptureWriter) Close() error { return nil }

func TestConciergeConversationIsDurableAndHiddenFromWorkState(t *testing.T) {
	store := configuredStore(t)
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	if conversation.ExecutionID == "" || conversation.Status != "idle" {
		t.Fatalf("unexpected conversation: %+v", conversation)
	}
	detail, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !detail.Execution.ToolsSnapshot[0].Parameters[0].Required || detail.Execution.Kind != "concierge" {
		t.Fatalf("unexpected concierge execution: %+v", detail.Execution)
	}
	state := store.State()
	for _, issue := range state.Issues {
		if issue.ID == detail.Conversation.IssueID {
			t.Fatal("hidden concierge issue leaked into work state")
		}
	}
	for _, execution := range state.Executions {
		if execution.ID == conversation.ExecutionID {
			t.Fatal("concierge execution leaked into Sessions state")
		}
	}

	reopened, err := NewStore(store.DataDir())
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := reopened.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Conversation.ID != conversation.ID || reloaded.Execution.SessionID != detail.Execution.SessionID {
		t.Fatalf("conversation did not survive reopen: %+v", reloaded)
	}
}

func TestConciergeAgentUsesOnlyControlledConversationTools(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent(conciergeAgentID)
	if err != nil {
		t.Fatal(err)
	}
	if agent.Internal || agent.Category != "concierge" {
		t.Fatalf("unexpected concierge identity: %+v", agent)
	}
	wanted := map[string]bool{
		"aegis_create_task": true,
		"aegis_get_memo":    true,
		"aegis_update_memo": true,
	}
	if len(agent.Tools) != len(wanted) {
		t.Fatalf("concierge tools=%v", agent.Tools)
	}
	for _, tool := range agent.Tools {
		if !wanted[tool] {
			t.Fatalf("unexpected concierge tool %q", tool)
		}
	}
	if agent.Permissions.AllowShell || agent.Permissions.AllowWrite || agent.Permissions.AllowNetwork {
		t.Fatalf("concierge permissions are too broad: %+v", agent.Permissions)
	}
}

func TestConciergeSystemPromptIncludesOnlyAssignableAgentRoster(t *testing.T) {
	prompt := conciergeRosterSystemPrompt("You are the concierge.", []AgentDefinition{
		{ID: conciergeAgentID, Name: "管家", Enabled: true, Description: "不应出现在名册中"},
		{ID: "frontend-engineer", Name: "前端工程师", Category: "frontend", Enabled: true, Description: "负责 React UI"},
		{ID: "disabled-agent", Name: "已停用", Enabled: false, Description: "不应出现在名册中"},
		{ID: "acceptance-validator", Name: "验收", Enabled: true, Internal: true, Description: "不应出现在名册中"},
	})
	for _, expected := range []string{"You are the concierge.", "frontend-engineer", "前端工程师", "负责 React UI"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("runtime prompt missing %q: %s", expected, prompt)
		}
	}
	for _, excluded := range []string{"disabled-agent", "acceptance-validator", "不应出现在名册中"} {
		if strings.Contains(prompt, excluded) {
			t.Fatalf("runtime prompt leaked unavailable Agent %q: %s", excluded, prompt)
		}
	}
}

func TestConciergeTaskInputLimitsObjective(t *testing.T) {
	valid := CreateConciergeTaskInput{
		Title: "实现登录页面", Description: "完成页面与接口联调。",
		Objective: "登录页面通过浏览器验收。",
	}
	if err := validateConciergeTaskInput(valid); err != nil {
		t.Fatalf("valid concierge task input: %v", err)
	}
	invalid := valid
	invalid.Objective = strings.Repeat("验", 20001)
	if err := validateConciergeTaskInput(invalid); err == nil || !strings.Contains(err.Error(), "任务目标") {
		t.Fatalf("expected objective length error, got %v", err)
	}
}

func TestConciergeSessionSendsOnlyRawOperatorMessage(t *testing.T) {
	store := configuredStore(t)
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	writer := &conciergeCaptureWriter{}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{
		manager: manager, executionID: detail.Execution.ID, issueID: conversation.IssueID,
		agentID: conciergeAgentID, kind: "concierge", stdin: writer,
	}
	operatorMessage := "请开发一个 React 管理后台"
	if _, err = manager.sendSessionPrompt(session, operatorMessage); err != nil {
		t.Fatal(err)
	}
	var rpc map[string]any
	if err = json.Unmarshal(bytes.TrimSpace(writer.Bytes()), &rpc); err != nil {
		t.Fatal(err)
	}
	runtimeMessage, _ := rpc["message"].(string)
	if runtimeMessage != operatorMessage {
		t.Fatalf("Pi runtime message contains injected context: %q", runtimeMessage)
	}
	var stored Message
	if err = store.db.Where("execution_id = ? AND role = ?", detail.Execution.ID, "user").Order("created_at desc, id desc").First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.Content != operatorMessage || strings.Contains(stored.Content, "aegis_available_agents") {
		t.Fatalf("visible user message contains internal roster context: %q", stored.Content)
	}
}

func TestConciergeProviderErrorIsPersistedAndCanRecover(t *testing.T) {
	store := configuredStore(t)
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{
		manager: manager, key: detail.Execution.ID, executionID: detail.Execution.ID,
		issueID: detail.Conversation.IssueID, agentID: conciergeAgentID,
		kind: "concierge", stdin: &trackedWriteCloser{},
	}

	manager.handleRPCLine(session, []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"error","errorMessage":"500: Internal server error"}}`))
	manager.handleRPCLine(session, []byte(`{"type":"agent_settled"}`))

	failed, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if failed.Conversation.Status != "error" || failed.Execution.Status != "idle" {
		t.Fatalf("provider error left unexpected state: conversation=%s execution=%s", failed.Conversation.Status, failed.Execution.Status)
	}
	if !strings.Contains(failed.Execution.Error, "500") || len(failed.Messages) != 1 || failed.Messages[0].Role != "assistant" || !strings.Contains(failed.Messages[0].Content, "模型服务返回错误") {
		t.Fatalf("provider error was not visible in chat: execution=%+v messages=%+v", failed.Execution, failed.Messages)
	}

	if _, err = manager.sendSessionPrompt(session, "请重试"); err != nil {
		t.Fatal(err)
	}
	manager.handleRPCLine(session, []byte(`{"type":"agent_start"}`))
	manager.handleRPCLine(session, []byte(`{"type":"message_update","assistantMessageEvent":{"type":"text_delta","delta":"现在可以正常响应。"}}`))
	manager.handleRPCLine(session, []byte(`{"type":"message_end","message":{"role":"assistant","stopReason":"stop"}}`))
	manager.handleRPCLine(session, []byte(`{"type":"agent_settled"}`))

	recovered, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.Conversation.Status != "idle" || recovered.Execution.Error != "" || recovered.Execution.Result != "现在可以正常响应。" {
		t.Fatalf("concierge did not recover after a later successful response: %+v", recovered)
	}
}

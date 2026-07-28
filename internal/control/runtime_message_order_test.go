package control

import "testing"

func TestSteerSealsStreamingAssistantMessageBeforeUserMessage(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Long-running employee chat", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "employee_chat")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	session := &PiSession{
		manager: manager, executionID: execution.ID, issueID: issue.ID,
		agentID: execution.AgentID, kind: execution.Kind, stdin: &trackedWriteCloser{},
	}
	session.busy.Store(true)

	manager.appendAssistantDelta(session, "AI 在用户插话前的内容。")
	if _, err = manager.sendSessionPrompt(session, "请和团队一起并行处理"); err != nil {
		t.Fatal(err)
	}
	manager.appendAssistantDelta(session, "AI 收到插话后的团队部署。")

	var messages []Message
	if err = store.db.Where("execution_id = ?", execution.ID).Order("created_at asc, id asc").Find(&messages).Error; err != nil {
		t.Fatal(err)
	}
	if len(messages) != 3 {
		t.Fatalf("messages=%+v, want assistant/user/assistant segments", messages)
	}
	if messages[0].Role != "assistant" || messages[0].Content != "AI 在用户插话前的内容。" || messages[0].Streaming {
		t.Fatalf("first assistant segment was not sealed: %+v", messages[0])
	}
	if messages[1].Role != "user" || messages[1].Content != "请和团队一起并行处理" {
		t.Fatalf("user steer is out of order: %+v", messages[1])
	}
	if messages[2].Role != "assistant" || messages[2].Content != "AI 收到插话后的团队部署。" || !messages[2].Streaming {
		t.Fatalf("post-steer assistant segment is out of order: %+v", messages[2])
	}
}

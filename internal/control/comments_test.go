package control

import (
	"slices"
	"strings"
	"testing"
)

func TestOperatorCommentWakesIssueAssignee(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Review API behavior", Priority: "medium", WorkMode: "guided", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	comment, err := manager.AddIssueComment(issue.ID, "Please verify the response contract.")
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var wakeups []AgentWakeup
	if err = store.db.Where("comment_id = ?", comment.ID).Find(&wakeups).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	if len(wakeups) != 1 || wakeups[0].AgentID != "backend-engineer" || wakeups[0].Reason != "issue_comment_assignee" {
		manager.scheduleMu.Unlock()
		t.Fatalf("unexpected assignee wakeups: %+v", wakeups)
	}
	prompt, err := wakeupPrompt(issue, wakeups[0], store.Agents(), store.db)
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	if !strings.Contains(prompt, comment.Body) {
		manager.scheduleMu.Unlock()
		t.Fatalf("wakeup prompt does not contain comment body: %s", prompt)
	}
	store.db.Delete(&AgentWakeup{}, "comment_id = ?", comment.ID)
	manager.scheduleMu.Unlock()
}

func TestOperatorCommentDeduplicatesAssigneeAndMentions(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Coordinate implementation", Priority: "medium", WorkMode: "guided", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.scheduleMu.Lock()
	body := "[@后端](agent://backend-engineer) 和 [@前端](agent://frontend-engineer) 请一起复核。"
	comment, err := manager.AddIssueComment(issue.ID, body)
	if err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	var wakeups []AgentWakeup
	if err = store.db.Where("comment_id = ?", comment.ID).Order("created_at asc").Find(&wakeups).Error; err != nil {
		manager.scheduleMu.Unlock()
		t.Fatal(err)
	}
	agentIDs := make([]string, len(wakeups))
	for index, wakeup := range wakeups {
		agentIDs[index] = wakeup.AgentID
	}
	if len(wakeups) != 2 || !slices.Contains(agentIDs, "backend-engineer") || !slices.Contains(agentIDs, "frontend-engineer") {
		manager.scheduleMu.Unlock()
		t.Fatalf("unexpected deduplicated wakeups: %+v", wakeups)
	}
	store.db.Delete(&AgentWakeup{}, "comment_id = ?", comment.ID)
	manager.scheduleMu.Unlock()
}

package control

import (
	"slices"
	"testing"
	"time"
)

func TestTaskTimelineAggregatesWholeIssueTree(t *testing.T) {
	store := configuredStore(t)
	task, err := store.CreateIssue(CreateIssueInput{
		Title: "Timeline task", Objective: "All important task events are visible.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	child, err := store.CreateIssue(CreateIssueInput{
		ParentID: task.ID, Title: "Child work", Objective: "The child result is recorded.",
		Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "frontend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateIssue(CreateIssueInput{
		Title: "Other task", Objective: "Remain outside the selected timeline.",
		Priority: "low", WorkMode: "autonomous",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(child, "frontend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	finished := time.Now().Add(time.Second)
	if err = store.updateExecution(execution.ID, map[string]any{
		"status": "completed", "result": "UI build and tests passed.", "finished_at": finished,
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&IssueComment{
		ID: nextID("comment"), IssueID: child.ID, AuthorType: "operator",
		AuthorID: "operator", Body: "Please include the test evidence.", CreatedAt: time.Now().Add(2 * time.Second),
	}).Error; err != nil {
		t.Fatal(err)
	}
	store.addEvent(execution.ID, child.ID, "delegation", "Agent 已拆分子 Issues", "已创建 2 个子 Issues。")
	if err = store.db.Create(&IssueComment{
		ID: nextID("comment"), IssueID: other.ID, AuthorType: "operator",
		AuthorID: "operator", Body: "This must not appear.", CreatedAt: time.Now().Add(3 * time.Second),
	}).Error; err != nil {
		t.Fatal(err)
	}

	timeline, err := store.TaskTimeline(child.ID)
	if err != nil {
		t.Fatal(err)
	}
	if timeline.Task.ID != task.ID || timeline.IssueCount != 2 {
		t.Fatalf("unexpected task tree: %+v", timeline)
	}
	kinds := make([]string, len(timeline.Events))
	for index, event := range timeline.Events {
		kinds[index] = event.Kind
		if event.IssueID == other.ID || event.Summary == "This must not appear." {
			t.Fatalf("timeline leaked another task: %+v", event)
		}
	}
	for _, expected := range []string{"issue", "execution", "result", "comment"} {
		if !slices.Contains(kinds, expected) {
			t.Fatalf("missing %s event: %+v", expected, timeline.Events)
		}
	}
	if timeline.Events[0].CreatedAt.Before(timeline.Events[len(timeline.Events)-1].CreatedAt) {
		t.Fatal("timeline is not newest first")
	}
	commentFound := slices.ContainsFunc(timeline.Events, func(event TaskTimelineEvent) bool {
		return event.Kind == "comment" && event.ActorName == "操作员" && event.Detail == "Please include the test evidence."
	})
	resultFound := slices.ContainsFunc(timeline.Events, func(event TaskTimelineEvent) bool {
		return event.Kind == "result" && event.ActorName == "前端工程师" && event.Detail == "UI build and tests passed."
	})
	if !commentFound || !resultFound {
		t.Fatalf("missing actor or result details: %+v", timeline.Events)
	}
}

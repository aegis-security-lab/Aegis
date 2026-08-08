package control

import (
	"archive/zip"
	"io"
	"strings"
	"testing"
	"time"
)

func TestRootIssueTaskReportKeepsImmutableHistory(t *testing.T) {
	store := configuredStore(t)
	task, root, err := store.CreateTask(CreateIssueInput{
		Title: "Consolidated delivery", Objective: "Deliver one complete report package.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, _ := store.createExecution(root, root.AssigneeAgentID, "work")
	oldBody := "obsolete first report"
	if _, err = store.captureUploadedAttachment(root, first.ID, PublishAttachmentInput{Path: "report.md", Description: "Old report"}, strings.NewReader(oldBody), int64(len(oldBody))); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err = manager.publishRootIssueTaskReport(root, first.ID, time.Now().Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}

	latest, _ := store.createExecution(root, root.AssigneeAgentID, "wakeup")
	finalBody := "# Complete final report\n\nReproduction steps and verified results."
	evidenceBody := "verification passed"
	if _, err = store.captureUploadedAttachment(root, latest.ID, PublishAttachmentInput{Path: "final-report.md", Description: "Complete report"}, strings.NewReader(finalBody), int64(len(finalBody))); err != nil {
		t.Fatal(err)
	}
	if _, err = store.captureUploadedAttachment(root, latest.ID, PublishAttachmentInput{Path: "evidence.txt", Description: "Supporting evidence"}, strings.NewReader(evidenceBody), int64(len(evidenceBody))); err != nil {
		t.Fatal(err)
	}
	if err = manager.publishRootIssueTaskReport(root, latest.ID, time.Now()); err != nil {
		t.Fatal(err)
	}

	detail, err := store.GetTaskDetail(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.RootReports) != 1 || len(detail.RootReports[0].Reports) != 2 {
		t.Fatalf("unexpected root report projection: %+v", detail.RootReports)
	}
	report := detail.RootReports[0].Reports[0]
	if report.SourceExecutionID != latest.ID || report.AttachmentCount != 2 {
		t.Fatalf("report does not point at latest submission: %+v", report)
	}
	if detail.RootReports[0].Reports[1].SourceExecutionID != first.ID {
		t.Fatalf("first report was not retained: %+v", detail.RootReports[0].Reports)
	}

	_, file, err := store.TaskReportFile(report.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	archive, err := zip.NewReader(file, report.Size)
	if err != nil {
		t.Fatal(err)
	}
	contents := map[string]string{}
	for _, entry := range archive.File {
		reader, openErr := entry.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		body, readErr := io.ReadAll(reader)
		_ = reader.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		contents[entry.Name] = string(body)
	}
	if contents["final-report.md"] != finalBody || contents["evidence.txt"] != evidenceBody || !strings.Contains(contents["manifest.json"], latest.ID) {
		t.Fatalf("unexpected task report ZIP contents: %+v", contents)
	}
	for _, body := range contents {
		if strings.Contains(body, oldBody) {
			t.Fatal("obsolete submission leaked into latest task report")
		}
	}
}

func TestTaskReportUsesResumeCommentAsTitle(t *testing.T) {
	store := configuredStore(t)
	_, root, err := store.CreateTask(CreateIssueInput{Title: "Initial task title", Priority: "middle", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := store.createExecution(root, root.AssigneeAgentID, "continuation")
	comment := IssueComment{ID: nextID("comment"), IssueID: root.ID, AuthorType: "operator", AuthorID: "operator", Body: "现在根据新的验证结果重新生成完整报告，并附上复现步骤。", CreatedAt: time.Now().Add(-time.Second)}
	if err = store.db.Create(&comment).Error; err != nil {
		t.Fatal(err)
	}
	wakeup := AgentWakeup{ID: nextID("wakeup"), IssueID: root.ID, CommentID: comment.ID, AgentID: root.AssigneeAgentID, ExecutionID: execution.ID, Reason: "issue_comment_resume", Status: "delivered", CreatedAt: comment.CreatedAt}
	if err = store.db.Create(&wakeup).Error; err != nil {
		t.Fatal(err)
	}
	body := "report"
	if _, err = store.captureUploadedAttachment(root, execution.ID, PublishAttachmentInput{Path: "report.md"}, strings.NewReader(body), int64(len(body))); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err = manager.publishRootIssueTaskReport(root, execution.ID, time.Now()); err != nil {
		t.Fatal(err)
	}
	detail, err := store.GetTaskDetail(root.TaskSourceID)
	if err != nil {
		t.Fatal(err)
	}
	report := detail.RootReports[0].Reports[0]
	if !strings.HasPrefix(report.Title, "现在根据新的验证结果重新生成完整报告") {
		t.Fatalf("report title=%q", report.Title)
	}
}

func TestTaskReportRequiresPublishedAttachments(t *testing.T) {
	store := configuredStore(t)
	_, root, err := store.CreateTask(CreateIssueInput{
		Title: "Missing delivery", Objective: "Deliver a report.", Priority: "middle",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	execution, _ := store.createExecution(root, root.AssigneeAgentID, "work")
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	if err = manager.publishRootIssueTaskReport(root, execution.ID, time.Now()); err == nil || !strings.Contains(err.Error(), "没有可打包的附件") {
		t.Fatalf("expected missing attachment error, got %v", err)
	}
}

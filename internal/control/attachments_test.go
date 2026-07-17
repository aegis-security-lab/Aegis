package control

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestAttachmentToolIsMigratedOntoExistingAgents(t *testing.T) {
	dataDir := t.TempDir()
	store, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	var record agentRecord
	if err := store.db.First(&record, "id = ?", "backend-engineer").Error; err != nil {
		t.Fatal(err)
	}
	record.Definition.Tools = slices.DeleteFunc(record.Definition.Tools, func(tool string) bool {
		return tool == "aegis_publish_attachment"
	})
	if err := store.db.Save(&record).Error; err != nil {
		t.Fatal(err)
	}
	database, err := store.db.DB()
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := NewStore(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		database, _ := reopened.db.DB()
		_ = database.Close()
	}()
	agents := reopened.Agents()
	index, exists := agentIndex(agents, "backend-engineer")
	if !exists || !slices.Contains(agents[index].Tools, "aegis_publish_attachment") {
		t.Fatalf("attachment tool was not migrated: %+v", agents)
	}
}

func TestAttachmentIsCopiedAndBoundToCompletionComment(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Generate report", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	reportPath := filepath.Join(issue.Workspace, "reports", "result.md")
	if err := os.MkdirAll(filepath.Dir(reportPath), 0o700); err != nil {
		t.Fatal(err)
	}
	content := []byte("# Result\n\nVerified.\n")
	if err := os.WriteFile(reportPath, content, 0o600); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions["exec-report"] = &PiSession{
		executionID: "exec-report", issueID: issue.ID, agentID: "backend-engineer", controlToken: "control-secret",
	}
	if _, err := manager.PublishExecutionAttachment("exec-report", "control-secrex", PublishAttachmentInput{Path: "reports/result.md"}); err == nil {
		t.Fatal("expected invalid attachment control token to be rejected")
	}
	attachment, err := manager.PublishExecutionAttachment("exec-report", "control-secret", PublishAttachmentInput{
		Path: "reports/result.md", Description: "Verification report",
	})
	if err != nil {
		t.Fatal(err)
	}
	if attachment.CommentID != "" || attachment.SourcePath != "reports/result.md" {
		t.Fatalf("unexpected pending attachment: %+v", attachment)
	}
	if err := os.Remove(reportPath); err != nil {
		t.Fatal(err)
	}

	manager.addAgentComment(issue.ID, "backend-engineer", "", "exec-report")
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != 1 || len(detail.Comments[0].Attachments) != 1 {
		t.Fatalf("comments=%+v", detail.Comments)
	}
	if detail.Comments[0].Body != "已生成并附上交付物。" {
		t.Fatalf("unexpected fallback comment: %q", detail.Comments[0].Body)
	}
	bound := detail.Comments[0].Attachments[0]
	if bound.ID != attachment.ID || bound.CommentID != detail.Comments[0].ID {
		t.Fatalf("attachment was not bound to comment: %+v", bound)
	}

	_, file, err := store.AttachmentFile(bound.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored attachment=%q, want %q", got, content)
	}
}

func TestAttachmentRejectsPathOutsideWorkspace(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Unsafe report", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.captureAttachment(issue, "exec-unsafe", PublishAttachmentInput{Path: outside}); err == nil {
		t.Fatal("expected out-of-workspace attachment to be rejected")
	}
}

func TestSuccessfulWriteIsCollectedAsAttachment(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Write artifact", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(issue.Workspace, "output.csv")
	if err := os.WriteFile(path, []byte("status\nverified\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	store.startToolEvent("exec-write", issue.ID, "call-write", "write", map[string]any{"path": "output.csv"})
	store.finishToolEvent("exec-write", issue.ID, "call-write", "write", map[string]any{"text": "written"}, false)

	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.collectExecutionAttachments(issue, "exec-write")
	manager.addAgentComment(issue.ID, "backend-engineer", "Output generated.", "exec-write")
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != 1 || len(detail.Comments[0].Attachments) != 1 {
		t.Fatalf("write artifact was not attached: %+v", detail.Comments)
	}
	if detail.Comments[0].Attachments[0].Name != "output.csv" {
		t.Fatalf("unexpected attachment: %+v", detail.Comments[0].Attachments[0])
	}
}

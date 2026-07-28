package control

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestUploadedAttachmentIsStoredAndBoundToCompletionComment(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Generate report", Objective: "A verified report is generated.", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer-002", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("# Result\n\nVerified.\n")
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions["exec-report"] = &PiSession{
		executionID: "exec-report", issueID: issue.ID, agentID: "backend-engineer", controlToken: "control-secret",
	}
	if _, err := manager.UploadExecutionAttachment("exec-report", "control-secrex", PublishAttachmentInput{Path: "reports/result.md"}, strings.NewReader(string(content)), int64(len(content))); err == nil {
		t.Fatal("expected invalid attachment control token to be rejected")
	}
	attachment, err := manager.UploadExecutionAttachment("exec-report", "control-secret", PublishAttachmentInput{
		Path: "reports/result.md", Description: "Verification report",
	}, strings.NewReader(string(content)), int64(len(content)))
	if err != nil {
		t.Fatal(err)
	}
	if attachment.CommentID != "" || attachment.SourcePath != "reports/result.md" {
		t.Fatalf("unexpected pending attachment: %+v", attachment)
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

func TestFinalResultUsesAnAlreadyUploadedAttachment(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Submit delivery", Objective: "Submit a result with evidence.", Priority: "medium", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("validation_disabled", true).Error; err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions[execution.ID] = &PiSession{
		executionID: execution.ID, issueID: issue.ID, agentID: "backend-engineer", controlToken: "delivery-secret",
	}
	if _, err = manager.SubmitFinalResult(execution.ID, "delivery-secret", SubmitFinalResultInput{Body: "Verified delivery.", Path: "/workspace/report.md"}); err == nil || !strings.Contains(err.Error(), "直传") {
		t.Fatalf("legacy path-based final result error = %v", err)
	}
	content := "verified evidence"
	if _, err = manager.UploadExecutionAttachment(execution.ID, "delivery-secret", PublishAttachmentInput{Path: "report.md"}, strings.NewReader(content), int64(len(content))); err != nil {
		t.Fatal(err)
	}
	result, err := manager.SubmitFinalResult(execution.ID, "delivery-secret", SubmitFinalResultInput{Body: "Verified delivery."})
	if err != nil {
		t.Fatal(err)
	}
	if !result.FinalResultSubmitted || result.FinalResult != "Verified delivery." {
		t.Fatalf("unexpected submitted final result: %+v", result)
	}
}

func TestValidationAgentCanReadOnlySourceExecutionAttachmentsInChunks(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Validate attached report", Objective: "The attached report contains verified evidence.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	source, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	report := "# Security report\n\nFinding A is verified with concrete evidence.\n"
	attachment, err := store.captureUploadedAttachment(issue, source.ID, PublishAttachmentInput{Path: "security-report.md", Description: "Complete report"}, strings.NewReader(report), int64(len(report)))
	if err != nil {
		t.Fatal(err)
	}
	validationExecution, _, err := store.createInternalExecution(issue, "acceptance-validator", "validation")
	if err != nil {
		t.Fatal(err)
	}
	validation := IssueValidation{
		ID: nextID("validation"), IssueID: issue.ID, SourceExecutionID: source.ID,
		ValidationExecutionID: validationExecution.ID, Attempt: 1, Objective: issue.Objective,
		CandidateResult: "The complete report is attached.", Status: "running", CreatedAt: time.Now(),
	}
	if err = store.db.Create(&validation).Error; err != nil {
		t.Fatal(err)
	}
	manager := &Manager{store: store, controlURL: "http://127.0.0.1:8080", sessions: map[string]*PiSession{}}
	manager.sessions[validationExecution.ID] = &PiSession{
		executionID: validationExecution.ID, issueID: issue.ID, agentID: "acceptance-validator",
		kind: "validation", controlToken: "validation-secret",
	}
	if _, err = manager.ValidationAttachments(validationExecution.ID, "wrong-secret"); err == nil {
		t.Fatal("expected invalid validation token to be rejected")
	}
	attachments, err := manager.ValidationAttachments(validationExecution.ID, "validation-secret")
	if err != nil {
		t.Fatal(err)
	}
	if len(attachments) != 1 || attachments[0].ID != attachment.ID || !attachments[0].Readable || !strings.Contains(attachments[0].DownloadURL, attachment.ID) {
		t.Fatalf("unexpected validation attachments: %+v", attachments)
	}
	first, err := manager.ReadValidationAttachment(validationExecution.ID, "validation-secret", attachment.ID, "", 0, 12)
	if err != nil {
		t.Fatal(err)
	}
	if first.EOF || first.NextOffset != 12 || first.Content != report[:12] {
		t.Fatalf("unexpected first attachment chunk: %+v", first)
	}
	second, err := manager.ReadValidationAttachment(validationExecution.ID, "validation-secret", attachment.ID, "", first.NextOffset, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if !second.EOF || first.Content+second.Content != report {
		t.Fatalf("unexpected complete attachment content: first=%+v second=%+v", first, second)
	}

	var archive bytes.Buffer
	zipWriter := zip.NewWriter(&archive)
	readme, err := zipWriter.Create("README.md")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = readme.Write([]byte("# Usage\n\npython bt_panel_rce.py --help\n")); err != nil {
		t.Fatal(err)
	}
	script, err := zipWriter.Create("tools/bt_panel_rce.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = script.Write([]byte("import argparse\nargparse.ArgumentParser().parse_args()\n")); err != nil {
		t.Fatal(err)
	}
	if err = zipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	zipAttachment, err := store.captureUploadedAttachment(issue, source.ID, PublishAttachmentInput{Path: "bt_panel_exploit_tools.zip"}, bytes.NewReader(archive.Bytes()), int64(archive.Len()))
	if err != nil {
		t.Fatal(err)
	}
	zipInfo := manager.validationAttachmentInfo(zipAttachment)
	if zipInfo.Readable || len(zipInfo.ArchiveEntries) != 2 || !zipInfo.ArchiveEntries[0].Readable {
		t.Fatalf("unexpected ZIP validation manifest: %+v", zipInfo)
	}
	zipChunk, err := manager.ReadValidationAttachment(validationExecution.ID, "validation-secret", zipAttachment.ID, "tools/bt_panel_rce.py", 0, 32768)
	if err != nil {
		t.Fatal(err)
	}
	if !zipChunk.EOF || !strings.Contains(zipChunk.Content, "ArgumentParser") || zipChunk.ArchivePath != "tools/bt_panel_rce.py" {
		t.Fatalf("unexpected ZIP entry chunk: %+v", zipChunk)
	}
	if _, err = manager.ReadValidationAttachment(validationExecution.ID, "validation-secret", zipAttachment.ID, "../README.md", 0, 100); err == nil {
		t.Fatal("expected unsafe ZIP path to be rejected")
	}

	otherIssue, err := store.CreateIssue(CreateIssueInput{
		Title: "Other report", Objective: "Keep unrelated evidence isolated.", Priority: "low", WorkMode: "autonomous",
		AssigneeAgentID: "backend-engineer-002", Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.captureUploadedAttachment(otherIssue, "other-execution", PublishAttachmentInput{Path: "other-report.md"}, strings.NewReader("unrelated"), int64(len("unrelated")))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = manager.ReadValidationAttachment(validationExecution.ID, "validation-secret", other.ID, "", 0, 100); err == nil {
		t.Fatal("validator should not read an attachment from another source Execution")
	}
}

func TestWriteOutputIsNotReadByServerWithoutExplicitUpload(t *testing.T) {
	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Write artifact", Objective: "The output artifact exists.", Priority: "medium", WorkMode: "autonomous",
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
	manager.addAgentComment(issue.ID, "backend-engineer", "Output generated.", "exec-write")
	detail, err := store.GetIssueDetail(issue.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Comments) != 1 || len(detail.Comments[0].Attachments) != 0 {
		t.Fatalf("server should not read tool output paths implicitly: %+v", detail.Comments)
	}
}

func TestContainerAttachmentUploadDoesNotInvokeDockerOrTranslatePath(t *testing.T) {
	fakeBin := t.TempDir()
	dockerPath := filepath.Join(fakeBin, "docker")
	logPath := filepath.Join(t.TempDir(), "docker.log")
	content := []byte("# Container report\n\nVerified inside Docker.\n")
	script := `#!/bin/sh
printf '%s\n' "$*" >> "$FAKE_DOCKER_LOG"
exit 99
`
	if err := os.WriteFile(dockerPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("FAKE_DOCKER_LOG", logPath)

	store := configuredStore(t)
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Container evidence", Objective: "Read evidence directly from the container.",
		Priority: "medium", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
		Workspace: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	container := ContainerInstance{
		ID: nextID("container"), ContainerProfileID: "profile-direct", TaskID: nextID("task"),
		Name: "aegis-task-direct", Image: WorkerContainerImage, NodePath: "node",
		PiPath: "/usr/local/bin/pi", WorkspacePath: "/workspace", NetworkMode: "bridge",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err = store.db.Create(&container).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", issue.ID).Update("container_id", container.ID).Error; err != nil {
		t.Fatal(err)
	}
	issue.ContainerID = container.ID
	manager := &Manager{store: store, sessions: map[string]*PiSession{}}
	manager.sessions["exec-container-report"] = &PiSession{
		executionID: "exec-container-report", issueID: issue.ID,
		agentID: "backend-engineer", controlToken: "container-secret",
	}
	attachment, err := manager.UploadExecutionAttachment(
		"exec-container-report",
		"container-secret",
		PublishAttachmentInput{Path: "reports/report.md"},
		strings.NewReader(string(content)),
		int64(len(content)),
	)
	if err != nil {
		t.Fatal(err)
	}
	if attachment.SourcePath != "reports/report.md" || attachment.Name != "report.md" {
		t.Fatalf("unexpected container attachment metadata: %+v", attachment)
	}
	_, file, err := store.AttachmentFile(attachment.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	got, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(content) {
		t.Fatalf("stored container attachment=%q, want %q", got, content)
	}
	if _, err = os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("attachment upload must not invoke Docker; log stat error = %v", err)
	}
	if _, err = manager.UploadExecutionAttachment(
		"exec-container-report",
		"container-secret",
		PublishAttachmentInput{Path: "reports/incomplete.md"},
		strings.NewReader("short"),
		20,
	); err == nil || !strings.Contains(err.Error(), "不完整") {
		t.Fatalf("incomplete upload error = %v", err)
	}
}

package control

import (
	"os"
	"strings"
	"testing"
)

func TestTaskInputAttachmentIsBoundAndMaterializedBeforeExecution(t *testing.T) {
	store := configuredStore(t)
	content := "large audit image placeholder"
	attachment, err := store.StageInputAttachment("task", "", "target-image.tar", "application/x-tar", strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	task, issue, err := store.CreateTask(CreateIssueInput{
		Title: "Audit uploaded image", Objective: "Report verified findings.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer-002",
		AttachmentIDs: []string{attachment.ID}, Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	var bound InputAttachment
	if err = store.db.First(&bound, "id = ?", attachment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if bound.TaskID != task.ID || bound.IssueID != issue.ID || bound.BoundAt == nil {
		t.Fatalf("attachment was not atomically bound to task and Issue: %+v", bound)
	}
	execution, err := store.createExecution(issue, issue.AssigneeAgentID, "work")
	if err != nil {
		t.Fatal(err)
	}
	runtimeAttachments, err := store.materializeInputAttachments(issue, execution, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimeAttachments) != 1 || runtimeAttachments[0].ID != attachment.ID {
		t.Fatalf("unexpected runtime attachments: %+v", runtimeAttachments)
	}
	data, err := os.ReadFile(runtimeAttachments[0].Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("materialized content = %q", data)
	}
	prompt := agentInputAttachmentsSystemPrompt("base", runtimeAttachments)
	if !strings.Contains(prompt, runtimeAttachments[0].Path) || !strings.Contains(prompt, "target-image.tar") {
		t.Fatalf("system prompt does not identify the runtime attachment: %s", prompt)
	}

	// A Task restart creates a fresh root Issue, but the original Task-bound
	// input remains discoverable and does not need another browser upload.
	restarted, err := store.CreateIssue(CreateIssueInput{
		Title: task.Title, Objective: task.Objective, Priority: task.Priority,
		WorkMode: task.WorkMode, TaskSourceID: task.ID, Workspace: task.Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	restartExecution, err := store.createExecution(restarted, "backend-engineer-003", "work")
	if err != nil {
		t.Fatal(err)
	}
	reused, err := store.inputAttachmentsForExecution(restarted, restartExecution)
	if err != nil || len(reused) != 1 || reused[0].ID != attachment.ID {
		t.Fatalf("restarted task did not inherit its input attachment: attachments=%+v err=%v", reused, err)
	}
}

func TestEmployeeInputAttachmentCanMoveToFirstBoardIssue(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.executionAgent("aegis-orchestrator")
	if err != nil {
		t.Fatal(err)
	}
	homeSession, err := store.ensureEmployeeWorkspace(agent)
	if err != nil {
		t.Fatal(err)
	}
	home, err := store.GetIssue(homeSession.HomeIssueID)
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := store.StageInputAttachment("employee", agent.ID, "audit.oci.tar", "application/x-tar", strings.NewReader("oci image"))
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(home, agent.ID, "employee_chat")
	if err != nil {
		t.Fatal(err)
	}
	if err = store.BindEmployeeInputAttachments(agent.ID, home.ID, execution.ID, []string{attachment.ID}); err != nil {
		t.Fatal(err)
	}
	ids := store.InputAttachmentIDsForExecution(execution.ID)
	if len(ids) != 1 || ids[0] != attachment.ID {
		t.Fatalf("unexpected employee attachment ids: %v", ids)
	}
	issue, err := store.CreateIssue(CreateIssueInput{
		Title: "Team image audit", Objective: "Audit the supplied image.", Priority: "high",
		WorkMode: "autonomous", AttachmentIDs: ids,
		AttachmentSourceExecutionID: execution.ID, Workspace: store.Config().Workspace,
	})
	if err != nil {
		t.Fatal(err)
	}
	var moved InputAttachment
	if err = store.db.First(&moved, "id = ?", attachment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if moved.IssueID != issue.ID || moved.ExecutionID != execution.ID {
		t.Fatalf("employee attachment was not handed to Board Issue: %+v", moved)
	}
	if remaining := store.InputAttachmentIDsForExecution(execution.ID); len(remaining) != 0 {
		t.Fatalf("the same attachment could be rebound by a second Board create: %v", remaining)
	}
}

func TestOnlyUnboundInputAttachmentsCanBeDeleted(t *testing.T) {
	store := configuredStore(t)
	attachment, err := store.StageInputAttachment("task", "", "remove-me.bin", "application/octet-stream", strings.NewReader("data"))
	if err != nil {
		t.Fatal(err)
	}
	storageDir := strings.TrimSuffix(attachment.StoragePath, string(os.PathSeparator)+attachment.Name)
	if err = store.DeleteStagedInputAttachment(attachment.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(storageDir); !os.IsNotExist(err) {
		t.Fatalf("staged attachment storage still exists: %v", err)
	}
}

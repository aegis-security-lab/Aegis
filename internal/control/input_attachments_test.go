package control

import (
	"os"
	"path"
	"strings"
	"testing"
)

func TestTaskInputAttachmentIsBoundAndMaterializedBeforeExecution(t *testing.T) {
	dockerLog, _ := installFakeDocker(t)
	store := configuredStore(t)
	content := "large audit image placeholder"
	attachment, err := store.StageInputAttachment("task", "", "target-image.tar", "application/x-tar", strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	task, issue, err := store.CreateTask(CreateIssueInput{
		Title: "Audit uploaded image", Objective: "Report verified findings.", Priority: "high",
		WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
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
	container, err := store.GetContainer(issue.ContainerID)
	if err != nil {
		t.Fatal(err)
	}
	runtimeAttachments, err := store.materializeInputAttachments(issue, execution, &container)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtimeAttachments) != 1 || runtimeAttachments[0].ID != attachment.ID {
		t.Fatalf("unexpected runtime attachments: %+v", runtimeAttachments)
	}
	wantPath := path.Join(TaskWorkspacePath, ".aegis", "input-attachments", attachment.ID, attachment.Name)
	if runtimeAttachments[0].Path != wantPath {
		t.Fatalf("materialized path = %q, want %q", runtimeAttachments[0].Path, wantPath)
	}
	data, err := os.ReadFile(dockerLog)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "cp "+attachment.StoragePath+" "+container.Name+":"+wantPath) {
		t.Fatalf("input attachment was not copied directly into the task container:\n%s", data)
	}
	prompt := agentInputAttachmentsSystemPrompt("base", runtimeAttachments)
	if !strings.Contains(prompt, runtimeAttachments[0].Path) || !strings.Contains(prompt, "target-image.tar") {
		t.Fatalf("system prompt does not identify the runtime attachment: %s", prompt)
	}

	// A Task restart creates a fresh root Issue, but the original Task-bound
	// input remains discoverable and does not need another browser upload.
	restarted, err := store.CreateIssue(CreateIssueInput{
		Title: task.Title, Objective: task.Objective, Priority: task.Priority,
		WorkMode: task.WorkMode, TaskSourceID: task.ID, Workspace: task.Workspace, AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	restartExecution, err := store.createExecution(restarted, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	reused, err := store.inputAttachmentsForExecution(restarted, restartExecution)
	if err != nil || len(reused) != 1 || reused[0].ID != attachment.ID {
		t.Fatalf("restarted task did not inherit its input attachment: attachments=%+v err=%v", reused, err)
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

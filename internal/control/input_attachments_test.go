package control

import (
	"io"
	"os"
	"path"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
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

func TestConciergeAttachmentIsScopedToConversationAndCopiedToEveryTask(t *testing.T) {
	store := configuredStore(t)
	conversation, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := store.StageInputAttachment("concierge", conversation.ID, "requirements.md", "text/markdown", strings.NewReader("build the requested service"))
	if err != nil {
		t.Fatal(err)
	}
	message := Message{
		ID: nextID("message"), ExecutionID: conversation.ExecutionID, IssueID: conversation.IssueID,
		Role: "user", Content: "请根据附件创建任务", CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	var sent []InputAttachment
	if err = store.db.Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Create(&message).Error; createErr != nil {
			return createErr
		}
		var bindErr error
		sent, bindErr = bindConciergeInputAttachmentsTx(tx, conversation, message, []string{attachment.ID})
		return bindErr
	}); err != nil {
		t.Fatal(err)
	}
	if len(sent) != 1 || sent[0].MessageID != message.ID || sent[0].ExecutionID != conversation.ExecutionID {
		t.Fatalf("attachment was not bound to concierge turn: %+v", sent)
	}
	detail, err := store.GetConciergeConversation(conversation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Messages) != 1 || len(detail.Messages[0].Attachments) != 1 || detail.Messages[0].Attachments[0].ID != attachment.ID {
		t.Fatalf("concierge message did not expose its attachment metadata: %+v", detail.Messages)
	}
	prompt := conciergeInputAttachmentsPrompt(message.Content, sent)
	for _, expected := range []string{"requirements.md", "copy the conversation attachments", "preserving the conversation source files", "Do not claim"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("concierge attachment prompt missing %q: %s", expected, prompt)
		}
	}

	manager := &Manager{store: store}
	issue, err := manager.createTaskFromConcierge(conversation.ExecutionID, CreateConciergeTaskInput{
		Title: "Implement attachment requirements", Objective: "Deliver and verify the requested implementation.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	var source InputAttachment
	if err = store.db.First(&source, "id = ?", attachment.ID).Error; err != nil {
		t.Fatal(err)
	}
	if source.TaskID != "" || source.IssueID != conversation.IssueID || source.ExecutionID != conversation.ExecutionID || source.MessageID != message.ID {
		t.Fatalf("concierge source attachment ownership changed: %+v", source)
	}
	issueDetail, err := store.GetIssueDetail(issue.ID)
	if err != nil || len(issueDetail.InputAttachments) != 1 || issueDetail.InputAttachments[0].ID == attachment.ID || issueDetail.InputAttachments[0].TaskID != issue.TaskSourceID {
		t.Fatalf("task detail does not expose an independent attachment copy: detail=%+v err=%v", issueDetail.InputAttachments, err)
	}
	firstCopy := issueDetail.InputAttachments[0]
	_, file, err := store.InputAttachmentFile(firstCopy.ID)
	if err != nil {
		t.Fatal(err)
	}
	downloaded, readErr := io.ReadAll(file)
	_ = file.Close()
	if readErr != nil || string(downloaded) != "build the requested service" {
		t.Fatalf("downloaded input attachment mismatch: data=%q err=%v", downloaded, readErr)
	}
	execution, err := store.createExecution(issue, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	available, err := store.inputAttachmentsForExecution(issue, execution)
	if err != nil || len(available) != 1 || available[0].ID != firstCopy.ID {
		t.Fatalf("task execution cannot discover concierge attachment: attachments=%+v err=%v", available, err)
	}
	secondIssue, err := manager.createTaskFromConcierge(conversation.ExecutionID, CreateConciergeTaskInput{
		Title: "Retry attachment requirements", Objective: "Deliver the same input in a separate Task.",
		Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer",
	})
	if err != nil {
		t.Fatal(err)
	}
	secondDetail, err := store.GetIssueDetail(secondIssue.ID)
	if err != nil || len(secondDetail.InputAttachments) != 1 {
		t.Fatalf("second Task did not receive an attachment copy: detail=%+v err=%v", secondDetail.InputAttachments, err)
	}
	secondCopy := secondDetail.InputAttachments[0]
	if secondCopy.ID == firstCopy.ID || secondCopy.ID == attachment.ID || secondCopy.StoragePath == firstCopy.StoragePath {
		t.Fatalf("Tasks must have independent attachment records and paths: source=%+v first=%+v second=%+v", attachment, firstCopy, secondCopy)
	}

	if err = store.deleteConciergeConversation(conversation.ID); err != nil {
		t.Fatal(err)
	}
	if err = store.db.First(&source, "id = ?", attachment.ID).Error; err == nil {
		t.Fatal("deleting concierge conversation must remove its source attachment record")
	}
	for _, copied := range []InputAttachment{firstCopy, secondCopy} {
		if _, statErr := os.Stat(copied.StoragePath); statErr != nil {
			t.Fatalf("deleting concierge conversation removed Task copy %s: %v", copied.ID, statErr)
		}
	}
}

func TestConciergeAttachmentCannotCrossConversationAndPendingFilesAreCleaned(t *testing.T) {
	store := configuredStore(t)
	first, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.CreateConciergeConversation()
	if err != nil {
		t.Fatal(err)
	}
	attachment, err := store.StageInputAttachment("concierge", first.ID, "private.txt", "text/plain", strings.NewReader("private"))
	if err != nil {
		t.Fatal(err)
	}
	message := Message{ID: nextID("message"), ExecutionID: second.ExecutionID, IssueID: second.IssueID, Role: "user", Content: "wrong conversation", CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err = store.db.Transaction(func(tx *gorm.DB) error {
		if createErr := tx.Create(&message).Error; createErr != nil {
			return createErr
		}
		_, bindErr := bindConciergeInputAttachmentsTx(tx, second, message, []string{attachment.ID})
		return bindErr
	}); err == nil || !strings.Contains(err.Error(), "不属于当前管家会话") {
		t.Fatalf("cross-conversation attachment binding should fail, got %v", err)
	}
	storageDir := path.Dir(attachment.StoragePath)
	if err = store.deleteConciergeConversation(first.ID); err != nil {
		t.Fatal(err)
	}
	if _, statErr := os.Stat(storageDir); !os.IsNotExist(statErr) {
		t.Fatalf("pending concierge attachment storage still exists: %v", statErr)
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

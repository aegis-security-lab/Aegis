package control

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"aegis/agentapp"
	"aegis/capability"
	"aegis/observability"
)

func TestTaskEvidenceExportsConversationLogsArtifactsAndChecksums(t *testing.T) {
	store := configuredStore(t)
	if _, err := EnableObservability(store, io.Discard, "debug"); err != nil {
		t.Fatal(err)
	}
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	root, err := store.CreateIssue(CreateIssueInput{Title: "Evidence task", Objective: "Produce a verified report.", Priority: "high", WorkMode: "autonomous", AssigneeAgentID: "backend-engineer"})
	if err != nil {
		t.Fatal(err)
	}
	execution, err := store.createExecution(root, "backend-engineer", "work")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	finished := now.Add(2 * time.Second)
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: root.ID, Role: "user", Content: "Analyze this. Authorization: Bearer secret-for-redaction", CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&Message{ID: nextID("message"), ExecutionID: execution.ID, IssueID: root.ID, Role: "assistant", Content: "Verified result", CreatedAt: finished, UpdatedAt: finished}).Error; err != nil {
		t.Fatal(err)
	}
	if err = store.db.Create(&ExecutionEvent{ID: nextID("event"), ExecutionID: execution.ID, IssueID: root.ID, Type: "tool", Title: "read", ToolName: "read_file", InputJSON: `{"path":"report.md"}`, OutputJSON: `{"ok":true}`, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err = store.captureUploadedAttachment(root, execution.ID, PublishAttachmentInput{Path: "report.md", Description: "final evidence"}, strings.NewReader("artifact body"), int64(len("artifact body"))); err != nil {
		t.Fatal(err)
	}
	if err = store.updateExecution(execution.ID, map[string]any{
		"status": "completed", "result": "Verified result", "final_result": "Verified result", "tokens": 321, "cost": 0.125, "finished_at": &finished,
		"capabilities_snapshot": []capability.Snapshot{{Kind: capability.KindPhone, Name: "default", Version: "phone-v1", Metadata: map[string]string{"apps": "aegis.board"}}},
	}); err != nil {
		t.Fatal(err)
	}
	if err = store.db.Model(&Issue{}).Where("id = ?", root.ID).Updates(map[string]any{"status": "done", "execution_phase": "completed", "result": "Verified result", "completed_at": &finished, "updated_at": finished}).Error; err != nil {
		t.Fatal(err)
	}
	_, phone, err := NewControlAgentPhone(manager)
	if err != nil {
		t.Fatal(err)
	}
	phoneSession, err := phone.Start(context.Background(), agentapp.StartSessionRequest{
		AgentID: "backend-engineer", TaskAgentID: root.AssigneeTaskAgentID, TaskID: root.ID, ExecutionID: "newer-execution-not-in-task-export",
		InstalledApps: []string{"aegis.board", "aegis.relay"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = phone.Act(context.Background(), agentapp.ActionRequest{
		PhoneSessionID: phoneSession.PhoneSessionID, PageRevision: phoneSession.Page.Revision,
		Action: agentapp.ActionOpenApp, Ref: "@1", IdempotencyKey: "evidence-open-board",
	}); err != nil {
		t.Fatal(err)
	}
	logContext := observability.WithScope(context.Background(), observability.Scope{TaskID: root.TaskSourceID, IssueID: root.ID, ExecutionID: execution.ID, Component: "test.worker"})
	observability.Default().Info(logContext, "test.task.completed", slog.String("authorization", "Bearer should-not-leak"))

	var output bytes.Buffer
	manifest, err := manager.WriteTaskEvidence(context.Background(), root.TaskSourceID, &output, TaskExportOptions{IncludeArtifacts: true, RedactSecrets: true})
	if err != nil {
		t.Fatal(err)
	}
	if !manifest.Complete || manifest.ActiveAtExport || !manifest.RedactionEnabled || !manifest.ArtifactsIncluded {
		t.Fatalf("unexpected manifest state: %+v", manifest)
	}
	reader, err := zip.NewReader(bytes.NewReader(output.Bytes()), int64(output.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, item := range reader.File {
		stream, openErr := item.Open()
		if openErr != nil {
			t.Fatal(openErr)
		}
		content, readErr := io.ReadAll(stream)
		_ = stream.Close()
		if readErr != nil {
			t.Fatal(readErr)
		}
		files[item.Name] = content
	}
	for _, required := range []string{"manifest.json", "evaluation_context.json", "conversations.json", "execution_events.json", "observability/logs.json", "attachments.json", "phone/sessions.json", "phone/audit_events.json"} {
		if _, ok := files[required]; !ok {
			t.Errorf("missing %s", required)
		}
	}
	conversation := string(files["conversations.json"])
	if !strings.Contains(conversation, "Verified result") || strings.Contains(conversation, "secret-for-redaction") {
		t.Fatalf("conversation redaction/content mismatch: %s", conversation)
	}
	logs := string(files["observability/logs.json"])
	if !strings.Contains(logs, "test.task.completed") || strings.Contains(logs, "should-not-leak") {
		t.Fatalf("log export redaction/content mismatch: %s", logs)
	}
	var exportedExecutions []Execution
	if err = json.Unmarshal(files["executions.json"], &exportedExecutions); err != nil {
		t.Fatal(err)
	}
	if len(exportedExecutions) != 1 || len(exportedExecutions[0].CapabilitiesSnapshot) != 1 || exportedExecutions[0].CapabilitiesSnapshot[0].Metadata["apps"] != "aegis.board" {
		t.Fatalf("execution capability evidence missing: %+v", exportedExecutions)
	}
	var exportedPhones []phoneSessionEvidence
	if err = json.Unmarshal(files["phone/sessions.json"], &exportedPhones); err != nil {
		t.Fatal(err)
	}
	if len(exportedPhones) != 1 || exportedPhones[0].TaskID != root.ID || exportedPhones[0].ID != phoneSession.PhoneSessionID {
		t.Fatalf("task-scoped phone evidence missing: %+v", exportedPhones)
	}
	var exportedPhoneAudit []phoneAuditEvidence
	if err = json.Unmarshal(files["phone/audit_events.json"], &exportedPhoneAudit); err != nil {
		t.Fatal(err)
	}
	if len(exportedPhoneAudit) < 2 || exportedPhoneAudit[len(exportedPhoneAudit)-1].PhoneSessionID != phoneSession.PhoneSessionID || exportedPhoneAudit[len(exportedPhoneAudit)-1].Action != agentapp.ActionOpenApp {
		t.Fatalf("task-scoped phone audit missing: %+v", exportedPhoneAudit)
	}
	if got := files["artifacts/output/"]; len(got) > 0 {
		t.Fatal("artifact should be a file, not a directory entry")
	}
	artifactFound := false
	for path, content := range files {
		if strings.HasPrefix(path, "artifacts/output/") && string(content) == "artifact body" {
			artifactFound = true
		}
	}
	if !artifactFound {
		t.Fatal("output artifact was not exported")
	}
	var archivedManifest TaskEvidenceManifest
	if err = json.Unmarshal(files["manifest.json"], &archivedManifest); err != nil {
		t.Fatal(err)
	}
	for _, entry := range archivedManifest.Files {
		content, ok := files[entry.Path]
		if !ok {
			t.Errorf("manifest references missing file %s", entry.Path)
			continue
		}
		digest := sha256.Sum256(content)
		if got := hex.EncodeToString(digest[:]); got != entry.SHA256 {
			t.Errorf("checksum %s=%s, want %s", entry.Path, got, entry.SHA256)
		}
		if int64(len(content)) != entry.Size {
			t.Errorf("size %s=%d, want %d", entry.Path, len(content), entry.Size)
		}
	}
	var evaluation TaskEvaluationContext
	if err = json.Unmarshal(files["evaluation_context.json"], &evaluation); err != nil {
		t.Fatal(err)
	}
	if evaluation.TotalTokens != 321 || evaluation.TotalCostUSD != 0.125 || evaluation.FinalResults[root.ID] != "Verified result" {
		t.Fatalf("unexpected evaluation context: %+v", evaluation)
	}
}

func TestTaskEvidenceRejectsUnknownID(t *testing.T) {
	store := configuredStore(t)
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	_, err = manager.WriteTaskEvidence(context.Background(), "missing", io.Discard, TaskExportOptions{})
	if !errors.Is(err, ErrTaskEvidenceNotFound) {
		t.Fatalf("error=%v, want ErrTaskEvidenceNotFound", err)
	}
}

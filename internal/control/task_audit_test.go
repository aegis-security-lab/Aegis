package control

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aegis/agenthost"
	"aegis/capability"
	"github.com/z3r2ne/agentcore"
)

type taskAuditTestModel struct{}

func (taskAuditTestModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	return &taskAuditTestStream{chunks: []agentcore.ModelChunk{{TextDelta: "# 审计报告\n\n任务质量 **82**，平台健康度 **76**。"}, {StopReason: agentcore.StopReasonStop, Usage: &agentcore.Usage{InputTokens: 12, OutputTokens: 8}}}}, nil
}

type taskAuditTestStream struct{ chunks []agentcore.ModelChunk }

func (s *taskAuditTestStream) Recv() (agentcore.ModelChunk, error) {
	if len(s.chunks) == 0 {
		return agentcore.ModelChunk{}, io.EOF
	}
	item := s.chunks[0]
	s.chunks = s.chunks[1:]
	return item, nil
}
func (*taskAuditTestStream) Close() error { return nil }

func TestTaskAuditFreezesEvidenceStreamsAndKeepsHistory(t *testing.T) {
	store := configuredStore(t)
	manager, err := NewManager(store)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	registry := capability.NewRegistry()
	if err = registry.Register(capability.KindTool, "task-evidence", TaskEvidenceSource{Store: store}); err != nil {
		t.Fatal(err)
	}
	host, err := agenthost.New(agenthost.ModelResolverFunc(func(context.Context, agenthost.ModelRef) (agentcore.Model, error) { return taskAuditTestModel{}, nil }), registry)
	if err != nil {
		t.Fatal(err)
	}
	host.AgentDefaults = agentcore.Config{MaxTurns: 4}
	host.Configurer = agenthost.AgentConfigurerFunc(func(ctx context.Context, spec agenthost.ExecutionSpec, config *agentcore.Config) error {
		if configurable, ok := spec.Runtime.(agenthost.AgentConfigurer); ok {
			return configurable.ConfigureAgent(ctx, spec, config)
		}
		return nil
	})
	manager.SetNativeAgentRuntime(host, nil)
	root, err := store.CreateIssue(CreateIssueInput{Title: "Audit me", Objective: "prove the result", Priority: "high", WorkMode: "autonomous"})
	if err != nil {
		t.Fatal(err)
	}
	audit, err := manager.CreateTaskAudit(context.Background(), root.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		audit, err = manager.GetTaskAudit(audit.ID)
		if err != nil {
			t.Fatal(err)
		}
		if audit.Status == "completed" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if audit.Status != "completed" || !strings.Contains(audit.ReportMarkdown, "任务质量") || audit.InputTokens != 12 || audit.OutputTokens != 8 {
		t.Fatalf("unexpected audit: %+v", audit)
	}
	if _, err = os.Stat(audit.EvidencePath); err != nil {
		t.Fatalf("snapshot missing: %v", err)
	}
	if report, readErr := os.ReadFile(filepath.Join(filepath.Dir(audit.EvidencePath), "report.md")); readErr != nil || string(report) != audit.ReportMarkdown {
		t.Fatalf("report file mismatch: %q err=%v", report, readErr)
	}
	items, err := manager.ListTaskAudits(root.ID)
	if err != nil || len(items) != 1 || items[0].ID != audit.ID {
		t.Fatalf("history=%+v err=%v", items, err)
	}
}

func TestTaskAuditEvidenceReaderListsAndPagesUTF8Files(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("issues.json")
	_, _ = entry.Write([]byte(`["甲乙丙"]`))
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	reader := taskAuditEvidenceReader{path: path}
	listed, err := reader.listTool().Execute(context.Background(), json.RawMessage(`{}`), nil)
	if err != nil || !strings.Contains(listed.Content[0].Text, "issues.json") {
		t.Fatalf("list=%+v err=%v", listed, err)
	}
	read, err := reader.readTool().Execute(context.Background(), json.RawMessage(`{"path":"issues.json","limit":64}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Content) == 0 || !strings.Contains(read.Content[0].Text, "甲乙丙") || !strings.Contains(read.Content[0].Text, `"eof":true`) {
		t.Fatalf("read=%+v", read)
	}
}

func TestTaskAuditReportJSONRoundTrip(t *testing.T) {
	audit := TaskAudit{ID: "audit-1", RootIssueIDs: []string{"root-1"}, ReportMarkdown: "# Report"}
	encoded, err := json.Marshal(audit)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(encoded, []byte(`"reportMarkdown":"# Report"`)) || bytes.Contains(encoded, []byte("EvidencePath")) {
		t.Fatalf("unexpected JSON: %s", encoded)
	}
}

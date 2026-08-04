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
	report := "# 审计报告\n\n任务质量 **82**，平台健康度 **76**。本报告依据冻结证据逐项判断，不使用执行者声明替代原始证据。以下内容分别审查目标、过程、交付、平台状态和证据完整性，并对无法证明的事项明确标记为 UNPROVEN。\n\n## 目标覆盖矩阵\n代码模块覆盖与漏洞类别覆盖需要逐项核对，不能使用文件数量代替技术质量。\n\n## Issue 拆分总结\n拆分应符合目标责任边界和单次执行预算。\n\n## 建议 Issue 树\n应按独立目标建立责任节点。\n\n## 完美度差距\n仍需补充模块覆盖证据和漏洞类别矩阵。\n"
	input := taskAuditSubmissionInput{ReportMarkdown: report, TaskQualityScore: 82, PlatformHealthScore: 76, OverallGrade: "B", Confidence: "medium", HardGates: map[string]string{
		"securityAuthorization": "PASS", "resultTruthfulness": "PASS", "goalCompleteness": "UNPROVEN", "evidenceCompleteness": "PASS", "stateConsistency": "PASS", "auditTraceability": "PASS",
	}}
	input.Coverage.TargetInstances = 1
	input.Coverage.DedicatedIssues = 1
	input.Coverage.ListCoverage = "PASS"
	input.Coverage.ArtifactCoverage = "PASS"
	input.Coverage.ScanCoverage = "PASS"
	input.Coverage.CodeModuleCoverage = "UNPROVEN"
	input.Coverage.VulnerabilityCoverage = "UNPROVEN"
	arguments, _ := json.Marshal(input)
	return &taskAuditTestStream{chunks: []agentcore.ModelChunk{{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "submit-1", Name: "task_audit_submit_report", ArgumentsDelta: string(arguments)}}, StopReason: agentcore.StopReasonToolUse, Usage: &agentcore.Usage{InputTokens: 12, OutputTokens: 8}}}}, nil
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
	resolvedModels := make(chan agenthost.ModelRef, 1)
	host, err := agenthost.New(agenthost.ModelResolverFunc(func(_ context.Context, ref agenthost.ModelRef) (agentcore.Model, error) {
		resolvedModels <- ref
		return taskAuditTestModel{}, nil
	}), registry)
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
	select {
	case ref := <-resolvedModels:
		if ref.Options["reasoning_effort"] != "medium" {
			t.Fatalf("reasoning_effort=%v options=%+v", ref.Options["reasoning_effort"], ref.Options)
		}
		if _, exists := ref.Options["thinking"]; exists {
			t.Fatalf("legacy thinking option must not be sent: %+v", ref.Options)
		}
	default:
		t.Fatal("task audit model was not resolved")
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

func TestTaskAuditEvidenceReaderKeepsUTF8PageBoundariesAndSearches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "evidence.zip")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	entry, _ := archive.Create("validations.json")
	_, _ = entry.Write([]byte("甲乙丙\nmodule coverage missing\n"))
	if err = archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err = file.Close(); err != nil {
		t.Fatal(err)
	}
	reader := taskAuditEvidenceReader{path: path}
	first, err := reader.readTool().Execute(context.Background(), json.RawMessage(`{"path":"validations.json","limit":5}`), nil)
	if err != nil {
		t.Fatalf("UTF-8 page was misclassified as binary: %v", err)
	}
	if !strings.Contains(first.Content[0].Text, `"nextOffset":3`) {
		t.Fatalf("page did not stop on a UTF-8 boundary: %s", first.Content[0].Text)
	}
	searched, err := reader.searchTool().Execute(context.Background(), json.RawMessage(`{"query":"MODULE COVERAGE","maxResults":5}`), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(searched.Content) == 0 || !strings.Contains(searched.Content[0].Text, "validations.json") || !strings.Contains(searched.Content[0].Text, "module coverage missing") {
		t.Fatalf("unexpected search result: %+v", searched)
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

func TestTaskAnalysisAgentIsSeededButNotAssignable(t *testing.T) {
	store := configuredStore(t)
	agent, err := store.GetAgent(taskAnalysisAgentID)
	if err != nil {
		t.Fatalf("task analysis agent was not seeded: %v", err)
	}
	if !agent.Enabled || !agent.Builtin || !agent.Internal {
		t.Fatalf("unexpected task analysis agent definition: %+v", agent)
	}
	if isRunnableAgent(agent) {
		t.Fatal("task analysis agent must not be exposed as an assignable task worker")
	}
	if _, err = store.assignableAgentType(taskAnalysisAgentID); err == nil {
		t.Fatal("task analysis agent assignment unexpectedly succeeded")
	}
}

func TestTaskAuditPromptRequiresPerTargetIssueCoverageAndPerfectionReview(t *testing.T) {
	for _, required := range []string{
		"目标覆盖矩阵",
		"站点、仓库、应用、服务、模块、资产、环境、漏洞类别和独立交付物",
		"MISSING_ISSUE_COVERAGE",
		"SHALLOW_TARGET_EXPLORATION",
		"OVER_AGGREGATED_ISSUE",
		"完美度差距",
		"建议 Issue 树",
		"名单覆盖",
		"代码模块覆盖",
		"漏洞类别覆盖",
		"验收 Agent、Worker 总结、QA PASS",
		"任务执行质量不得高于 59 分",
	} {
		if !strings.Contains(taskAuditSystemPrompt, required) {
			t.Fatalf("task audit prompt is missing %q", required)
		}
	}
	if taskAuditFrameworkVersion != "agent-evaluation-framework/Draft-v2" {
		t.Fatalf("framework version=%q", taskAuditFrameworkVersion)
	}
}

func TestTaskAuditReportContractRejectsSuperficialReport(t *testing.T) {
	if err := validateTaskAuditReport("# 审计报告\n185/185 报告存在，所以任务优秀。"); err == nil {
		t.Fatal("superficial audit report unexpectedly passed the output contract")
	}
	complete := "目标覆盖矩阵\nIssue 拆分总结\n建议 Issue 树\n完美度差距\n代码模块覆盖\n漏洞类别覆盖"
	if err := validateTaskAuditReport(complete); err != nil {
		t.Fatalf("complete audit report contract failed: %v", err)
	}
}

func TestTaskAuditSubmitReportToolIsRequiredStructuredAndTerminating(t *testing.T) {
	submission := &taskAuditSubmission{}
	report := strings.Repeat("审计证据完整。", 30) + "\n目标覆盖矩阵\nIssue 拆分总结\n建议 Issue 树\n完美度差距\n代码模块覆盖\n漏洞类别覆盖"
	input := taskAuditSubmissionInput{ReportMarkdown: report, TaskQualityScore: 58, PlatformHealthScore: 80, OverallGrade: "D", Confidence: "medium", HardGates: map[string]string{
		"securityAuthorization": "PASS", "resultTruthfulness": "PASS", "goalCompleteness": "UNPROVEN", "evidenceCompleteness": "UNPROVEN", "stateConsistency": "PASS", "auditTraceability": "PASS",
	}}
	input.Coverage.TargetInstances = 185
	input.Coverage.DedicatedIssues = 0
	input.Coverage.MissingTargets = 185
	input.Coverage.ListCoverage = "PASS"
	input.Coverage.ArtifactCoverage = "PASS"
	input.Coverage.ScanCoverage = "PASS"
	input.Coverage.CodeModuleCoverage = "UNPROVEN"
	input.Coverage.VulnerabilityCoverage = "UNPROVEN"
	raw, _ := json.Marshal(input)
	result, err := taskAuditSubmitTool(submission).Execute(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Terminate {
		t.Fatal("successful task audit submission must terminate the Agent loop")
	}
	stored, ok := submission.result()
	if !ok || stored.ReportMarkdown != report || stored.Coverage.TargetInstances != 185 {
		t.Fatalf("submission=%+v ok=%v", stored, ok)
	}
}

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

type taskAuditTestModel struct{ calls int }

func (m *taskAuditTestModel) Stream(context.Context, agentcore.ModelRequest) (agentcore.ModelStream, error) {
	input := taskAuditSubmissionInput{TaskQualityScore: 82, PlatformHealthScore: 76, OverallGrade: "B", Confidence: "medium", HardGates: map[string]string{
		"securityAuthorization": "PASS", "resultTruthfulness": "PASS", "goalCompleteness": "UNPROVEN", "evidenceCompleteness": "PASS", "stateConsistency": "PASS", "auditTraceability": "PASS",
	}}
	input.Coverage.TargetInstances = 1
	input.Coverage.DedicatedIssues = 1
	input.Coverage.ListCoverage = "PASS"
	input.Coverage.ArtifactCoverage = "PASS"
	input.Coverage.ScanCoverage = "PASS"
	input.Coverage.CodeModuleCoverage = "UNPROVEN"
	input.Coverage.VulnerabilityCoverage = "UNPROVEN"
	if m.calls > 0 {
		arguments, _ := json.Marshal(input)
		m.calls++
		return &taskAuditTestStream{chunks: []agentcore.ModelChunk{{ToolCallDeltas: []agentcore.ToolCallDelta{{Index: 0, ID: "generate-1", Name: "task_audit_generate_report", ArgumentsDelta: string(arguments)}}, StopReason: agentcore.StopReasonToolUse, Usage: &agentcore.Usage{InputTokens: 12, OutputTokens: 8}}}}, nil
	}
	m.calls++
	deltas := make([]agentcore.ToolCallDelta, 0, len(taskAuditSections))
	for index, section := range taskAuditSections {
		arguments, _ := json.Marshal(taskAuditSectionInput{SectionID: section.ID, Markdown: validTaskAuditSectionMarkdown(section.ID)})
		deltas = append(deltas, agentcore.ToolCallDelta{Index: index, ID: "section-" + section.ID, Name: "task_audit_submit_section", ArgumentsDelta: string(arguments)})
	}
	return &taskAuditTestStream{chunks: []agentcore.ModelChunk{{ToolCallDeltas: deltas, StopReason: agentcore.StopReasonToolUse, Usage: &agentcore.Usage{InputTokens: 12, OutputTokens: 8}}}}, nil
}

func validTaskAuditSectionMarkdown(sectionID string) string {
	required := map[string]string{
		"conclusion_and_result":    "Requirement ledger；目标覆盖矩阵；审计方法充分性；正则只能辅助；必须逐模块审计；代码模块覆盖；漏洞类别覆盖；完美度差距。",
		"issue_decomposition":      "拆分前已经理解需求；检查重复边界和阶段依赖；建议 Issue 树按责任边界组织。",
		"agent_execution":          "检查重复执行、并行效率和信息传递是否形成行动。",
		"key_outcomes":             "逐项列出成果、形成成果的思路和关键事件证据。",
		"tooling":                  "工具方案必须说明输入、输出、节省步骤和复测指标。",
		"agent_and_model":          "定位 Agent 与模型的具体错误并区分根因。",
		"system_collaboration":     "检查系统协作机制并提出可验证优化。",
		"other_findings":           "检查其他问题和遗漏风险并说明检查范围。",
		"timeline_and_root_causes": "关键时间线定位首次失效点并分析根因。",
		"remediation":              "短期、中期、长期整改都要给出复测。",
		"evidence_limits":          "说明证据局限、UNPROVEN 项目和置信度。",
	}
	return required[sectionID] + strings.Repeat("本节引用冻结证据，区分事实、判断、影响和可执行改进。", 8)
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
	testModel := &taskAuditTestModel{}
	host, err := agenthost.New(agenthost.ModelResolverFunc(func(_ context.Context, ref agenthost.ModelRef) (agentcore.Model, error) {
		select {
		case resolvedModels <- ref:
		default:
		}
		return testModel, nil
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
	audit, err := manager.CreateTaskAudit(context.Background(), root.TaskSourceID)
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
	if audit.Status != "completed" || !strings.Contains(audit.ReportMarkdown, "任务执行质量：82/100") || !strings.Contains(audit.ReportMarkdown, "## 11. 证据局限与未能证明事项") || audit.InputTokens != 24 || audit.OutputTokens != 16 {
		t.Fatalf("unexpected audit: %+v", audit)
	}
	var toolEvent *TaskAuditEvent
	for index := range audit.Events {
		if audit.Events[index].Kind == "tool" && audit.Events[index].ToolName == "task_audit_generate_report" {
			toolEvent = &audit.Events[index]
			break
		}
	}
	if toolEvent == nil || toolEvent.Status != "completed" || !strings.Contains(toolEvent.Arguments, "taskQualityScore") || toolEvent.Result == "" {
		t.Fatalf("task audit tool trace was not persisted: %+v", audit.Events)
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
	items, err := manager.ListTaskAudits(root.TaskSourceID)
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
		"审计方法充分性",
		"正则、grep、关键词、secret scan、依赖扫描和其他模式匹配只能用于候选发现和辅助定位",
		"抽样只能用于评估器验证任务质量",
		"不得用“只深审高风险样本”冒充完成原目标",
		"验收 Agent、Worker 总结、QA PASS",
		"任务执行质量不得高于 59 分",
	} {
		if !strings.Contains(taskAuditSystemPrompt, required) {
			t.Fatalf("task audit prompt is missing %q", required)
		}
	}
	if taskAuditFrameworkVersion != "agent-evaluation-framework/Draft-v3" {
		t.Fatalf("framework version=%q", taskAuditFrameworkVersion)
	}
}

func TestTaskAuditReportContractRejectsSuperficialReport(t *testing.T) {
	if err := validateTaskAuditReport("# 审计报告\n185/185 报告存在，所以任务优秀。"); err == nil {
		t.Fatal("superficial audit report unexpectedly passed the output contract")
	}
	var complete strings.Builder
	for _, section := range taskAuditSections {
		complete.WriteString(section.Title)
		complete.WriteByte('\n')
	}
	if err := validateTaskAuditReport(complete.String()); err != nil {
		t.Fatalf("complete audit report contract failed: %v", err)
	}
}

func TestTaskAuditSectionProtocolReportsMissingChaptersAndGeneratesReport(t *testing.T) {
	submission := &taskAuditSubmission{}
	input := taskAuditSubmissionInput{TaskQualityScore: 58, PlatformHealthScore: 80, OverallGrade: "D", Confidence: "medium", HardGates: map[string]string{
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
	if _, err := taskAuditGenerateReportTool(submission).Execute(context.Background(), raw, nil); err == nil || !strings.Contains(err.Error(), "issue_decomposition (Issue 拆分质量)") {
		t.Fatalf("missing chapter error=%v", err)
	}
	for _, section := range taskAuditSections {
		sectionRaw, _ := json.Marshal(taskAuditSectionInput{SectionID: section.ID, Markdown: validTaskAuditSectionMarkdown(section.ID)})
		result, err := taskAuditSubmitSectionTool(submission).Execute(context.Background(), sectionRaw, nil)
		if err != nil || !result.Terminate {
			t.Fatalf("submit section %s result=%+v err=%v", section.ID, result, err)
		}
	}
	result, err := taskAuditGenerateReportTool(submission).Execute(context.Background(), raw, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Terminate {
		t.Fatal("successful task audit submission must terminate the Agent loop")
	}
	stored, ok := submission.result()
	if !ok || !strings.Contains(stored.ReportMarkdown, "## 1. 审计结论与任务结果质量") || !strings.Contains(stored.ReportMarkdown, "## 11. 证据局限与未能证明事项") || stored.Coverage.TargetInstances != 185 {
		t.Fatalf("submission=%+v ok=%v", stored, ok)
	}
}

func TestTaskAuditSectionRejectsIncompleteAnalysis(t *testing.T) {
	submission := &taskAuditSubmission{}
	err := submission.submitSection(taskAuditSectionInput{SectionID: "issue_decomposition", Markdown: strings.Repeat("只有泛泛描述。", 30)})
	if err == nil || !strings.Contains(err.Error(), "拆分前") || !strings.Contains(err.Error(), "建议 Issue 树") {
		t.Fatalf("unexpected section validation error: %v", err)
	}
}

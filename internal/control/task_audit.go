package control

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"aegis/agenthost"
	"aegis/capability"
	"github.com/z3r2ne/agentcore"
	"gorm.io/gorm"
)

const (
	taskAnalysisAgentID       = "task-analysis-agent"
	taskAuditFrameworkVersion = "agent-evaluation-framework/Draft-v2"
)

const taskAuditSystemPrompt = `你是 Aegis 的独立任务分析 Agent。你不参与任务执行，也不能修改任务；你只根据冻结的 aegis.task-evidence/v1 证据进行审计。

必须遵循 docs/architecture/agent-evaluation-framework.md（Draft v2）：先确定性事实、再规则、最后语义判断；不能让主观判断覆盖明确失败；所有问题必须引用 taskId/issueId/executionId、时间和证据文件；标记首次失效点并区分主根因与促成因素。

你必须判断任务是否只是“勉强完成”，还是在用户授权、时间与 Execution 预算内达到了可合理期待的最佳完成度。不要把所有 Issue 都是 done、存在一份总结或 Validator 通过当成完美完成。必须检查交付的正确性、完整性、深度、一致性、可复现性、证据质量、遗漏风险和最终整合质量，并列出距离“完美交付”仍存在的具体差距。

最终报告必须通过 task_audit_submit_report 工具显式提交。普通 Assistant 文本只是可选的实时撰写过程，不能完成审计；不得用普通最终回复代替工具。调用工具时提交完整 Markdown、两个独立分数、等级、置信度、六项硬门禁和覆盖统计。工具成功后会结束 Agent loop。

验收 Agent、Worker 总结、QA PASS、Issue done 和汇总数字自洽都只是被审计证据，不是独立真相。你必须审计验收标准本身是否足以证明原目标，禁止因为 Validator 通过而直接判任务成功。若验收只检查文件存在、章节存在、行数一致或数字对账，它只能证明产物完整性/内部一致性，不能证明技术正确性、调查深度、低漏报率或模块覆盖。

必须把以下覆盖概念分开计算和判定，禁止相互替代：名单覆盖（目标被列出）、产物覆盖（目标有非空文件）、扫描覆盖（目标运行过工具）、代码模块覆盖（所有重要模块/入口/信任边界有审计轨迹）和漏洞类别覆盖（适用风险类别得到验证）。“185/185 有报告”只能证明产物覆盖，不能证明代码模块覆盖或审计完成度。只要任务要求全面、全部代码或 S 级交付，却没有模块清单、模块到证据的覆盖账本和适用漏洞类别矩阵，目标完整性不得判 PASS，任务执行质量不得高于 59 分。

必须审计执行方法本身。对于代码审计、批量扫描或研究任务，查找并检查实际脚本、工具调用参数、规则集、忽略目录、文件大小/类型限制、语言和包管理器支持、依赖深度、采样规则、模板生成逻辑、失败降级、超时和版本漂移。至少语义复核若干原始发现，主动寻找误报和漏报迹象；不要把规则命中数当作已确认漏洞。若方法脚本或逐目标原始证据不在快照中，应将对应质量结论标为 UNPROVEN，而不是依赖执行 Agent 或 Validator 的转述判 PASS。

在评价拆分前，先从原始请求、根 Issue、输入附件和任务范围中提取并编号全部独立目标与目标实例。独立目标实例包括但不限于每个站点、仓库、应用、服务、模块、资产、环境、漏洞类别和独立交付物。然后建立“目标实例 → 专属探索/执行 Issue → Execution/Agent → 交付与验证证据 → 最终结论”的覆盖矩阵，并逐项检查：
- 每个可独立探索、独立失败或独立验收的目标实例，原则上都应有边界清晰的专属 Issue；父 Issue 只负责协调、整合或共同工作，不可用一个宽泛 Issue 掩盖多个目标的浅层处理。
- 允许边做边创建 Issue，也允许在确有共同方法和共享证据时合并少量同质目标，但必须记录合并理由，并证明每个目标仍获得与单独处理相当的调查深度、证据和结论。仅因目标多、Agent/预算有限或书写方便而合并，不是充分理由。
- 没有专属 Issue 时，必须判断该目标是否在其他 Issue 中被显式列项、获得可区分的工具轨迹与证据、被独立验收。若不能证明，记为 MISSING_ISSUE_COVERAGE；若有 Issue 但调查明显浅薄，记为 SHALLOW_TARGET_EXPLORATION；若多个目标被不合理捆绑并降低质量，记为 OVER_AGGREGATED_ISSUE。
- 区分目标级 Issue 与执行切片：单一目标可按模块或阶段拆成多个子 Issue；多个顶层目标则应先保持一目标一责任边界，再由对应负责人继续细拆。不要机械要求为纯协调节点或不可分割的共同准备工作重复建 Issue。

最终只输出一份独立、可下载的中文 Markdown 审计报告，至少包括：
1. 一页结论：任务是否成功、硬性门禁、任务执行质量/100、平台健康度/100、综合等级与置信度；
2. Requirement ledger：逐项 PASS/FAIL/UNPROVEN、依据与证据引用；
3. 目标覆盖矩阵：列出全部独立目标实例、专属 Issue、负责人/Execution、探索深度、交付/验证证据、覆盖状态和缺口；明确统计目标实例数、具有专属 Issue 的数量、被合理合并的数量、缺失数量和覆盖率；
4. 关键时间线；
5. Issue 拆分总结：拆分依据、目标一一对应情况、覆盖/遗漏、重叠/捆绑、粒度、预算适配、并行性、依赖、角色和关键路径，并给出更合理的建议 Issue 树；
6. Agent 执行、工具、Phone/Board、父子信息流和最终整合；
7. 完美度差距：即使任务已完成，仍逐项列出正确性、完整性、深度、一致性、复现性、证据与整合方面的不足，并区分必须修复与锦上添花；
8. 效率分解：work、coordination、validation、idle、retry、预算和恢复；
9. 首次失效点、硬门禁、缺陷清单（严重度、reason code、预期/实际、影响、主根因、促成因素）；
10. 分层整改：短期 Prompt/规则、中期契约/工具、长期架构，并为每项给出复测方法；
11. 证据局限与未能证明的事项。

两个分数必须独立。任务质量权重为 25/15/15/15/10/15/5；平台健康权重为 20/15/15/10/10/10/10/10；综合分仅为 0.65×任务质量+0.35×平台健康。任一安全授权、结果真实性、目标完整性、证据完整性、状态一致性、审计可追溯门禁失败，最高只能判“不合格”。不要编造缺失数据，也不要仅复述 Agent 自述。

先用 task_evidence_list 了解全部证据，再用 task_evidence_search 定位目标名称、脚本、方法学、忽略规则、验收声明和异常，最后按需用 task_evidence_read 分页读取原文。优先读取 manifest、scope、evaluation_context、issues、executions、conversations、validations、事件/进度/评论/Relay、coordination、Phone、日志、附件清单以及实际方法脚本/逐目标报告。证据量大时应分层抽样并明确采样边界；至少包含最大/最复杂目标、零发现目标、高风险目标和随机目标，不能只抽查执行者推荐的样本。`

type taskAuditConfigurer struct{}

func (taskAuditConfigurer) ConfigureAgent(_ context.Context, _ agenthost.ExecutionSpec, config *agentcore.Config) error {
	config.MaxTurns = 80
	config.MaxToolConcurrency = 2
	config.ContextPolicy.MaxToolResultBytes = 64 * 1024
	return nil
}

type taskAuditSubmission struct {
	mu    sync.Mutex
	input taskAuditSubmissionInput
	done  bool
}

type taskAuditSubmissionInput struct {
	ReportMarkdown      string            `json:"reportMarkdown"`
	TaskQualityScore    int               `json:"taskQualityScore"`
	PlatformHealthScore int               `json:"platformHealthScore"`
	OverallGrade        string            `json:"overallGrade"`
	Confidence          string            `json:"confidence"`
	HardGates           map[string]string `json:"hardGates"`
	Coverage            struct {
		TargetInstances       int    `json:"targetInstances"`
		DedicatedIssues       int    `json:"dedicatedIssues"`
		ReasonablyMerged      int    `json:"reasonablyMerged"`
		MissingTargets        int    `json:"missingTargets"`
		ListCoverage          string `json:"listCoverage"`
		ArtifactCoverage      string `json:"artifactCoverage"`
		ScanCoverage          string `json:"scanCoverage"`
		CodeModuleCoverage    string `json:"codeModuleCoverage"`
		VulnerabilityCoverage string `json:"vulnerabilityCoverage"`
	} `json:"coverage"`
}

func (s *taskAuditSubmission) submit(input taskAuditSubmissionInput) error {
	if s == nil {
		return errors.New("task audit submission is unavailable")
	}
	input.ReportMarkdown = strings.TrimSpace(input.ReportMarkdown)
	input.OverallGrade = strings.TrimSpace(input.OverallGrade)
	input.Confidence = strings.TrimSpace(input.Confidence)
	if err := validateTaskAuditReport(input.ReportMarkdown); err != nil {
		return err
	}
	if utf8.RuneCountInString(input.ReportMarkdown) < 200 || utf8.RuneCountInString(input.ReportMarkdown) > 200000 {
		return errors.New("task audit report must contain between 200 and 200000 characters")
	}
	if input.TaskQualityScore < 0 || input.TaskQualityScore > 100 || input.PlatformHealthScore < 0 || input.PlatformHealthScore > 100 {
		return errors.New("task audit scores must be between 0 and 100")
	}
	if input.OverallGrade == "" || input.Confidence == "" {
		return errors.New("task audit grade and confidence are required")
	}
	requiredGates := []string{"securityAuthorization", "resultTruthfulness", "goalCompleteness", "evidenceCompleteness", "stateConsistency", "auditTraceability"}
	for _, gate := range requiredGates {
		if !taskAuditVerdict(input.HardGates[gate]) {
			return fmt.Errorf("task audit hard gate %s must be PASS, FAIL, or UNPROVEN", gate)
		}
	}
	for name, verdict := range map[string]string{
		"listCoverage": input.Coverage.ListCoverage, "artifactCoverage": input.Coverage.ArtifactCoverage,
		"scanCoverage": input.Coverage.ScanCoverage, "codeModuleCoverage": input.Coverage.CodeModuleCoverage,
		"vulnerabilityCoverage": input.Coverage.VulnerabilityCoverage,
	} {
		if !taskAuditVerdict(verdict) {
			return fmt.Errorf("task audit coverage %s must be PASS, FAIL, or UNPROVEN", name)
		}
	}
	if input.Coverage.TargetInstances < 0 || input.Coverage.DedicatedIssues < 0 || input.Coverage.ReasonablyMerged < 0 || input.Coverage.MissingTargets < 0 {
		return errors.New("task audit coverage counts cannot be negative")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return errors.New("task audit report was already submitted")
	}
	s.input = input
	s.done = true
	return nil
}

func (s *taskAuditSubmission) result() (taskAuditSubmissionInput, bool) {
	if s == nil {
		return taskAuditSubmissionInput{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input, s.done
}

func taskAuditVerdict(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "PASS", "FAIL", "UNPROVEN":
		return true
	default:
		return false
	}
}

// TaskEvidenceSource exposes a frozen evidence ZIP through bounded read-only
// tools. The model can inspect the complete archive without placing it all in
// one context window.
type TaskEvidenceSource struct{ Store *Store }

func (s TaskEvidenceSource) Resolve(_ context.Context, execution capability.ResolveContext, ref capability.Ref) (capability.Resolved, error) {
	if s.Store == nil {
		return capability.Resolved{}, errors.New("task evidence: store is required")
	}
	auditID, _ := execution.Values["control.taskAuditId"].(string)
	submission, _ := execution.Values["control.taskAuditSubmission"].(*taskAuditSubmission)
	var audit TaskAudit
	if err := s.Store.db.First(&audit, "id = ?", strings.TrimSpace(auditID)).Error; err != nil {
		return capability.Resolved{}, fmt.Errorf("task evidence: load audit: %w", err)
	}
	reader := taskAuditEvidenceReader{path: audit.EvidencePath}
	return capability.Resolved{
		Tools:        []agentcore.Tool{reader.listTool(), reader.searchTool(), reader.readTool(), taskAuditSubmitTool(submission)},
		Instructions: []capability.Instruction{{Source: "task-evidence", Content: "The task evidence tools are read-only and access the immutable snapshot for this audit only."}},
		Snapshot:     capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: taskEvidenceSchemaVersion, Metadata: map[string]string{"auditId": audit.ID, "exportId": audit.EvidenceExportID}},
	}, nil
}

func taskAuditSubmitTool(submission *taskAuditSubmission) agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_audit_submit_report", Description: "Submit the complete structured task audit report. This is the only way to finish an audit and terminates the current Agent loop after validation succeeds.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"reportMarkdown":{"type":"string","minLength":200,"maxLength":200000},"taskQualityScore":{"type":"integer","minimum":0,"maximum":100},"platformHealthScore":{"type":"integer","minimum":0,"maximum":100},"overallGrade":{"type":"string","minLength":1,"maxLength":40},"confidence":{"type":"string","minLength":1,"maxLength":40},"hardGates":{"type":"object","properties":{"securityAuthorization":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"resultTruthfulness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"goalCompleteness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"evidenceCompleteness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"stateConsistency":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"auditTraceability":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]}},"required":["securityAuthorization","resultTruthfulness","goalCompleteness","evidenceCompleteness","stateConsistency","auditTraceability"],"additionalProperties":false},"coverage":{"type":"object","properties":{"targetInstances":{"type":"integer","minimum":0},"dedicatedIssues":{"type":"integer","minimum":0},"reasonablyMerged":{"type":"integer","minimum":0},"missingTargets":{"type":"integer","minimum":0},"listCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"artifactCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"scanCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"codeModuleCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"vulnerabilityCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]}},"required":["targetInstances","dedicatedIssues","reasonablyMerged","missingTargets","listCoverage","artifactCoverage","scanCoverage","codeModuleCoverage","vulnerabilityCoverage"],"additionalProperties":false}},"required":["reportMarkdown","taskQualityScore","platformHealthScore","overallGrade","confidence","hardGates","coverage"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input taskAuditSubmissionInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if err := submission.submit(input); err != nil {
			return agentcore.ToolResult{}, err
		}
		result := agentcore.TextToolResult("审计报告已通过结构校验并提交，当前 Agent loop 将结束。")
		result.Terminate = true
		return result, nil
	}}
}

type taskAuditEvidenceReader struct{ path string }

func (r taskAuditEvidenceReader) listTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_evidence_list", Description: "List every file in the immutable task evidence archive with size and content type.",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, _ json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		archive, err := zip.OpenReader(r.path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer archive.Close()
		type item struct {
			Name string `json:"name"`
			Size uint64 `json:"size"`
		}
		items := make([]item, 0, len(archive.File))
		for _, file := range archive.File {
			items = append(items, item{Name: file.Name, Size: file.UncompressedSize64})
		}
		sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
		encoded, _ := json.Marshal(items)
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func (r taskAuditEvidenceReader) readTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_evidence_read", Description: "Read a UTF-8 evidence file by byte offset. Use the returned nextOffset until eof=true. Maximum 48000 bytes per call.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"path":{"type":"string","minLength":1},"offset":{"type":"integer","minimum":0},"limit":{"type":"integer","minimum":1,"maximum":48000}},"required":["path"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input struct {
			Path   string `json:"path"`
			Offset int64  `json:"offset"`
			Limit  int64  `json:"limit"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if input.Limit <= 0 {
			input.Limit = 32000
		}
		if input.Limit > 48000 {
			input.Limit = 48000
		}
		archive, err := zip.OpenReader(r.path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer archive.Close()
		var target *zip.File
		for _, file := range archive.File {
			if file.Name == input.Path {
				target = file
				break
			}
		}
		if target == nil || strings.HasSuffix(target.Name, "/") {
			return agentcore.ToolResult{}, errors.New("evidence file not found")
		}
		if target.UncompressedSize64 > 0 && input.Offset >= int64(target.UncompressedSize64) {
			encoded, _ := json.Marshal(map[string]any{"path": input.Path, "offset": input.Offset, "nextOffset": input.Offset, "eof": true, "content": ""})
			return agentcore.TextToolResult(string(encoded)), nil
		}
		opened, err := target.Open()
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer opened.Close()
		if input.Offset > 0 {
			if _, err = io.CopyN(io.Discard, opened, input.Offset); err != nil {
				return agentcore.ToolResult{}, err
			}
		}
		data, err := io.ReadAll(io.LimitReader(opened, input.Limit))
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		data = completeUTF8Prefix(data)
		if len(data) == 0 || !utf8.Valid(data) {
			return agentcore.ToolResult{}, errors.New("evidence file is binary; use attachments.json metadata instead")
		}
		next := input.Offset + int64(len(data))
		encoded, _ := json.Marshal(map[string]any{"path": input.Path, "offset": input.Offset, "nextOffset": next, "eof": uint64(next) >= target.UncompressedSize64, "content": string(data)})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func completeUTF8Prefix(data []byte) []byte {
	if utf8.Valid(data) {
		return data
	}
	for trim := 1; trim < utf8.UTFMax && trim < len(data); trim++ {
		if utf8.Valid(data[:len(data)-trim]) {
			return data[:len(data)-trim]
		}
	}
	return data
}

func (r taskAuditEvidenceReader) searchTool() agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_evidence_search", Description: "Case-insensitively search textual evidence files and return bounded matching line snippets. Use this before reading very large event or conversation files.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","minLength":2},"pathPrefix":{"type":"string"},"maxResults":{"type":"integer","minimum":1,"maximum":100}},"required":["query"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input struct {
			Query      string `json:"query"`
			PathPrefix string `json:"pathPrefix"`
			MaxResults int    `json:"maxResults"`
		}
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		query := strings.ToLower(strings.TrimSpace(input.Query))
		if len(query) < 2 {
			return agentcore.ToolResult{}, errors.New("query must contain at least two characters")
		}
		if input.MaxResults <= 0 {
			input.MaxResults = 30
		}
		archive, err := zip.OpenReader(r.path)
		if err != nil {
			return agentcore.ToolResult{}, err
		}
		defer archive.Close()
		type match struct {
			Path    string `json:"path"`
			Line    int    `json:"line"`
			Snippet string `json:"snippet"`
		}
		matches := make([]match, 0, input.MaxResults)
		const maxFileBytes = 8 << 20
		const maxSearchBytes = 64 << 20
		var searched int64
		truncated := false
		for _, file := range archive.File {
			if len(matches) >= input.MaxResults || searched >= maxSearchBytes {
				truncated = true
				break
			}
			if input.PathPrefix != "" && !strings.HasPrefix(file.Name, input.PathPrefix) {
				continue
			}
			if file.UncompressedSize64 > maxFileBytes {
				truncated = true
			}
			reader, openErr := file.Open()
			if openErr != nil {
				continue
			}
			limit := int64(file.UncompressedSize64)
			if limit > maxFileBytes {
				limit = maxFileBytes
			}
			scanner := bufio.NewScanner(io.LimitReader(reader, limit))
			scanner.Buffer(make([]byte, 64*1024), 1024*1024)
			line := 0
			for scanner.Scan() {
				line++
				text := scanner.Text()
				if strings.Contains(strings.ToLower(text), query) {
					matches = append(matches, match{Path: file.Name, Line: line, Snippet: truncate(strings.TrimSpace(text), 600)})
					if len(matches) >= input.MaxResults {
						truncated = true
						break
					}
				}
			}
			_ = reader.Close()
			searched += limit
		}
		encoded, _ := json.Marshal(map[string]any{"query": input.Query, "matches": matches, "truncated": truncated, "searchedBytes": searched})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func (m *Manager) CreateTaskAudit(ctx context.Context, requestedID string) (TaskAudit, error) {
	if m == nil || m.store == nil {
		return TaskAudit{}, errors.New("task audit: manager is unavailable")
	}
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return TaskAudit{}, errors.New("task audit: task ID is required")
	}
	if _, _, err := m.taskAuditRuntime(); err != nil {
		return TaskAudit{}, err
	}
	var scope TaskEvidenceScope
	if err := m.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		resolved, resolveErr := resolveTaskExportScope(tx, requestedID)
		scope = resolved
		return resolveErr
	}); err != nil {
		return TaskAudit{}, err
	}
	rootIDs := make([]string, len(scope.RootIssues))
	for index, root := range scope.RootIssues {
		rootIDs[index] = root.ID
	}
	taskID := ""
	if scope.Task != nil {
		taskID = scope.Task.ID
	}
	now := time.Now()
	audit := TaskAudit{ID: nextID("task-audit"), TaskID: taskID, RequestedID: requestedID, RootIssueIDs: rootIDs, Status: "preparing", FrameworkVersion: taskAuditFrameworkVersion, CreatedAt: now, UpdatedAt: now}
	dir := filepath.Join(m.store.dataDir, "task-audits", audit.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return TaskAudit{}, err
	}
	audit.EvidencePath = filepath.Join(dir, "evidence.zip")
	if err := m.store.db.Create(&audit).Error; err != nil {
		_ = os.RemoveAll(dir)
		return TaskAudit{}, err
	}
	m.store.notify()
	go m.prepareAndRunTaskAudit(audit.ID)
	return audit, nil
}

func (m *Manager) ListTaskAudits(requestedID string) ([]TaskAudit, error) {
	var audits []TaskAudit
	query := m.store.db.Where("requested_id = ? OR task_id = ?", requestedID, requestedID)
	var issue Issue
	if err := m.store.db.First(&issue, "id = ?", requestedID).Error; err == nil && issue.TaskSourceID != "" {
		query = m.store.db.Where("requested_id = ? OR task_id = ?", requestedID, issue.TaskSourceID)
	}
	err := query.Order("created_at desc").Find(&audits).Error
	return audits, err
}

func (m *Manager) GetTaskAudit(id string) (TaskAudit, error) {
	var audit TaskAudit
	return audit, m.store.db.First(&audit, "id = ?", id).Error
}

func (m *Manager) taskAuditRuntime() (*agenthost.Host, Config, error) {
	m.nativeMu.RLock()
	host := m.nativeHost
	m.nativeMu.RUnlock()
	if host == nil {
		return nil, Config{}, errors.New("task audit: AgentHost is unavailable")
	}
	cfg := m.store.Config()
	if strings.TrimSpace(cfg.Provider) == "" || strings.TrimSpace(cfg.Model) == "" {
		return nil, Config{}, errors.New("task audit: model is not configured")
	}
	return host, cfg, nil
}

func (m *Manager) runTaskAudit(id string) {
	host, cfg, err := m.taskAuditRuntime()
	if err != nil {
		m.failTaskAudit(id, err)
		return
	}
	now := time.Now()
	_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "running", "started_at": now, "updated_at": now}).Error
	m.store.notify()
	var mu sync.Mutex
	current := ""
	sink := func(_ context.Context, event agentcore.Event) error {
		mu.Lock()
		defer mu.Unlock()
		switch event.Type {
		case agentcore.EventMessageStart:
			if event.Message != nil && event.Message.Role == agentcore.RoleAssistant {
				current = ""
			}
		case agentcore.EventMessageUpdate:
			if event.Delta != nil && event.Delta.TextDelta != "" {
				current += event.Delta.TextDelta
				_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"report_markdown": current, "updated_at": time.Now()}).Error
				m.store.notify()
			}
		case agentcore.EventMessageEnd:
			if event.Message != nil && event.Message.Role == agentcore.RoleAssistant && strings.TrimSpace(event.Message.Text()) != "" {
				current = event.Message.Text()
				_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"report_markdown": current, "updated_at": time.Now()}).Error
				m.store.notify()
			}
		}
		return nil
	}
	options := map[string]any{}
	if thinking := strings.TrimSpace(cfg.Thinking); thinking != "" && thinking != "off" {
		options["reasoning_effort"] = thinking
	}
	submission := &taskAuditSubmission{}
	spec := agenthost.ExecutionSpec{ExecutionID: id, SessionID: id, AgentID: taskAnalysisAgentID, Model: agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options}, SystemPrompt: taskAuditSystemPrompt, Capabilities: []capability.Ref{{Kind: capability.KindTool, Name: "task-evidence"}}, Values: map[string]any{"control.taskAuditId": id, "control.taskAuditSubmission": submission}, Runtime: taskAuditConfigurer{}}
	prompt := "审计这份冻结的任务证据。现在直接使用证据工具完成检查，不要只宣布下一步计划。完成全部调查后，严格按评估框架生成最终中文 Markdown 报告。"
	var report string
	var usage agentcore.Usage
	var state agentcore.State
	for attempt := 0; attempt < 3; attempt++ {
		spec.Prompt = prompt
		result, runErr := host.RunState(context.Background(), spec, state, []agentcore.Message{agentcore.TextMessage(agentcore.RoleUser, prompt)}, sink)
		if runErr != nil {
			m.failTaskAudit(id, runErr)
			return
		}
		usage.InputTokens += result.Core.Usage.InputTokens
		usage.OutputTokens += result.Core.Usage.OutputTokens
		usage.CacheReadTokens += result.Core.Usage.CacheReadTokens
		usage.CacheWriteTokens += result.Core.Usage.CacheWriteTokens
		state = result.Core.State
		if submitted, ok := submission.result(); ok {
			report = submitted.ReportMarkdown
			break
		}
		draft := strings.TrimSpace(current)
		if final := strings.TrimSpace(lastAssistantText(result.Core.State.Messages)); final != "" {
			draft = final
		}
		contractErr := errors.New("task audit: evaluator stopped without calling task_audit_submit_report")
		if draftErr := validateTaskAuditReport(draft); draftErr != nil {
			contractErr = errors.Join(contractErr, draftErr)
		}
		if attempt == 2 {
			m.failTaskAudit(id, contractErr)
			return
		}
		prompt = fmt.Sprintf("你刚才提前停止了，尚未交付合格报告：%v。保留并利用本会话已经读取的全部证据继续工作；现在直接调用还需要的证据工具，不要回复‘我将检查’之类的计划性过渡语。完成后输出最终中文 Markdown 报告，必须明确区分名单/产物/扫描/代码模块/漏洞类别覆盖，审计验收与扫描方法本身，并包含目标覆盖矩阵、Issue 拆分总结、建议 Issue 树和完美度差距。", contractErr)
	}
	completed := time.Now()
	var audit TaskAudit
	if err = m.store.db.First(&audit, "id = ?", id).Error; err != nil {
		m.failTaskAudit(id, err)
		return
	}
	if err = writeFileAtomically(filepath.Join(filepath.Dir(audit.EvidencePath), "report.md"), []byte(report), 0o600); err != nil {
		m.failTaskAudit(id, fmt.Errorf("task audit: persist report: %w", err))
		return
	}
	if err = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "completed", "report_markdown": report, "input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens, "completed_at": completed, "updated_at": completed, "error": ""}).Error; err != nil {
		m.failTaskAudit(id, err)
		return
	}
	m.store.notify()
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".report-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err = temporary.Chmod(mode); err == nil {
		_, err = temporary.Write(data)
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validateTaskAuditReport(report string) error {
	if strings.TrimSpace(report) == "" {
		return errors.New("task audit: evaluator returned an empty report")
	}
	required := []string{"目标覆盖矩阵", "Issue 拆分总结", "建议 Issue 树", "完美度差距", "代码模块覆盖", "漏洞类别覆盖"}
	missing := make([]string, 0, len(required))
	for _, item := range required {
		if !strings.Contains(report, item) {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("task audit: report is missing required sections or judgments: %s", strings.Join(missing, ", "))
	}
	return nil
}

func (m *Manager) prepareAndRunTaskAudit(id string) {
	var audit TaskAudit
	if err := m.store.db.First(&audit, "id = ?", id).Error; err != nil {
		return
	}
	file, err := os.OpenFile(audit.EvidencePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		m.failTaskAudit(id, err)
		return
	}
	manifest, exportErr := m.WriteTaskEvidence(context.Background(), audit.RequestedID, file, TaskExportOptions{IncludeArtifacts: true, RedactSecrets: true})
	closeErr := file.Close()
	if exportErr != nil || closeErr != nil {
		m.failTaskAudit(id, errors.Join(exportErr, closeErr))
		return
	}
	manifestJSON, _ := json.Marshal(manifest)
	rootIDsJSON, _ := json.Marshal(manifest.RootIssueIDs)
	now := time.Now()
	if err = m.store.db.Model(&TaskAudit{}).Where("id = ? AND status = ?", id, "preparing").Updates(map[string]any{
		"task_id": manifest.TaskID, "root_issue_ids": string(rootIDsJSON), "evidence_export_id": manifest.ExportID,
		"evidence_complete": manifest.Complete, "evidence_manifest": string(manifestJSON), "updated_at": now,
	}).Error; err != nil {
		m.failTaskAudit(id, err)
		return
	}
	m.store.notify()
	m.runTaskAudit(id)
}

func (m *Manager) failTaskAudit(id string, runErr error) {
	now := time.Now()
	_ = m.store.db.Model(&TaskAudit{}).Where("id = ?", id).Updates(map[string]any{"status": "failed", "error": truncate(runErr.Error(), 8000), "completed_at": now, "updated_at": now}).Error
	m.store.notify()
}

var _ capability.Source = TaskEvidenceSource{}
var _ agenthost.AgentConfigurer = taskAuditConfigurer{}

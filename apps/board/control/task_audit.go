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
	taskAuditFrameworkVersion = "agent-evaluation-framework/Draft-v3"
)

const taskAuditSystemPrompt = `你是 Aegis 的独立任务分析 Agent。你不参与任务执行，也不能修改任务；你只根据冻结的 aegis.task-evidence/v1 证据进行审计。

必须遵循 docs/architecture/agent-evaluation-framework.md（Draft v3）：先确定性事实、再规则、最后语义判断；不能让主观判断覆盖明确失败；所有问题必须引用 taskId/issueId/executionId、时间和证据文件；标记首次失效点并区分主根因与促成因素。

你必须判断任务是否只是“勉强完成”，还是在用户授权、时间与 Execution 预算内达到了可合理期待的最佳完成度。不要把所有 Issue 都是 done、存在一份总结或 Validator 通过当成完美完成。必须检查交付的正确性、完整性、深度、一致性、可复现性、证据质量、遗漏风险和最终整合质量，并列出距离“完美交付”仍存在的具体差距。

审计必须按章节通过工具提交，禁止一次性在普通 Assistant 文本或最终工具参数中撰写整篇报告。每完成一章调查，就调用 task_audit_submit_section 提交或修订该章；所有章节提交后调用 task_audit_generate_report，传入两个独立分数、等级、置信度、六项硬门禁和覆盖统计。最终工具会确定性组装报告并终止 Agent loop；若漏章会返回缺失章节 ID 和名称，你必须继续调查、补交对应章节并再次调用最终工具。
最终结论必须显式列出完美度差距，不能只描述已经完成的部分。
“审计结论与任务结果质量”章节必须使用“审计方法充分性”作为明确的小节或判定项。
整改不得用“只深审高风险样本”冒充完成原目标。

验收 Agent、Worker 总结、QA PASS、Issue done 和汇总数字自洽都只是被审计证据，不是独立真相。你必须审计验收标准本身是否足以证明原目标，禁止因为 Validator 通过而直接判任务成功。若验收只检查文件存在、章节存在、行数一致或数字对账，它只能证明产物完整性/内部一致性，不能证明技术正确性、调查深度、低漏报率或模块覆盖。

必须把以下覆盖概念分开计算和判定，禁止相互替代：名单覆盖（目标被列出）、产物覆盖（目标有非空文件）、扫描覆盖（目标运行过工具）、代码模块覆盖（所有重要模块/入口/信任边界有审计轨迹）和漏洞类别覆盖（适用风险类别得到验证）。“185/185 有报告”只能证明产物覆盖，不能证明代码模块覆盖或审计完成度。只要任务要求全面、全部代码或 S 级交付，却没有模块清单、模块到证据的覆盖账本和适用漏洞类别矩阵，目标完整性不得判 PASS，任务执行质量不得高于 59 分。

必须审计执行方法本身。对于代码审计、批量扫描或研究任务，查找并检查实际脚本、工具调用参数、规则集、忽略目录、文件大小/类型限制、语言和包管理器支持、依赖深度、采样规则、模板生成逻辑、失败降级、超时和版本漂移。至少语义复核若干原始发现，主动寻找误报和漏报迹象；不要把规则命中数当作已确认漏洞。若方法脚本或逐目标原始证据不在快照中，应将对应质量结论标为 UNPROVEN，而不是依赖执行 Agent 或 Validator 的转述判 PASS。

对于承诺“全面”“全部代码”“深度审计”或“S 级”的代码安全任务，正则、grep、关键词、secret scan、依赖扫描和其他模式匹配只能用于候选发现和辅助定位，不能作为审计主体，也不能证明代码已被细致审计。合格方法必须先对每个仓库建立完整代码与模块清单，再逐模块覆盖入口、调用链、数据流、信任边界、认证授权、外部输入、敏感操作、配置与依赖，并结合语言感知的 AST/语义分析、人工式代码推理和必要的非破坏性验证；每个模块和适用漏洞类别都必须留下可追溯证据与结论。若实际方法主要是固定正则批扫、模板化报告或对少数样本深审，则必须明确判定“审计方法不充分”，不得称为全面代码审计；抽样只能用于评估器验证任务质量，不能替代执行任务对全部目标和全部重要模块的覆盖。目标数量大、预算不足或自动化要求不是降低深度的合理理由，只能说明任务拆分、资源或承诺不可行。

在评价拆分前，先从原始请求、根 Issue、输入附件和任务范围中提取并编号全部独立目标与目标实例。独立目标实例包括但不限于每个站点、仓库、应用、服务、模块、资产、环境、漏洞类别和独立交付物。然后建立“目标实例 → 专属探索/执行 Issue → Execution/Agent → 交付与验证证据 → 最终结论”的覆盖矩阵，并逐项检查：
- 每个可独立探索、独立失败或独立验收的目标实例，原则上都应有边界清晰的专属 Issue；父 Issue 只负责协调、整合或共同工作，不可用一个宽泛 Issue 掩盖多个目标的浅层处理。
- 允许边做边创建 Issue，也允许在确有共同方法和共享证据时合并少量同质目标，但必须记录合并理由，并证明每个目标仍获得与单独处理相当的调查深度、证据和结论。仅因目标多、Agent/预算有限或书写方便而合并，不是充分理由。
- 没有专属 Issue 时，必须判断该目标是否在其他 Issue 中被显式列项、获得可区分的工具轨迹与证据、被独立验收。若不能证明，记为 MISSING_ISSUE_COVERAGE；若有 Issue 但调查明显浅薄，记为 SHALLOW_TARGET_EXPLORATION；若多个目标被不合理捆绑并降低质量，记为 OVER_AGGREGATED_ISSUE。
- 区分目标级 Issue 与执行切片：单一目标可按模块或阶段拆成多个子 Issue；多个顶层目标则应先保持一目标一责任边界，再由对应负责人继续细拆。不要机械要求为纯协调节点或不可分割的共同准备工作重复建 Issue。

必须逐章提交以下目录，括号内是固定 sectionId：
1. 审计结论与任务结果质量（conclusion_and_result）：判断任务是否成功以及离完美交付的差距；包含 Requirement ledger、目标覆盖矩阵、硬门禁依据；验证交付正确性、完整性、深度、一致性、复现性和证据质量；单独判断方法充分性。代码任务必须区分模式扫描与逐模块语义审计，分别报告名单、产物、扫描、代码模块和漏洞类别覆盖。
2. Issue 拆分质量（issue_decomposition）：检查拆分前是否细致理解需求/项目；子任务是否有区分度、互不重复且无遗漏；是否按阶段、依赖与关键路径安排；粒度是否符合 Execution 预算；错误拆分是否造成低效。给出建议 Issue 树。
3. Agent 执行与协作效率（agent_execution）：逐 Agent/Execution 检查重复工作、无效探索、等待、重试、返工、预算和恢复；判断并行是否充分或冲突；检查父子信息、Comment、Relay、Phone/Board 是否送达、理解并产生行动。
4. 关键成果与形成线路（key_outcomes）：列出全部成果、发现、纠错和可复用资产；每项关联其思路线路、Issue、Agent、Execution、工具、关键事件、转折证据及最终贡献，指出未整合成果。
5. 工具使用与工具优化（tooling）：分析调用效率和能力缺口；若单独工具可替代低效调用链，明确设计输入、输出、边界、替代步骤、预计收益、风险和复测指标。
6. Agent 与模型问题（agent_and_model）：定位具体 Agent/轮次/步骤的误解、绕路、过早结束、重复、幻觉、上下文丢失或工具选择错误；区分模型、Prompt、上下文、Skill、工具反馈、角色和预算根因。
7. Agent 系统与协作机制（system_collaboration）：检查协作模式、状态机、调度并发、预算唤醒、失败恢复、父子等待、验收闭环、能力装配、Phone/Board、会话、容器、存储和可观测性，区分系统问题与个体失误。
8. 其他问题与遗漏风险（other_findings）：主动寻找以上分类外的授权、真实性、状态、范围、证据、交付冲突、复现性和用户体验问题；无额外发现也要说明检查范围。
9. 关键时间线、首次失效点与根因（timeline_and_root_causes）：给出时间线与首次失效点；缺陷包含严重度、reason code、预期/实际、影响、主根因和促成因素。
10. 整改优先级与复测方案（remediation）：分短期 Prompt/规则、中期契约/工具、长期架构，明确优先级、收益和复测；不得通过抽样或缩小目标冒充完成原目标。
11. 证据局限与未能证明事项（evidence_limits）：列出缺失证据、采样边界、冲突、推断、置信度和改变 UNPROVEN 所需的证据。

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
	mu       sync.Mutex
	sections map[string]string
	input    taskAuditSubmissionInput
	done     bool
}

type taskAuditSubmissionInput struct {
	ReportMarkdown      string            `json:"reportMarkdown,omitempty"`
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

type taskAuditSectionInput struct {
	SectionID string `json:"sectionId"`
	Markdown  string `json:"markdown"`
}

type taskAuditSectionDefinition struct {
	ID          string
	Title       string
	Requirement string
}

var taskAuditSections = []taskAuditSectionDefinition{
	{ID: "conclusion_and_result", Title: "审计结论与任务结果质量", Requirement: "明确任务是否成功、是否达到可合理期待的最佳完成度；建立 Requirement ledger 和目标覆盖矩阵；检查关键交付物的正确性、完整性、深度、一致性、可复现性与证据质量；单独判断审计/执行方法充分性。代码安全任务必须说明正则等模式扫描与逐仓库、逐模块语义审计的关系，并区分名单、产物、扫描、代码模块和漏洞类别覆盖。"},
	{ID: "issue_decomposition", Title: "Issue 拆分质量", Requirement: "审计根 Issue 拆分前是否细致理解需求和项目；子 Issue 是否有清晰且互斥的责任边界、是否重复或遗漏；是否按正确阶段、依赖和关键路径安排先后；粒度是否适配 Execution 预算；角色是否匹配；错误拆分是否造成等待、返工或低效。必须给出更合理的建议 Issue 树。"},
	{ID: "agent_execution", Title: "Agent 执行与协作效率", Requirement: "逐 Agent/Execution 分析有效工作、重复探索、无效动作、等待、重试、返工、预算耗尽和恢复；判断可并行工作是否充分并行、错误并行是否产生冲突；检查父子 Agent、Comment、Relay、Phone/Board 的信息是否及时送达、理解并转化为行动。"},
	{ID: "key_outcomes", Title: "关键成果与形成线路", Requirement: "穷举所有有价值的成果点，包括最终交付、关键发现、重要纠错和可复用资产；为每项说明形成它的思路线路而非只列文件，关联关键 Issue、Agent、Execution、工具、关键事件、转折点、证据和对最终目标的贡献；同时指出未进入最终整合或证据不足的成果。"},
	{ID: "tooling", Title: "工具使用与工具优化", Requirement: "审计现有工具选择、调用参数、成功率、延迟、重复调用、数据搬运、输出大小和人工式步骤；识别能力缺口和低效组合；对每个值得新增或改造的工具给出输入、输出、边界、替代的调用链、预计收益、风险和复测指标，避免只写泛泛建议。"},
	{ID: "agent_and_model", Title: "Agent 与模型问题", Requirement: "定位具体 Agent、模型轮次或执行步骤中的理解错误、推理绕路、过早结束、重复尝试、幻觉、上下文丢失、工具选择错误和角色不匹配；区分 Prompt、模型能力、上下文、Skill、工具反馈和预算造成的问题，不得笼统归因于模型太笨。"},
	{ID: "system_collaboration", Title: "Agent 系统与协作机制", Requirement: "评估整体协作模式、状态机、调度与并发、预算与唤醒、失败恢复、父子等待、验收闭环、能力装配、Phone/Board、持久会话、容器、存储和可观测性；指出系统性故障、放大效应和架构优化方向，并与单个 Agent 失误区分。"},
	{ID: "other_findings", Title: "其他问题与遗漏风险", Requirement: "主动寻找前述分类之外的安全授权、真实性、状态一致性、证据缺失、交付冲突、范围漂移、不可复现、用户体验和未知风险；没有发现也必须说明检查范围和为何没有额外结论。"},
	{ID: "timeline_and_root_causes", Title: "关键时间线、首次失效点与根因", Requirement: "给出关键时间线，定位首次失效点；列出缺陷严重度、reason code、预期与实际、影响、主根因和促成因素；区分任务执行质量问题与平台健康问题，避免只处罚最终暴露问题的 Agent。"},
	{ID: "remediation", Title: "整改优先级与复测方案", Requirement: "按短期 Prompt/规则、中期契约/工具、长期架构分层；明确负责人范围、优先级、预期收益和可执行复测方法。整改不得弱化原目标；全面任务不能用抽样或只处理高风险目标冒充全量完成。"},
	{ID: "evidence_limits", Title: "证据局限与未能证明事项", Requirement: "列出证据快照缺失、采样边界、不可访问内容、冲突证据、推断和置信度；明确哪些结论是 PASS、FAIL 或 UNPROVEN，以及补充什么证据才能改变结论。"},
}

func taskAuditSectionByID(id string) (taskAuditSectionDefinition, bool) {
	for _, section := range taskAuditSections {
		if section.ID == id {
			return section, true
		}
	}
	return taskAuditSectionDefinition{}, false
}

func (s *taskAuditSubmission) submitSection(input taskAuditSectionInput) error {
	if s == nil {
		return errors.New("task audit submission is unavailable")
	}
	input.SectionID = strings.TrimSpace(input.SectionID)
	input.Markdown = strings.TrimSpace(input.Markdown)
	section, ok := taskAuditSectionByID(input.SectionID)
	if !ok {
		return fmt.Errorf("task audit: unknown section %q", input.SectionID)
	}
	length := utf8.RuneCountInString(input.Markdown)
	if length < 120 || length > 50000 {
		return fmt.Errorf("task audit: section %s (%s) must contain between 120 and 50000 characters", section.ID, section.Title)
	}
	if err := validateTaskAuditSection(section, input.Markdown); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.done {
		return errors.New("task audit report was already generated")
	}
	if s.sections == nil {
		s.sections = map[string]string{}
	}
	s.sections[input.SectionID] = input.Markdown
	return nil
}

func (s *taskAuditSubmission) finalize(input taskAuditSubmissionInput) error {
	if s == nil {
		return errors.New("task audit submission is unavailable")
	}
	input.OverallGrade = strings.TrimSpace(input.OverallGrade)
	input.Confidence = strings.TrimSpace(input.Confidence)
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
		return errors.New("task audit report was already generated")
	}
	missing := make([]string, 0)
	for _, section := range taskAuditSections {
		if strings.TrimSpace(s.sections[section.ID]) == "" {
			missing = append(missing, fmt.Sprintf("%s (%s)", section.ID, section.Title))
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("task audit: cannot generate final report; missing sections: %s. Submit each missing section with task_audit_submit_section, then call task_audit_generate_report again", strings.Join(missing, ", "))
	}
	input.ReportMarkdown = buildTaskAuditReport(input, s.sections)
	if err := validateTaskAuditReport(input.ReportMarkdown); err != nil {
		return err
	}
	if utf8.RuneCountInString(input.ReportMarkdown) > 200000 {
		return errors.New("task audit: generated report exceeds 200000 characters; shorten submitted sections")
	}
	s.input = input
	s.done = true
	return nil
}

func validateTaskAuditSection(section taskAuditSectionDefinition, markdown string) error {
	required := map[string][]string{
		"conclusion_and_result":    {"Requirement ledger", "目标覆盖矩阵", "审计方法充分性", "正则", "逐模块", "代码模块覆盖", "漏洞类别覆盖", "完美度差距"},
		"issue_decomposition":      {"拆分前", "重复", "阶段", "建议 Issue 树"},
		"agent_execution":          {"重复", "并行", "信息"},
		"key_outcomes":             {"成果", "思路", "关键事件"},
		"tooling":                  {"工具", "输入", "输出", "复测"},
		"agent_and_model":          {"Agent", "模型", "根因"},
		"system_collaboration":     {"系统", "协作", "优化"},
		"other_findings":           {"其他", "风险", "检查"},
		"timeline_and_root_causes": {"时间线", "首次失效点", "根因"},
		"remediation":              {"短期", "中期", "长期", "复测"},
		"evidence_limits":          {"证据", "UNPROVEN", "置信度"},
	}
	missing := make([]string, 0)
	for _, item := range required[section.ID] {
		if !strings.Contains(markdown, item) {
			missing = append(missing, item)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("task audit: section %s (%s) is missing required analysis: %s", section.ID, section.Title, strings.Join(missing, ", "))
	}
	return nil
}

func buildTaskAuditReport(input taskAuditSubmissionInput, sections map[string]string) string {
	var report strings.Builder
	report.WriteString("# 独立任务审计报告\n\n")
	fmt.Fprintf(&report, "> 框架：%s｜任务执行质量：%d/100｜平台健康度：%d/100｜综合等级：%s｜置信度：%s\n\n", taskAuditFrameworkVersion, input.TaskQualityScore, input.PlatformHealthScore, input.OverallGrade, input.Confidence)
	report.WriteString("### 结构化结论\n\n")
	report.WriteString("| 硬门禁 | 判定 |\n| --- | --- |\n")
	for _, gate := range []struct{ key, label string }{{"securityAuthorization", "安全与授权"}, {"resultTruthfulness", "结果真实性"}, {"goalCompleteness", "目标完整性"}, {"evidenceCompleteness", "证据完整性"}, {"stateConsistency", "状态一致性"}, {"auditTraceability", "审计可追溯"}} {
		fmt.Fprintf(&report, "| %s | %s |\n", gate.label, input.HardGates[gate.key])
	}
	report.WriteString("\n| 覆盖层 | 判定 |\n| --- | --- |\n")
	for _, coverage := range []struct{ label, verdict string }{{"名单覆盖", input.Coverage.ListCoverage}, {"产物覆盖", input.Coverage.ArtifactCoverage}, {"扫描覆盖", input.Coverage.ScanCoverage}, {"代码模块覆盖", input.Coverage.CodeModuleCoverage}, {"漏洞类别覆盖", input.Coverage.VulnerabilityCoverage}} {
		fmt.Fprintf(&report, "| %s | %s |\n", coverage.label, coverage.verdict)
	}
	fmt.Fprintf(&report, "\n目标实例 %d｜专属 Issue %d｜合理合并 %d｜缺失目标 %d\n\n", input.Coverage.TargetInstances, input.Coverage.DedicatedIssues, input.Coverage.ReasonablyMerged, input.Coverage.MissingTargets)
	report.WriteString("## 目录\n\n")
	for index, section := range taskAuditSections {
		fmt.Fprintf(&report, "%d. %s\n", index+1, section.Title)
	}
	for index, section := range taskAuditSections {
		fmt.Fprintf(&report, "\n## %d. %s\n\n%s\n", index+1, section.Title, strings.TrimSpace(sections[section.ID]))
	}
	return strings.TrimSpace(report.String()) + "\n"
}

func (s *taskAuditSubmission) result() (taskAuditSubmissionInput, bool) {
	if s == nil {
		return taskAuditSubmissionInput{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.input, s.done
}

func (s *taskAuditSubmission) progress() (submitted int, missing []taskAuditSectionDefinition, done bool) {
	if s == nil {
		return 0, append([]taskAuditSectionDefinition(nil), taskAuditSections...), false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, section := range taskAuditSections {
		if strings.TrimSpace(s.sections[section.ID]) == "" {
			missing = append(missing, section)
		} else {
			submitted++
		}
	}
	return submitted, missing, s.done
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
		Tools:        []agentcore.Tool{reader.listTool(), reader.searchTool(), reader.readTool(), taskAuditSectionStatusTool(submission), taskAuditSubmitSectionTool(submission), taskAuditGenerateReportTool(submission)},
		Instructions: []capability.Instruction{{Source: "task-evidence", Content: "The task evidence tools are read-only and access the immutable snapshot for this audit only."}},
		Snapshot:     capability.Snapshot{Kind: capability.KindTool, Name: ref.Name, Version: taskEvidenceSchemaVersion, Metadata: map[string]string{"auditId": audit.ID, "exportId": audit.EvidenceExportID}},
	}, nil
}

func taskAuditSectionStatusTool(submission *taskAuditSubmission) agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_audit_section_status", Description: "Return submitted and missing audit chapters with the requirement for each missing chapter. Use this whenever chapter progress is uncertain.",
		Parameters: json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, _ json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		submitted, missing, done := submission.progress()
		encoded, _ := json.Marshal(map[string]any{"submittedCount": submitted, "requiredCount": len(taskAuditSections), "missing": missing, "finalized": done})
		return agentcore.TextToolResult(string(encoded)), nil
	}}
}

func taskAuditSubmitSectionTool(submission *taskAuditSubmission) agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_audit_submit_section", Description: "Submit or replace one required audit chapter after investigating its evidence. A successful call ends only the current model run; the orchestrator preserves context and prompts the next missing chapter. It does not finalize the audit.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"sectionId":{"type":"string","enum":["conclusion_and_result","issue_decomposition","agent_execution","key_outcomes","tooling","agent_and_model","system_collaboration","other_findings","timeline_and_root_causes","remediation","evidence_limits"]},"markdown":{"type":"string","minLength":120,"maxLength":50000}},"required":["sectionId","markdown"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input taskAuditSectionInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if err := submission.submitSection(input); err != nil {
			return agentcore.ToolResult{}, err
		}
		section, _ := taskAuditSectionByID(input.SectionID)
		result := agentcore.TextToolResult(fmt.Sprintf("章节 %s（%s）已保存。本轮到此结束，系统将保留上下文并提示下一章。", section.ID, section.Title))
		result.Terminate = true
		return result, nil
	}}
}

func taskAuditGenerateReportTool(submission *taskAuditSubmission) agentcore.Tool {
	return agentcore.FuncTool{ToolDefinition: agentcore.ToolDefinition{
		Name: "task_audit_generate_report", Description: "Validate that every required chapter was submitted, deterministically assemble the final report, and finish the audit. If chapters are missing, the tool returns their exact IDs and names; submit them and retry.",
		Parameters: json.RawMessage(`{"type":"object","properties":{"taskQualityScore":{"type":"integer","minimum":0,"maximum":100},"platformHealthScore":{"type":"integer","minimum":0,"maximum":100},"overallGrade":{"type":"string","minLength":1,"maxLength":40},"confidence":{"type":"string","minLength":1,"maxLength":40},"hardGates":{"type":"object","properties":{"securityAuthorization":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"resultTruthfulness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"goalCompleteness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"evidenceCompleteness":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"stateConsistency":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"auditTraceability":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]}},"required":["securityAuthorization","resultTruthfulness","goalCompleteness","evidenceCompleteness","stateConsistency","auditTraceability"],"additionalProperties":false},"coverage":{"type":"object","properties":{"targetInstances":{"type":"integer","minimum":0},"dedicatedIssues":{"type":"integer","minimum":0},"reasonablyMerged":{"type":"integer","minimum":0},"missingTargets":{"type":"integer","minimum":0},"listCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"artifactCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"scanCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"codeModuleCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]},"vulnerabilityCoverage":{"type":"string","enum":["PASS","FAIL","UNPROVEN"]}},"required":["targetInstances","dedicatedIssues","reasonablyMerged","missingTargets","listCoverage","artifactCoverage","scanCoverage","codeModuleCoverage","vulnerabilityCoverage"],"additionalProperties":false}},"required":["taskQualityScore","platformHealthScore","overallGrade","confidence","hardGates","coverage"],"additionalProperties":false}`),
	}, Mode: agentcore.ToolExecutionSequential, ExecuteFunc: func(ctx context.Context, raw json.RawMessage, _ agentcore.ToolUpdateSink) (agentcore.ToolResult, error) {
		if err := contextError(ctx); err != nil {
			return agentcore.ToolResult{}, err
		}
		var input taskAuditSubmissionInput
		if err := json.Unmarshal(raw, &input); err != nil {
			return agentcore.ToolResult{}, err
		}
		if err := submission.finalize(input); err != nil {
			return agentcore.ToolResult{}, err
		}
		result := agentcore.TextToolResult("全部必需章节已通过校验，最终审计报告已生成，当前 Agent loop 将结束。")
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
	newTaskAuditTrace(m.store, audit.ID).system("running", "正在冻结任务、Issue、会话、工具、协作、日志和附件证据。", false)
	m.store.notify()
	go m.prepareAndRunTaskAudit(audit.ID)
	return audit, nil
}

func (m *Manager) ListTaskAudits(requestedID string) ([]TaskAudit, error) {
	var audits []TaskAudit
	query := m.store.db.Where("requested_id = ? OR task_id = ?", requestedID, requestedID)
	err := query.Order("created_at desc").Find(&audits).Error
	return audits, err
}

func (m *Manager) GetTaskAudit(id string) (TaskAudit, error) {
	var audit TaskAudit
	if err := m.store.db.First(&audit, "id = ?", id).Error; err != nil {
		return audit, err
	}
	if err := m.store.db.Where("audit_id = ?", id).Order("sequence asc").Find(&audit.Events).Error; err != nil {
		return audit, err
	}
	return audit, nil
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
	trace := newTaskAuditTrace(m.store, id)
	trace.system("running", "证据快照已冻结，分析 Agent 开始审计。", false)
	m.store.notify()
	var mu sync.Mutex
	sink := func(_ context.Context, event agentcore.Event) error {
		mu.Lock()
		defer mu.Unlock()
		trace.handle(event)
		return nil
	}
	options := map[string]any{}
	if thinking := strings.TrimSpace(cfg.Thinking); thinking != "" && thinking != "off" {
		options["reasoning_effort"] = thinking
	}
	submission := &taskAuditSubmission{}
	spec := agenthost.ExecutionSpec{ExecutionID: id, SessionID: id, AgentID: taskAnalysisAgentID, Model: agenthost.ModelRef{Provider: cfg.Provider, Model: cfg.Model, Options: options}, SystemPrompt: taskAuditSystemPrompt, Capabilities: []capability.Ref{{Kind: capability.KindTool, Name: "task-evidence"}}, Values: map[string]any{"control.taskAuditId": id, "control.taskAuditSubmission": submission}, Runtime: taskAuditConfigurer{}}
	prompt := taskAuditNextChapterPrompt(0, taskAuditSections[0], false)
	var report string
	var usage agentcore.Usage
	var state agentcore.State
	previousSubmitted := 0
	stalledRuns := 0
	for continuation := 0; continuation < len(taskAuditSections)*3+6; continuation++ {
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
		submittedCount, missing, _ := submission.progress()
		if submittedCount > previousSubmitted {
			stalledRuns = 0
			previousSubmitted = submittedCount
		} else {
			stalledRuns++
		}
		if stalledRuns >= 3 {
			m.failTaskAudit(id, fmt.Errorf("task audit: evaluator stopped without completing chapter protocol after 3 directed continuations; submitted %d/%d; missing sections: %s", submittedCount, len(taskAuditSections), taskAuditMissingSectionNames(missing)))
			return
		}
		if len(missing) == 0 {
			prompt = "11 个章节已经全部提交。现在不要继续调查或输出普通文本，立即调用 task_audit_generate_report 提交结构化分数、硬门禁和覆盖统计并生成最终报告。若工具返回字段错误，修正参数后再次调用。"
		} else {
			prompt = taskAuditNextChapterPrompt(submittedCount, missing[0], true)
		}
	}
	if report == "" {
		_, missing, _ := submission.progress()
		m.failTaskAudit(id, fmt.Errorf("task audit: chapter protocol exceeded continuation limit; missing sections: %s", taskAuditMissingSectionNames(missing)))
		return
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
	trace.system("completed", "最终审计报告已提交并通过结构校验。", false)
	m.store.notify()
}

func taskAuditNextChapterPrompt(submitted int, section taskAuditSectionDefinition, continuation bool) string {
	prefix := "开始章节化审计。"
	if continuation {
		prefix = "上一轮在完成最终协议前停止；保留并利用已经读取的证据与已提交章节继续。"
	}
	return fmt.Sprintf("%s 当前已提交 %d/%d 章。现在只聚焦下一章 %s（%s）：%s 使用证据 list/search/read 工具完成该章所需调查，然后必须调用 task_audit_submit_section 提交这一章；不要等到调查完全部章节再提交，也不要用普通 Assistant 文本代替工具。章节工具成功后本轮会结束，系统随后提示下一章。", prefix, submitted, len(taskAuditSections), section.ID, section.Title, section.Requirement)
}

func taskAuditMissingSectionNames(sections []taskAuditSectionDefinition) string {
	if len(sections) == 0 {
		return "none (final report tool was not completed)"
	}
	items := make([]string, 0, len(sections))
	for _, section := range sections {
		items = append(items, fmt.Sprintf("%s (%s)", section.ID, section.Title))
	}
	return strings.Join(items, ", ")
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
	required := make([]string, 0, len(taskAuditSections))
	for _, section := range taskAuditSections {
		required = append(required, section.Title)
	}
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

type taskAuditTrace struct {
	store            *Store
	auditID          string
	sequence         int64
	messageID        string
	messageText      string
	messageFlushed   int
	messageLastFlush time.Time
	toolByCallID     map[string]string
	toolByIndex      map[int]string
	toolArguments    map[string]string
	toolFlushed      map[string]int
	toolLastFlush    map[string]time.Time
}

func newTaskAuditTrace(store *Store, auditID string) *taskAuditTrace {
	trace := &taskAuditTrace{store: store, auditID: auditID, toolByCallID: map[string]string{}, toolByIndex: map[int]string{}, toolArguments: map[string]string{}, toolFlushed: map[string]int{}, toolLastFlush: map[string]time.Time{}}
	if store != nil {
		_ = store.db.Model(&TaskAuditEvent{}).Where("audit_id = ?", auditID).Select("coalesce(max(sequence), 0)").Scan(&trace.sequence).Error
	}
	return trace
}

func (t *taskAuditTrace) handle(event agentcore.Event) {
	if t == nil || t.store == nil {
		return
	}
	switch event.Type {
	case agentcore.EventMessageStart:
		if event.Message != nil && event.Message.Role == agentcore.RoleAssistant {
			t.messageID = t.create("assistant", "streaming", event.Turn, "", "")
			t.messageText = ""
			t.messageFlushed = 0
			t.messageLastFlush = time.Now()
			t.toolByIndex = map[int]string{}
		}
	case agentcore.EventMessageUpdate:
		if event.Delta == nil {
			return
		}
		if event.Delta.TextDelta != "" && t.messageID != "" {
			t.messageText += event.Delta.TextDelta
			if len(t.messageText)-t.messageFlushed >= 256 || time.Since(t.messageLastFlush) >= 150*time.Millisecond {
				t.update(t.messageID, map[string]any{"content": truncate(t.messageText, 200000), "updated_at": time.Now()})
				t.messageFlushed = len(t.messageText)
				t.messageLastFlush = time.Now()
			}
		}
		for _, delta := range event.Delta.ToolCallDeltas {
			id := t.toolByIndex[delta.Index]
			if id == "" {
				id = t.create("tool", "generating", event.Turn, delta.ID, delta.Name)
				t.toolByIndex[delta.Index] = id
			}
			if delta.ID != "" {
				t.toolByCallID[delta.ID] = id
				t.update(id, map[string]any{"tool_call_id": delta.ID, "updated_at": time.Now()})
			}
			if delta.Name != "" {
				t.update(id, map[string]any{"tool_name": delta.Name, "updated_at": time.Now()})
			}
			if delta.ArgumentsDelta != "" {
				t.toolArguments[id] += delta.ArgumentsDelta
				if len(t.toolArguments[id])-t.toolFlushed[id] >= 256 || time.Since(t.toolLastFlush[id]) >= 150*time.Millisecond {
					t.update(id, map[string]any{"arguments": truncate(t.toolArguments[id], 200000), "updated_at": time.Now()})
					t.toolFlushed[id] = len(t.toolArguments[id])
					t.toolLastFlush[id] = time.Now()
				}
			}
		}
	case agentcore.EventMessageEnd:
		if t.messageID != "" {
			content := t.messageText
			if event.Message != nil && strings.TrimSpace(event.Message.Text()) != "" {
				content = event.Message.Text()
			}
			if strings.TrimSpace(content) == "" {
				_ = t.store.db.Delete(&TaskAuditEvent{}, "id = ?", t.messageID).Error
				t.store.notify()
			} else {
				t.update(t.messageID, map[string]any{"status": "completed", "content": truncate(content, 200000), "updated_at": time.Now()})
			}
			t.messageID = ""
		}
		for _, id := range t.toolByIndex {
			t.update(id, map[string]any{"arguments": truncate(t.toolArguments[id], 200000), "updated_at": time.Now()})
		}
	case agentcore.EventToolExecutionStart:
		id := t.toolByCallID[event.ToolCallID]
		if id == "" {
			id = t.create("tool", "running", event.Turn, event.ToolCallID, event.ToolName)
			t.toolByCallID[event.ToolCallID] = id
		}
		arguments := string(event.Arguments)
		if arguments == "" {
			arguments = t.toolArguments[id]
		}
		t.update(id, map[string]any{"status": "running", "tool_name": event.ToolName, "tool_call_id": event.ToolCallID, "arguments": truncate(arguments, 200000), "updated_at": time.Now()})
	case agentcore.EventToolExecutionUpdate:
		id := t.toolByCallID[event.ToolCallID]
		if id != "" && event.ToolResult != nil {
			t.update(id, map[string]any{"result": taskAuditToolResultText(event.ToolResult), "updated_at": time.Now()})
		}
	case agentcore.EventToolExecutionEnd:
		id := t.toolByCallID[event.ToolCallID]
		if id == "" {
			id = t.create("tool", "completed", event.Turn, event.ToolCallID, event.ToolName)
		}
		updates := map[string]any{"status": "completed", "is_error": event.IsError, "updated_at": time.Now()}
		if event.ToolResult != nil {
			updates["result"] = taskAuditToolResultText(event.ToolResult)
		}
		if event.Error != "" {
			updates["result"] = truncate(event.Error, 200000)
			updates["is_error"] = true
		}
		t.update(id, updates)
	case agentcore.EventAutoRetryStart:
		t.system("running", fmt.Sprintf("模型请求失败，正在进行第 %d/%d 次自动重试。", event.Attempt, event.MaxAttempts), false)
	case agentcore.EventContextCompactStart:
		t.system("running", "上下文接近容量限制，正在压缩历史证据。", false)
	case agentcore.EventContextCompactEnd:
		t.system("completed", fmt.Sprintf("上下文压缩完成：%d → %d tokens。", event.BeforeTokens, event.AfterTokens), !event.Success)
	}
}

func (t *taskAuditTrace) create(kind, status string, turn int, callID, toolName string) string {
	t.sequence++
	now := time.Now()
	event := TaskAuditEvent{ID: nextID("task-audit-event"), AuditID: t.auditID, Sequence: t.sequence, Kind: kind, Status: status, Turn: turn, ToolCallID: callID, ToolName: toolName, CreatedAt: now, UpdatedAt: now}
	if err := t.store.db.Create(&event).Error; err != nil {
		return ""
	}
	return event.ID
}

func (t *taskAuditTrace) update(id string, values map[string]any) {
	if id == "" {
		return
	}
	_ = t.store.db.Model(&TaskAuditEvent{}).Where("id = ? AND audit_id = ?", id, t.auditID).Updates(values).Error
}

func (t *taskAuditTrace) system(status, content string, isError bool) {
	id := t.create("system", status, 0, "", "")
	t.update(id, map[string]any{"content": truncate(content, 200000), "is_error": isError, "updated_at": time.Now()})
}

func taskAuditToolResultText(result *agentcore.ToolResult) string {
	if result == nil {
		return ""
	}
	text := result.Text()
	if result.Details != nil {
		if encoded, err := json.Marshal(result.Details); err == nil {
			if text != "" {
				text += "\n"
			}
			text += string(encoded)
		}
	}
	return truncate(text, 200000)
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
	newTaskAuditTrace(m.store, id).system("failed", truncate(runErr.Error(), 8000), true)
	m.store.notify()
}

var _ capability.Source = TaskEvidenceSource{}
var _ agenthost.AgentConfigurer = taskAuditConfigurer{}

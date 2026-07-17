package control

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/goccy/go-yaml"
)

var requiredAgentTools = []string{"aegis_create_subissues", "aegis_publish_attachment"}
var defaultAgentTools = ensureRequiredAgentTools([]string{"read", "grep", "find", "ls", "bash", "edit", "write"})

func defaultSkills(now time.Time) []SkillDefinition {
	definitions := []struct {
		id, displayName, description, body string
	}{
		{"decompose-issues", "Issue 拆解", "把软件目标拆成可并行、可验证且依赖明确的 Issues。", `# Issue decomposition

- Inspect the repository before assuming its structure.
- Split work by independently verifiable outcomes rather than arbitrary files.
- Keep dependencies acyclic and reference only earlier issues.
- Include implementation, validation, and security work when relevant.
- Assign each issue to the most suitable enabled Agent.`},
		{"go-service-engineering", "Go 服务工程", "使用 Go、Gin、GORM 构建可维护的后端服务。", `# Go service engineering

- Preserve package boundaries and keep handlers thin.
- Validate inputs at the API boundary and return actionable errors.
- Use GORM transactions when multiple records must change atomically.
- Keep concurrent runtime state race-free and test failure paths.
- Run gofmt, focused tests, the race detector when concurrency changes, and go vet.`},
		{"api-contract-testing", "API 契约测试", "为 HTTP API、持久化和错误语义设计测试。", `# API contract testing

- Test successful requests and invalid input with exact status semantics.
- Assert stable JSON fields and empty collection shapes.
- Exercise persistence by reopening the database where relevant.
- Use real httptest routing instead of testing handlers in isolation.
- Avoid brittle assertions on timestamps and generated identifiers.`},
		{"sqlite-schema-safety", "SQLite 变更安全", "安全演进 SQLite/GORM 数据模型并验证数据一致性。", `# SQLite schema safety

- Use explicit migrations and stable serialized field names.
- Validate constraints and data consistency after schema changes.
- Make seeds idempotent and never overwrite user edits on restart.
- Test reopening the database after the new data is written.
- Keep secrets out of API views and diagnostic output.`},
		{"react-dashboard-engineering", "React 后台工程", "使用 React、Tailwind 和 shadcn/ui 构建清晰的多页面后台。", `# React dashboard engineering

- Keep domain pages separate and use routes instead of stacking all features together.
- Compose installed shadcn/ui primitives before creating custom controls.
- Use FieldGroup and Field for forms, semantic tokens for colors, and responsive layouts.
- Keep API types explicit and represent loading, empty, success, and error states.
- Run TypeScript checks, lint, production builds, and browser smoke tests.`},
		{"accessible-interactions", "可访问交互", "检查表单、对话框、导航和键盘操作的可访问性。", `# Accessible interactions

- Give every form control an accessible label and useful description.
- Include titles and descriptions in every Dialog or Sheet.
- Preserve keyboard submission, focus behavior, and disabled states.
- Use semantic roles and avoid clickable non-interactive elements.
- Verify narrow viewports and inspect browser console warnings.`},
		{"frontend-state-contracts", "前端状态契约", "维护 React 状态、SSE 更新和后端 JSON 契约的一致性。", `# Frontend state contracts

- Update TypeScript domain types before consuming new API fields.
- Keep SSE snapshots and mutation responses on one canonical schema.
- Fail visibly on invalid new data instead of hiding contract errors.
- Avoid duplicated derived state when it can be computed from the snapshot.
- Verify deep routes after a production build.`},
		{"threat-model-workflows", "威胁建模", "识别信任边界、攻击面、滥用路径和缓解措施。", `# Threat modeling

- Identify assets, actors, entry points, and trust boundaries first.
- Trace untrusted input through storage, process execution, network, and UI output.
- Separate exploitable findings from defense-in-depth recommendations.
- Describe impact, preconditions, evidence, and a scoped mitigation.
- Never perform destructive exploitation or access data outside the authorized workspace.`},
		{"appsec-code-review", "应用安全审查", "针对命令执行、路径、认证、注入和敏感信息进行代码审查。", `# Application security review

- Review process execution, path normalization, archive extraction, and URL handling.
- Check authorization at mutation boundaries rather than only in the UI.
- Look for secret disclosure in state responses, logs, errors, and exported files.
- Validate size limits and reject ambiguous or malformed imports.
- Provide reproducible non-destructive evidence for each finding.`},
		{"security-validation", "安全验证", "以非破坏方式验证权限边界与安全修复。", `# Security validation

- Prefer unit or integration tests that demonstrate the boundary without harming data.
- Test denied shell, network, write, and out-of-workspace operations.
- Confirm failed attempts are visible in diagnostics and do not alter task state.
- Re-test the intended allowed workflow after every restriction.
- Stop when validation would require external access or destructive actions.`},
	}
	result := make([]SkillDefinition, 0, len(definitions))
	for _, definition := range definitions {
		result = append(result, SkillDefinition{
			ID: definition.id, Name: definition.id, DisplayName: definition.displayName,
			Description: definition.description,
			Content:     renderSkillContent(definition.id, definition.description, definition.body),
			Source:      "builtin", Version: "1.0.0", Builtin: true, CreatedAt: now, UpdatedAt: now,
		})
	}
	return result
}

func defaultAgents(now time.Time) []AgentDefinition {
	sharedPermissions := PermissionBoundary{
		WorkspaceScope: "run_workspace", AllowNetwork: false, AllowShell: true,
		AllowWrite: true, ApprovalMode: "",
	}
	return []AgentDefinition{
		{
			ID: "aegis-orchestrator", Name: "Aegis Orchestrator", Description: "拆解任务、分配专业 Agent 并协调执行顺序。",
			Avatar: "route", Category: "orchestrator", Enabled: true, Builtin: true,
			SystemPrompt: `You are the Aegis Orchestrator. Inspect the workspace when useful, decompose objectives into verifiable Issues, assign each Issue to the best enabled specialist, keep dependencies explicit, and never claim implementation work yourself. Return exactly the requested planning JSON when planning.`,
			Tools:        ensureRequiredAgentTools([]string{"read", "grep", "find", "ls"}), SkillIDs: []string{"decompose-issues"},
			Permissions: PermissionBoundary{WorkspaceScope: "run_workspace", AllowNetwork: false, AllowShell: false, AllowWrite: false},
			CreatedAt:   now, UpdatedAt: now,
		},
		{
			ID: "knowledge-retriever", Name: builtinKnowledgeRetrievalAgent.Name, Description: "对知识库 Markdown 候选片段进行只读排序、摘要并返回可溯源检索结果。",
			Avatar: "database", Category: "knowledge", Enabled: true, Builtin: true, Internal: true,
			SystemPrompt: builtinKnowledgeRetrievalAgent.SystemPrompt,
			Tools:        []string{}, SkillIDs: []string{}, KnowledgeBaseIDs: []string{},
			Permissions: builtinKnowledgeRetrievalAgent.Permissions,
			CreatedAt:   now, UpdatedAt: now,
		},
		{
			ID: "backend-engineer", Name: "后端工程师", Description: "负责 Go 服务、Gin API、GORM/SQLite、并发运行时与后端测试。",
			Avatar: "server", Category: "backend", Enabled: true, Builtin: true,
			SystemPrompt: `You are Aegis's senior backend engineer. Own server-side implementation end to end: understand current contracts, make coherent changes, choose the cleanest architecture, handle concurrency explicitly, and prove behavior with focused and integration tests. Communicate concrete evidence and remaining risk; never report work you did not perform.`,
			Tools:        append([]string{}, defaultAgentTools...), SkillIDs: []string{"go-service-engineering", "api-contract-testing", "sqlite-schema-safety"},
			Permissions: sharedPermissions, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "frontend-engineer", Name: "前端工程师", Description: "负责 React、Tailwind、shadcn/ui、交互状态与浏览器验收。",
			Avatar: "panels", Category: "frontend", Enabled: true, Builtin: true,
			SystemPrompt: `You are Aegis's senior frontend engineer. Build restrained, polished, accessible product interfaces with React, Tailwind, and the project's shadcn/ui primitives. Keep pages and domain state clear, match backend contracts exactly, support desktop and mobile, and validate with type checks, lint, builds, and browser evidence.`,
			Tools:        append([]string{}, defaultAgentTools...), SkillIDs: []string{"react-dashboard-engineering", "accessible-interactions", "frontend-state-contracts"},
			Permissions: sharedPermissions, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "red-team-lead", Name: "红队负责人", Description: "评估安全任务复杂度与工作量，规划测试范围，拆分 Issues 并协调红队执行。",
			Avatar: "route", Category: "security", Enabled: true, Builtin: true,
			SystemPrompt: `You are Aegis's red-team lead. Your primary responsibility is to make the whole security task succeed as completely as possible. Begin by investigating enough of the authorized target and context to understand the real scope. Assess complexity, likely workload, meaningful testing areas, dependencies, opportunities for parallel work, and the risk of incomplete coverage. Then choose the best execution strategy.

For a simple, bounded task that you can complete thoroughly and reliably in one execution, perform the work yourself and report concrete evidence. Do not create unnecessary coordination overhead.

For a broad task, especially when it contains more than three meaningful testing areas, would benefit from parallel investigation, or is unlikely to be completed thoroughly in one execution, plan the work and call aegis_create_subissues. Create independently verifiable child Issues with clear scope, useful context from your initial investigation, acceptance criteria, dependencies, and appropriate Agent assignments. Assign security testing children to red-team-engineer unless another enabled specialist is clearly more suitable. After the tool succeeds, stop working on the parent; the scheduler will execute the children and later resume the parent for consolidation.

On continuation, review all child results, identify duplication and coverage gaps, perform any bounded integration or validation still needed, and create another small wave of child Issues only when material work remains. Always respect the task's explicit authorization, target boundaries, testing mode, and safety constraints. Do not claim coverage or findings that were not actually verified. Your output format is flexible; prioritize sound judgment, complete coverage, actionable delegation, and an evidence-based final result.`,
			Tools: append([]string{}, defaultAgentTools...), SkillIDs: []string{"decompose-issues", "threat-model-workflows", "security-validation"},
			Permissions: PermissionBoundary{WorkspaceScope: "run_workspace", AllowNetwork: true, AllowShell: true, AllowWrite: false, ApprovalMode: "all"},
			CreatedAt:   now, UpdatedAt: now,
		},
		{
			ID: "red-team-engineer", Name: "红队攻防工程师", Description: "负责威胁建模、应用安全审查与非破坏性安全验证。",
			Avatar: "shield", Category: "security", Enabled: true, Builtin: true,
			SystemPrompt: `You are Aegis's red-team application security engineer. Review authorized code and runtime boundaries adversarially, produce reproducible non-destructive evidence, distinguish confirmed vulnerabilities from hypotheses, and recommend scoped fixes. Never access resources outside the task workspace, exfiltrate data, or perform destructive actions.`,
			Tools:        append([]string{}, defaultAgentTools...), SkillIDs: []string{"threat-model-workflows", "appsec-code-review", "security-validation"},
			Permissions: PermissionBoundary{WorkspaceScope: "run_workspace", AllowNetwork: true, AllowShell: true, AllowWrite: false, ApprovalMode: "all"},
			CreatedAt:   now, UpdatedAt: now,
		},
	}
}

func (s *Store) loadRegistry() error {
	var skillRecords []skillRecord
	if err := s.db.Order("created_at ASC").Find(&skillRecords).Error; err != nil {
		return fmt.Errorf("load skills: %w", err)
	}
	for _, record := range skillRecords {
		s.skills = append(s.skills, cloneSkill(record.Definition))
	}
	var agentRecords []agentRecord
	if err := s.db.Order("created_at ASC").Find(&agentRecords).Error; err != nil {
		return fmt.Errorf("load agents: %w", err)
	}
	for _, record := range agentRecords {
		s.agents = append(s.agents, cloneAgent(record.Definition))
	}
	if err := s.seedRegistry(); err != nil {
		return err
	}
	for _, skill := range s.skills {
		if err := s.materializeSkill(skill); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) seedRegistry() error {
	now := time.Now()
	for _, skill := range defaultSkills(now) {
		if _, exists := skillIndex(s.skills, skill.ID); exists {
			continue
		}
		if err := s.db.Create(&skillRecord{ID: skill.ID, Definition: skill, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return fmt.Errorf("seed skill %s: %w", skill.ID, err)
		}
		s.skills = append(s.skills, skill)
	}
	for _, agent := range defaultAgents(now) {
		if _, exists := agentIndex(s.agents, agent.ID); exists {
			continue
		}
		if err := s.db.Create(&agentRecord{ID: agent.ID, Definition: agent, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
			return fmt.Errorf("seed agent %s: %w", agent.ID, err)
		}
		s.agents = append(s.agents, agent)
	}
	// Control-plane tools are available to every Agent. Persist them explicitly
	// so saved definitions and the runtime tool list stay in sync.
	for index := range s.agents {
		agent := &s.agents[index]
		if agent.Internal {
			continue
		}
		missingRequiredTool := false
		for _, required := range requiredAgentTools {
			if !slices.Contains(agent.Tools, required) {
				missingRequiredTool = true
				break
			}
		}
		if !missingRequiredTool {
			continue
		}
		agent.Tools = ensureRequiredAgentTools(agent.Tools)
		agent.UpdatedAt = now
		var record agentRecord
		if err := s.db.First(&record, "id = ?", agent.ID).Error; err != nil {
			return fmt.Errorf("load agent %s for tool migration: %w", agent.ID, err)
		}
		record.Definition = *agent
		record.UpdatedAt = now
		if err := s.db.Save(&record).Error; err != nil {
			return fmt.Errorf("migrate agent %s tools: %w", agent.ID, err)
		}
	}
	return nil
}

func (s *Store) Agents() []AgentDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneAgents(s.agents)
}

func (s *Store) Skills() []SkillDefinition {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSkills(s.skills)
}

func (s *Store) GetAgent(id string) (AgentDefinition, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if index, ok := agentIndex(s.agents, id); ok {
		return cloneAgent(s.agents[index]), nil
	}
	return AgentDefinition{}, errors.New("agent not found")
}

func (s *Store) CreateAgent(input SaveAgentInput) (AgentDefinition, error) {
	return s.saveAgent("", input)
}

func (s *Store) UpdateAgent(id string, input SaveAgentInput) (AgentDefinition, error) {
	return s.saveAgent(id, input)
}

func (s *Store) saveAgent(id string, input SaveAgentInput) (AgentDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return AgentDefinition{}, errors.New("Agent 名称不能为空")
	}
	creating := id == ""
	if creating {
		id = normalizeRegistryID(fallback(strings.TrimSpace(input.ID), name))
		if id == "" {
			id = "agent-" + strconv.FormatInt(time.Now().UnixMilli(), 10)
		}
		if _, exists := agentIndex(s.agents, id); exists {
			return AgentDefinition{}, errors.New("Agent ID 已存在")
		}
	}
	index, exists := agentIndex(s.agents, id)
	if !creating && !exists {
		return AgentDefinition{}, errors.New("agent not found")
	}
	internal := exists && s.agents[index].Internal
	if !internal {
		input.Tools = ensureRequiredAgentTools(input.Tools)
	}
	knowledgeBaseIDs, err := s.knowledgeBaseIDSet()
	if err != nil {
		return AgentDefinition{}, err
	}
	if err := validateAgentInput(input, s.skills, knowledgeBaseIDs, internal); err != nil {
		return AgentDefinition{}, err
	}
	now := time.Now()
	createdAt := now
	builtin := false
	if exists {
		createdAt = s.agents[index].CreatedAt
		builtin = s.agents[index].Builtin
	}
	agent := AgentDefinition{
		ID: id, Name: name, Description: strings.TrimSpace(input.Description), Avatar: strings.TrimSpace(input.Avatar),
		Category: fallback(strings.TrimSpace(input.Category), "general"), Enabled: input.Enabled, Builtin: builtin, Internal: internal,
		Model: input.Model, SystemPrompt: strings.TrimSpace(input.SystemPrompt),
		Tools: ensureRequiredAgentTools(input.Tools), SkillIDs: uniqueStrings(input.SkillIDs), KnowledgeBaseIDs: uniqueStrings(input.KnowledgeBaseIDs), Permissions: input.Permissions,
		CreatedAt: createdAt, UpdatedAt: now,
	}
	if internal {
		agent.Category = "knowledge"
		agent.Enabled = true
		agent.Tools = []string{}
		agent.SkillIDs = []string{}
		agent.KnowledgeBaseIDs = []string{}
		agent.Permissions = builtinKnowledgeRetrievalAgent.Permissions
	}
	if agent.Avatar == "" {
		agent.Avatar = "bot"
	}
	if agent.Permissions.WorkspaceScope == "" {
		agent.Permissions.WorkspaceScope = "run_workspace"
	}
	record := agentRecord{ID: id, Definition: agent, CreatedAt: createdAt, UpdatedAt: now}
	if err := s.db.Save(&record).Error; err != nil {
		return AgentDefinition{}, err
	}
	if exists {
		s.agents[index] = agent
	} else {
		s.agents = append(s.agents, agent)
	}
	s.updatedAt = now
	s.broadcastLocked()
	return cloneAgent(agent), nil
}

func (s *Store) DeleteAgent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, exists := agentIndex(s.agents, id)
	if !exists {
		return errors.New("agent not found")
	}
	if s.agents[index].Builtin {
		return errors.New("内置 Agent 不能删除，可以停用或修改")
	}
	var references int64
	if err := s.db.Model(&Execution{}).Where("agent_id = ?", id).Count(&references).Error; err != nil {
		return err
	}
	if references > 0 {
		return errors.New("Agent 已被 Session 引用，不能删除")
	}
	if err := s.db.Delete(&agentRecord{}, "id = ?", id).Error; err != nil {
		return err
	}
	s.agents = append(s.agents[:index], s.agents[index+1:]...)
	s.updatedAt = time.Now()
	s.broadcastLocked()
	return nil
}

func (s *Store) CreateSkill(input SaveSkillInput) (SkillDefinition, error) {
	return s.saveSkill("", input, false)
}

func (s *Store) UpdateSkill(id string, input SaveSkillInput) (SkillDefinition, error) {
	return s.saveSkill(id, input, false)
}

func (s *Store) saveSkill(id string, input SaveSkillInput, imported bool) (SkillDefinition, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSkillLocked(id, input, imported)
}

func (s *Store) saveSkillLocked(id string, input SaveSkillInput, imported bool) (SkillDefinition, error) {
	content := strings.TrimSpace(input.Content)
	name := normalizeRegistryID(input.Name)
	description := strings.TrimSpace(input.Description)
	if content != "" {
		parsedName, parsedDescription, err := parseSkillContent(content)
		if err != nil {
			return SkillDefinition{}, err
		}
		if name == "" {
			name = parsedName
		}
		if description == "" {
			description = parsedDescription
		}
	}
	if name == "" {
		name = normalizeRegistryID(input.DisplayName)
	}
	if name == "" || description == "" {
		return SkillDefinition{}, errors.New("Skill name 和 description 不能为空")
	}
	if content == "" {
		content = renderSkillContent(name, description, "# Workflow\n\n- Add concise, actionable instructions for this capability.")
	}
	if len(content) > 512*1024 {
		return SkillDefinition{}, errors.New("SKILL.md 不能超过 512 KiB")
	}
	creating := id == ""
	if creating {
		id = normalizeRegistryID(fallback(strings.TrimSpace(input.ID), name))
		if _, exists := skillIndex(s.skills, id); exists {
			if imported {
				return SkillDefinition{}, errors.New("同名 Skill 已安装")
			}
			return SkillDefinition{}, errors.New("Skill ID 已存在")
		}
	}
	index, exists := skillIndex(s.skills, id)
	if !creating && !exists {
		return SkillDefinition{}, errors.New("skill not found")
	}
	now := time.Now()
	createdAt := now
	builtin := false
	if exists {
		createdAt = s.skills[index].CreatedAt
		builtin = s.skills[index].Builtin
	}
	skill := SkillDefinition{
		ID: id, Name: name, DisplayName: fallback(strings.TrimSpace(input.DisplayName), name),
		Description: description, Content: content, Source: fallback(strings.TrimSpace(input.Source), "local"),
		Version: fallback(strings.TrimSpace(input.Version), "1.0.0"), Builtin: builtin,
		CreatedAt: createdAt, UpdatedAt: now,
	}
	if err := s.materializeSkill(skill); err != nil {
		return SkillDefinition{}, err
	}
	if err := s.db.Save(&skillRecord{ID: id, Definition: skill, CreatedAt: createdAt, UpdatedAt: now}).Error; err != nil {
		return SkillDefinition{}, err
	}
	if exists {
		s.skills[index] = skill
	} else {
		s.skills = append(s.skills, skill)
	}
	s.updatedAt = now
	s.broadcastLocked()
	return cloneSkill(skill), nil
}

func (s *Store) DeleteSkill(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	index, exists := skillIndex(s.skills, id)
	if !exists {
		return errors.New("skill not found")
	}
	if s.skills[index].Builtin {
		return errors.New("内置 Skill 不能删除，可以修改内容")
	}
	for _, agent := range s.agents {
		if slices.Contains(agent.SkillIDs, id) {
			return fmt.Errorf("Skill 正被 Agent %s 使用", agent.Name)
		}
	}
	if err := s.db.Delete(&skillRecord{}, "id = ?", id).Error; err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(s.dataDir, "skills", id)); err != nil {
		return err
	}
	s.skills = append(s.skills[:index], s.skills[index+1:]...)
	s.updatedAt = time.Now()
	s.broadcastLocked()
	return nil
}

func (s *Store) ImportSkill(filename string, data []byte) (SkillDefinition, error) {
	if len(data) == 0 || len(data) > 2*1024*1024 {
		return SkillDefinition{}, errors.New("导入文件必须小于 2 MiB")
	}
	content, err := importedSkillContent(filename, data)
	if err != nil {
		return SkillDefinition{}, err
	}
	return s.saveSkill("", SaveSkillInput{Content: content, Source: "import:" + filepath.Base(filename)}, true)
}

func (s *Store) InstallSkill(source string) (SkillDefinition, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return SkillDefinition{}, errors.New("安装来源不能为空")
	}
	absolute, err := filepath.Abs(source)
	if err != nil {
		return SkillDefinition{}, err
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return SkillDefinition{}, fmt.Errorf("无法读取安装来源: %w", err)
	}
	path := absolute
	if info.IsDir() {
		path = filepath.Join(absolute, "SKILL.md")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return SkillDefinition{}, fmt.Errorf("读取 Skill 失败: %w", err)
	}
	content, err := importedSkillContent(path, data)
	if err != nil {
		return SkillDefinition{}, err
	}
	return s.saveSkill("", SaveSkillInput{Content: content, Source: "local:" + absolute}, true)
}

func (s *Store) ExportSkill(id string) ([]byte, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	index, exists := skillIndex(s.skills, id)
	if !exists {
		return nil, "", errors.New("skill not found")
	}
	skill := s.skills[index]
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	entry, err := archive.Create(skill.Name + "/SKILL.md")
	if err != nil {
		return nil, "", err
	}
	if _, err := entry.Write([]byte(skill.Content)); err != nil {
		return nil, "", err
	}
	metadata, _ := json.MarshalIndent(map[string]any{
		"id": skill.ID, "displayName": skill.DisplayName, "version": skill.Version, "source": skill.Source,
	}, "", "  ")
	metaEntry, err := archive.Create(skill.Name + "/aegis.json")
	if err != nil {
		return nil, "", err
	}
	if _, err := metaEntry.Write(metadata); err != nil {
		return nil, "", err
	}
	if err := archive.Close(); err != nil {
		return nil, "", err
	}
	return buffer.Bytes(), skill.Name + ".zip", nil
}

func (s *Store) skillPaths(ids []string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	paths := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, exists := skillIndex(s.skills, id); exists {
			paths = append(paths, filepath.Join(s.dataDir, "skills", id))
		}
	}
	return paths
}

func (s *Store) materializeSkill(skill SkillDefinition) error {
	directory := filepath.Join(s.dataDir, "skills", skill.ID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create skill directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(directory, "SKILL.md"), []byte(skill.Content+"\n"), 0o600); err != nil {
		return fmt.Errorf("write SKILL.md: %w", err)
	}
	return nil
}

func (s *Store) effectiveAgentConfig(agent AgentDefinition) Config {
	config := s.Config()
	if strings.TrimSpace(agent.Model.Provider) != "" {
		config.Provider = strings.TrimSpace(agent.Model.Provider)
	}
	if strings.TrimSpace(agent.Model.Model) != "" {
		config.Model = strings.TrimSpace(agent.Model.Model)
	}
	if strings.TrimSpace(agent.Model.BaseURL) != "" {
		config.BaseURL = strings.TrimSpace(agent.Model.BaseURL)
	}
	if strings.TrimSpace(agent.Model.Thinking) != "" {
		config.Thinking = strings.TrimSpace(agent.Model.Thinking)
	}
	if agent.Model.Pricing != nil {
		config.Pricing = *agent.Model.Pricing
	}
	return config
}

func (s *Store) executionAgent(id string) (AgentDefinition, error) {
	agent, err := s.GetAgent(id)
	if err != nil {
		return AgentDefinition{}, err
	}
	if !agent.Enabled {
		return AgentDefinition{}, errors.New("Agent 已停用")
	}
	if agent.Internal {
		return AgentDefinition{}, errors.New("系统内部 Agent 不能执行 Issue")
	}
	return agent, nil
}

func (s *Store) chooseAgent(issue Issue) (AgentDefinition, error) {
	agents := s.Agents()
	wanted := issue.AssigneeAgentID
	if wanted == "" {
		text := strings.ToLower(strings.Join([]string{issue.Title, issue.Description, issue.AcceptanceCriteria}, " "))
		switch {
		case containsAny(text, "security", "secure", "threat", "attack", "vulnerability", "auth", "安全", "攻防", "漏洞", "权限", "注入"):
			if issue.ParentID == "" {
				wanted = "red-team-lead"
			} else {
				wanted = "red-team-engineer"
			}
		case containsAny(text, "frontend", "react", "tailwind", "css", "browser", "ui", "ux", "前端", "页面", "交互", "界面"):
			wanted = "frontend-engineer"
		default:
			wanted = "backend-engineer"
		}
	}
	for _, agent := range agents {
		if agent.ID == wanted && agent.Enabled && !agent.Internal {
			return agent, nil
		}
	}
	if issue.AssigneeAgentID != "" {
		return AgentDefinition{}, fmt.Errorf("Issue 指定的 Agent 不存在或已停用: %s", issue.AssigneeAgentID)
	}
	for _, agent := range agents {
		if agent.Category != "orchestrator" && agent.Enabled && !agent.Internal {
			return agent, nil
		}
	}
	return AgentDefinition{}, errors.New("没有可用于执行 Issue 的已启用 Agent")
}

func validateAgentInput(input SaveAgentInput, skills []SkillDefinition, knowledgeBaseIDs map[string]struct{}, internal bool) error {
	if strings.TrimSpace(input.SystemPrompt) == "" {
		return errors.New("系统提示词不能为空")
	}
	if len(input.SystemPrompt) > 100000 {
		return errors.New("系统提示词过长")
	}
	if len(input.Tools) == 0 && !internal {
		return errors.New("至少选择一个工具")
	}
	for _, id := range uniqueStrings(input.SkillIDs) {
		if _, exists := skillIndex(skills, id); !exists {
			return fmt.Errorf("Skill 不存在: %s", id)
		}
	}
	for _, id := range uniqueStrings(input.KnowledgeBaseIDs) {
		if _, exists := knowledgeBaseIDs[id]; !exists {
			return fmt.Errorf("知识库不存在: %s", id)
		}
	}
	if input.Permissions.WorkspaceScope != "" && input.Permissions.WorkspaceScope != "run_workspace" {
		return errors.New("当前仅支持 run_workspace 权限范围")
	}
	if !slices.Contains([]string{"", "all", "risky", "none"}, input.Permissions.ApprovalMode) {
		return errors.New("无效的审批策略")
	}
	if input.Model.Pricing != nil {
		if err := validateModelPricing(*input.Model.Pricing); err != nil {
			return err
		}
	}
	return nil
}

func importedSkillContent(filename string, data []byte) (string, error) {
	if strings.EqualFold(filepath.Ext(filename), ".zip") || bytes.HasPrefix(data, []byte("PK\x03\x04")) {
		reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return "", errors.New("无效的 Skill ZIP")
		}
		for _, file := range reader.File {
			if !strings.EqualFold(filepath.Base(file.Name), "SKILL.md") || file.FileInfo().IsDir() {
				continue
			}
			if file.UncompressedSize64 > 512*1024 {
				return "", errors.New("ZIP 中的 SKILL.md 过大")
			}
			stream, err := file.Open()
			if err != nil {
				return "", err
			}
			content, readErr := io.ReadAll(io.LimitReader(stream, 512*1024+1))
			_ = stream.Close()
			if readErr != nil {
				return "", readErr
			}
			return strings.TrimSpace(string(content)), nil
		}
		return "", errors.New("ZIP 中未找到 SKILL.md")
	}
	return strings.TrimSpace(string(data)), nil
}

func parseSkillContent(content string) (string, string, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---\n") {
		return "", "", errors.New("SKILL.md 缺少 YAML frontmatter")
	}
	rest := strings.TrimPrefix(trimmed, "---\n")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", "", errors.New("SKILL.md frontmatter 未闭合")
	}
	var metadata struct {
		Name        string `yaml:"name"`
		Description string `yaml:"description"`
	}
	if err := yaml.Unmarshal([]byte(rest[:end]), &metadata); err != nil {
		return "", "", fmt.Errorf("解析 SKILL.md frontmatter: %w", err)
	}
	name := normalizeRegistryID(metadata.Name)
	if name == "" || strings.TrimSpace(metadata.Description) == "" {
		return "", "", errors.New("SKILL.md frontmatter 必须包含 name 和 description")
	}
	return name, strings.TrimSpace(metadata.Description), nil
}

func renderSkillContent(name, description, body string) string {
	return fmt.Sprintf("---\nname: %s\ndescription: %s\n---\n\n%s", normalizeRegistryID(name), strconv.Quote(strings.TrimSpace(description)), strings.TrimSpace(body))
}

func normalizeRegistryID(value string) string {
	var result strings.Builder
	lastDash := false
	for _, character := range strings.ToLower(strings.TrimSpace(value)) {
		if (unicode.IsLetter(character) || unicode.IsDigit(character)) && character <= unicode.MaxASCII {
			result.WriteRune(character)
			lastDash = false
			continue
		}
		if !lastDash && result.Len() > 0 {
			result.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(result.String(), "-")
}

func uniqueStrings(values []string) []string {
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func ensureRequiredAgentTools(tools []string) []string {
	values := append([]string{}, tools...)
	values = append(values, requiredAgentTools...)
	return uniqueStrings(values)
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func agentIndex(items []AgentDefinition, id string) (int, bool) {
	for index := range items {
		if items[index].ID == id {
			return index, true
		}
	}
	return -1, false
}

func skillIndex(items []SkillDefinition, id string) (int, bool) {
	for index := range items {
		if items[index].ID == id {
			return index, true
		}
	}
	return -1, false
}

func cloneAgent(source AgentDefinition) AgentDefinition {
	result := source
	if source.Model.Pricing != nil {
		pricing := *source.Model.Pricing
		result.Model.Pricing = &pricing
	}
	result.Tools = append([]string{}, source.Tools...)
	result.SkillIDs = append([]string{}, source.SkillIDs...)
	result.KnowledgeBaseIDs = append([]string{}, source.KnowledgeBaseIDs...)
	return result
}

func cloneAgents(source []AgentDefinition) []AgentDefinition {
	result := make([]AgentDefinition, len(source))
	for index := range source {
		result[index] = cloneAgent(source[index])
	}
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].CreatedAt.Before(result[right].CreatedAt)
	})
	return result
}

func cloneSkill(source SkillDefinition) SkillDefinition { return source }

func cloneSkills(source []SkillDefinition) []SkillDefinition {
	result := append([]SkillDefinition{}, source...)
	sort.SliceStable(result, func(left, right int) bool {
		return result[left].DisplayName < result[right].DisplayName
	})
	return result
}

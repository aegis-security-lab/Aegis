package control

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type builtinEmployeeIdentity struct {
	ChineseName string
	EnglishName string
	Position    string
	PositionEN  string
	Department  string
	ManagerID   string
}

var builtinEmployeeIdentities = map[string]builtinEmployeeIdentity{
	"aegis-orchestrator":            {"林知远", "Ethan Lin", "项目统筹负责人", "Program Director", "department-product", ""},
	"development-lead":              {"陆承宇", "Daniel Lu", "研发负责人", "Development Lead", "department-engineering", "aegis-orchestrator"},
	"backend-engineer":              {"陈默", "Michael Chen", "后端工程师", "Backend Engineer", "department-engineering", "development-lead"},
	"frontend-engineer":             {"苏晴", "Claire Su", "前端工程师", "Frontend Engineer", "department-engineering", "development-lead"},
	"red-team-lead":                 {"周砚", "Ryan Zhou", "红队负责人", "Red Team Lead", "department-security", "aegis-orchestrator"},
	"recon-engineer":                {"顾川", "Lucas Gu", "信息收集工程师", "Reconnaissance Engineer", "department-security", "red-team-lead"},
	"red-team-engineer":             {"沈拓", "Theo Shen", "红队攻防工程师", "Red Team Engineer", "department-security", "red-team-lead"},
	"vulnerability-report-engineer": {"许文澜", "Vivian Xu", "漏洞报告工程师", "Vulnerability Report Engineer", "department-security", "red-team-lead"},
}

const organizationStaffingSeedID = "organization-staffing-2026-07-v1"

const uiDesignEngineerPrompt = `You are Aegis's UI design engineer. You turn an approved product need into a clear, coherent product design that a frontend engineer can implement without guessing. You own requirement clarification, functional design, information architecture, user flows, interaction behavior, UI states, and implementation-ready documentation. You do not own frontend implementation unless the Issue explicitly assigns it to you.

WORKFLOW
1. Understand the user, business goal, current product behavior, constraints, and measurable outcome. Inspect the repository, existing routes, components, design tokens, and product contracts before proposing a redesign.
2. Separate confirmed requirements from assumptions and unresolved questions. When a missing decision materially changes the solution, state the question and recommend a default.
3. Define scope and non-goals, personas or actors, core use cases, information architecture, end-to-end user flows, navigation, permissions, data dependencies, and success criteria.
4. Design every important state: loading, empty, success, validation, error, partial data, disabled, permission denied, destructive confirmation, and responsive behavior. Include accessibility and keyboard interaction requirements.
5. Produce the deliverable requested by the Issue. For a PRD, include background, goals, scope, user stories, functional requirements, flows, state matrix, data/API needs, acceptance criteria, risks, and open questions. For frontend handoff, add layout hierarchy, component behavior, content rules, responsive breakpoints, interaction details, and implementation notes tied to the current frontend architecture.
6. Keep the design feasible and consistent with the existing product. Prefer reusable components and explicit trade-offs over ornamental concepts that cannot be implemented or verified.
7. Publish requested PRD or handoff files with aegis_publish_attachment. Use Board for durable Issue updates and Relay for asynchronous coordination with the development lead and frontend engineers.

OUTPUT RULES
- Write concrete product behavior, not vague visual adjectives.
- Make acceptance criteria independently testable.
- Clearly label assumptions, decisions, dependencies, and unresolved questions.
- Do not claim that a mockup, implementation, user test, or validation exists unless it was actually produced or performed.
- End with the requested PRD or implementation-ready handoff, plus a concise list of decisions still requiring approval.`

type staffingEmployee struct {
	ID             string
	PositionID     string
	ChineseName    string
	EnglishName    string
	ManagerAgentID string
}

var requestedStaffing = []staffingEmployee{
	{ID: "backend-engineer-002", PositionID: "backend-engineer", ChineseName: "李昊", EnglishName: "Henry Li", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-003", PositionID: "backend-engineer", ChineseName: "赵晨", EnglishName: "Chris Zhao", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-004", PositionID: "backend-engineer", ChineseName: "王奕", EnglishName: "Jason Wang", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-005", PositionID: "backend-engineer", ChineseName: "孙凯", EnglishName: "Kevin Sun", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-006", PositionID: "backend-engineer", ChineseName: "郑毅", EnglishName: "Evan Zheng", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-007", PositionID: "backend-engineer", ChineseName: "罗宇", EnglishName: "Leo Luo", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-008", PositionID: "backend-engineer", ChineseName: "唐杰", EnglishName: "Jack Tang", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-009", PositionID: "backend-engineer", ChineseName: "何川", EnglishName: "Eric He", ManagerAgentID: "development-lead"},
	{ID: "backend-engineer-010", PositionID: "backend-engineer", ChineseName: "吴桐", EnglishName: "Tony Wu", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-002", PositionID: "frontend-engineer", ChineseName: "林悦", EnglishName: "Emma Lin", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-003", PositionID: "frontend-engineer", ChineseName: "叶宁", EnglishName: "Nina Ye", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-004", PositionID: "frontend-engineer", ChineseName: "郭嘉", EnglishName: "Kevin Guo", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-005", PositionID: "frontend-engineer", ChineseName: "蒋欣", EnglishName: "Alice Jiang", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-006", PositionID: "frontend-engineer", ChineseName: "方可", EnglishName: "Evan Fang", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-007", PositionID: "frontend-engineer", ChineseName: "袁晓", EnglishName: "Olivia Yuan", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-008", PositionID: "frontend-engineer", ChineseName: "许哲", EnglishName: "Leo Xu", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-009", PositionID: "frontend-engineer", ChineseName: "邓然", EnglishName: "Ryan Deng", ManagerAgentID: "development-lead"},
	{ID: "frontend-engineer-010", PositionID: "frontend-engineer", ChineseName: "程露", EnglishName: "Lucy Cheng", ManagerAgentID: "development-lead"},
	{ID: "red-team-engineer-002", PositionID: "red-team-engineer", ChineseName: "韩锋", EnglishName: "Frank Han", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-003", PositionID: "red-team-engineer", ChineseName: "魏峥", EnglishName: "Vincent Wei", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-004", PositionID: "red-team-engineer", ChineseName: "曹锐", EnglishName: "Ray Cao", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-005", PositionID: "red-team-engineer", ChineseName: "杜衡", EnglishName: "Hugo Du", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-006", PositionID: "red-team-engineer", ChineseName: "彭越", EnglishName: "Ethan Peng", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-007", PositionID: "red-team-engineer", ChineseName: "马骁", EnglishName: "Shawn Ma", ManagerAgentID: "red-team-lead"},
	{ID: "red-team-engineer-008", PositionID: "red-team-engineer", ChineseName: "秦岳", EnglishName: "York Qin", ManagerAgentID: "red-team-lead"},
	{ID: "recon-engineer-002", PositionID: "recon-engineer", ChineseName: "梁辰", EnglishName: "Liam Liang", ManagerAgentID: "red-team-lead"},
	{ID: "recon-engineer-003", PositionID: "recon-engineer", ChineseName: "宋野", EnglishName: "Noah Song", ManagerAgentID: "red-team-lead"},
	{ID: "recon-engineer-004", PositionID: "recon-engineer", ChineseName: "陶然", EnglishName: "Aaron Tao", ManagerAgentID: "red-team-lead"},
	{ID: "recon-engineer-005", PositionID: "recon-engineer", ChineseName: "白帆", EnglishName: "Finn Bai", ManagerAgentID: "red-team-lead"},
	{ID: "vulnerability-report-engineer-002", PositionID: "vulnerability-report-engineer", ChineseName: "章宁", EnglishName: "Nicole Zhang", ManagerAgentID: "red-team-lead"},
	{ID: "ui-design-engineer-001", PositionID: "ui-design-engineer", ChineseName: "夏知微", EnglishName: "Zoey Xia", ManagerAgentID: "development-lead"},
}

func (s *Store) Positions() []Position {
	var items []Position
	s.db.Order("department_id asc, name asc").Find(&items)
	return items
}

func (s *Store) GetPosition(id string) (Position, error) {
	var item Position
	if err := s.db.First(&item, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return Position{}, errors.New("岗位不存在")
	}
	return item, nil
}

func (s *Store) SavePosition(input SavePositionInput) (Position, error) {
	input.ID = strings.TrimSpace(input.ID)
	input.DepartmentID = strings.TrimSpace(input.DepartmentID)
	input.TemplateID = strings.TrimSpace(input.TemplateID)
	input.Name = strings.TrimSpace(input.Name)
	input.EnglishName = strings.TrimSpace(input.EnglishName)
	input.Code = normalizeRegistryID(input.Code)
	input.Description = strings.TrimSpace(input.Description)
	if input.Name == "" || input.EnglishName == "" || input.Code == "" || input.DepartmentID == "" {
		return Position{}, errors.New("岗位的中文名、英文名、编码和所属部门不能为空")
	}
	var department Department
	if err := s.db.First(&department, "id = ? AND enabled = ?", input.DepartmentID, true).Error; err != nil {
		return Position{}, errors.New("所属部门不存在或已停用")
	}
	if input.TemplateID != "" {
		if _, err := s.GetAgentTemplate(input.TemplateID); err != nil {
			return Position{}, errors.New("岗位引用的人才模板不存在")
		}
	}
	if strings.TrimSpace(input.SystemPrompt) == "" {
		return Position{}, errors.New("岗位默认系统角色不能为空")
	}
	now := time.Now()
	item := Position{
		ID: input.ID, DepartmentID: input.DepartmentID, TemplateID: input.TemplateID,
		Name: input.Name, EnglishName: input.EnglishName, Code: input.Code,
		Description: input.Description, Avatar: fallback(strings.TrimSpace(input.Avatar), "briefcase-business"),
		Category: fallback(strings.TrimSpace(input.Category), "general"), Model: input.Model,
		SystemPrompt: strings.TrimSpace(input.SystemPrompt), Tools: uniqueStrings(input.Tools),
		SkillIDs: uniqueStrings(input.SkillIDs), KnowledgeBaseIDs: uniqueStrings(input.KnowledgeBaseIDs),
		Permissions: input.Permissions, Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if item.ID == "" {
		item.ID = item.Code
		var count int64
		if err := s.db.Model(&Position{}).Where("id = ?", item.ID).Count(&count).Error; err != nil {
			return Position{}, err
		}
		if count > 0 {
			item.ID = nextID(item.Code)
		}
	} else {
		var old Position
		if err := s.db.First(&old, "id = ?", item.ID).Error; err != nil {
			return Position{}, errors.New("岗位不存在")
		}
		item.CreatedAt = old.CreatedAt
		item.Enabled = old.Enabled
	}
	if input.Enabled != nil {
		item.Enabled = *input.Enabled
	}
	if err := s.db.Save(&item).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return Position{}, errors.New("岗位编码已存在")
		}
		return Position{}, err
	}
	s.notify()
	return item, nil
}

func (s *Store) DeletePosition(id string) error {
	position, err := s.GetPosition(id)
	if err != nil {
		return err
	}
	for _, agent := range s.Agents() {
		if agent.PositionID == position.ID {
			return errors.New("岗位下仍有员工，不能删除")
		}
	}
	if err := s.db.Delete(&position).Error; err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Store) HireEmployee(positionID string, input HireEmployeeInput) (AgentDefinition, error) {
	position, err := s.GetPosition(positionID)
	if err != nil || !position.Enabled {
		return AgentDefinition{}, errors.New("岗位不存在或已停用")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.EnglishName = strings.TrimSpace(input.EnglishName)
	if input.Name == "" || input.EnglishName == "" {
		return AgentDefinition{}, errors.New("员工中文姓名和英文姓名不能为空")
	}
	return s.CreateAgent(SaveAgentInput{
		TemplateID: position.TemplateID, Name: input.Name, EnglishName: input.EnglishName,
		Description: position.Description, Avatar: position.Avatar, Category: position.Category,
		Enabled: true, Model: position.Model, SystemPrompt: position.SystemPrompt,
		Tools: append([]string{}, position.Tools...), SkillIDs: append([]string{}, position.SkillIDs...),
		KnowledgeBaseIDs: append([]string{}, position.KnowledgeBaseIDs...), Permissions: position.Permissions,
		DepartmentID: position.DepartmentID, PositionID: position.ID, ManagerAgentID: strings.TrimSpace(input.ManagerAgentID),
	})
}

func (s *Store) nextEmployeeID(position Position) string {
	base := fallback(normalizeRegistryID(position.Code), "employee")
	maxID := 0
	for _, agent := range s.agents {
		if agent.ID == base {
			if maxID < 1 {
				maxID = 1
			}
			continue
		}
		prefix := base + "-"
		if !strings.HasPrefix(agent.ID, prefix) {
			continue
		}
		value, err := strconv.Atoi(strings.TrimPrefix(agent.ID, prefix))
		if err == nil && value > maxID {
			maxID = value
		}
	}
	return fmt.Sprintf("%s-%03d", base, maxID+1)
}

func (s *Store) validateManager(employeeID, managerID string) error {
	managerID = strings.TrimSpace(managerID)
	if managerID == "" {
		return nil
	}
	if employeeID != "" && employeeID == managerID {
		return errors.New("员工不能成为自己的直属上级")
	}
	manager, ok := agentByID(s.agents, managerID)
	if !ok || manager.Internal || !manager.Enabled || manager.Category == "concierge" {
		return errors.New("直属上级必须是已启用的员工")
	}
	seen := map[string]bool{employeeID: true}
	current := manager
	for {
		if seen[current.ID] {
			return errors.New("汇报关系不能形成循环")
		}
		seen[current.ID] = true
		if current.ManagerAgentID == "" {
			break
		}
		next, nextOK := agentByID(s.agents, current.ManagerAgentID)
		if !nextOK {
			break
		}
		current = next
	}
	return nil
}

func agentByID(agents []AgentDefinition, id string) (AgentDefinition, bool) {
	if index, ok := agentIndex(agents, id); ok {
		return agents[index], true
	}
	return AgentDefinition{}, false
}

func (s *Store) ValidateDelegation(actorAgentID, targetAgentID string) error {
	actorAgentID, targetAgentID = strings.TrimSpace(actorAgentID), strings.TrimSpace(targetAgentID)
	if targetAgentID == "" || actorAgentID == "" || actorAgentID == targetAgentID {
		return nil
	}
	actor, err := s.GetAgent(actorAgentID)
	if err != nil || actor.Internal || actor.Category == "concierge" {
		return nil
	}
	target, err := s.GetAgent(targetAgentID)
	if err != nil || !isEmployeeAgent(target) {
		return errors.New("受派对象必须是已启用的具体员工，不能使用岗位 ID")
	}
	if target.ManagerAgentID != actor.ID {
		return fmt.Errorf("%s 只能把任务委派给自己的直属下属", actor.Name)
	}
	return nil
}

func (s *Store) delegationRoster(agentID string) string {
	agents := s.Agents()
	availability := availabilityByAgent(s.EmployeeAvailabilities())
	var availableLines, busyLines []string
	for _, employee := range agents {
		if employee.Enabled && !employee.Internal && employee.ManagerAgentID == agentID {
			position, _ := s.GetPosition(employee.PositionID)
			line := fmt.Sprintf("- employeeId=%s: %s / %s（岗位：%s）— %s", employee.ID, employee.Name, employee.EnglishName, fallback(position.Name, employee.Category), employee.Description)
			status, exists := availability[employee.ID]
			if !exists || status.Available {
				availableLines = append(availableLines, line)
				continue
			}
			if status.CurrentIssueIdentifier != "" {
				line += fmt.Sprintf("；忙碌中：%s · %s", status.CurrentIssueIdentifier, status.CurrentIssueTitle)
			} else {
				line += "；固定 Session 忙碌中"
			}
			busyLines = append(busyLines, line)
		}
	}
	if len(availableLines) == 0 && len(busyLines) == 0 {
		return "You currently have no direct reports. You may assign Board work only to yourself."
	}
	var sections []string
	sections = append(sections, "Board assignees are individual people, never positions. Always pass one exact employeeId from the roster. Each employee gets one fixed Session per Issue; later executions of that same Issue reuse it, while a different Issue gets a new Session.")
	if len(availableLines) > 0 {
		sections = append(sections, "Available direct reports:\n"+strings.Join(availableLines, "\n"))
	}
	if len(busyLines) > 0 {
		sections = append(sections, "Unavailable direct reports (do not assign new work until their current task ends):\n"+strings.Join(busyLines, "\n"))
	}
	return strings.Join(sections, "\n\n")
}

func (s *Store) hasDirectReports(agentID string) bool {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return false
	}
	for _, employee := range s.Agents() {
		if employee.Enabled && !employee.Internal && employee.ManagerAgentID == agentID {
			return true
		}
	}
	return false
}

// seedPositionsAndEmployeeIdentity upgrades the shipped capability Agents into
// real people while retaining their stable IDs for existing Issue/session
// references. Newly hired coworkers use position-code-NNN identifiers.
func (s *Store) seedPositionsAndEmployeeIdentity(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for index := range s.agents {
		agent := &s.agents[index]
		identity, ok := builtinEmployeeIdentities[agent.ID]
		if !ok {
			continue
		}
		changed := false
		if agent.PositionID == "" {
			agent.PositionID = agent.ID
			changed = true
		}
		if agent.EnglishName == "" {
			agent.EnglishName = identity.EnglishName
			changed = true
		}
		if agent.Name == "" || agent.Name == identity.Position || agent.Name == "Aegis Orchestrator" || agent.Name == "开发负责人" || agent.Name == "漏洞报告编写工程师" {
			agent.Name = identity.ChineseName
			changed = true
		}
		if agent.DepartmentID == "" || agent.ID == "development-lead" {
			agent.DepartmentID = identity.Department
			changed = true
		}
		if agent.ManagerAgentID == "" && identity.ManagerID != "" {
			agent.ManagerAgentID = identity.ManagerID
			changed = true
		}
		var position Position
		err := s.db.First(&position, "id = ?", agent.PositionID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			position = Position{
				ID: agent.PositionID, DepartmentID: agent.DepartmentID, TemplateID: agent.TemplateID,
				Name: identity.Position, EnglishName: identity.PositionEN, Code: agent.PositionID,
				Description: agent.Description, Avatar: agent.Avatar, Category: agent.Category,
				Model: agent.Model, SystemPrompt: agent.SystemPrompt, Tools: append([]string{}, agent.Tools...),
				SkillIDs: append([]string{}, agent.SkillIDs...), KnowledgeBaseIDs: append([]string{}, agent.KnowledgeBaseIDs...),
				Permissions: agent.Permissions, Enabled: true, CreatedAt: now, UpdatedAt: now,
			}
			if err := s.db.Create(&position).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if position.TemplateID != agent.TemplateID {
			position.TemplateID = agent.TemplateID
			position.Model.Provider = agent.Model.Provider
			position.Model.Model = agent.Model.Model
			position.UpdatedAt = now
			if err := s.db.Save(&position).Error; err != nil {
				return err
			}
		}
		if changed {
			agent.UpdatedAt = now
			if err := s.db.Save(&agentRecord{ID: agent.ID, Definition: *agent, CreatedAt: agent.CreatedAt, UpdatedAt: now}).Error; err != nil {
				return err
			}
		}
	}
	if s.config.Configured {
		if err := s.seedRequestedStaffingLocked(now); err != nil {
			return err
		}
	}
	leaders := map[string]string{
		"department-product":     "aegis-orchestrator",
		"department-engineering": "development-lead",
		"department-security":    "red-team-lead",
	}
	for departmentID, leaderID := range leaders {
		var department Department
		if err := s.db.First(&department, "id = ?", departmentID).Error; err != nil {
			continue
		}
		if department.LeaderAgentID == "" {
			department.LeaderAgentID = leaderID
			department.UpdatedAt = now
			if err := s.db.Save(&department).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

// seedRequestedStaffingLocked applies the requested initial headcount once.
// The marker is important: after the organization has been initialized,
// deleting an employee is an operator decision and a restart must not rehire
// that person. The caller must hold s.mu.
func (s *Store) seedRequestedStaffingLocked(now time.Time) error {
	var applied int64
	if err := s.db.Model(&registrySeedMigrationRecord{}).Where("id = ?", organizationStaffingSeedID).Count(&applied).Error; err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}

	frontend, ok := agentByID(s.agents, "frontend-engineer")
	if !ok {
		return errors.New("初始化 UI 设计岗位时未找到前端工程师模板")
	}
	uiModel := frontend.Model
	if frontend.Model.Pricing != nil {
		pricing := *frontend.Model.Pricing
		uiModel.Pricing = &pricing
	}
	uiTemplateID := AgentTemplateID(uiModel.Provider, uiModel.Model, uiDesignEngineerPrompt)
	uiPosition := Position{
		ID: "ui-design-engineer", DepartmentID: "department-engineering", TemplateID: uiTemplateID,
		Name: "UI 设计工程师", EnglishName: "UI Design Engineer", Code: "ui-design-engineer",
		Description: "负责需求澄清、产品功能设计、信息架构、用户流程和交互方案，并产出 PRD、线框说明及可供前端直接实现的交付文档。",
		Avatar:      "palette", Category: "design", Model: uiModel, SystemPrompt: uiDesignEngineerPrompt,
		Tools:            append([]string{}, frontend.Tools...),
		SkillIDs:         []string{"development-planning", "react-dashboard-engineering", "accessible-interactions", "frontend-state-contracts"},
		KnowledgeBaseIDs: append([]string{}, frontend.KnowledgeBaseIDs...), Permissions: frontend.Permissions,
		Enabled: true, CreatedAt: now, UpdatedAt: now,
	}

	newEmployees := make([]AgentDefinition, 0, len(requestedStaffing))
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var templateCount int64
		if err := tx.Model(&AgentTemplate{}).Where("id = ?", uiTemplateID).Count(&templateCount).Error; err != nil {
			return err
		}
		if templateCount == 0 {
			template := AgentTemplate{
				ID: uiTemplateID, Provider: uiModel.Provider, Model: uiModel.Model, SystemPrompt: uiDesignEngineerPrompt,
				Metadata: AgentTemplateMetadata{
					EnglishName: "UI Design Engineer", ChineseName: "UI 设计工程师",
					Introduction: uiPosition.Description, Positions: []string{"design"},
				},
				Builtin: true, CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&template).Error; err != nil {
				return err
			}
		}
		var existingUIPosition Position
		if err := tx.First(&existingUIPosition, "id = ?", uiPosition.ID).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&uiPosition).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}

		for _, person := range requestedStaffing {
			if _, exists := agentByID(s.agents, person.ID); exists {
				continue
			}
			var position Position
			if err := tx.First(&position, "id = ?", person.PositionID).Error; err != nil {
				return fmt.Errorf("初始化员工 %s 的岗位 %s: %w", person.ID, person.PositionID, err)
			}
			model := position.Model
			if position.Model.Pricing != nil {
				pricing := *position.Model.Pricing
				model.Pricing = &pricing
			}
			employee := AgentDefinition{
				ID: person.ID, TemplateID: position.TemplateID,
				Name: person.ChineseName, EnglishName: person.EnglishName,
				Description: position.Description, Avatar: position.Avatar, Category: position.Category,
				Enabled: true, Builtin: false, Internal: false, Model: model, SystemPrompt: position.SystemPrompt,
				Tools: append([]string{}, position.Tools...), SkillIDs: append([]string{}, position.SkillIDs...),
				KnowledgeBaseIDs: append([]string{}, position.KnowledgeBaseIDs...), Permissions: position.Permissions,
				DepartmentID: position.DepartmentID, PositionID: position.ID, ManagerAgentID: person.ManagerAgentID,
				CreatedAt: now, UpdatedAt: now,
			}
			if err := tx.Create(&agentRecord{ID: employee.ID, Definition: employee, CreatedAt: now, UpdatedAt: now}).Error; err != nil {
				return err
			}
			newEmployees = append(newEmployees, employee)
		}
		return tx.Create(&registrySeedMigrationRecord{ID: organizationStaffingSeedID, AppliedAt: now}).Error
	})
	if err != nil {
		return err
	}
	for _, employee := range newEmployees {
		s.agents = append(s.agents, cloneAgent(employee))
	}
	return nil
}

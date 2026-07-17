package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var sequence atomic.Uint64

type Store struct {
	mu          sync.RWMutex
	dataDir     string
	db          *gorm.DB
	config      Config
	agents      []AgentDefinition
	skills      []SkillDefinition
	runtime     RuntimeProbe
	updatedAt   time.Time
	subscribers map[chan StateView]struct{}
}

type configRecord struct {
	ID        uint   `gorm:"primaryKey"`
	Value     Config `gorm:"serializer:json;type:text"`
	UpdatedAt time.Time
}
type agentRecord struct {
	ID         string          `gorm:"primaryKey"`
	Definition AgentDefinition `gorm:"serializer:json;type:text"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}
type skillRecord struct {
	ID         string          `gorm:"primaryKey"`
	Definition SkillDefinition `gorm:"serializer:json;type:text"`
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

func NewStore(dataDir string) (*Store, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("data directory is required")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return nil, err
	}
	dbLog := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{SlowThreshold: time.Second, LogLevel: logger.Error, IgnoreRecordNotFoundError: true})
	db, err := gorm.Open(sqlite.Open(filepath.Join(abs, "aegis.db")+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"), &gorm.Config{Logger: dbLog})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.AutoMigrate(&configRecord{}, &agentRecord{}, &skillRecord{}, &Project{}, &Issue{}, &IssueRelation{}, &Execution{}, &ExecutionEvent{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &AgentWakeup{}, &IssueDecomposition{}, &Finding{}); err != nil {
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}
	s := &Store{dataDir: abs, db: db, subscribers: make(map[chan StateView]struct{}), updatedAt: time.Now()}
	var settings configRecord
	if err := db.First(&settings, 1).Error; err == nil {
		s.config = settings.Value
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := s.loadRegistry(); err != nil {
		return nil, err
	}
	if err := s.seedProject(); err != nil {
		return nil, err
	}
	if err := s.normalizeIssueDepths(); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&Execution{}).Where("status IN ?", []string{"queued", "starting", "running", "waiting_approval"}).Updates(map[string]any{"status": "disconnected", "pid": 0, "finished_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Model(&Issue{}).Where("status = ? AND execution_phase <> ?", "in_progress", "waiting_children").Updates(map[string]any{"status": "todo", "execution_phase": "active", "checkout_execution_id": "", "current_execution_id": "", "updated_at": now}).Error
	}); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) normalizeIssueDepths() error {
	var issues []Issue
	if err := s.db.Find(&issues).Error; err != nil {
		return fmt.Errorf("load issue hierarchy: %w", err)
	}
	byID := make(map[string]Issue, len(issues))
	for _, issue := range issues {
		byID[issue.ID] = issue
	}
	depths := make(map[string]int, len(issues))
	visiting := make(map[string]bool, len(issues))
	var depthFor func(string) (int, error)
	depthFor = func(id string) (int, error) {
		if depth, ok := depths[id]; ok {
			return depth, nil
		}
		issue, ok := byID[id]
		if !ok || issue.ParentID == "" {
			depths[id] = 0
			return 0, nil
		}
		if visiting[id] {
			return 0, fmt.Errorf("issue hierarchy contains a cycle at %s", issue.Identifier)
		}
		visiting[id] = true
		parentDepth, err := depthFor(issue.ParentID)
		delete(visiting, id)
		if err != nil {
			return 0, err
		}
		depths[id] = parentDepth + 1
		return parentDepth + 1, nil
	}
	for _, issue := range issues {
		if _, err := depthFor(issue.ID); err != nil {
			return err
		}
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		for _, issue := range issues {
			if issue.RequestDepth == depths[issue.ID] {
				continue
			}
			if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Update("request_depth", depths[issue.ID]).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) seedProject() error {
	var count int64
	if err := s.db.Model(&Project{}).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return nil
	}
	now := time.Now()
	workspace := s.config.Workspace
	if workspace == "" {
		workspace, _ = os.Getwd()
	}
	return s.db.Create(&Project{ID: nextID("project"), Key: "AEG", Name: "Aegis Workspace", Description: "Default local workspace", Workspace: workspace, Status: "active", CreatedAt: now, UpdatedAt: now}).Error
}

func (s *Store) DataDir() string { return s.dataDir }
func (s *Store) Config() Config  { s.mu.RLock(); defer s.mu.RUnlock(); return s.config }
func (s *Store) SaveConfig(input SaveConfigInput) (ConfigView, error) {
	input.NodePath = strings.TrimSpace(input.NodePath)
	input.PiPath = strings.TrimSpace(input.PiPath)
	input.Provider = strings.TrimSpace(input.Provider)
	input.Model = strings.TrimSpace(input.Model)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Thinking = strings.TrimSpace(input.Thinking)
	input.AuthMode = strings.TrimSpace(input.AuthMode)
	input.Workspace = strings.TrimSpace(input.Workspace)
	input.ApprovalMode = strings.TrimSpace(input.ApprovalMode)
	if input.NodePath == "" || input.PiPath == "" {
		return ConfigView{}, errors.New("Node.js 和 Pi CLI 路径不能为空")
	}
	if _, err := os.Stat(input.NodePath); err != nil {
		return ConfigView{}, fmt.Errorf("Node.js 路径不可用: %w", err)
	}
	if _, err := os.Stat(input.PiPath); err != nil {
		return ConfigView{}, fmt.Errorf("Pi CLI 路径不可用: %w", err)
	}
	if input.Provider == "" || input.Model == "" {
		return ConfigView{}, errors.New("Provider 和模型不能为空")
	}
	if err := validateModelPricing(input.Pricing); err != nil {
		return ConfigView{}, err
	}
	info, err := os.Stat(input.Workspace)
	if err != nil || !info.IsDir() {
		return ConfigView{}, errors.New("工作目录不存在或不是目录")
	}
	workspace, err := filepath.Abs(input.Workspace)
	if err != nil {
		return ConfigView{}, err
	}
	if !slices.Contains([]string{"api_key", "environment", "pi_auth"}, input.AuthMode) {
		return ConfigView{}, errors.New("不支持的认证方式")
	}
	if !slices.Contains([]string{"all", "risky", "none"}, input.ApprovalMode) {
		return ConfigView{}, errors.New("不支持的审批模式")
	}
	if input.Concurrency < 1 {
		input.Concurrency = 1
	}
	if input.Concurrency > 8 {
		input.Concurrency = 8
	}
	if input.Thinking == "" {
		input.Thinking = "medium"
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if input.APIKey == "" {
		input.APIKey = s.config.APIKey
	}
	if input.AuthMode == "api_key" && input.APIKey == "" {
		return ConfigView{}, errors.New("API Key 认证需要填写密钥")
	}
	now := time.Now()
	s.config = Config{Configured: true, NodePath: input.NodePath, PiPath: input.PiPath, Provider: input.Provider, Model: input.Model, Pricing: input.Pricing, BaseURL: input.BaseURL, Thinking: input.Thinking, AuthMode: input.AuthMode, APIKey: input.APIKey, Workspace: workspace, Concurrency: input.Concurrency, ApprovalMode: input.ApprovalMode, UpdatedAt: now}
	if err := s.db.Save(&configRecord{ID: 1, Value: s.config, UpdatedAt: now}).Error; err != nil {
		return ConfigView{}, err
	}
	s.updatedAt = now
	s.broadcastLocked()
	return configView(s.config), nil
}

func configView(c Config) ConfigView {
	return ConfigView{Configured: c.Configured, NodePath: c.NodePath, PiPath: c.PiPath, Provider: c.Provider, Model: c.Model, Pricing: c.Pricing, BaseURL: c.BaseURL, Thinking: c.Thinking, AuthMode: c.AuthMode, HasAPIKey: c.APIKey != "", Workspace: c.Workspace, Concurrency: c.Concurrency, ApprovalMode: c.ApprovalMode, UpdatedAt: c.UpdatedAt}
}
func (s *Store) SetRuntimeProbe(p RuntimeProbe) {
	s.mu.Lock()
	s.runtime = p
	s.updatedAt = time.Now()
	s.broadcastLocked()
	s.mu.Unlock()
}

func (s *Store) State() StateView { s.mu.RLock(); defer s.mu.RUnlock(); return s.stateViewLocked() }
func (s *Store) stateViewLocked() StateView {
	var projects []Project
	var issues []Issue
	var relations []IssueRelation
	var executions []Execution
	var approvals []Approval
	s.db.Order("created_at asc").Find(&projects)
	s.db.Order("updated_at desc").Find(&issues)
	s.db.Order("created_at asc").Find(&relations)
	s.db.Order("started_at desc").Limit(300).Find(&executions)
	s.db.Order("created_at desc").Limit(200).Find(&approvals)
	return StateView{Configured: s.config.Configured, Config: configView(s.config), Runtime: s.runtime, Projects: projects, Issues: issues, Relations: relations, Executions: executions, Approvals: approvals, Agents: cloneAgents(s.agents), Skills: cloneSkills(s.skills), Sessions: s.sessionSummariesLocked(executions, issues), UpdatedAt: s.updatedAt}
}
func (s *Store) Subscribe() (<-chan StateView, func()) {
	ch := make(chan StateView, 4)
	s.mu.Lock()
	s.subscribers[ch] = struct{}{}
	view := s.stateViewLocked()
	s.mu.Unlock()
	ch <- view
	return ch, func() {
		s.mu.Lock()
		if _, ok := s.subscribers[ch]; ok {
			delete(s.subscribers, ch)
			close(ch)
		}
		s.mu.Unlock()
	}
}
func (s *Store) broadcastLocked() {
	view := s.stateViewLocked()
	for ch := range s.subscribers {
		select {
		case ch <- view:
		default:
		}
	}
}
func (s *Store) changedLocked() { s.updatedAt = time.Now(); s.broadcastLocked() }

func (s *Store) CreateIssue(input CreateIssueInput) (Issue, error) {
	input.Title = strings.TrimSpace(input.Title)
	if input.Title == "" {
		return Issue{}, errors.New("Issue 标题不能为空")
	}
	if input.Priority == "" {
		input.Priority = "medium"
	}
	if !slices.Contains([]string{"critical", "high", "medium", "low"}, input.Priority) {
		return Issue{}, errors.New("invalid priority")
	}
	if input.WorkMode == "" {
		input.WorkMode = "guided"
	}
	if !slices.Contains([]string{"guided", "autonomous"}, input.WorkMode) {
		return Issue{}, errors.New("invalid work mode")
	}
	if input.AssigneeAgentID != "" {
		if _, err := s.executionAgent(input.AssigneeAgentID); err != nil {
			return Issue{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.config.Configured {
		return Issue{}, errors.New("请先完成初始化配置")
	}
	if input.ParentID != "" {
		var parent Issue
		if err := s.db.First(&parent, "id = ?", input.ParentID).Error; err != nil {
			return Issue{}, errors.New("parent issue not found")
		}
		if s.belongsToCancelledTask(parent) {
			return Issue{}, errors.New("所属任务已取消，不能创建新的子 Issue")
		}
	}
	var project Project
	if input.ProjectID != "" {
		if err := s.db.First(&project, "id = ?", input.ProjectID).Error; err != nil {
			return Issue{}, errors.New("project not found")
		}
	} else if err := s.db.Order("created_at asc").First(&project).Error; err != nil {
		return Issue{}, err
	}
	workspace := strings.TrimSpace(input.Workspace)
	if workspace == "" {
		workspace = project.Workspace
	}
	if workspace == "" {
		workspace = s.config.Workspace
	}
	info, err := os.Stat(workspace)
	if err != nil || !info.IsDir() {
		return Issue{}, errors.New("工作目录不存在或不是目录")
	}
	workspace, err = filepath.Abs(workspace)
	if err != nil {
		return Issue{}, err
	}
	status := input.Status
	if status == "" {
		if input.AssigneeAgentID != "" {
			status = "todo"
		} else {
			status = "backlog"
		}
	}
	if !validIssueStatus(status) {
		return Issue{}, errors.New("invalid issue status")
	}
	var issue Issue
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var max int64
		if err := tx.Model(&Issue{}).Select("coalesce(max(number),0)").Scan(&max).Error; err != nil {
			return err
		}
		now := time.Now()
		issue = Issue{ID: nextID("issue"), Number: max + 1, Identifier: fmt.Sprintf("%s-%04d", project.Key, max+1), ProjectID: project.ID, ParentID: input.ParentID, Title: input.Title, Description: strings.TrimSpace(input.Description), AcceptanceCriteria: strings.TrimSpace(input.AcceptanceCriteria), Status: status, Priority: input.Priority, WorkMode: input.WorkMode, ExecutionPhase: "active", AssigneeAgentID: input.AssigneeAgentID, Workspace: workspace, Context: strings.TrimSpace(input.Context), Constraints: fallback(strings.TrimSpace(input.Constraints), "仅在指定工作目录中操作；避免破坏性命令；完成后运行相关验证。"), CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
		if issue.ParentID != "" {
			var parent Issue
			if err := tx.First(&parent, "id = ?", issue.ParentID).Error; err != nil {
				return errors.New("parent issue not found")
			}
			if parent.RequestDepth >= maxIssueDepth {
				return fmt.Errorf("Issue 已达到最大层级 %d", maxIssueDepth)
			}
			issue.RequestDepth = parent.RequestDepth + 1
		}
		if err := tx.Create(&issue).Error; err != nil {
			return err
		}
		for _, blocker := range uniqueStrings(input.BlockedBy) {
			if blocker == issue.ID {
				continue
			}
			rel := IssueRelation{ID: nextID("relation"), IssueID: blocker, RelatedIssueID: issue.ID, Type: "blocks", CreatedAt: now, UpdatedAt: now}
			if err := tx.Create(&rel).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Issue{}, err
	}
	s.changedLocked()
	return issue, nil
}

func (s *Store) GetIssue(id string) (Issue, error) {
	var issue Issue
	err := s.db.Where("id = ? OR identifier = ?", id, id).First(&issue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Issue{}, errors.New("issue not found")
	}
	return issue, err
}
func (s *Store) GetIssueDetail(id string) (IssueDetail, error) {
	issue, err := s.GetIssue(id)
	if err != nil {
		return IssueDetail{}, err
	}
	d := IssueDetail{Issue: issue, Children: []Issue{}, BlockedBy: []Issue{}, Blocks: []Issue{}, Executions: []Execution{}, Comments: []IssueComment{}, Messages: []Message{}, Events: []ExecutionEvent{}, Approvals: []Approval{}, Wakeups: []AgentWakeup{}}
	d.Decompositions = []IssueDecomposition{}
	s.db.Where("parent_id = ?", issue.ID).Order("number asc").Find(&d.Children)
	var incoming, outgoing []IssueRelation
	s.db.Where("related_issue_id = ? AND type = ?", issue.ID, "blocks").Find(&incoming)
	s.db.Where("issue_id = ? AND type = ?", issue.ID, "blocks").Find(&outgoing)
	for _, r := range incoming {
		var x Issue
		if s.db.First(&x, "id = ?", r.IssueID).Error == nil {
			d.BlockedBy = append(d.BlockedBy, x)
		}
	}
	for _, r := range outgoing {
		var x Issue
		if s.db.First(&x, "id = ?", r.RelatedIssueID).Error == nil {
			d.Blocks = append(d.Blocks, x)
		}
	}
	s.db.Where("issue_id = ?", issue.ID).Order("started_at desc").Find(&d.Executions)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at asc").Find(&d.Comments)
	var attachments []IssueAttachment
	s.db.Where("issue_id = ? AND comment_id <> ''", issue.ID).Order("created_at asc").Find(&attachments)
	attachmentsByComment := make(map[string][]IssueAttachment)
	for _, attachment := range attachments {
		attachmentsByComment[attachment.CommentID] = append(attachmentsByComment[attachment.CommentID], attachment)
	}
	for index := range d.Comments {
		d.Comments[index].Attachments = attachmentsByComment[d.Comments[index].ID]
		if d.Comments[index].Attachments == nil {
			d.Comments[index].Attachments = []IssueAttachment{}
		}
	}
	s.db.Where("issue_id = ?", issue.ID).Order("created_at asc").Find(&d.Messages)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Events)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Approvals)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Wakeups)
	s.db.Where("parent_issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Decompositions)
	return d, nil
}

func (s *Store) UpdateIssue(id string, input UpdateIssueInput) (Issue, error) {
	if input.AssigneeAgentID != nil && *input.AssigneeAgentID != "" {
		if _, e := s.executionAgent(*input.AssigneeAgentID); e != nil {
			return Issue{}, e
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, err := s.GetIssue(id)
	if err != nil {
		return Issue{}, err
	}
	if input.Status != nil && *input.Status != "cancelled" && s.belongsToCancelledTask(issue) {
		return Issue{}, errors.New("所属任务已取消，不能重新打开 Issue")
	}
	updates := map[string]any{"updated_at": time.Now()}
	if input.Title != nil {
		v := strings.TrimSpace(*input.Title)
		if v == "" {
			return Issue{}, errors.New("标题不能为空")
		}
		updates["title"] = v
	}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
	}
	if input.AcceptanceCriteria != nil {
		updates["acceptance_criteria"] = strings.TrimSpace(*input.AcceptanceCriteria)
	}
	if input.Priority != nil {
		if !slices.Contains([]string{"critical", "high", "medium", "low"}, *input.Priority) {
			return Issue{}, errors.New("invalid priority")
		}
		updates["priority"] = *input.Priority
	}
	if input.AssigneeAgentID != nil {
		updates["assignee_agent_id"] = *input.AssigneeAgentID
	}
	if input.ParentID != nil {
		if *input.ParentID == issue.ID {
			return Issue{}, errors.New("Issue 不能成为自己的父级")
		}
		ancestorID := strings.TrimSpace(*input.ParentID)
		visited := map[string]bool{}
		for ancestorID != "" {
			if ancestorID == issue.ID {
				return Issue{}, errors.New("父子层级不能形成循环")
			}
			if visited[ancestorID] {
				return Issue{}, errors.New("目标父级已存在循环")
			}
			visited[ancestorID] = true
			var ancestor Issue
			if err := s.db.First(&ancestor, "id = ?", ancestorID).Error; err != nil {
				return Issue{}, errors.New("parent issue not found")
			}
			ancestorID = ancestor.ParentID
		}
		updates["parent_id"] = *input.ParentID
	}
	if input.Status != nil {
		if !validIssueStatus(*input.Status) {
			return Issue{}, errors.New("invalid issue status")
		}
		updates["status"] = *input.Status
		now := time.Now()
		if *input.Status == "done" {
			updates["completed_at"] = &now
			updates["checkout_execution_id"] = ""
			updates["execution_phase"] = "completed"
		} else if *input.Status == "cancelled" {
			updates["cancelled_at"] = &now
			updates["checkout_execution_id"] = ""
			updates["execution_phase"] = "completed"
		} else {
			updates["completed_at"] = nil
			updates["cancelled_at"] = nil
			if *input.Status == "todo" || *input.Status == "backlog" {
				updates["execution_phase"] = "active"
			}
		}
	}
	if err := s.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(updates).Error; err != nil {
		return Issue{}, err
	}
	s.db.First(&issue, "id = ?", issue.ID)
	s.changedLocked()
	return issue, nil
}

func validIssueStatus(v string) bool {
	return slices.Contains([]string{"backlog", "todo", "in_progress", "in_review", "done", "blocked", "cancelled"}, v)
}

var validFindingCategories = []string{"recon", "headers", "tls", "cors", "cookie", "subdomain", "vuln"}
var validFindingSeverities = []string{"critical", "high", "medium", "low", "info"}
var validFindingStatuses = []string{"open", "verified", "false-positive", "closed"}

func validFindingCategory(v string) bool { return slices.Contains(validFindingCategories, v) }
func validFindingSeverity(v string) bool { return slices.Contains(validFindingSeverities, v) }
func validFindingStatus(v string) bool   { return slices.Contains(validFindingStatuses, v) }
func (s *Store) unresolvedBlockers(issueID string) (int64, error) {
	var count int64
	err := s.db.Table("issue_relations r").Joins("join issues i on i.id = r.issue_id").Where("r.related_issue_id = ? AND r.type = ? AND i.status NOT IN ?", issueID, "blocks", []string{"done", "cancelled"}).Count(&count).Error
	return count, err
}

// CheckoutIssue performs the ownership transition with one conditional SQL update.
func (s *Store) CheckoutIssue(id string, input CheckoutIssueInput) (Issue, error) {
	if strings.TrimSpace(input.AgentID) == "" || strings.TrimSpace(input.ExecutionID) == "" {
		return Issue{}, errors.New("agentId 和 executionId 必填")
	}
	if len(input.ExpectedStatuses) == 0 {
		input.ExpectedStatuses = []string{"todo", "backlog"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	issue, err := s.GetIssue(id)
	if err != nil {
		return Issue{}, err
	}
	if s.belongsToCancelledTask(issue) {
		return Issue{}, errors.New("所属任务已取消，不能 checkout")
	}
	blockers, err := s.unresolvedBlockers(issue.ID)
	if err != nil {
		return Issue{}, err
	}
	if blockers > 0 {
		return Issue{}, errors.New("issue has unresolved blockers")
	}
	now := time.Now()
	result := s.db.Model(&Issue{}).Where("id = ? AND status IN ? AND (assignee_agent_id = '' OR assignee_agent_id = ?) AND (checkout_execution_id = '' OR checkout_execution_id = ?)", issue.ID, input.ExpectedStatuses, input.AgentID, input.ExecutionID).Updates(map[string]any{"status": "in_progress", "execution_phase": "active", "assignee_agent_id": input.AgentID, "checkout_execution_id": input.ExecutionID, "current_execution_id": input.ExecutionID, "started_at": now, "updated_at": now})
	if result.Error != nil {
		return Issue{}, result.Error
	}
	if result.RowsAffected != 1 {
		return Issue{}, errors.New("checkout conflict")
	}
	s.db.First(&issue, "id = ?", issue.ID)
	s.changedLocked()
	return issue, nil
}

func (s *Store) AddRelation(issueID string, input CreateRelationInput) (IssueRelation, error) {
	if input.Type == "" {
		input.Type = "blocks"
	}
	if input.Type != "blocks" {
		return IssueRelation{}, errors.New("only blocks relations are supported")
	}
	if issueID == input.RelatedIssueID {
		return IssueRelation{}, errors.New("Issue 不能阻塞自己")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	a, err := s.GetIssue(issueID)
	if err != nil {
		return IssueRelation{}, err
	}
	b, err := s.GetIssue(input.RelatedIssueID)
	if err != nil {
		return IssueRelation{}, err
	}
	if s.relationPathExists(b.ID, a.ID) {
		return IssueRelation{}, errors.New("relation would create a cycle")
	}
	now := time.Now()
	r := IssueRelation{ID: nextID("relation"), IssueID: a.ID, RelatedIssueID: b.ID, Type: "blocks", CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&r).Error; err != nil {
		return IssueRelation{}, err
	}
	s.changedLocked()
	return r, nil
}
func (s *Store) relationPathExists(from, to string) bool {
	seen := map[string]bool{}
	queue := []string{from}
	for len(queue) > 0 {
		x := queue[0]
		queue = queue[1:]
		if x == to {
			return true
		}
		if seen[x] {
			continue
		}
		seen[x] = true
		var rs []IssueRelation
		s.db.Where("issue_id = ? AND type = ?", x, "blocks").Find(&rs)
		for _, r := range rs {
			queue = append(queue, r.RelatedIssueID)
		}
	}
	return false
}
func (s *Store) DeleteRelation(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := s.db.Delete(&IssueRelation{}, "id = ?", id)
	if r.Error != nil {
		return r.Error
	}
	if r.RowsAffected == 0 {
		return errors.New("relation not found")
	}
	s.changedLocked()
	return nil
}

// ── Finding CRUD ──────────────────────────────────────────────────────────

func (s *Store) CreateFinding(input CreateFindingInput) (Finding, error) {
	domain := strings.TrimSpace(input.Domain)
	category := strings.TrimSpace(input.Category)
	severity := strings.TrimSpace(input.Severity)
	title := strings.TrimSpace(input.Title)
	if domain == "" {
		return Finding{}, errors.New("domain is required")
	}
	if !validFindingCategory(category) {
		return Finding{}, errors.New("invalid category, must be one of: recon, headers, tls, cors, cookie, subdomain, vuln")
	}
	if !validFindingSeverity(severity) {
		return Finding{}, errors.New("invalid severity, must be one of: critical, high, medium, low, info")
	}
	if title == "" {
		return Finding{}, errors.New("title is required")
	}
	now := time.Now()
	finding := Finding{
		ID:               nextID("finding"),
		Domain:           domain,
		Category:         category,
		Severity:         severity,
		Title:            title,
		Description:      strings.TrimSpace(input.Description),
		Evidence:         input.Evidence,
		StepsToReproduce: strings.TrimSpace(input.StepsToReproduce),
		Remediation:      strings.TrimSpace(input.Remediation),
		Status:           "open",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := s.db.Create(&finding).Error; err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func (s *Store) GetFinding(id string) (Finding, error) {
	var finding Finding
	if err := s.db.First(&finding, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return Finding{}, errors.New("finding not found")
		}
		return Finding{}, err
	}
	return finding, nil
}

func (s *Store) ListFindings(category, severity string, page, pageSize int) (FindingList, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	query := s.db.Model(&Finding{})
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if severity != "" {
		query = query.Where("severity = ?", severity)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return FindingList{}, err
	}
	var findings []Finding
	offset := (page - 1) * pageSize
	if err := query.Order("created_at desc").Offset(offset).Limit(pageSize).Find(&findings).Error; err != nil {
		return FindingList{}, err
	}
	if findings == nil {
		findings = []Finding{}
	}
	return FindingList{Findings: findings, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Store) UpdateFinding(id string, input UpdateFindingInput) (Finding, error) {
	finding, err := s.GetFinding(id)
	if err != nil {
		return Finding{}, err
	}
	now := time.Now()
	finding.UpdatedAt = now

	if input.Domain != nil {
		v := strings.TrimSpace(*input.Domain)
		if v == "" {
			return Finding{}, errors.New("domain cannot be empty")
		}
		finding.Domain = v
	}
	if input.Category != nil {
		v := strings.TrimSpace(*input.Category)
		if !validFindingCategory(v) {
			return Finding{}, errors.New("invalid category")
		}
		finding.Category = v
	}
	if input.Severity != nil {
		v := strings.TrimSpace(*input.Severity)
		if !validFindingSeverity(v) {
			return Finding{}, errors.New("invalid severity")
		}
		finding.Severity = v
	}
	if input.Title != nil {
		v := strings.TrimSpace(*input.Title)
		if v == "" {
			return Finding{}, errors.New("title cannot be empty")
		}
		finding.Title = v
	}
	if input.Description != nil {
		finding.Description = strings.TrimSpace(*input.Description)
	}
	if input.Evidence != nil {
		finding.Evidence = input.Evidence
	}
	if input.StepsToReproduce != nil {
		finding.StepsToReproduce = strings.TrimSpace(*input.StepsToReproduce)
	}
	if input.Remediation != nil {
		finding.Remediation = strings.TrimSpace(*input.Remediation)
	}
	if input.Status != nil {
		v := strings.TrimSpace(*input.Status)
		if !validFindingStatus(v) {
			return Finding{}, errors.New("invalid status, must be one of: open, verified, false-positive, closed")
		}
		finding.Status = v
	}

	if err := s.db.Save(&finding).Error; err != nil {
		return Finding{}, err
	}
	return finding, nil
}

func (s *Store) createExecution(issue Issue, agentID, kind string) (Execution, error) {
	agent, err := s.executionAgent(agentID)
	if err != nil {
		return Execution{}, err
	}
	cfg := s.effectiveAgentConfig(agent)
	now := time.Now()
	e := Execution{ID: nextID("execution"), IssueID: issue.ID, AgentID: agentID, Kind: kind, Status: "queued", Provider: cfg.Provider, Model: cfg.Model, Pricing: cfg.Pricing, Thinking: cfg.Thinking, SessionID: nextID("pi-session"), SystemPrompt: agent.SystemPrompt, ToolsSnapshot: snapshotTools(agent.Tools), StartedAt: now, UpdatedAt: now}
	err = s.db.Create(&e).Error
	return e, err
}
func (s *Store) updateExecution(id string, updates map[string]any) error {
	if snapshot, ok := updates["tools_snapshot"].([]ToolSnapshot); ok {
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode execution tools snapshot: %w", err)
		}
		updates["tools_snapshot"] = string(encoded)
	}
	updates["updated_at"] = time.Now()
	query := s.db.Model(&Execution{}).Where("id = ?", id)
	if status, ok := updates["status"]; ok && status != "cancelled" {
		query = query.Where("status <> ?", "cancelled")
	}
	return query.Updates(updates).Error
}
func (s *Store) addEvent(executionID, issueID, kind, title, detail string) {
	_ = s.db.Create(&ExecutionEvent{ID: nextID("event"), ExecutionID: executionID, IssueID: issueID, Type: kind, Title: title, Detail: detail, CreatedAt: time.Now()}).Error
	s.notify()
}
func (s *Store) notify() { s.mu.Lock(); s.changedLocked(); s.mu.Unlock() }

func nextID(prefix string) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixMilli(), sequence.Add(1))
}
func truncate(v string, n int) string {
	if len(v) <= n {
		return v
	}
	return v[:n] + "…"
}
func fallback(v, r string) string {
	if strings.TrimSpace(v) == "" {
		return r
	}
	return v
}
func stringValue(v any) string {
	if x, ok := v.(string); ok {
		return x
	}
	return fmt.Sprint(v)
}
func compactJSON(value any, limit int) string { return truncate(fmt.Sprint(value), limit) }

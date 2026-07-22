package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
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
	mu               sync.RWMutex
	dataDir          string
	db               *gorm.DB
	config           Config
	agents           []AgentDefinition
	skills           []SkillDefinition
	runtime          RuntimeProbe
	updatedAt        time.Time
	subscribers      map[chan StateView]struct{}
	broadcastPending bool
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
type registrySeedMigrationRecord struct {
	ID        string `gorm:"primaryKey"`
	AppliedAt time.Time
}

func migrateTasks(db *gorm.DB) error {
	var roots []Issue
	if err := db.Where("hidden = ? AND parent_id = ? AND task_source_id = ?", false, "", "").Find(&roots).Error; err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, root := range roots {
			now := root.CreatedAt
			if now.IsZero() {
				now = time.Now()
			}
			task := Task{ID: nextID("task"), ProjectID: root.ProjectID, Title: root.Title, Description: root.Description, Objective: root.Objective, Priority: root.Priority, WorkMode: root.WorkMode, AssigneeAgentID: root.AssigneeAgentID, Workspace: root.Workspace, ContainerProfileID: root.ContainerProfileID, Context: root.Context, Constraints: root.Constraints, CreatedAt: now, UpdatedAt: root.UpdatedAt}
			if err := tx.Create(&task).Error; err != nil {
				return err
			}
			if err := tx.Model(&Issue{}).Where("id = ?", root.ID).Update("task_source_id", task.ID).Error; err != nil {
				return err
			}
		}
		return nil
	})
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
	if err := os.Chmod(abs, 0o700); err != nil {
		return nil, fmt.Errorf("secure data directory: %w", err)
	}
	dbLog := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{SlowThreshold: time.Second, LogLevel: logger.Error, IgnoreRecordNotFoundError: true})
	dbPath := filepath.Join(abs, "aegis.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=5000&_foreign_keys=on"), &gorm.Config{Logger: dbLog})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure sqlite database: %w", err)
	}
	// A rework Execution resumes its source Pi conversation. Execution IDs stay
	// unique audit records, while one Session ID may span multiple executions.
	if err := db.Exec("DROP INDEX IF EXISTS idx_executions_session_id").Error; err != nil {
		return nil, fmt.Errorf("migrate reusable execution sessions: %w", err)
	}
	if err := db.AutoMigrate(&configRecord{}, &agentRecord{}, &skillRecord{}, &registrySeedMigrationRecord{}, &AgentTemplate{}, &uncoverProviderRecord{}, &KnowledgeBase{}, &KnowledgeDocument{}, &Project{}, &ContainerProfile{}, &Task{}, &Issue{}, &ConciergeConversation{}, &IssueRelation{}, &IssueAgentSession{}, &Execution{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{}, &TaskBroadcast{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &AgentWakeup{}, &IssueDecomposition{}, &IssueChildWait{}, &Finding{}); err != nil {
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}
	if err := db.Model(&IssueComment{}).Where("type = '' OR type IS NULL").Update("type", "normal").Error; err != nil {
		return nil, fmt.Errorf("migrate Issue comment types: %w", err)
	}
	if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_issue_agent_session_current ON issue_agent_sessions(issue_id, agent_id) WHERE status = 'active'").Error; err != nil {
		return nil, fmt.Errorf("create current Issue-Agent session constraint: %w", err)
	}
	if err := migrateIssueAgentSessions(db); err != nil {
		return nil, fmt.Errorf("migrate Issue-Agent sessions: %w", err)
	}
	if err := migrateTasks(db); err != nil {
		return nil, fmt.Errorf("migrate reusable Tasks: %w", err)
	}
	if err := db.Where("status = ? AND type = ?", "pending", "tool_call").Delete(&Approval{}).Error; err != nil {
		return nil, fmt.Errorf("remove invalid approvals: %w", err)
	}
	s := &Store{dataDir: abs, db: db, subscribers: make(map[chan StateView]struct{}), updatedAt: time.Now()}
	var settings configRecord
	if err := db.First(&settings, 1).Error; err == nil {
		s.config = settings.Value
		s.config.ValidationMode, s.config.MaxValidationAttempts = normalizeValidationPolicy(s.config.ValidationMode, s.config.MaxValidationAttempts)
		s.config.MaxIssueDepth, s.config.MaxChildrenPerRequest, s.config.MaxDirectChildren = normalizeDecompositionLimits(s.config.MaxIssueDepth, s.config.MaxChildrenPerRequest, s.config.MaxDirectChildren)
		s.config.IssueBudget = normalizeIssueBudget(s.config.IssueBudget)
		s.config.IssueHeartbeat = normalizeIssueHeartbeat(s.config.IssueHeartbeat)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err := s.loadRegistry(); err != nil {
		return nil, err
	}
	if err := s.seedAgentTemplates(time.Now()); err != nil {
		return nil, fmt.Errorf("seed Agent templates: %w", err)
	}
	if err := s.seedProject(); err != nil {
		return nil, err
	}
	now := time.Now()
	if err := db.Transaction(func(tx *gorm.DB) error {
		var activeExecutions []Execution
		if err := tx.Where("status IN ?", activeExecutionStatuses).Find(&activeExecutions).Error; err != nil {
			return err
		}
		activeByIssue := make(map[string][]Execution, len(activeExecutions))
		activeIDs := make([]string, 0, len(activeExecutions))
		for _, execution := range activeExecutions {
			activeByIssue[execution.IssueID] = append(activeByIssue[execution.IssueID], execution)
			activeIDs = append(activeIDs, execution.ID)
		}
		if len(activeIDs) > 0 {
			if err := tx.Model(&Execution{}).Where("id IN ?", activeIDs).Updates(map[string]any{
				"status": "disconnected", "pid": 0, "finished_at": now, "updated_at": now,
			}).Error; err != nil {
				return err
			}
			if err := tx.Model(&Message{}).Where("execution_id IN ? AND streaming = ?", activeIDs, true).Updates(map[string]any{"streaming": false, "updated_at": now}).Error; err != nil {
				return err
			}
			if err := tx.Where("execution_id IN ? AND status = ?", activeIDs, "pending").Delete(&Approval{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&ExecutionEvent{}).Where("execution_id IN ? AND status = ?", activeIDs, "running").Updates(map[string]any{"status": "interrupted", "updated_at": now}).Error; err != nil {
				return err
			}
			// Heartbeats are coordination turns over an already waiting Issue. If
			// the service stops mid-heartbeat, restore the durable waiting state
			// instead of treating the heartbeat as interrupted primary work.
			var interruptedHeartbeats []AgentWakeup
			if err := tx.Where("execution_id IN ? AND reason = ? AND status = ?", activeIDs, issueHeartbeatReason, "delivered").Find(&interruptedHeartbeats).Error; err != nil {
				return err
			}
			for _, wakeup := range interruptedHeartbeats {
				if err := tx.Model(&AgentWakeup{}).Where("id = ?", wakeup.ID).Updates(map[string]any{
					"status": "failed", "error": "Aegis 服务重启，本次心跳已中断", "completed_at": now,
				}).Error; err != nil {
					return err
				}
				priorStatus := fallback(wakeup.PriorIssueStatus, "todo")
				priorPhase := fallback(wakeup.PriorExecutionPhase, "active")
				if err := tx.Model(&Issue{}).Where("id = ? AND current_execution_id = ?", wakeup.IssueID, wakeup.ExecutionID).Updates(map[string]any{
					"status": priorStatus, "execution_phase": priorPhase, "checkout_execution_id": "",
					"current_execution_id": wakeup.PriorExecutionID, "error": "", "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
		}
		if err := tx.Model(&IssueValidation{}).Where("status = ?", "running").Updates(map[string]any{"status": "interrupted", "error": "Aegis 服务重启，正在恢复原验收轮次"}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Issue{}).Where("status = ? AND execution_phase = ?", "in_progress", "summarizing").Updates(map[string]any{
			"status": "cancelled", "execution_phase": "completed", "checkout_execution_id": "",
			"objective_abandoned": true, "abandoned_at": now, "cancelled_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		var interruptedIssues []Issue
		if err := tx.Where("hidden = ? AND status = ? AND execution_phase NOT IN ?", false, "in_progress", []string{"waiting_children", "summarizing"}).Find(&interruptedIssues).Error; err != nil {
			return err
		}
		for _, issue := range interruptedIssues {
			executionID := strings.TrimSpace(issue.CheckoutExecutionID)
			if executionID == "" {
				executionID = strings.TrimSpace(issue.CurrentExecutionID)
			}
			if executionID == "" {
				candidates := activeByIssue[issue.ID]
				for _, candidate := range candidates {
					if candidate.Kind != "wakeup" {
						executionID = candidate.ID
						break
					}
				}
				if executionID == "" && len(candidates) > 0 {
					executionID = candidates[0].ID
				}
			}
			var recoveryExecution Execution
			if executionID != "" {
				_ = tx.First(&recoveryExecution, "id = ? AND issue_id = ?", executionID, issue.ID).Error
			}
			settledCommentWakeup := false
			if recoveryExecution.Kind == "wakeup" {
				var settledWakeups int64
				if err := tx.Model(&AgentWakeup{}).Where("execution_id = ? AND status IN ?", recoveryExecution.ID, []string{"delivered", "completed"}).Count(&settledWakeups).Error; err != nil {
					return err
				}
				settledCommentWakeup = settledWakeups > 0
			}
			if recoveryExecution.ID != "" && recoveryExecution.Status == "completed" &&
				(slices.Contains([]string{"work", "rework", "continuation"}, recoveryExecution.Kind) || settledCommentWakeup) &&
				strings.TrimSpace(recoveryExecution.Result) != "" {
				if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
					"status": "todo", "execution_phase": "recovering", "checkout_execution_id": "",
					"current_execution_id": executionID, "recovery_execution_id": executionID,
					"recovery_phase": "settled", "recovery_requested_at": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
				continue
			}
			updates := map[string]any{
				"status": "todo", "checkout_execution_id": "", "updated_at": now,
			}
			if executionID == "" {
				updates["execution_phase"] = "active"
				updates["current_execution_id"] = ""
			} else {
				updates["execution_phase"] = "recovering"
				updates["recovery_execution_id"] = executionID
				updates["recovery_phase"] = issue.ExecutionPhase
				updates["recovery_requested_at"] = now
				updates["current_execution_id"] = executionID
				if err := tx.Model(&Execution{}).Where("id = ? AND status NOT IN ?", executionID, []string{"cancelled", "failed"}).Updates(map[string]any{
					"status": "disconnected", "pid": 0, "finished_at": now, "updated_at": now,
				}).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return s, nil
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
	input.ReworkApprovalMode = strings.TrimSpace(input.ReworkApprovalMode)
	input.ValidationMode = strings.TrimSpace(input.ValidationMode)
	if input.ReworkApprovalMode == "" {
		input.ReworkApprovalMode = "all"
	}
	if input.ValidationMode == "" {
		input.ValidationMode = "fixed"
	}
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
	if !slices.Contains([]string{"all", "none"}, input.ReworkApprovalMode) {
		return ConfigView{}, errors.New("不支持的返工审批模式")
	}
	if !slices.Contains([]string{"fixed", "automatic"}, input.ValidationMode) {
		return ConfigView{}, errors.New("不支持的验收策略")
	}
	input.ValidationMode, input.MaxValidationAttempts = normalizeValidationPolicy(input.ValidationMode, input.MaxValidationAttempts)
	input.MaxIssueDepth, input.MaxChildrenPerRequest, input.MaxDirectChildren = normalizeDecompositionLimits(input.MaxIssueDepth, input.MaxChildrenPerRequest, input.MaxDirectChildren)
	input.IssueBudget = normalizeIssueBudget(input.IssueBudget)
	if err := validateIssueBudget(input.IssueBudget); err != nil {
		return ConfigView{}, err
	}
	input.IssueHeartbeat = normalizeIssueHeartbeat(input.IssueHeartbeat)
	if err := validateIssueHeartbeat(input.IssueHeartbeat); err != nil {
		return ConfigView{}, err
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
	if input.APIKey == "" {
		input.APIKey = s.config.APIKey
	}
	if input.AuthMode == "api_key" && input.APIKey == "" {
		s.mu.Unlock()
		return ConfigView{}, errors.New("API Key 认证需要填写密钥")
	}
	now := time.Now()
	s.config = Config{Configured: true, NodePath: input.NodePath, PiPath: input.PiPath, Provider: input.Provider, Model: input.Model, Pricing: input.Pricing, BaseURL: input.BaseURL, Thinking: input.Thinking, AuthMode: input.AuthMode, APIKey: input.APIKey, Workspace: workspace, Concurrency: input.Concurrency, ApprovalMode: input.ApprovalMode, ReworkApprovalMode: input.ReworkApprovalMode, ValidationMode: input.ValidationMode, MaxValidationAttempts: input.MaxValidationAttempts, MaxIssueDepth: input.MaxIssueDepth, MaxChildrenPerRequest: input.MaxChildrenPerRequest, MaxDirectChildren: input.MaxDirectChildren, IssueBudget: input.IssueBudget, IssueHeartbeat: input.IssueHeartbeat, UpdatedAt: now}
	if err := s.db.Save(&configRecord{ID: 1, Value: s.config, UpdatedAt: now}).Error; err != nil {
		s.mu.Unlock()
		return ConfigView{}, err
	}
	savedConfig := s.config
	s.updatedAt = now
	s.broadcastLocked()
	s.mu.Unlock()
	if err := s.seedAgentTemplates(now); err != nil {
		return ConfigView{}, fmt.Errorf("refresh Agent templates: %w", err)
	}
	s.notify()
	return configView(savedConfig), nil
}

func configView(c Config) ConfigView {
	mode, attempts := normalizeValidationPolicy(c.ValidationMode, c.MaxValidationAttempts)
	depth, perRequest, direct := normalizeDecompositionLimits(c.MaxIssueDepth, c.MaxChildrenPerRequest, c.MaxDirectChildren)
	return ConfigView{Configured: c.Configured, NodePath: c.NodePath, PiPath: c.PiPath, Provider: c.Provider, Model: c.Model, Pricing: c.Pricing, BaseURL: c.BaseURL, Thinking: c.Thinking, AuthMode: c.AuthMode, HasAPIKey: c.APIKey != "", Workspace: c.Workspace, Concurrency: c.Concurrency, ApprovalMode: c.ApprovalMode, ReworkApprovalMode: fallback(c.ReworkApprovalMode, "all"), ValidationMode: mode, MaxValidationAttempts: attempts, MaxIssueDepth: depth, MaxChildrenPerRequest: perRequest, MaxDirectChildren: direct, IssueBudget: normalizeIssueBudget(c.IssueBudget), IssueHeartbeat: normalizeIssueHeartbeat(c.IssueHeartbeat), UpdatedAt: c.UpdatedAt}
}

func normalizeIssueHeartbeat(heartbeat IssueHeartbeatConfig) IssueHeartbeatConfig {
	if heartbeat.IntervalSeconds <= 0 {
		heartbeat.IntervalSeconds = 60
	}
	return heartbeat
}

func validateIssueHeartbeat(heartbeat IssueHeartbeatConfig) error {
	if heartbeat.IntervalSeconds < 1 || heartbeat.IntervalSeconds > 3600 {
		return errors.New("Issue 心跳间隔必须在 1 到 3600 秒之间")
	}
	return nil
}

func normalizeIssueBudget(budget IssueBudgetConfig) IssueBudgetConfig {
	// Older config records have no issueBudget object. A zero polling interval
	// identifies that migration case and applies the requested 10-minute default.
	if budget.CheckIntervalSeconds <= 0 {
		minutes := 10
		budget.TimeLimitMinutes = &minutes
		budget.CheckIntervalSeconds = 60
	}
	return budget
}

func validateIssueBudget(budget IssueBudgetConfig) error {
	if budget.CheckIntervalSeconds < 1 || budget.CheckIntervalSeconds > 3600 {
		return errors.New("Issue 预算轮询间隔必须在 1 到 3600 秒之间")
	}
	if budget.TokenLimit != nil && *budget.TokenLimit <= 0 {
		return errors.New("Issue Token 预算必须大于 0，或留空禁用")
	}
	if budget.CostLimit != nil && (*budget.CostLimit <= 0 || math.IsNaN(*budget.CostLimit) || math.IsInf(*budget.CostLimit, 0)) {
		return errors.New("Issue 成本预算必须是大于 0 的有限数字，或留空禁用")
	}
	if budget.TimeLimitMinutes != nil && *budget.TimeLimitMinutes <= 0 {
		return errors.New("Issue 时间预算必须大于 0 分钟，或留空禁用")
	}
	return nil
}

func normalizeDecompositionLimits(depth, perRequest, direct int) (int, int, int) {
	if depth < 1 {
		depth = 4
	}
	if depth > 20 {
		depth = 20
	}
	if perRequest < 2 {
		perRequest = 8
	}
	if perRequest > 50 {
		perRequest = 50
	}
	if direct < 2 {
		direct = 16
	}
	if direct > 200 {
		direct = 200
	}
	if direct < perRequest {
		direct = perRequest
	}
	return depth, perRequest, direct
}

func normalizeValidationPolicy(mode string, attempts int) (string, int) {
	mode = strings.TrimSpace(mode)
	if mode != "automatic" {
		mode = "fixed"
	}
	if attempts < 1 {
		attempts = 3
	}
	if attempts > 20 {
		attempts = 20
	}
	return mode, attempts
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
	var containerProfiles []ContainerProfile
	var tasks []Task
	var issues []Issue
	var relations []IssueRelation
	var executions []Execution
	var approvals []Approval
	knowledgeBases, _ := s.listKnowledgeBases()
	s.db.Order("created_at asc").Find(&projects)
	s.db.Order("created_at asc").Find(&containerProfiles)
	s.db.Order("updated_at desc").Find(&tasks)
	for index := range containerProfiles {
		containerProfiles[index] = containerRuntimeState(containerProfiles[index])
	}
	s.db.Where("hidden = ?", false).Order("updated_at desc").Find(&issues)
	s.db.Order("created_at asc").Find(&relations)
	s.db.Where("issue_id IN (?)", s.db.Model(&Issue{}).Select("id").Where("hidden = ?", false)).Order("started_at desc").Limit(300).Find(&executions)
	s.db.Order("created_at desc").Limit(200).Find(&approvals)
	for index := range issues {
		issues[index] = compactIssueForState(issues[index])
	}
	for index := range executions {
		executions[index] = compactExecution(executions[index])
	}
	return StateView{Configured: s.config.Configured, Config: configView(s.config), Runtime: s.runtime, Projects: projects, ContainerProfiles: containerProfiles, Tasks: tasks, Issues: issues, Relations: relations, Executions: executions, Approvals: approvals, Agents: cloneAgents(s.agents), Skills: cloneSkills(s.skills), KnowledgeBases: knowledgeBases, Sessions: s.sessionSummariesLocked(executions, issues), UpdatedAt: s.updatedAt}
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
	if len(s.subscribers) == 0 {
		return
	}
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
	var err error
	input.Title, err = normalizeIssueTitle(input.Title)
	if err != nil {
		return Issue{}, err
	}
	input.Objective = strings.TrimSpace(input.Objective)
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
		if input.ContainerProfileID == "" {
			input.ContainerProfileID = parent.ContainerProfileID
		}
	}
	var selectedContainerProfile *ContainerProfile
	if input.ContainerProfileID != "" {
		var profile ContainerProfile
		if err := s.db.First(&profile, "id = ? AND enabled = ?", input.ContainerProfileID, true).Error; err != nil {
			return Issue{}, errors.New("容器执行环境不存在或已停用")
		}
		profile = containerRuntimeState(profile)
		if profile.RuntimeStatus != "running" {
			return Issue{}, errors.New("只能选择已启动的容器")
		}
		selectedContainerProfile = &profile
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
	if input.TimeBudgetMinutes != nil && *input.TimeBudgetMinutes <= 0 {
		return Issue{}, errors.New("任务时间预算必须大于 0 分钟，或留空使用全局配置")
	}
	if selectedContainerProfile != nil && (selectedContainerProfile.HostWorkspace == "" || filepath.Clean(workspace) != filepath.Clean(selectedContainerProfile.HostWorkspace)) {
		return Issue{}, errors.New("任务工作目录必须与容器绑定的宿主机工作目录一致")
	}
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
	validationMode, maxValidationAttempts := normalizeValidationPolicy(s.config.ValidationMode, s.config.MaxValidationAttempts)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var max int64
		if err := tx.Model(&Issue{}).Select("coalesce(max(number),0)").Scan(&max).Error; err != nil {
			return err
		}
		now := time.Now()
		issue = Issue{ID: nextID("issue"), Number: max + 1, Identifier: fmt.Sprintf("%s-%04d", project.Key, max+1), ProjectID: project.ID, ParentID: input.ParentID, TaskSourceID: input.TaskSourceID, Title: input.Title, Description: strings.TrimSpace(input.Description), Objective: input.Objective, Status: status, Priority: input.Priority, WorkMode: input.WorkMode, ExecutionPhase: "active", ValidationMode: validationMode, MaxValidationAttempts: maxValidationAttempts, AssigneeAgentID: input.AssigneeAgentID, Workspace: workspace, ContainerProfileID: input.ContainerProfileID, Context: strings.TrimSpace(input.Context), Constraints: fallback(strings.TrimSpace(input.Constraints), "仅在指定工作目录中操作；避免破坏性命令；完成后运行相关验证。"), TimeBudgetMinutes: input.TimeBudgetMinutes, CreatedBy: "operator", CreatedAt: now, UpdatedAt: now}
		if issue.ParentID != "" {
			var parent Issue
			if err := tx.First(&parent, "id = ?", issue.ParentID).Error; err != nil {
				return errors.New("parent issue not found")
			}
			configuredDepth, _, _ := normalizeDecompositionLimits(s.config.MaxIssueDepth, s.config.MaxChildrenPerRequest, s.config.MaxDirectChildren)
			if parent.RequestDepth >= configuredDepth {
				return fmt.Errorf("Issue 已达到最大层级 %d", configuredDepth)
			}
			issue.RequestDepth = parent.RequestDepth + 1
			issue.ValidationMode, issue.MaxValidationAttempts = normalizeValidationPolicy(parent.ValidationMode, parent.MaxValidationAttempts)
			issue.ValidationDisabled = parent.ValidationDisabled || strings.TrimSpace(parent.Objective) == ""
		}
		if err := tx.Create(&issue).Error; err != nil {
			return err
		}
		if issue.AssigneeAgentID != "" {
			if _, err := ensureIssueAgentSessionOnDB(tx, issue, issue.AssigneeAgentID, ""); err != nil {
				return err
			}
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

func (s *Store) CreateTask(input CreateIssueInput) (Task, Issue, error) {
	if input.ParentID != "" {
		return Task{}, Issue{}, errors.New("任务定义不能包含父 Issue")
	}
	now := time.Now()
	task := Task{ID: nextID("task"), ProjectID: input.ProjectID, Title: strings.TrimSpace(input.Title), Description: strings.TrimSpace(input.Description), Objective: strings.TrimSpace(input.Objective), Priority: input.Priority, WorkMode: input.WorkMode, AssigneeAgentID: input.AssigneeAgentID, Workspace: strings.TrimSpace(input.Workspace), ContainerProfileID: input.ContainerProfileID, Context: strings.TrimSpace(input.Context), Constraints: strings.TrimSpace(input.Constraints), TimeBudgetMinutes: input.TimeBudgetMinutes, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&task).Error; err != nil {
		return Task{}, Issue{}, err
	}
	input.TaskSourceID = task.ID
	issue, err := s.CreateIssue(input)
	if err != nil {
		_ = s.db.Delete(&Task{}, "id = ?", task.ID).Error
		return Task{}, Issue{}, err
	}
	// Persist normalized/defaulted values from the actual run.
	task.ProjectID, task.Title, task.Description, task.Objective = issue.ProjectID, issue.Title, issue.Description, issue.Objective
	task.Priority, task.WorkMode, task.AssigneeAgentID = issue.Priority, issue.WorkMode, issue.AssigneeAgentID
	task.Workspace, task.ContainerProfileID, task.Context, task.Constraints = issue.Workspace, issue.ContainerProfileID, issue.Context, issue.Constraints
	task.TimeBudgetMinutes = issue.TimeBudgetMinutes
	if err = s.db.Save(&task).Error; err != nil {
		return Task{}, Issue{}, err
	}
	s.notify()
	return task, issue, nil
}

func (s *Store) GetTask(id string) (Task, error) {
	var task Task
	if err := s.db.First(&task, "id = ?", id).Error; err != nil {
		return Task{}, errors.New("task not found")
	}
	return task, nil
}

func (s *Store) GetIssue(id string) (Issue, error) {
	var issue Issue
	err := s.db.First(&issue, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Issue{}, errors.New("issue not found")
	}
	return issue, err
}

func (s *Store) TaskWorkspace(id string) (TaskWorkspace, error) {
	workspace := ""
	if task, err := s.GetTask(id); err == nil {
		workspace = task.Workspace
	} else if issue, issueErr := s.GetIssue(id); issueErr == nil && issue.ParentID == "" {
		workspace = issue.Workspace
	} else {
		return TaskWorkspace{}, errors.New("task not found")
	}
	root, err := filepath.Abs(workspace)
	if err != nil {
		return TaskWorkspace{}, err
	}
	result := TaskWorkspace{Root: root, Entries: []WorkspaceEntry{}}
	err = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		if path == root {
			return nil
		}
		relative, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil
		}
		kind := "file"
		if entry.IsDir() {
			kind = "directory"
		} else if entry.Type()&os.ModeSymlink != 0 {
			kind = "symlink"
		}
		result.Entries = append(result.Entries, WorkspaceEntry{Path: filepath.ToSlash(relative), Kind: kind, Size: info.Size(), ModifiedAt: info.ModTime()})
		return nil
	})
	if err != nil {
		return TaskWorkspace{}, err
	}
	slices.SortFunc(result.Entries, func(a, b WorkspaceEntry) int { return strings.Compare(a.Path, b.Path) })
	return result, nil
}
func (s *Store) GetIssueDetail(id string) (IssueDetail, error) {
	issue, err := s.GetIssue(id)
	if err != nil {
		return IssueDetail{}, err
	}
	d := IssueDetail{Issue: issue, Children: []Issue{}, BlockedBy: []Issue{}, Blocks: []Issue{}, Executions: []Execution{}, AgentSessions: []IssueAgentSession{}, Comments: []IssueComment{}, Messages: []Message{}, Events: []ExecutionEvent{}, Approvals: []Approval{}, Wakeups: []AgentWakeup{}, Validations: []IssueValidation{}, Broadcasts: []TaskBroadcast{}, Watermark: time.Now()}
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
	executions, err := s.IssueExecutionsPage(issue.ID, "", detailPageSize)
	if err != nil {
		return IssueDetail{}, err
	}
	d.Executions, d.ExecutionsPage = executions.Items, executions.Page
	s.db.Where("issue_id = ?", issue.ID).Order("created_at asc").Find(&d.AgentSessions)
	comments, err := s.IssueCommentsPage(issue.ID, "", detailPageSize)
	if err != nil {
		return IssueDetail{}, err
	}
	d.Comments, d.CommentsPage = comments.Items, comments.Page
	events, err := s.IssueEventsPage(issue.ID, "", detailPageSize)
	if err != nil {
		return IssueDetail{}, err
	}
	d.Events, d.EventsPage = events.Items, events.Page
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Approvals)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Wakeups)
	s.db.Where("parent_issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Decompositions)
	s.db.Where("issue_id = ?", issue.ID).Order("attempt desc").Find(&d.Validations)
	for index := range d.Validations {
		d.Validations[index].Objective = ""
		d.Validations[index].CandidateResult = ""
	}
	root, err := s.taskRoot(issue)
	if err != nil {
		return IssueDetail{}, err
	}
	d.Broadcasts, err = s.taskBroadcasts(root.ID, 100)
	if err != nil {
		return IssueDetail{}, err
	}
	return d, nil
}

func (s *Store) GetExecutionEvent(id string) (ExecutionEvent, error) {
	var event ExecutionEvent
	if err := s.db.First(&event, "id = ?", id).Error; err != nil {
		return ExecutionEvent{}, errors.New("execution event not found")
	}
	return event, nil
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
		v, err := normalizeIssueTitle(*input.Title)
		if err != nil {
			return Issue{}, err
		}
		updates["title"] = v
	}
	if input.Description != nil {
		updates["description"] = strings.TrimSpace(*input.Description)
	}
	if input.Objective != nil {
		updates["objective"] = strings.TrimSpace(*input.Objective)
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
	if issue.AssigneeAgentID != "" {
		if _, err := s.ensureIssueAgentSession(issue, issue.AssigneeAgentID, ""); err != nil {
			return Issue{}, err
		}
	}
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
	return s.createExecutionRecord(issue, agent, kind, "")
}

func (s *Store) createExecutionWithSession(issue Issue, agentID, kind, sessionID string) (Execution, error) {
	agent, err := s.executionAgent(agentID)
	if err != nil {
		return Execution{}, err
	}
	return s.createExecutionRecord(issue, agent, kind, sessionID)
}

func (s *Store) createInternalExecution(issue Issue, agentID, kind string) (Execution, AgentDefinition, error) {
	agent, err := s.GetAgent(agentID)
	if err != nil {
		return Execution{}, AgentDefinition{}, err
	}
	if !agent.Enabled || !agent.Internal {
		return Execution{}, AgentDefinition{}, errors.New("internal agent is unavailable")
	}
	execution, err := s.createExecutionRecord(issue, agent, kind, "")
	return execution, agent, err
}

func (s *Store) createExecutionRecord(issue Issue, agent AgentDefinition, kind, sessionID string) (Execution, error) {
	cfg := s.effectiveAgentConfig(agent)
	now := time.Now()
	runtimeType := "host"
	containerImage := ""
	if issue.ContainerProfileID != "" {
		var profile ContainerProfile
		if err := s.db.First(&profile, "id = ? AND enabled = ?", issue.ContainerProfileID, true).Error; err != nil {
			return Execution{}, errors.New("容器执行环境不存在或已停用")
		}
		runtimeType = "container"
		containerImage = profile.Image
	}
	binding, err := s.ensureIssueAgentSession(issue, agent.ID, sessionID)
	if err != nil {
		return Execution{}, err
	}
	e := Execution{ID: nextID("execution"), IssueID: issue.ID, AgentID: agent.ID, Kind: kind, Status: "queued", Provider: cfg.Provider, Model: cfg.Model, Pricing: cfg.Pricing, Thinking: cfg.Thinking, SessionID: binding.SessionID, IssueAgentSessionID: binding.ID, RuntimeType: runtimeType, ContainerProfileID: issue.ContainerProfileID, ContainerImage: containerImage, SystemPrompt: agent.SystemPrompt, ToolsSnapshot: snapshotTools(agent.Tools), StartedAt: now, UpdatedAt: now}
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
func (s *Store) notify() {
	s.mu.Lock()
	s.updatedAt = time.Now()
	if len(s.subscribers) == 0 {
		s.mu.Unlock()
		return
	}
	if s.broadcastPending {
		s.mu.Unlock()
		return
	}
	s.broadcastPending = true
	s.mu.Unlock()
	time.AfterFunc(500*time.Millisecond, func() {
		s.mu.Lock()
		s.broadcastPending = false
		s.broadcastLocked()
		s.mu.Unlock()
	})
}

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

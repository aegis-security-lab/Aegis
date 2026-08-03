package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aegis/capability"
	"aegis/observability"
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
	updatedAt        time.Time
	subscribers      map[chan StateView]struct{}
	broadcastPending bool
	observabilityMu  sync.RWMutex
	observability    *SystemObservability
}

func withSQLiteRetry(operation func() error) error {
	var err error
	for attempt := 0; attempt < 4; attempt++ {
		err = operation()
		if err == nil || !strings.Contains(strings.ToLower(err.Error()), "database is locked") {
			return err
		}
		observability.Default().Warn(context.Background(), "sqlite.operation.retry", slog.Int("attempt", attempt+1), slog.String("error", err.Error()), slog.String("component", "control.store"))
		time.Sleep(time.Duration(100*(1<<attempt)) * time.Millisecond)
	}
	return err
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

const decompositionDefaults100MigrationID = "decomposition-defaults-100-v1"

func (s *Store) migrateDecompositionDefaults(now time.Time) error {
	var applied int64
	if err := s.db.Model(&registrySeedMigrationRecord{}).Where("id = ?", decompositionDefaults100MigrationID).Count(&applied).Error; err != nil {
		return err
	}
	if applied > 0 {
		return nil
	}
	return s.db.Transaction(func(tx *gorm.DB) error {
		// 8/16 were the former shipped defaults. Upgrade only that exact pair;
		// custom configurations are preserved.
		if s.config.Configured && s.config.MaxChildrenPerRequest == 8 && s.config.MaxDirectChildren == 16 {
			s.config.MaxChildrenPerRequest = 100
			s.config.MaxDirectChildren = 100
			s.config.UpdatedAt = now
			if err := tx.Save(&configRecord{ID: 1, Value: s.config, UpdatedAt: now}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&registrySeedMigrationRecord{ID: decompositionDefaults100MigrationID, AppliedAt: now}).Error
	})
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
			task := Task{ID: nextID("task"), ProjectID: root.ProjectID, Title: root.Title, Description: root.Description, Objective: root.Objective, Priority: root.Priority, WorkMode: root.WorkMode, AssigneeAgentID: root.AssigneeAgentID, Workspace: root.Workspace, ContainerProfileID: root.ContainerProfileID, ContainerID: root.ContainerID, Context: root.Context, Constraints: root.Constraints, CreatedAt: now, UpdatedAt: root.UpdatedAt}
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

func migrateTaskContainers(db *gorm.DB) error {
	var tasks []Task
	var issues []Issue
	var containers []ContainerInstance
	if err := db.Find(&tasks).Error; err != nil {
		return err
	}
	if err := db.Select("id", "parent_id", "task_source_id", "container_profile_id", "container_id").Find(&issues).Error; err != nil {
		return err
	}
	if err := db.Find(&containers).Error; err != nil {
		return err
	}
	var defaultProfile ContainerProfile
	if err := db.Where("enabled = ?", true).
		Order("case when lower(name) = 'default' then 0 else 1 end").
		Order("created_at asc").First(&defaultProfile).Error; err != nil {
		return errors.New("没有可用的 Docker 容器配置，无法迁移任务容器")
	}
	containerByTask := make(map[string]ContainerInstance, len(containers))
	for _, container := range containers {
		containerByTask[container.TaskID] = container
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, task := range tasks {
			profileID := strings.TrimSpace(task.ContainerProfileID)
			if profileID == "" {
				profileID = defaultProfile.ID
			}
			container, exists := containerByTask[task.ID]
			if !exists {
				var profile ContainerProfile
				if err := tx.First(&profile, "id = ?", profileID).Error; err != nil || !profile.Enabled {
					profile = defaultProfile
					profileID = profile.ID
				}
				now := time.Now()
				container = ContainerInstance{
					ID: nextID("container"), ContainerProfileID: profile.ID, TaskID: task.ID,
					Image:         profile.Image,
					WorkspacePath: profile.WorkspacePath, NetworkMode: profile.NetworkMode,
					MemoryMB: profile.MemoryMB, CPUs: profile.CPUs, CreatedAt: now, UpdatedAt: now,
				}
				container.Name = taskContainerName(container.ID)
				if err := tx.Create(&container).Error; err != nil {
					return err
				}
			}
			if err := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(map[string]any{
				"container_profile_id": container.ContainerProfileID,
				"container_id":         container.ID,
			}).Error; err != nil {
				return err
			}
			bound := make(map[string]bool)
			for _, issue := range issues {
				if issue.TaskSourceID == task.ID {
					bound[issue.ID] = true
				}
			}
			for changed := true; changed; {
				changed = false
				for _, issue := range issues {
					if issue.ParentID != "" && bound[issue.ParentID] && !bound[issue.ID] {
						bound[issue.ID] = true
						changed = true
					}
				}
			}
			ids := make([]string, 0, len(bound))
			for id := range bound {
				ids = append(ids, id)
			}
			if len(ids) > 0 {
				if err := tx.Model(&Issue{}).Where("id IN ?", ids).Updates(map[string]any{
					"container_profile_id": container.ContainerProfileID,
					"container_id":         container.ID,
				}).Error; err != nil {
					return err
				}
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
	// Attachments are untrusted user/Agent output and may themselves contain Go
	// source. A nested module boundary prevents `go test ./...` and `go vet
	// ./...` from treating runtime artifacts as Aegis packages when the default
	// data directory lives below the repository root.
	moduleBoundary := filepath.Join(abs, "go.mod")
	if _, statErr := os.Stat(moduleBoundary); errors.Is(statErr, os.ErrNotExist) {
		if writeErr := os.WriteFile(moduleBoundary, []byte("module aegis-runtime-data\n\ngo 1.25\n"), 0o600); writeErr != nil {
			return nil, fmt.Errorf("isolate runtime data from Go source tree: %w", writeErr)
		}
	} else if statErr != nil {
		return nil, fmt.Errorf("inspect runtime data Go boundary: %w", statErr)
	}
	dbLog := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{SlowThreshold: time.Second, LogLevel: logger.Error, IgnoreRecordNotFoundError: true})
	dbPath := filepath.Join(abs, "aegis.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=30000&_foreign_keys=on&_synchronous=NORMAL"), &gorm.Config{Logger: dbLog})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite permits concurrent readers but only one writer. A single pooled
	// connection prevents goroutines in this process from competing through
	// multiple SQLite connections and lets the busy timeout serialize writes.
	if sqlDB, dbErr := db.DB(); dbErr != nil {
		return nil, fmt.Errorf("configure sqlite pool: %w", dbErr)
	} else {
		sqlDB.SetMaxOpenConns(1)
		sqlDB.SetMaxIdleConns(1)
		sqlDB.SetConnMaxLifetime(0)
	}
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure sqlite database: %w", err)
	}
	if err := db.AutoMigrate(&configRecord{}, &agentRecord{}, &skillRecord{}, &registrySeedMigrationRecord{}, &uncoverProviderRecord{}, &KnowledgeBase{}, &KnowledgeDocument{}, &Project{}, &ContainerProfile{}, &ContainerInstance{}, &Task{}, &Issue{}, &TaskAgent{}, &ConciergeConversation{}, &IssueRelation{}, &Execution{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{}, &Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &InputAttachment{}, &AgentWakeup{}, &IssueDecomposition{}, &IssueChildWait{}, &RelayThread{}, &RelayMessage{}, &RelayReceipt{}, &Finding{}); err != nil {
		return nil, fmt.Errorf("initialize sqlite schema: %w", err)
	}
	// Keep one public Board priority vocabulary. These aliases existed in older
	// releases, so normalize them in place before any scheduler reads the queue.
	if err := db.Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&Issue{}, &Task{}} {
			if err := tx.Model(model).Where("priority = ?", "medium").Update("priority", "middle").Error; err != nil {
				return err
			}
			if err := tx.Model(model).Where("priority = ?", "critical").Update("priority", "high").Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("migrate Issue priorities: %w", err)
	}
	// Older releases created blocks edges between sibling Issues and from each
	// child to its parent. Hierarchy plus IssueChildWait now owns coordination.
	if err := db.Exec(`DELETE FROM issue_relations
		WHERE EXISTS (
			SELECT 1 FROM issues blocker, issues blocked
			WHERE blocker.id = issue_relations.issue_id
			  AND blocked.id = issue_relations.related_issue_id
			  AND (blocker.parent_id = blocked.id
			    OR blocked.parent_id = blocker.id
			    OR (blocker.parent_id <> '' AND blocker.parent_id = blocked.parent_id))
		)`).Error; err != nil {
		return nil, fmt.Errorf("remove legacy child Issue dependencies: %w", err)
	}
	var containerProfileCount int64
	if err := db.Model(&ContainerProfile{}).Count(&containerProfileCount).Error; err != nil {
		return nil, fmt.Errorf("inspect default container profile: %w", err)
	}
	if containerProfileCount == 0 {
		now := time.Now()
		profile := ContainerProfile{
			ID: "container-profile-default", Name: "default", Description: "Aegis 默认 Docker 隔离环境",
			Image: WorkerContainerImage, WorkspacePath: "/workspace",
			NetworkMode: "bridge", Enabled: true, CreatedAt: now, UpdatedAt: now,
		}
		if err := db.Create(&profile).Error; err != nil {
			return nil, fmt.Errorf("create default container profile: %w", err)
		}
	}
	if err := migrateLegacyUserPromptContext(db); err != nil {
		return nil, fmt.Errorf("migrate legacy user prompt context: %w", err)
	}
	if err := db.Model(&IssueComment{}).Where("type = '' OR type IS NULL").Update("type", "normal").Error; err != nil {
		return nil, fmt.Errorf("migrate Issue comment types: %w", err)
	}
	if err := migrateTasks(db); err != nil {
		return nil, fmt.Errorf("migrate reusable Tasks: %w", err)
	}
	if err := migrateTaskContainers(db); err != nil {
		return nil, fmt.Errorf("migrate one container per Task: %w", err)
	}
	if err := migrateTaskWorkspaceIsolation(db); err != nil {
		return nil, fmt.Errorf("migrate private Task workspaces: %w", err)
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
	if err := s.migrateDecompositionDefaults(time.Now()); err != nil {
		return nil, fmt.Errorf("migrate decomposition defaults: %w", err)
	}
	if err := s.loadRegistry(); err != nil {
		return nil, err
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
		// Older versions represented runtime failures as blocked Issues. A failed
		// execution is terminal, so migrate those records before Coordination
		// evaluates dependency edges and waiting parents.
		failedExecutions := tx.Model(&Execution{}).Select("id").Where("status IN ?", []string{"failed", "disconnected", "stopped"})
		if err := tx.Model(&Issue{}).
			Where("status = ? AND current_execution_id IN (?)", "blocked", failedExecutions).
			Updates(map[string]any{
				"status": "failed", "execution_phase": "completed", "checkout_execution_id": "",
				"completed_at": now, "updated_at": now,
			}).Error; err != nil {
			return err
		}
		if err := tx.Model(&Issue{}).Where("status = ? AND execution_phase = ?", "in_progress", "summarizing").Updates(map[string]any{
			"status": "cancelled", "execution_phase": "completed", "checkout_execution_id": "",
			"objective_abandoned": true, "abandoned_at": now, "cancelled_at": now, "updated_at": now,
		}).Error; err != nil {
			return err
		}
		var interruptedIssues []Issue
		if err := tx.Where("hidden = ? AND status = ? AND execution_phase NOT IN ?", false, "in_progress", []string{"waiting_children", "sleeping", "summarizing"}).Find(&interruptedIssues).Error; err != nil {
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

func migrateTaskWorkspaceIsolation(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var tasks []Task
		if err := tx.Where("container_profile_id <> ''").Find(&tasks).Error; err != nil {
			return err
		}
		for _, task := range tasks {
			if strings.TrimSpace(filepath.ToSlash(task.Workspace)) == TaskWorkspacePath {
				continue
			}
			updates := map[string]any{
				"workspace":   TaskWorkspacePath,
				"description": rewriteWorkspacePaths(task.Description, TaskWorkspacePath, task.Workspace),
				"objective":   rewriteWorkspacePaths(task.Objective, TaskWorkspacePath, task.Workspace),
				"context":     rewriteWorkspacePaths(task.Context, TaskWorkspacePath, task.Workspace),
				"constraints": rewriteWorkspacePaths(task.Constraints, TaskWorkspacePath, task.Workspace),
			}
			if err := tx.Model(&Task{}).Where("id = ?", task.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		var issues []Issue
		if err := tx.Where("container_profile_id <> ''").Find(&issues).Error; err != nil {
			return err
		}
		for _, issue := range issues {
			if strings.TrimSpace(filepath.ToSlash(issue.Workspace)) == TaskWorkspacePath {
				continue
			}
			updates := map[string]any{
				"workspace":   TaskWorkspacePath,
				"description": rewriteWorkspacePaths(issue.Description, TaskWorkspacePath, issue.Workspace),
				"objective":   rewriteWorkspacePaths(issue.Objective, TaskWorkspacePath, issue.Workspace),
				"context":     rewriteWorkspacePaths(issue.Context, TaskWorkspacePath, issue.Workspace),
				"constraints": rewriteWorkspacePaths(issue.Constraints, TaskWorkspacePath, issue.Workspace),
			}
			if err := tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&ContainerProfile{}).Where("workspace_path <> ?", TaskWorkspacePath).Update("workspace_path", TaskWorkspacePath).Error; err != nil {
			return err
		}
		return tx.Model(&ContainerInstance{}).Where("workspace_path <> ?", TaskWorkspacePath).Update("workspace_path", TaskWorkspacePath).Error
	})
}

func stripLegacyUserPromptContext(content string) string {
	cut := len(content)
	for _, marker := range []string{
		"\n\n## Organization delegation boundary\n",
		"\n\n<organization_delegation_boundary>",
		"\n\n<agent_permission_boundary>",
		"\n\n<agent_memo>",
	} {
		if index := strings.Index(content, marker); index >= 0 && index < cut {
			cut = index
		}
	}
	if cut == len(content) {
		return content
	}
	return strings.TrimSpace(content[:cut])
}

func migrateLegacyUserPromptContext(db *gorm.DB) error {
	var messages []Message
	if err := db.Where("role = ?", "user").Find(&messages).Error; err != nil {
		return err
	}
	for _, message := range messages {
		cleaned := stripLegacyUserPromptContext(message.Content)
		if cleaned != message.Content {
			if err := db.Model(&Message{}).Where("id = ?", message.ID).UpdateColumn("content", cleaned).Error; err != nil {
				return err
			}
		}
	}
	var executions []Execution
	if err := db.Where("initial_prompt <> ?", "").Find(&executions).Error; err != nil {
		return err
	}
	for _, execution := range executions {
		cleaned := stripLegacyUserPromptContext(execution.InitialPrompt)
		if cleaned != execution.InitialPrompt {
			if err := db.Model(&Execution{}).Where("id = ?", execution.ID).UpdateColumn("initial_prompt", cleaned).Error; err != nil {
				return err
			}
		}
	}
	return nil
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
	input.Language = strings.TrimSpace(input.Language)
	if input.Language == "" {
		input.Language = "zh"
	}
	input.Provider = strings.TrimSpace(input.Provider)
	input.Model = strings.TrimSpace(input.Model)
	input.BaseURL = strings.TrimSpace(input.BaseURL)
	input.Thinking = strings.TrimSpace(input.Thinking)
	input.AuthMode = strings.TrimSpace(input.AuthMode)
	input.Workspace = strings.TrimSpace(input.Workspace)
	input.ApprovalMode = strings.TrimSpace(input.ApprovalMode)
	input.ReworkApprovalMode = strings.TrimSpace(input.ReworkApprovalMode)
	input.ValidationMode = strings.TrimSpace(input.ValidationMode)
	input.WebSearch.Engine = strings.TrimSpace(input.WebSearch.Engine)
	input.WebSearch.BaseURL = strings.TrimSpace(input.WebSearch.BaseURL)
	if input.ReworkApprovalMode == "" {
		input.ReworkApprovalMode = "all"
	}
	if input.ValidationMode == "" {
		input.ValidationMode = "fixed"
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
	if !slices.Contains([]string{"api_key", "environment"}, input.AuthMode) {
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
	if !slices.Contains([]string{"zh", "en"}, input.Language) {
		return ConfigView{}, errors.New("不支持的输出语言")
	}
	if input.WebSearch.Engine == "" {
		input.WebSearch.Engine = "tavily"
	}
	if input.WebSearch.BaseURL == "" {
		input.WebSearch.BaseURL = "https://api.tavily.com/search"
	}
	if input.WebSearch.Engine != "tavily" {
		return ConfigView{}, errors.New("当前仅支持 Tavily 搜索引擎")
	}
	if parsed, parseErr := url.ParseRequestURI(input.WebSearch.BaseURL); parseErr != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ConfigView{}, errors.New("搜索服务 URL 必须是有效的 HTTP 或 HTTPS 地址")
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
	if input.Concurrency > 100 {
		input.Concurrency = 100
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
	if input.WebSearch.APIKey == "" {
		input.WebSearch.APIKey = s.config.WebSearch.APIKey
	}
	if input.WebSearch.APIKey != "" {
		input.WebSearch.Enabled = true
	}
	if input.WebSearch.Enabled && input.WebSearch.APIKey == "" {
		s.mu.Unlock()
		return ConfigView{}, errors.New("启用 Tavily 搜索需要填写 API Key")
	}
	now := time.Now()
	s.config = Config{Configured: true, Language: input.Language, Provider: input.Provider, Model: input.Model, Pricing: input.Pricing, BaseURL: input.BaseURL, Thinking: input.Thinking, AuthMode: input.AuthMode, APIKey: input.APIKey, Workspace: workspace, Concurrency: input.Concurrency, ApprovalMode: input.ApprovalMode, ReworkApprovalMode: input.ReworkApprovalMode, ValidationMode: input.ValidationMode, MaxValidationAttempts: input.MaxValidationAttempts, MaxIssueDepth: input.MaxIssueDepth, MaxChildrenPerRequest: input.MaxChildrenPerRequest, MaxDirectChildren: input.MaxDirectChildren, IssueBudget: input.IssueBudget, IssueHeartbeat: input.IssueHeartbeat, WebSearch: input.WebSearch, UpdatedAt: now}
	if err := s.db.Save(&configRecord{ID: 1, Value: s.config, UpdatedAt: now}).Error; err != nil {
		s.mu.Unlock()
		return ConfigView{}, err
	}
	savedConfig := s.config
	s.updatedAt = now
	s.broadcastLocked()
	s.mu.Unlock()
	if system := s.Observability(); system != nil && system.Redactor != nil {
		system.Redactor.RegisterSecret(savedConfig.APIKey)
		system.Redactor.RegisterSecret(savedConfig.WebSearch.APIKey)
	}
	s.notify()
	return configView(savedConfig), nil
}

func configView(c Config) ConfigView {
	mode, attempts := normalizeValidationPolicy(c.ValidationMode, c.MaxValidationAttempts)
	depth, perRequest, direct := normalizeDecompositionLimits(c.MaxIssueDepth, c.MaxChildrenPerRequest, c.MaxDirectChildren)
	search := c.WebSearch
	search.Engine = fallback(search.Engine, "tavily")
	search.BaseURL = fallback(search.BaseURL, "https://api.tavily.com/search")
	return ConfigView{Configured: c.Configured, Language: fallback(c.Language, "zh"), Provider: c.Provider, Model: c.Model, Pricing: c.Pricing, BaseURL: c.BaseURL, Thinking: c.Thinking, AuthMode: c.AuthMode, HasAPIKey: c.APIKey != "", Workspace: c.Workspace, Concurrency: c.Concurrency, ApprovalMode: c.ApprovalMode, ReworkApprovalMode: fallback(c.ReworkApprovalMode, "all"), ValidationMode: mode, MaxValidationAttempts: attempts, MaxIssueDepth: depth, MaxChildrenPerRequest: perRequest, MaxDirectChildren: direct, IssueBudget: normalizeIssueBudget(c.IssueBudget), IssueHeartbeat: normalizeIssueHeartbeat(c.IssueHeartbeat), WebSearch: WebSearchConfigView{Engine: search.Engine, BaseURL: search.BaseURL, HasAPIKey: search.APIKey != "", Enabled: search.Enabled}, UpdatedAt: c.UpdatedAt}
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
	if budget.MaxTurns <= 0 {
		budget.MaxTurns = 100
	}
	if budget.ActiveTimeMinutes <= 0 {
		budget.ActiveTimeMinutes = 20
	}
	if budget.SummaryTurns <= 0 {
		budget.SummaryTurns = 10
	}
	if budget.SummaryTimeMinutes <= 0 {
		budget.SummaryTimeMinutes = 3
	}
	return budget
}

func validateIssueBudget(budget IssueBudgetConfig) error {
	if budget.MaxTurns < 1 || budget.MaxTurns > 1000 {
		return errors.New("子 Issue 单次 Execution 轮数预算必须在 1 到 1000 之间")
	}
	if budget.ActiveTimeMinutes < 1 || budget.ActiveTimeMinutes > 1440 {
		return errors.New("子 Issue 单次 Execution 时间预算必须在 1 到 1440 分钟之间")
	}
	if budget.SummaryTurns < 1 || budget.SummaryTurns > 100 {
		return errors.New("预算耗尽后的总结轮数必须在 1 到 100 之间")
	}
	if budget.SummaryTimeMinutes < 1 || budget.SummaryTimeMinutes > 60 {
		return errors.New("预算耗尽后的总结时间必须在 1 到 60 分钟之间")
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
		perRequest = 100
	}
	if perRequest > 100 {
		perRequest = 100
	}
	if direct < 2 {
		direct = 100
	}
	if direct > 100 {
		direct = 100
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
func (s *Store) State() StateView { s.mu.RLock(); defer s.mu.RUnlock(); return s.stateViewLocked() }
func (s *Store) stateViewLocked() StateView {
	var projects []Project
	var containerProfiles []ContainerProfile
	var containers []ContainerInstance
	var tasks []Task
	var issues []Issue
	var relations []IssueRelation
	var executions []Execution
	var approvals []Approval
	var taskAgents []TaskAgent
	knowledgeBases, _ := s.listKnowledgeBases()
	s.db.Order("created_at asc").Find(&projects)
	s.db.Order("created_at asc").Find(&containerProfiles)
	s.db.Order("created_at desc").Find(&containers)
	s.db.Order("updated_at desc").Find(&tasks)
	for index := range containers {
		containers[index] = containerRuntimeState(containers[index])
	}
	s.db.Where("hidden = ?", false).Order("updated_at desc").Find(&issues)
	s.db.Order("created_at asc").Find(&relations)
	s.db.Where("issue_id IN (?)", s.db.Model(&Issue{}).Select("id").Where("hidden = ?", false)).Order("started_at desc").Limit(300).Find(&executions)
	s.db.Order("created_at desc").Limit(200).Find(&approvals)
	s.db.Order("created_at asc").Find(&taskAgents)
	for index := range issues {
		issues[index] = compactIssueForState(issues[index])
	}
	for index := range executions {
		executions[index] = compactExecution(executions[index])
	}
	return StateView{Configured: s.config.Configured, Config: configView(s.config), Projects: projects, ContainerProfiles: containerProfiles, Containers: containers, Tasks: tasks, Issues: issues, IssueRuntimes: s.issueRuntimeViews(issues), Relations: relations, Executions: executions, Approvals: approvals, Agents: cloneAgents(s.agents), TaskAgents: taskAgents, Skills: cloneSkills(s.skills), KnowledgeBases: knowledgeBases, Sessions: s.sessionSummariesLocked(executions, issues), UpdatedAt: s.updatedAt}
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
	requestedWorkspace := strings.TrimSpace(input.Workspace)
	parentWorkspace := ""
	// Every visible root Issue belongs to a reusable Task. Keep the lower-level
	// Issue API backward compatible by promoting unsourced roots instead of
	// allowing a second, task-less root concept to leak into the product model.
	if strings.TrimSpace(input.ParentID) == "" && strings.TrimSpace(input.TaskSourceID) == "" {
		_, issue, err := s.CreateTask(input)
		return issue, err
	}
	var err error
	input.Title, err = normalizeIssueTitle(input.Title)
	if err != nil {
		return Issue{}, err
	}
	input.Objective = strings.TrimSpace(input.Objective)
	input.CapabilitySelection = strings.TrimSpace(input.CapabilitySelection)
	if input.CapabilitySelection != "" && !slices.Contains([]string{"inherit", "merge", "replace"}, input.CapabilitySelection) {
		return Issue{}, errors.New("invalid capability selection")
	}
	if len(input.Capabilities) > 64 {
		return Issue{}, errors.New("Issue capabilities cannot exceed 64 entries")
	}
	input.Priority = normalizeIssuePriority(input.Priority)
	if !slices.Contains([]string{"high", "middle", "low"}, input.Priority) {
		return Issue{}, errors.New("invalid priority")
	}
	if input.WorkMode == "" {
		input.WorkMode = "guided"
	}
	if !slices.Contains([]string{"guided", "autonomous"}, input.WorkMode) {
		return Issue{}, errors.New("invalid work mode")
	}
	if input.AssigneeAgentID != "" {
		if _, err := s.assignableAgentType(input.AssigneeAgentID); err != nil {
			return Issue{}, err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.config.Configured {
		return Issue{}, errors.New("请先完成初始化配置")
	}
	if input.ParentID == "" && strings.TrimSpace(input.TaskSourceID) != "" {
		var sourceTask Task
		if err := s.db.First(&sourceTask, "id = ?", input.TaskSourceID).Error; err != nil {
			return Issue{}, errors.New("task not found")
		}
		input.ContainerProfileID = sourceTask.ContainerProfileID
		input.ContainerID = sourceTask.ContainerID
		input.Workspace = sourceTask.Workspace
		parentWorkspace = sourceTask.Workspace
	}
	if input.ParentID == "" && strings.TrimSpace(input.ContainerProfileID) == "" && s.config.Provider != "test" {
		profile, profileErr := s.DefaultContainerProfile()
		if profileErr != nil {
			return Issue{}, profileErr
		}
		input.ContainerProfileID = profile.ID
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
		if input.ContainerID == "" {
			input.ContainerID = parent.ContainerID
		}
		parentWorkspace = parent.Workspace
	}
	if input.ContainerProfileID != "" {
		var profile ContainerProfile
		if err := s.db.First(&profile, "id = ?", input.ContainerProfileID).Error; err != nil || (!profile.Enabled && input.ContainerID == "") {
			return Issue{}, errors.New("容器执行环境不存在或已停用")
		}
		if input.ContainerID != "" {
			var container ContainerInstance
			if err := s.db.First(&container, "id = ? AND container_profile_id = ?", input.ContainerID, input.ContainerProfileID).Error; err != nil {
				return Issue{}, errors.New("任务绑定的容器不存在或与环境配置不匹配")
			}
		}
		// Container-backed work has a logical runtime path only. Never preserve,
		// validate, or expose a host workspace supplied by an API caller or Agent.
		input.Workspace = profile.WorkspacePath
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
	if input.ParentID != "" {
		var parent Issue
		if err := s.db.First(&parent, "id = ?", input.ParentID).Error; err != nil {
			return Issue{}, errors.New("parent issue not found")
		}
		workspace = parent.Workspace
	} else if input.ContainerProfileID != "" {
		workspace = TaskWorkspacePath
	} else if workspace == "" {
		// A directly-created root Issue starts in the currently configured
		// workspace. Project.Workspace may refer to an old checkout and is not a
		// host-path mapping for new Board work.
		base := s.config.Workspace
		workspace = filepath.Join(base, ".aegis", "workspaces", nextID("workspace"))
		if err := os.MkdirAll(workspace, 0o700); err != nil {
			return Issue{}, err
		}
	}
	if input.ContainerProfileID == "" {
		info, statErr := os.Stat(workspace)
		if statErr != nil || !info.IsDir() {
			return Issue{}, errors.New("工作目录不存在或不是目录")
		}
		workspace, err = filepath.Abs(workspace)
	} else {
		workspace = TaskWorkspacePath
		// Agent-authored child descriptions are durable UI data, so normalize
		// paths before persistence instead of relying on a later runtime rewrite.
		input.Description = rewriteWorkspacePaths(input.Description, workspace, requestedWorkspace, parentWorkspace, s.config.Workspace)
		input.Objective = rewriteWorkspacePaths(input.Objective, workspace, requestedWorkspace, parentWorkspace, s.config.Workspace)
		input.Context = rewriteWorkspacePaths(input.Context, workspace, requestedWorkspace, parentWorkspace, s.config.Workspace)
		input.Constraints = rewriteWorkspacePaths(input.Constraints, workspace, requestedWorkspace, parentWorkspace, s.config.Workspace)
	}
	if input.TimeBudgetMinutes != nil && *input.TimeBudgetMinutes <= 0 {
		return Issue{}, errors.New("任务时间预算必须大于 0 分钟，或留空使用全局配置")
	}
	if err != nil {
		return Issue{}, err
	}
	status := input.Status
	if status == "" {
		status = "todo"
	}
	if !validIssueStatus(status) {
		return Issue{}, errors.New("invalid issue status")
	}
	if status == "in_progress" && strings.TrimSpace(input.AssigneeAgentID) == "" {
		return Issue{}, errors.New("in_progress Issue 必须有负责人")
	}
	var issue Issue
	validationMode, maxValidationAttempts := normalizeValidationPolicy(s.config.ValidationMode, s.config.MaxValidationAttempts)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		var max int64
		if err := tx.Model(&Issue{}).Select("coalesce(max(number),0)").Scan(&max).Error; err != nil {
			return err
		}
		now := time.Now()
		issue = Issue{ID: fallback(strings.TrimSpace(input.RequestedID), nextID("issue")), Number: max + 1, Identifier: fmt.Sprintf("%s-%04d", project.Key, max+1), ProjectID: project.ID, ParentID: input.ParentID, TaskSourceID: input.TaskSourceID, Title: input.Title, Description: strings.TrimSpace(input.Description), Objective: input.Objective, Status: status, Priority: input.Priority, WorkMode: input.WorkMode, ExecutionPhase: "active", ValidationMode: validationMode, MaxValidationAttempts: maxValidationAttempts, AssigneeAgentID: input.AssigneeAgentID, Capabilities: append([]capability.Ref(nil), input.Capabilities...), CapabilitySelection: strings.TrimSpace(input.CapabilitySelection), Workspace: workspace, ContainerProfileID: input.ContainerProfileID, ContainerID: input.ContainerID, Context: strings.TrimSpace(input.Context), Constraints: fallback(strings.TrimSpace(input.Constraints), "允许访问任务 Docker 容器内的任意文件路径；避免无关或破坏性操作；完成后运行相关验证。"), TimeBudgetMinutes: input.TimeBudgetMinutes, HumanValidationFallback: input.HumanValidationFallback, CreatedBy: fallback(strings.TrimSpace(input.CreatedBy), "operator"), CreatedAt: now, UpdatedAt: now}
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
			issue.HumanValidationFallback = parent.HumanValidationFallback
		}
		if issue.AssigneeAgentID != "" {
			taskID := issue.ID
			if issue.ParentID != "" {
				root, rootErr := taskRootWithDB(tx, issue)
				if rootErr != nil {
					return rootErr
				}
				taskID = root.ID
			}
			identity, identityErr := claimTaskAgentTx(tx, taskID, issue.AssigneeAgentID, input.AssigneeTaskAgentID)
			if identityErr != nil {
				return identityErr
			}
			issue.AssigneeTaskAgentID = identity.ID
		}
		if err := tx.Create(&issue).Error; err != nil {
			return err
		}
		if err := s.bindInputAttachmentsTx(tx, issue, input); err != nil {
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

func (s *Store) CreateTask(input CreateIssueInput) (Task, Issue, error) {
	if input.ParentID != "" {
		return Task{}, Issue{}, errors.New("任务定义不能包含父 Issue")
	}
	requestedWorkspace := strings.TrimSpace(input.Workspace)
	if strings.TrimSpace(input.ContainerProfileID) == "" {
		profile, err := s.DefaultContainerProfile()
		if err != nil {
			return Task{}, Issue{}, err
		}
		input.ContainerProfileID = profile.ID
	}
	profile, err := s.GetContainerProfile(input.ContainerProfileID)
	if err != nil || !profile.Enabled {
		return Task{}, Issue{}, errors.New("容器执行环境不存在或已停用")
	}
	input.Workspace = profile.WorkspacePath
	input.Description = rewriteWorkspacePaths(input.Description, input.Workspace, requestedWorkspace, s.Config().Workspace)
	input.Objective = rewriteWorkspacePaths(input.Objective, input.Workspace, requestedWorkspace, s.Config().Workspace)
	input.Context = rewriteWorkspacePaths(input.Context, input.Workspace, requestedWorkspace, s.Config().Workspace)
	input.Constraints = rewriteWorkspacePaths(input.Constraints, input.Workspace, requestedWorkspace, s.Config().Workspace)
	now := time.Now()
	task := Task{ID: nextID("task"), ProjectID: input.ProjectID, Title: strings.TrimSpace(input.Title), Description: strings.TrimSpace(input.Description), Objective: strings.TrimSpace(input.Objective), Priority: input.Priority, WorkMode: input.WorkMode, AssigneeAgentID: input.AssigneeAgentID, Workspace: strings.TrimSpace(input.Workspace), ContainerProfileID: input.ContainerProfileID, ContainerID: input.ContainerID, Context: strings.TrimSpace(input.Context), Constraints: strings.TrimSpace(input.Constraints), TimeBudgetMinutes: input.TimeBudgetMinutes, HumanValidationFallback: input.HumanValidationFallback, CreatedAt: now, UpdatedAt: now}
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
	task.Workspace, task.ContainerProfileID, task.ContainerID, task.Context, task.Constraints = issue.Workspace, issue.ContainerProfileID, issue.ContainerID, issue.Context, issue.Constraints
	task.TimeBudgetMinutes = issue.TimeBudgetMinutes
	task.HumanValidationFallback = issue.HumanValidationFallback
	if err = s.db.Save(&task).Error; err != nil {
		return Task{}, Issue{}, err
	}
	container, err := s.createTaskContainerBinding(task)
	if err != nil {
		return Task{}, Issue{}, err
	}
	task.ContainerID = container.ID
	issue.ContainerID = container.ID
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

func (s *Store) UpdateTaskBudget(id string, minutes *int) (Task, Issue, error) {
	if minutes == nil || *minutes <= 0 {
		return Task{}, Issue{}, errors.New("任务时间预算必须大于 0 分钟")
	}
	var task Task
	if err := s.db.First(&task, "id = ?", id).Error; err != nil {
		return Task{}, Issue{}, errors.New("task not found")
	}
	var issue Issue
	if err := s.db.Where("task_source_id = ? AND parent_id = ''", id).Order("created_at desc").First(&issue).Error; err != nil {
		return Task{}, Issue{}, errors.New("根 Issue 不存在")
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&task).Updates(map[string]any{"time_budget_minutes": *minutes, "updated_at": time.Now()}).Error; err != nil {
			return err
		}
		return tx.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"time_budget_minutes": *minutes, "updated_at": time.Now()}).Error
	}); err != nil {
		return Task{}, Issue{}, err
	}
	updated, err := s.GetTask(id)
	return updated, issue, err
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
	containerID := ""
	containerProfileID := ""
	if task, err := s.GetTask(id); err == nil {
		workspace = task.Workspace
		containerID = task.ContainerID
		containerProfileID = task.ContainerProfileID
	} else if issue, issueErr := s.GetIssue(id); issueErr == nil && issue.ParentID == "" {
		workspace = issue.Workspace
		containerID = issue.ContainerID
		containerProfileID = issue.ContainerProfileID
	} else {
		return TaskWorkspace{}, errors.New("task not found")
	}
	if containerID != "" {
		container, err := s.GetContainer(containerID)
		if err != nil {
			return TaskWorkspace{}, err
		}
		return TaskWorkspace{Root: container.WorkspacePath, Entries: []WorkspaceEntry{}}, nil
	}
	if containerID == "" && containerProfileID != "" {
		if profile, err := s.GetContainerProfile(containerProfileID); err == nil {
			return TaskWorkspace{Root: profile.WorkspacePath, Entries: []WorkspaceEntry{}}, nil
		}
	}
	// Non-container workspaces are retained only for internal conversations.
	// Product Tasks never expose a host file tree; deliverables leave through
	// aegis_publish_attachment instead.
	return TaskWorkspace{Root: workspace, Entries: []WorkspaceEntry{}}, nil
}
func (s *Store) GetIssueDetail(id string) (IssueDetail, error) {
	issue, err := s.GetIssue(id)
	if err != nil {
		return IssueDetail{}, err
	}
	d := IssueDetail{Issue: issue, Children: []Issue{}, BlockedBy: []Issue{}, Blocks: []Issue{}, Executions: []Execution{}, Comments: []IssueComment{}, Messages: []Message{}, Events: []ExecutionEvent{}, Approvals: []Approval{}, Wakeups: []AgentWakeup{}, Validations: []IssueValidation{}, Watermark: time.Now()}
	if runtimes := s.issueRuntimeViews([]Issue{issue}); len(runtimes) == 1 {
		d.Runtime = runtimes[0]
	}
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
	if err := s.db.Where("issue_id = ?", issue.ID).Order("created_at desc, id desc").Limit(detailPageSize).Find(&d.Messages).Error; err != nil {
		return IssueDetail{}, err
	}
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Approvals)
	s.db.Where("issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Wakeups)
	s.db.Where("parent_issue_id = ?", issue.ID).Order("created_at desc").Find(&d.Decompositions)
	s.db.Where("issue_id = ?", issue.ID).Order("attempt desc").Find(&d.Validations)
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
		if _, e := s.assignableAgentType(*input.AssigneeAgentID); e != nil {
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
	effectiveStatus, effectiveAssignee := issue.Status, issue.AssigneeAgentID
	if input.Status != nil {
		effectiveStatus = strings.TrimSpace(*input.Status)
	}
	if input.AssigneeAgentID != nil {
		effectiveAssignee = strings.TrimSpace(*input.AssigneeAgentID)
	}
	if effectiveStatus == "in_progress" && effectiveAssignee == "" {
		return Issue{}, errors.New("in_progress Issue 必须有负责人")
	}
	if input.AssigneeAgentID != nil && *input.AssigneeAgentID != issue.AssigneeAgentID {
		var active int64
		if err := s.db.Model(&Execution{}).Where("issue_id = ? AND status IN ?", issue.ID, activeExecutionStatuses).Count(&active).Error; err != nil {
			return Issue{}, err
		}
		if active > 0 {
			return Issue{}, errors.New("Issue 正在执行，不能更换负责人")
		}
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
	if input.TimeBudgetMinutes != nil {
		if *input.TimeBudgetMinutes <= 0 {
			return Issue{}, errors.New("任务时间预算必须大于 0 分钟")
		}
		if issue.ParentID != "" {
			return Issue{}, errors.New("只有根 Issue 可以调整任务时间预算")
		}
		updates["time_budget_minutes"] = *input.TimeBudgetMinutes
	}
	if input.Priority != nil {
		value := normalizeIssuePriority(*input.Priority)
		if !slices.Contains([]string{"high", "middle", "low"}, value) {
			return Issue{}, errors.New("invalid priority")
		}
		updates["priority"] = value
	}
	if input.AssigneeAgentID != nil {
		if issue.AssigneeTaskAgentID != "" && *input.AssigneeAgentID != issue.AssigneeAgentID {
			_ = s.db.Model(&TaskAgent{}).Where("id = ?", issue.AssigneeTaskAgentID).Updates(map[string]any{"status": "released", "updated_at": time.Now().UTC()}).Error
		}
		updates["assignee_agent_id"] = *input.AssigneeAgentID
		updates["assignee_task_agent_id"] = ""
		if *input.AssigneeAgentID != "" {
			root, rootErr := taskRootWithDB(s.db, issue)
			if rootErr != nil {
				return Issue{}, rootErr
			}
			identity, identityErr := claimTaskAgentTx(s.db, root.ID, *input.AssigneeAgentID, "")
			if identityErr != nil {
				return Issue{}, identityErr
			}
			updates["assignee_task_agent_id"] = identity.ID
		}
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
		if *input.Status == "done" || *input.Status == "failed" || *input.Status == "budget_exceeded" {
			updates["completed_at"] = &now
			updates["checkout_execution_id"] = ""
			updates["execution_phase"] = fallback(map[string]string{"budget_exceeded": "budget_exceeded"}[*input.Status], "completed")
		} else if *input.Status == "cancelled" {
			updates["cancelled_at"] = &now
			updates["checkout_execution_id"] = ""
			updates["execution_phase"] = "completed"
		} else {
			updates["completed_at"] = nil
			updates["cancelled_at"] = nil
			if *input.Status == "todo" || *input.Status == "backlog" {
				updates["execution_phase"] = "active"
				if slices.Contains([]string{"failed", "budget_exceeded"}, issue.Status) {
					updates["error"] = ""
					updates["checkout_execution_id"] = ""
				}
			}
		}
	}
	if err := s.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(updates).Error; err != nil {
		return Issue{}, err
	}
	var refreshed Issue
	if err := s.db.First(&refreshed, "id = ?", issue.ID).Error; err != nil {
		return Issue{}, err
	}
	issue = refreshed
	if issue.AssigneeTaskAgentID != "" && input.Status != nil {
		identityStatus := "active"
		if issueStatusTerminal(*input.Status) {
			identityStatus = "completed"
		}
		_ = s.db.Model(&TaskAgent{}).Where("id = ?", issue.AssigneeTaskAgentID).Updates(map[string]any{"status": identityStatus, "updated_at": time.Now().UTC()}).Error
	}
	s.changedLocked()
	return issue, nil
}

func validIssueStatus(v string) bool {
	return slices.Contains([]string{"backlog", "todo", "in_progress", "in_review", "done", "blocked", "failed", "budget_exceeded", "cancelled"}, v)
}

func normalizeIssuePriority(value string) string {
	switch strings.TrimSpace(strings.ToLower(value)) {
	case "", "medium", "middle":
		return "middle"
	case "critical", "high":
		return "high"
	case "low":
		return "low"
	default:
		return strings.TrimSpace(strings.ToLower(value))
	}
}

var terminalIssueStatuses = []string{"done", "failed", "budget_exceeded", "cancelled"}

func issueStatusTerminal(status string) bool {
	return slices.Contains(terminalIssueStatuses, status)
}

var validFindingCategories = []string{"recon", "headers", "tls", "cors", "cookie", "subdomain", "vuln"}
var validFindingSeverities = []string{"critical", "high", "medium", "low", "info"}
var validFindingStatuses = []string{"open", "verified", "false-positive", "closed"}

func validFindingCategory(v string) bool { return slices.Contains(validFindingCategories, v) }
func validFindingSeverity(v string) bool { return slices.Contains(validFindingSeverities, v) }
func validFindingStatus(v string) bool   { return slices.Contains(validFindingStatuses, v) }
func (s *Store) unresolvedBlockers(issueID string) (int64, error) {
	var count int64
	err := s.db.Table("issue_relations r").Joins("join issues i on i.id = r.issue_id").Where("r.related_issue_id = ? AND r.type = ? AND i.status NOT IN ?", issueID, "blocks", terminalIssueStatuses).Count(&count).Error
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
		query := s.db.First(&profile, "id = ?", issue.ContainerProfileID)
		if query.Error != nil || (!profile.Enabled && issue.ContainerID == "") {
			return Execution{}, errors.New("容器执行环境不存在或已停用")
		}
		runtimeType = "container"
		containerImage = profile.Image
	}
	if agent.Internal || agent.ID == conciergeAgentID || agent.Category == "concierge" {
		sessionID = fallback(strings.TrimSpace(sessionID), nextID("pi-session"))
	} else {
		if strings.TrimSpace(issue.AssigneeTaskAgentID) == "" {
			return Execution{}, errors.New("Issue 尚未分配任务内 Agent 身份")
		}
		stableSessionID := "task-session-" + issue.AssigneeTaskAgentID
		if strings.TrimSpace(sessionID) != "" && sessionID != stableSessionID {
			return Execution{}, errors.New("任务内 Agent Session 固定，不能绑定其他 Session")
		}
		sessionID = stableSessionID
	}
	e := Execution{ID: nextID("execution"), IssueID: issue.ID, AgentID: agent.ID, TaskAgentID: issue.AssigneeTaskAgentID, Kind: kind, Status: "queued", Provider: cfg.Provider, Model: cfg.Model, Pricing: cfg.Pricing, Thinking: cfg.Thinking, SessionID: sessionID, RuntimeType: runtimeType, RuntimeID: issue.ContainerID, ContainerProfileID: issue.ContainerProfileID, ContainerImage: containerImage, SystemPrompt: agent.SystemPrompt, ToolsSnapshot: snapshotTools(agent.Tools), StartedAt: now, UpdatedAt: now}
	if issue.ParentID != "" && slices.Contains([]string{"work", "wakeup", "planning", "rework", "continuation", "recovery"}, kind) {
		budget := normalizeIssueBudget(s.Config().IssueBudget)
		e.BudgetMaxTurns = budget.MaxTurns
		e.BudgetActiveMinutes = budget.ActiveTimeMinutes
		e.BudgetSummaryTurns = budget.SummaryTurns
		e.BudgetSummaryMinutes = budget.SummaryTimeMinutes
		e.BudgetPhase = "active"
	}
	err := s.db.Create(&e).Error
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
	if snapshot, ok := updates["capabilities_snapshot"].([]capability.Snapshot); ok {
		encoded, err := json.Marshal(snapshot)
		if err != nil {
			return fmt.Errorf("encode execution capabilities snapshot: %w", err)
		}
		updates["capabilities_snapshot"] = string(encoded)
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
	ctx := observability.WithScope(context.Background(), observability.Scope{IssueID: issueID, ExecutionID: executionID, Component: "control.domain"})
	level := slog.LevelInfo
	if kind == "error" || kind == "stderr" {
		level = slog.LevelError
	}
	observability.Default().Log(ctx, level, "control.execution_event", slog.String("event_type", kind), slog.String("title", title), slog.String("detail", truncate(detail, 4000)))
	observability.DefaultMetrics().AddCounter("control_execution_events_total", 1, observability.Labels{"type": kind})
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

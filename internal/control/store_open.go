package control

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func NewStore(dataDir string) (*Store, error) {
	abs, err := prepareStoreDataDir(dataDir)
	if err != nil {
		return nil, err
	}
	db, err := openStoreDatabase(abs)
	if err != nil {
		return nil, err
	}
	if err := initializeStoreSchema(db); err != nil {
		return nil, err
	}
	if err := recoverInterruptedTaskAudits(db, time.Now()); err != nil {
		return nil, err
	}
	if err := ensureDefaultContainerProfile(db, time.Now()); err != nil {
		return nil, err
	}
	s := &Store{dataDir: abs, db: db, subscribers: make(map[chan StateView]struct{}), updatedAt: time.Now()}
	if err := s.loadConfig(); err != nil {
		return nil, err
	}
	if err := s.loadRegistry(); err != nil {
		return nil, err
	}
	if err := s.seedProject(); err != nil {
		return nil, err
	}
	if err := recoverInterruptedRuntime(db, time.Now()); err != nil {
		return nil, err
	}
	return s, nil
}

func prepareStoreDataDir(dataDir string) (string, error) {
	if strings.TrimSpace(dataDir) == "" {
		return "", errors.New("data directory is required")
	}
	abs, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(abs, 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return "", fmt.Errorf("secure data directory: %w", err)
	}
	// Runtime attachments can contain Go source. A nested module boundary keeps
	// repository-wide Go tooling from treating untrusted artifacts as packages.
	moduleBoundary := filepath.Join(abs, "go.mod")
	if _, statErr := os.Stat(moduleBoundary); errors.Is(statErr, os.ErrNotExist) {
		if writeErr := os.WriteFile(moduleBoundary, []byte("module aegis-runtime-data\n\ngo 1.25\n"), 0o600); writeErr != nil {
			return "", fmt.Errorf("isolate runtime data from Go source tree: %w", writeErr)
		}
	} else if statErr != nil {
		return "", fmt.Errorf("inspect runtime data Go boundary: %w", statErr)
	}
	return abs, nil
}

func openStoreDatabase(dataDir string) (*gorm.DB, error) {
	dbLog := logger.New(log.New(os.Stderr, "", log.LstdFlags), logger.Config{
		SlowThreshold: time.Second, LogLevel: logger.Error, IgnoreRecordNotFoundError: true,
	})
	dbPath := filepath.Join(dataDir, "aegis.db")
	db, err := gorm.Open(sqlite.Open(dbPath+"?_journal_mode=WAL&_busy_timeout=30000&_foreign_keys=on&_synchronous=NORMAL"), &gorm.Config{Logger: dbLog})
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	// SQLite has one writer. One pooled connection lets the busy timeout
	// serialize writes from all goroutines in this process.
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("configure sqlite pool: %w", err)
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)
	sqlDB.SetConnMaxLifetime(0)
	if err := os.Chmod(dbPath, 0o600); err != nil {
		return nil, fmt.Errorf("secure sqlite database: %w", err)
	}
	return db, nil
}

func initializeStoreSchema(db *gorm.DB) error {
	if db.Migrator().HasIndex(&TaskAgent{}, "idx_task_agent_name") {
		if err := db.Migrator().DropIndex(&TaskAgent{}, "idx_task_agent_name"); err != nil {
			return fmt.Errorf("remove retired task Agent alias index: %w", err)
		}
	}
	models := []any{
		&configRecord{}, &agentRecord{}, &skillRecord{}, &uncoverProviderRecord{},
		&KnowledgeBase{}, &KnowledgeDocument{}, &Project{}, &ContainerProfile{}, &ContainerInstance{},
		&Task{}, &TaskReport{}, &TaskAudit{}, &TaskAuditEvent{}, &Issue{}, &TaskAgent{}, &ConciergeConversation{},
		&IssueRelation{}, &Execution{}, &IssueValidation{}, &ExecutionEvent{}, &ExecutionProgress{},
		&Message{}, &Approval{}, &IssueComment{}, &IssueAttachment{}, &InputAttachment{}, &AgentWakeup{},
		&IssueDecomposition{}, &IssueChildWait{}, &RelayThread{}, &RelayMessage{}, &RelayReceipt{}, &Finding{},
	}
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("initialize sqlite schema: %w", err)
	}
	return nil
}

func recoverInterruptedTaskAudits(db *gorm.DB, now time.Time) error {
	// Audit streams are process-local. Preserve partial evidence, but never
	// report an interrupted audit as active after a restart.
	if err := db.Model(&TaskAudit{}).Where("status IN ?", []string{"preparing", "running"}).Updates(map[string]any{
		"status": "failed", "error": "服务重启中断了本次审计；请新建审计以生成新的冻结证据快照。",
		"completed_at": now, "updated_at": now,
	}).Error; err != nil {
		return fmt.Errorf("recover interrupted task audits: %w", err)
	}
	return nil
}

func ensureDefaultContainerProfile(db *gorm.DB, now time.Time) error {
	var count int64
	if err := db.Model(&ContainerProfile{}).Count(&count).Error; err != nil {
		return fmt.Errorf("inspect default container profile: %w", err)
	}
	if count > 0 {
		return nil
	}
	profile := ContainerProfile{
		ID: "container-profile-default", Name: "default", Description: "Aegis 默认 Docker 隔离环境",
		Image: WorkerContainerImage, WorkspacePath: TaskWorkspacePath,
		NetworkMode: "bridge", Enabled: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := db.Create(&profile).Error; err != nil {
		return fmt.Errorf("create default container profile: %w", err)
	}
	return nil
}

func (s *Store) loadConfig() error {
	var settings configRecord
	if err := s.db.First(&settings, 1).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	} else if err != nil {
		return err
	}
	s.config = settings.Value
	s.config.ValidationMode, s.config.MaxValidationAttempts = normalizeValidationPolicy(s.config.ValidationMode, s.config.MaxValidationAttempts)
	s.config.MaxIssueDepth, s.config.MaxChildrenPerRequest, s.config.MaxDirectChildren = normalizeDecompositionLimits(s.config.MaxIssueDepth, s.config.MaxChildrenPerRequest, s.config.MaxDirectChildren)
	s.config.IssueBudget = normalizeIssueBudget(s.config.IssueBudget)
	s.config.IssueHeartbeat = normalizeIssueHeartbeat(s.config.IssueHeartbeat)
	return nil
}

func recoverInterruptedRuntime(db *gorm.DB, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
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
		if err := tx.Where("hidden = ? AND status = ? AND execution_phase NOT IN ?", false, "in_progress", []string{"waiting_children", "sleeping", "summarizing", "blocked"}).Find(&interruptedIssues).Error; err != nil {
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
	})
}

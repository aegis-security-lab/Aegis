package control

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func runtimeBindingSnapshot(issue Issue) (string, string) {
	if strings.TrimSpace(issue.ContainerProfileID) != "" {
		return "container", issue.ContainerProfileID
	}
	return "host", ""
}

func (s *Store) ensureIssueAgentSession(issue Issue, agentID, preferredSessionID string) (IssueAgentSession, error) {
	var binding IssueAgentSession
	err := s.db.Transaction(func(tx *gorm.DB) error {
		var err error
		binding, err = ensureIssueAgentSessionOnDB(tx, issue, agentID, preferredSessionID)
		return err
	})
	return binding, err
}

func ensureIssueAgentSessionOnDB(db *gorm.DB, issue Issue, agentID, preferredSessionID string) (IssueAgentSession, error) {
	agentID = strings.TrimSpace(agentID)
	if issue.ID == "" || agentID == "" {
		return IssueAgentSession{}, errors.New("Issue 和 Agent 不能为空")
	}
	var current IssueAgentSession
	err := db.Where("issue_id = ? AND agent_id = ? AND status = ?", issue.ID, agentID, "active").First(&current).Error
	if err == nil {
		runtimeType, containerProfileID := runtimeBindingSnapshot(issue)
		if current.RuntimeType == runtimeType && current.ContainerProfileID == containerProfileID && current.Workspace == issue.Workspace {
			return current, nil
		}
		if err := db.Model(&IssueAgentSession{}).Where("id = ? AND status = ?", current.ID, "active").Updates(map[string]any{
			"status": "superseded", "updated_at": time.Now(),
		}).Error; err != nil {
			return IssueAgentSession{}, err
		}
		preferredSessionID = ""
		err = gorm.ErrRecordNotFound
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return IssueAgentSession{}, err
	}
	sessionID := strings.TrimSpace(preferredSessionID)
	if sessionID == "" {
		sessionID = nextID("pi-session")
	}
	var owner IssueAgentSession
	if err = db.Where("session_id = ?", sessionID).First(&owner).Error; err == nil {
		if owner.IssueID != issue.ID || owner.AgentID != agentID {
			return IssueAgentSession{}, errors.New("Pi Session 已绑定到其他 Issue 或 Agent")
		}
		return owner, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return IssueAgentSession{}, err
	}
	runtimeType, containerProfileID := runtimeBindingSnapshot(issue)
	now := time.Now()
	generation := current.Generation + 1
	if generation < 1 {
		generation = 1
	}
	current = IssueAgentSession{
		ID: nextID("issue-agent-session"), IssueID: issue.ID, AgentID: agentID,
		SessionID: sessionID, Status: "active", Generation: generation,
		RuntimeType: runtimeType, ContainerProfileID: containerProfileID,
		Workspace: issue.Workspace, CreatedAt: now, UpdatedAt: now,
	}
	if err = db.Create(&current).Error; err != nil {
		// A concurrent creator may have won the partial unique-index race.
		if lookupErr := db.Where("issue_id = ? AND agent_id = ? AND status = ?", issue.ID, agentID, "active").First(&current).Error; lookupErr == nil {
			return current, nil
		}
		return IssueAgentSession{}, err
	}
	return current, nil
}

// migrateIssueAgentSessions adopts the most recently started historical
// conversation for every Issue-Agent pair without rewriting audit history.
func migrateIssueAgentSessions(db *gorm.DB) error {
	var executions []Execution
	if err := db.Where("session_id <> ''").Order("started_at asc, id asc").Find(&executions).Error; err != nil {
		return err
	}
	type pairHistory struct {
		issue    Issue
		agentID  string
		sessions []string
	}
	histories := make(map[string]*pairHistory)
	for _, execution := range executions {
		key := execution.IssueID + "\x00" + execution.AgentID
		history := histories[key]
		if history == nil {
			var issue Issue
			if err := db.First(&issue, "id = ?", execution.IssueID).Error; err != nil {
				continue
			}
			history = &pairHistory{issue: issue, agentID: execution.AgentID}
			histories[key] = history
		}
		if len(history.sessions) == 0 || history.sessions[len(history.sessions)-1] != execution.SessionID {
			alreadySeen := false
			for _, sessionID := range history.sessions {
				alreadySeen = alreadySeen || sessionID == execution.SessionID
			}
			if !alreadySeen {
				history.sessions = append(history.sessions, execution.SessionID)
			}
		}
	}
	for _, history := range histories {
		var active IssueAgentSession
		hasActive := db.Where("issue_id = ? AND agent_id = ? AND status = ?", history.issue.ID, history.agentID, "active").First(&active).Error == nil
		activeSessionID := active.SessionID
		if !hasActive {
			activeSessionID = history.sessions[len(history.sessions)-1]
		}
		for index, sessionID := range history.sessions {
			var binding IssueAgentSession
			err := db.Where("session_id = ?", sessionID).First(&binding).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				if sessionID == activeSessionID {
					binding, err = ensureIssueAgentSessionOnDB(db, history.issue, history.agentID, sessionID)
				} else {
					runtimeType, containerProfileID := runtimeBindingSnapshot(history.issue)
					now := time.Now()
					binding = IssueAgentSession{ID: nextID("issue-agent-session"), IssueID: history.issue.ID, AgentID: history.agentID, SessionID: sessionID, Status: "superseded", Generation: index + 1, RuntimeType: runtimeType, ContainerProfileID: containerProfileID, Workspace: history.issue.Workspace, CreatedAt: now, UpdatedAt: now}
					err = db.Create(&binding).Error
				}
			}
			if err != nil || binding.IssueID != history.issue.ID || binding.AgentID != history.agentID {
				continue
			}
			if err := db.Model(&Execution{}).Where("issue_id = ? AND agent_id = ? AND session_id = ?", history.issue.ID, history.agentID, sessionID).Update("issue_agent_session_id", binding.ID).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

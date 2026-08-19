package control

import (
	"errors"
	"strings"
	"time"

	"aegis/agentapp"
	"gorm.io/gorm"
)

func claimTaskAgentTx(tx *gorm.DB, taskID, agentID, agentName, requestedID string) (TaskAgent, error) {
	taskID, agentID, agentName, requestedID = strings.TrimSpace(taskID), strings.TrimSpace(agentID), strings.TrimSpace(agentName), strings.TrimSpace(requestedID)
	if tx == nil || taskID == "" || agentID == "" {
		return TaskAgent{}, errors.New("task Agent requires taskId and agentId")
	}
	if requestedID != "" {
		var existing TaskAgent
		if err := tx.First(&existing, "id = ? AND task_id = ?", requestedID, taskID).Error; err != nil {
			return TaskAgent{}, errors.New("task Agent identity not found in this task")
		}
		if existing.AgentID != agentID {
			return TaskAgent{}, errors.New("task Agent identity belongs to a different Agent type")
		}
		if agentName != "" && existing.Name != agentName {
			existing.Name = agentName
			existing.UpdatedAt = time.Now().UTC()
			if err := tx.Model(&TaskAgent{}).Where("id = ?", existing.ID).Updates(map[string]any{"name": existing.Name, "updated_at": existing.UpdatedAt}).Error; err != nil {
				return TaskAgent{}, err
			}
		}
		return existing, nil
	}
	if agentName == "" {
		agentName = agentID
	}
	now := time.Now().UTC()
	identity := TaskAgent{ID: nextID("task-agent"), TaskID: taskID, AgentID: agentID, Name: agentName, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&identity).Error; err != nil {
		return TaskAgent{}, err
	}
	return identity, nil
}

func (s *Store) syncTaskAgentNames() error {
	for _, agent := range s.agents {
		if err := s.db.Model(&TaskAgent{}).Where("agent_id = ? AND name <> ?", agent.ID, agent.Name).Updates(map[string]any{"name": agent.Name, "updated_at": time.Now().UTC()}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) TaskAgents(taskID string) ([]TaskAgent, error) {
	_, roots, _, err := taskIssueScopeWithDB(s.db, taskID)
	if err != nil {
		return nil, err
	}
	rootIDs := issueIDsOf(roots)
	if len(rootIDs) == 0 {
		return []TaskAgent{}, nil
	}
	var identities []TaskAgent
	if err := s.db.Where("task_id IN ?", rootIDs).Order("created_at asc, id asc").Find(&identities).Error; err != nil {
		return nil, err
	}
	return identities, nil
}

func (s *Store) taskAgent(taskID, identityID string) (TaskAgent, error) {
	var identity TaskAgent
	query := s.db.Where("id = ?", strings.TrimSpace(identityID))
	if strings.TrimSpace(taskID) != "" {
		query = query.Where("task_id = ?", strings.TrimSpace(taskID))
	}
	if err := query.First(&identity).Error; err != nil {
		return TaskAgent{}, err
	}
	return identity, nil
}

func actorRoutingID(actor agentapp.Actor) string {
	if value := strings.TrimSpace(actor.TaskAgentID); value != "" {
		return value
	}
	return strings.TrimSpace(actor.AgentID)
}

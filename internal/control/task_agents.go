package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"aegis/agentapp"
	"gorm.io/gorm"
)

// taskAgentNames are role-neutral. A name may be reused in another task, but
// never by two identities inside the same task.
var taskAgentNames = []string{
	"云舟", "星野", "青岚", "知夏", "临川", "望舒", "景行", "清和", "砚秋", "闻溪",
	"南乔", "北辰", "予安", "时雨", "明澈", "怀瑾", "昭宁", "若谷", "长风", "静川",
	"向晚", "疏影", "映川", "凌霄", "远山", "星河", "白榆", "青禾", "今安", "见微",
	"承泽", "嘉树", "修远", "既白", "清晏", "知衡", "言蹊", "维舟", "明夷", "初晴",
	"千帆", "观澜", "书昀", "叙白", "含章", "怀川", "若水", "知远", "允中", "明序",
	"清越", "和光", "照野", "云起", "闻舟", "星临", "归鸿", "叙川", "景明", "安澜",
	"知行", "清野", "嘉言", "望川", "云程", "明川", "若衡", "承安", "景澄", "书言",
	"时安", "言川", "清许", "远知", "云岫", "星阑", "嘉禾", "明远", "予川", "知临",
	"青屿", "清川", "映舟", "昭野", "临安", "时清", "景舟", "闻野", "云澄", "明舟",
	"予宁", "知澜", "清远", "星川", "景和", "承远", "书澜", "时川", "言舟", "安和",
}

func TaskAgentNameCatalog() []string {
	return append([]string(nil), taskAgentNames...)
}

func claimTaskAgentTx(tx *gorm.DB, taskID, agentID, requestedID string) (TaskAgent, error) {
	taskID, agentID, requestedID = strings.TrimSpace(taskID), strings.TrimSpace(agentID), strings.TrimSpace(requestedID)
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
		return existing, nil
	}
	var used []string
	if err := tx.Model(&TaskAgent{}).Where("task_id = ?", taskID).Pluck("name", &used).Error; err != nil {
		return TaskAgent{}, err
	}
	usedSet := make(map[string]bool, len(used))
	for _, name := range used {
		usedSet[name] = true
	}
	name := ""
	for _, candidate := range taskAgentNames {
		if !usedSet[candidate] {
			name = candidate
			break
		}
	}
	if name == "" {
		return TaskAgent{}, fmt.Errorf("task Agent name pool exhausted: maximum %d identities per task", len(taskAgentNames))
	}
	now := time.Now().UTC()
	identity := TaskAgent{ID: nextID("task-agent"), TaskID: taskID, AgentID: agentID, Name: name, Status: "active", CreatedAt: now, UpdatedAt: now}
	if err := tx.Create(&identity).Error; err != nil {
		return TaskAgent{}, err
	}
	return identity, nil
}

func (s *Store) TaskAgents(taskID string) ([]TaskAgent, error) {
	issue, err := s.GetIssue(strings.TrimSpace(taskID))
	if err != nil {
		return nil, err
	}
	root, err := s.taskRoot(issue)
	if err != nil {
		return nil, err
	}
	var identities []TaskAgent
	if err := s.db.Where("task_id = ?", root.ID).Order("created_at asc, id asc").Find(&identities).Error; err != nil {
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

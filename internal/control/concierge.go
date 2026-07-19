package control

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const conciergeAgentID = "aegis-concierge"

func (s *Store) CreateConciergeConversation() (ConciergeConversation, error) {
	if !s.Config().Configured {
		return ConciergeConversation{}, errors.New("请先完成初始化配置")
	}
	agent, err := s.GetAgent(conciergeAgentID)
	if err != nil || !agent.Enabled {
		return ConciergeConversation{}, errors.New("管家 Agent 当前不可用")
	}
	var project Project
	if err = s.db.Order("created_at asc").First(&project).Error; err != nil {
		return ConciergeConversation{}, err
	}
	now := time.Now()
	conversationID := nextID("conversation")
	issueID := nextID("concierge-issue")
	executionID := nextID("execution")
	cfg := s.effectiveAgentConfig(agent)
	issue := Issue{
		ID: issueID, Number: -now.UnixNano(), Identifier: "CHAT-" + strings.TrimPrefix(conversationID, "conversation-"),
		ProjectID: project.ID, Title: "与管家对话", Status: "done", Priority: "medium", WorkMode: "autonomous",
		ExecutionPhase: "completed", AssigneeAgentID: agent.ID, CurrentExecutionID: executionID,
		ValidationDisabled: true, Workspace: s.Config().Workspace, CreatedBy: "operator", Hidden: true,
		CreatedAt: now, UpdatedAt: now,
	}
	execution := Execution{
		ID: executionID, IssueID: issue.ID, AgentID: agent.ID, Kind: "concierge", Status: "idle",
		Provider: cfg.Provider, Model: cfg.Model, Pricing: cfg.Pricing, Thinking: cfg.Thinking,
		SessionID: nextID("pi-session"), SystemPrompt: agent.SystemPrompt, ToolsSnapshot: snapshotTools(agent.Tools),
		StartedAt: now, UpdatedAt: now,
	}
	conversation := ConciergeConversation{
		ID: conversationID, Title: "新对话", IssueID: issue.ID, ExecutionID: execution.ID,
		Status: "idle", CreatedAt: now, UpdatedAt: now,
	}
	if err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&issue).Error; err != nil {
			return err
		}
		if err := tx.Create(&execution).Error; err != nil {
			return err
		}
		return tx.Create(&conversation).Error
	}); err != nil {
		return ConciergeConversation{}, err
	}
	s.notify()
	return conversation, nil
}

func (s *Store) ListConciergeConversations() ([]ConciergeConversation, error) {
	items := []ConciergeConversation{}
	err := s.db.Order("updated_at desc, id desc").Find(&items).Error
	return items, err
}

func (s *Store) GetConciergeConversation(id string) (ConciergeConversationDetail, error) {
	var conversation ConciergeConversation
	if err := s.db.First(&conversation, "id = ?", id).Error; err != nil {
		return ConciergeConversationDetail{}, errors.New("conversation not found")
	}
	var execution Execution
	if err := s.db.First(&execution, "id = ?", conversation.ExecutionID).Error; err != nil {
		return ConciergeConversationDetail{}, errors.New("conversation execution not found")
	}
	page, err := s.SessionMessagesPage(execution.ID, "", detailPageSize)
	if err != nil {
		return ConciergeConversationDetail{}, err
	}
	return ConciergeConversationDetail{
		Conversation: conversation, Execution: execution, Messages: page.Items,
		MessagesPage: page.Page, Watermark: time.Now(),
	}, nil
}

func (s *Store) deleteConciergeConversation(id string) error {
	var conversation ConciergeConversation
	if err := s.db.First(&conversation, "id = ?", id).Error; err != nil {
		return errors.New("conversation not found")
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, model := range []any{&Approval{}, &ExecutionProgress{}, &ExecutionEvent{}, &Message{}} {
			if err := tx.Where("execution_id = ?", conversation.ExecutionID).Delete(model).Error; err != nil {
				return err
			}
		}
		if err := tx.Delete(&Execution{}, "id = ?", conversation.ExecutionID).Error; err != nil {
			return err
		}
		if err := tx.Delete(&Issue{}, "id = ?", conversation.IssueID).Error; err != nil {
			return err
		}
		return tx.Delete(&conversation).Error
	}); err != nil {
		return err
	}
	s.notify()
	return nil
}

func (s *Store) touchConciergeConversation(executionID, status, lastMessage string) {
	updates := map[string]any{"status": status, "updated_at": time.Now()}
	if strings.TrimSpace(lastMessage) != "" {
		updates["last_message"] = truncate(strings.TrimSpace(lastMessage), 500)
	}
	_ = s.db.Model(&ConciergeConversation{}).Where("execution_id = ?", executionID).Updates(updates).Error
}

func (s *Store) titleConciergeConversation(executionID, message string) {
	message = strings.Join(strings.Fields(strings.TrimSpace(message)), " ")
	if message == "" {
		return
	}
	var conversation ConciergeConversation
	if s.db.First(&conversation, "execution_id = ?", executionID).Error != nil || conversation.MessageCount > 0 {
		return
	}
	runes := []rune(message)
	if len(runes) > 42 {
		message = string(runes[:42]) + "…"
	}
	_ = s.db.Model(&ConciergeConversation{}).Where("id = ?", conversation.ID).Updates(map[string]any{
		"title": message, "updated_at": time.Now(),
	}).Error
}

func (s *Store) incrementConciergeMessages(executionID string) {
	_ = s.db.Model(&ConciergeConversation{}).Where("execution_id = ?", executionID).Updates(map[string]any{
		"message_count": gorm.Expr("message_count + 1"), "updated_at": time.Now(),
	}).Error
}

func (s *Store) recordConciergeTask(executionID string, issue Issue) {
	_ = s.db.Model(&ConciergeConversation{}).Where("execution_id = ?", executionID).Updates(map[string]any{
		"created_task_id": issue.ID, "updated_at": time.Now(),
	}).Error
}

func validateConciergeTaskInput(input CreateConciergeTaskInput) error {
	if strings.TrimSpace(input.Title) == "" {
		return errors.New("任务标题不能为空")
	}
	if utf8.RuneCountInString(input.Title) > IssueTitleMaxLength {
		return fmt.Errorf("任务标题不能超过 %d 个字符", IssueTitleMaxLength)
	}
	if utf8.RuneCountInString(input.Description)+utf8.RuneCountInString(input.Objective) > 50000 {
		return errors.New("任务描述和目标过长")
	}
	return nil
}

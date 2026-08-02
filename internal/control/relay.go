package control

import (
	"errors"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

func relayDirectKey(taskID, first, second string) string {
	ids := []string{strings.TrimSpace(first), strings.TrimSpace(second)}
	sort.Strings(ids)
	return strings.TrimSpace(taskID) + "\x00" + strings.Join(ids, "\x00")
}

func (s *Store) ensureRelayThread(taskID, senderID, recipientID string) (RelayThread, error) {
	key := relayDirectKey(taskID, senderID, recipientID)
	var thread RelayThread
	if err := s.db.Where("direct_key = ?", key).First(&thread).Error; err == nil {
		return thread, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return RelayThread{}, err
	}
	now := time.Now()
	thread = RelayThread{ID: nextID("relay-thread"), TaskID: strings.TrimSpace(taskID), Kind: "direct", Title: "Direct message", DirectKey: key, ParticipantIDs: uniqueStrings([]string{senderID, recipientID}), LastMessageAt: now, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&thread).Error; err != nil {
		if lookupErr := s.db.Where("direct_key = ?", key).First(&thread).Error; lookupErr == nil {
			return thread, nil
		}
		return RelayThread{}, err
	}
	return thread, nil
}

func (s *Store) SendRelayMessage(senderType, senderID string, input SendRelayMessageInput) (RelayMessage, error) {
	input.Body = strings.TrimSpace(input.Body)
	input.RecipientID = strings.TrimSpace(input.RecipientID)
	input.RecipientTaskAgentID = strings.TrimSpace(input.RecipientTaskAgentID)
	if input.Body == "" || (input.RecipientID == "" && input.RecipientTaskAgentID == "") {
		return RelayMessage{}, errors.New("收件人和消息内容不能为空")
	}
	if utf8.RuneCountInString(input.Body) > 10000 {
		return RelayMessage{}, errors.New("消息内容不能超过 10000 个字符")
	}
	taskID := strings.TrimSpace(input.TaskID)
	if taskID == "" && strings.TrimSpace(input.IssueID) != "" {
		if issue, issueErr := s.GetIssue(strings.TrimSpace(input.IssueID)); issueErr == nil {
			if root, rootErr := s.taskRoot(issue); rootErr == nil {
				taskID = root.ID
			}
		}
	}
	recipientRoutingID := input.RecipientTaskAgentID
	if recipientRoutingID != "" {
		var identity TaskAgent
		if err := s.db.First(&identity, "id = ? AND task_id = ?", recipientRoutingID, taskID).Error; err != nil || (input.RecipientID != "" && identity.AgentID != input.RecipientID) {
			return RelayMessage{}, errors.New("收件任务 Agent 不存在或不属于当前任务")
		}
		input.RecipientID = identity.AgentID
	} else {
		recipient, err := s.executionAgent(input.RecipientID)
		if err != nil || !isRunnableAgent(recipient) {
			return RelayMessage{}, errors.New("收件 Agent 不存在或不可用")
		}
		recipientRoutingID = input.RecipientID
	}
	thread, err := s.ensureRelayThread(taskID, senderID, recipientRoutingID)
	if err != nil {
		return RelayMessage{}, err
	}
	now := time.Now()
	var deliveryKey *string
	if key := strings.TrimSpace(input.IdempotencyKey); key != "" {
		deliveryKey = &key
		var existing RelayMessage
		if err := s.db.First(&existing, "delivery_key = ?", key).Error; err == nil {
			return existing, nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return RelayMessage{}, err
		}
	}
	message := RelayMessage{ID: nextID("relay-message"), ThreadID: thread.ID, SenderType: fallback(strings.TrimSpace(senderType), "agent"), SenderID: strings.TrimSpace(senderID), RecipientIDs: []string{recipientRoutingID}, Body: input.Body, IssueID: strings.TrimSpace(input.IssueID), DeliveryKey: deliveryKey, CreatedAt: now}
	err = s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&message).Error; err != nil {
			return err
		}
		receipt := RelayReceipt{ID: nextID("relay-receipt"), MessageID: message.ID, AgentID: recipientRoutingID, CreatedAt: now}
		if err := tx.Create(&receipt).Error; err != nil {
			return err
		}
		return tx.Model(&RelayThread{}).Where("id = ?", thread.ID).Updates(map[string]any{"last_message_at": now, "updated_at": now}).Error
	})
	if err != nil {
		if deliveryKey != nil {
			var existing RelayMessage
			if lookupErr := s.db.First(&existing, "delivery_key = ?", *deliveryKey).Error; lookupErr == nil {
				return existing, nil
			}
		}
		return RelayMessage{}, err
	}
	s.notify()
	return message, nil
}

func (s *Store) RelayInbox(agentID string) ([]RelayThreadSummary, error) {
	return s.relayInbox(agentID, "")
}

func (s *Store) RelayInboxForTask(agentID, taskID string) ([]RelayThreadSummary, error) {
	if strings.TrimSpace(taskID) == "" {
		return nil, errors.New("Task ID 不能为空")
	}
	return s.relayInbox(agentID, strings.TrimSpace(taskID))
}

func (s *Store) relayInbox(agentID, taskID string) ([]RelayThreadSummary, error) {
	var threads []RelayThread
	query := s.db.Order("last_message_at desc")
	if taskID != "" {
		query = query.Where("task_id = ?", taskID)
	}
	if err := query.Find(&threads).Error; err != nil {
		return nil, err
	}
	items := make([]RelayThreadSummary, 0)
	for _, thread := range threads {
		if !containsString(thread.ParticipantIDs, agentID) {
			continue
		}
		item := RelayThreadSummary{Thread: thread}
		var last RelayMessage
		if err := s.db.Where("thread_id = ?", thread.ID).Order("created_at desc, id desc").First(&last).Error; err == nil {
			item.LastMessage = &last
		}
		_ = s.db.Model(&RelayReceipt{}).Where("agent_id = ? AND read_at IS NULL AND message_id IN (?)", agentID, s.db.Model(&RelayMessage{}).Select("id").Where("thread_id = ?", thread.ID)).Count(&item.UnreadCount).Error
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) RelayConversation(agentID, threadID string, markRead bool) (RelayConversation, error) {
	return s.relayConversation(agentID, threadID, "", markRead)
}

func (s *Store) RelayConversationForTask(agentID, threadID, taskID string, markRead bool) (RelayConversation, error) {
	if strings.TrimSpace(taskID) == "" {
		return RelayConversation{}, errors.New("Task ID 不能为空")
	}
	return s.relayConversation(agentID, threadID, strings.TrimSpace(taskID), markRead)
}

func (s *Store) relayConversation(agentID, threadID, taskID string, markRead bool) (RelayConversation, error) {
	var thread RelayThread
	query := s.db.Where("id = ?", threadID)
	if taskID != "" {
		query = query.Where("task_id = ?", taskID)
	}
	if err := query.First(&thread).Error; err != nil || !containsString(thread.ParticipantIDs, agentID) {
		return RelayConversation{}, errors.New("Relay conversation not found")
	}
	var messages []RelayMessage
	if err := s.db.Where("thread_id = ?", thread.ID).Order("created_at asc, id asc").Find(&messages).Error; err != nil {
		return RelayConversation{}, err
	}
	var unread int64
	receiptQuery := s.db.Model(&RelayReceipt{}).Where("agent_id = ? AND read_at IS NULL AND message_id IN (?)", agentID, s.db.Model(&RelayMessage{}).Select("id").Where("thread_id = ?", thread.ID))
	if err := receiptQuery.Count(&unread).Error; err != nil {
		return RelayConversation{}, err
	}
	if markRead && unread > 0 {
		now := time.Now()
		messageIDs := make([]string, 0, len(messages))
		for _, message := range messages {
			messageIDs = append(messageIDs, message.ID)
		}
		if err := s.db.Model(&RelayReceipt{}).Where("agent_id = ? AND read_at IS NULL AND message_id IN ?", agentID, messageIDs).Update("read_at", now).Error; err != nil {
			return RelayConversation{}, err
		}
		s.notify()
	}
	return RelayConversation{Thread: thread, Messages: messages, UnreadCount: unread}, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

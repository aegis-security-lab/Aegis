package control

import (
	"errors"
	"slices"
	"time"

	"gorm.io/gorm"
)

const (
	detailPageSize = 50
	maxPageSize    = 100
	maxDeltaRows   = 500
)

func normalizePageLimit(limit int) int {
	if limit <= 0 {
		return detailPageSize
	}
	if limit > maxPageSize {
		return maxPageSize
	}
	return limit
}

func applyBeforeCursor(query *gorm.DB, model any, scope string, scopeValue any, before string) (*gorm.DB, error) {
	if before == "" {
		return query, nil
	}
	var point struct {
		ID        string
		CreatedAt time.Time
	}
	if err := query.Session(&gorm.Session{}).Model(model).Select("id", "created_at").Where(scope, scopeValue).Where("id = ?", before).Take(&point).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("invalid pagination cursor")
		}
		return nil, err
	}
	return query.Where("created_at < ? OR (created_at = ? AND id < ?)", point.CreatedAt, point.CreatedAt, point.ID), nil
}

func pageInfo(total int64, hasMore bool, cursor string) PageInfo {
	if !hasMore {
		cursor = ""
	}
	return PageInfo{NextCursor: cursor, HasMore: hasMore, Total: total}
}

func reverseComments(items []IssueComment) { slices.Reverse(items) }
func reverseMessages(items []Message)      { slices.Reverse(items) }
func reverseProgress(items []ExecutionProgress) {
	slices.Reverse(items)
}

func (s *Store) IssueCommentsPage(issueID, before string, limit int) (IssueCommentPage, error) {
	issue, err := s.GetIssue(issueID)
	if err != nil {
		return IssueCommentPage{}, err
	}
	limit = normalizePageLimit(limit)
	query := s.db.Model(&IssueComment{}).Where("issue_id = ?", issue.ID)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return IssueCommentPage{}, err
	}
	query, err = applyBeforeCursor(query, &IssueComment{}, "issue_id = ?", issue.ID, before)
	if err != nil {
		return IssueCommentPage{}, err
	}
	var items []IssueComment
	if err = query.Order("created_at desc, id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return IssueCommentPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	reverseComments(items)
	if err = s.attachComments(items); err != nil {
		return IssueCommentPage{}, err
	}
	cursor := ""
	if len(items) > 0 {
		cursor = items[0].ID
	}
	return IssueCommentPage{Items: items, Page: pageInfo(total, hasMore, cursor)}, nil
}

func (s *Store) attachComments(comments []IssueComment) error {
	if len(comments) == 0 {
		return nil
	}
	ids := make([]string, len(comments))
	for index := range comments {
		ids[index] = comments[index].ID
	}
	var attachments []IssueAttachment
	if err := s.db.Where("comment_id IN ?", ids).Order("created_at asc").Find(&attachments).Error; err != nil {
		return err
	}
	byComment := make(map[string][]IssueAttachment)
	for _, attachment := range attachments {
		byComment[attachment.CommentID] = append(byComment[attachment.CommentID], attachment)
	}
	for index := range comments {
		comments[index].Attachments = byComment[comments[index].ID]
		if comments[index].Attachments == nil {
			comments[index].Attachments = []IssueAttachment{}
		}
	}
	return nil
}

func (s *Store) IssueEventsPage(issueID, before string, limit int) (ExecutionEventPage, error) {
	issue, err := s.GetIssue(issueID)
	if err != nil {
		return ExecutionEventPage{}, err
	}
	return s.executionEventsPage("issue_id = ?", issue.ID, before, limit)
}

func (s *Store) SessionEventsPage(sessionID, before string, limit int) (ExecutionEventPage, error) {
	execution, err := s.sessionExecution(sessionID)
	if err != nil {
		return ExecutionEventPage{}, err
	}
	return s.executionEventsPage("execution_id IN ?", s.executionIDsForPiSession(execution), before, limit)
}

func (s *Store) executionEventsPage(scope string, scopeValue any, before string, limit int) (ExecutionEventPage, error) {
	limit = normalizePageLimit(limit)
	query := s.db.Model(&ExecutionEvent{}).Where(scope, scopeValue)
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return ExecutionEventPage{}, err
	}
	var err error
	query, err = applyBeforeCursor(query, &ExecutionEvent{}, scope, scopeValue, before)
	if err != nil {
		return ExecutionEventPage{}, err
	}
	var items []ExecutionEvent
	if err = query.Order("created_at desc, id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return ExecutionEventPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for index := range items {
		items[index] = compactExecutionEvent(items[index])
	}
	cursor := ""
	if len(items) > 0 {
		cursor = items[len(items)-1].ID
	}
	return ExecutionEventPage{Items: items, Page: pageInfo(total, hasMore, cursor)}, nil
}

func (s *Store) IssueExecutionsPage(issueID, before string, limit int) (ExecutionPage, error) {
	issue, err := s.GetIssue(issueID)
	if err != nil {
		return ExecutionPage{}, err
	}
	limit = normalizePageLimit(limit)
	query := s.db.Model(&Execution{}).Where("issue_id = ?", issue.ID)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return ExecutionPage{}, err
	}
	if before != "" {
		var point Execution
		if err = s.db.Select("id", "started_at").Where("issue_id = ? AND id = ?", issue.ID, before).Take(&point).Error; err != nil {
			return ExecutionPage{}, errors.New("invalid pagination cursor")
		}
		query = query.Where("started_at < ? OR (started_at = ? AND id < ?)", point.StartedAt, point.StartedAt, point.ID)
	}
	var items []Execution
	if err = query.Order("started_at desc, id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return ExecutionPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	for index := range items {
		items[index] = compactExecution(items[index])
	}
	cursor := ""
	if len(items) > 0 {
		cursor = items[len(items)-1].ID
	}
	return ExecutionPage{Items: items, Page: pageInfo(total, hasMore, cursor)}, nil
}

func (s *Store) SessionMessagesPage(sessionID, before string, limit int) (MessagePage, error) {
	execution, err := s.sessionExecution(sessionID)
	if err != nil {
		return MessagePage{}, err
	}
	limit = normalizePageLimit(limit)
	executionIDs := s.executionIDsForPiSession(execution)
	query := s.db.Model(&Message{}).Where("execution_id IN ?", executionIDs)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return MessagePage{}, err
	}
	query, err = applyBeforeCursor(query, &Message{}, "execution_id IN ?", executionIDs, before)
	if err != nil {
		return MessagePage{}, err
	}
	var items []Message
	if err = query.Order("created_at desc, id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return MessagePage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	reverseMessages(items)
	cursor := ""
	if len(items) > 0 {
		cursor = items[0].ID
	}
	return MessagePage{Items: items, Page: pageInfo(total, hasMore, cursor)}, nil
}

func (s *Store) SessionProgressPage(sessionID, before string, limit int) (ExecutionProgressPage, error) {
	execution, err := s.sessionExecution(sessionID)
	if err != nil {
		return ExecutionProgressPage{}, err
	}
	limit = normalizePageLimit(limit)
	executionIDs := s.executionIDsForPiSession(execution)
	query := s.db.Model(&ExecutionProgress{}).Where("execution_id IN ?", executionIDs)
	var total int64
	if err = query.Count(&total).Error; err != nil {
		return ExecutionProgressPage{}, err
	}
	query, err = applyBeforeCursor(query, &ExecutionProgress{}, "execution_id IN ?", executionIDs, before)
	if err != nil {
		return ExecutionProgressPage{}, err
	}
	var items []ExecutionProgress
	if err = query.Order("created_at desc, id desc").Limit(limit + 1).Find(&items).Error; err != nil {
		return ExecutionProgressPage{}, err
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	reverseProgress(items)
	cursor := ""
	if len(items) > 0 {
		cursor = items[0].ID
	}
	return ExecutionProgressPage{Items: items, Page: pageInfo(total, hasMore, cursor)}, nil
}

func (s *Store) sessionExecution(id string) (Execution, error) {
	var execution Execution
	if err := s.db.First(&execution, "id = ?", id).Error; err != nil {
		return Execution{}, errors.New("session not found")
	}
	return execution, nil
}

func (s *Store) executionIDsForPiSession(execution Execution) []string {
	var ids []string
	if execution.SessionID != "" {
		s.db.Model(&Execution{}).
			Where("issue_id = ? AND agent_id = ? AND session_id = ?", execution.IssueID, execution.AgentID, execution.SessionID).
			Order("started_at asc, id asc").Pluck("id", &ids)
	}
	if len(ids) == 0 {
		ids = []string{execution.ID}
	}
	return ids
}

func (s *Store) SessionDelta(sessionID string, since time.Time) (SessionDelta, error) {
	execution, err := s.sessionExecution(sessionID)
	if err != nil {
		return SessionDelta{}, err
	}
	watermark := time.Now()
	executionIDs := s.executionIDsForPiSession(execution)
	var messages []Message
	if err = s.db.Where("execution_id IN ? AND updated_at > ? AND updated_at <= ?", executionIDs, since, watermark).
		Order("updated_at asc, id asc").Limit(maxDeltaRows + 1).Find(&messages).Error; err != nil {
		return SessionDelta{}, err
	}
	var events []ExecutionEvent
	if err = s.db.Where("execution_id IN ? AND (updated_at > ? OR created_at > ?) AND created_at <= ?", executionIDs, since, since, watermark).
		Order("updated_at asc, id asc").Limit(maxDeltaRows + 1).Find(&events).Error; err != nil {
		return SessionDelta{}, err
	}
	var progress []ExecutionProgress
	if err = s.db.Where("execution_id IN ? AND created_at > ? AND created_at <= ?", executionIDs, since, watermark).
		Order("created_at asc, id asc").Limit(maxDeltaRows + 1).Find(&progress).Error; err != nil {
		return SessionDelta{}, err
	}
	if len(messages) > maxDeltaRows || len(events) > maxDeltaRows || len(progress) > maxDeltaRows {
		return SessionDelta{}, errors.New("session delta exceeded safe limit; reload the latest page")
	}
	for index := range events {
		events[index] = compactExecutionEvent(events[index])
	}
	return SessionDelta{Execution: execution, Messages: messages, Events: events, ProgressUpdates: progress, Watermark: watermark}, nil
}

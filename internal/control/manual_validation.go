package control

import (
	"errors"
	"strings"
	"time"
)

// ManuallyRejectValidation reopens a passed Issue for the same persistent
// acceptance session. The manual reason is stored on the new validation round
// and injected into the validator prompt as the operator's authoritative
// baseline.
func (m *Manager) ManuallyRejectValidation(issueID, reason string) (Issue, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()

	reason = strings.TrimSpace(reason)
	if reason == "" {
		return Issue{}, errors.New("手动验收不通过必须填写原因")
	}
	if len([]rune(reason)) > 10000 {
		return Issue{}, errors.New("手动验收原因不能超过 10000 个字符")
	}
	issue, err := m.store.GetIssue(issueID)
	if err != nil {
		return Issue{}, err
	}
	if issue.ValidationDisabled || strings.TrimSpace(issue.Objective) == "" {
		return Issue{}, errors.New("当前 Issue 未启用目标验收")
	}
	if issue.Status != "done" && issue.Status != "in_review" {
		return Issue{}, errors.New("只有验收通过并已结束的 Issue 才能手动改为不通过")
	}
	var validation IssueValidation
	if err := m.store.db.Where("issue_id = ? AND status = ?", issue.ID, "passed").Order("attempt desc, completed_at desc").First(&validation).Error; err != nil {
		return Issue{}, errors.New("当前 Issue 没有可修改的验收通过记录")
	}
	var source Execution
	if err := m.store.db.First(&source, "id = ? AND issue_id = ?", validation.SourceExecutionID, issue.ID).Error; err != nil {
		return Issue{}, errors.New("验收来源 Execution 不存在")
	}
	now := time.Now()
	if err := m.store.db.Model(&Issue{}).Where("id = ? AND status IN ?", issue.ID, []string{"done", "in_review"}).Updates(map[string]any{
		"status": "in_progress", "execution_phase": "active", "checkout_execution_id": "", "current_execution_id": source.ID,
		"error": "", "completed_at": nil, "updated_at": now,
	}).Error; err != nil {
		return Issue{}, err
	}
	comment := IssueComment{
		ID: nextID("comment"), IssueID: issue.ID, Type: "manual_validation_override", AuthorType: "operator", AuthorID: "operator",
		Body: "## 手动验收改判为不通过\n\n" + reason, CreatedAt: now,
	}
	if err := m.store.db.Create(&comment).Error; err != nil {
		return Issue{}, err
	}
	if err := m.beginIssueValidationWithContext(issue, source, validation.CandidateResult, reason); err != nil {
		return Issue{}, err
	}
	m.store.addEvent(source.ID, issue.ID, "validation", "操作员手动改判验收不通过", "已记录手动原因，并要求固定验收 Agent 以该原因作为后续验收基准继续核验。")
	m.store.notify()
	return m.store.GetIssue(issue.ID)
}

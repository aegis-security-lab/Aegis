package control

import (
	"fmt"
	"sort"
	"strings"
)

var keyTimelineEventTypes = map[string]string{
	"delegation":   "issue",
	"plan":         "issue",
	"continuation": "execution",
	"rework":       "execution",
	"error":        "result",
	"attachment":   "system",
	"cancellation": "system",
}

func (s *Store) TaskTimeline(id string) (TaskTimeline, error) {
	task, roots, issues, err := taskIssueScopeWithDB(s.db, id)
	if err != nil {
		return TaskTimeline{}, err
	}
	issueByID := make(map[string]Issue)
	for _, issue := range issues {
		issueByID[issue.ID] = issue
	}
	issueIDs := issueIDsOf(issues)

	agentNames := make(map[string]string)
	for _, agent := range s.Agents() {
		agentNames[agent.ID] = agent.Name
	}
	taskAgentNames := make(map[string]string)
	var taskAgents []TaskAgent
	rootIDs := issueIDsOf(roots)
	if len(rootIDs) > 0 {
		if err := s.db.Where("task_id IN ?", rootIDs).Find(&taskAgents).Error; err != nil {
			return TaskTimeline{}, err
		}
	}
	for _, identity := range taskAgents {
		taskAgentNames[identity.ID] = fallback(agentNames[identity.AgentID], identity.Name)
	}
	actor := func(id string) (string, string) {
		id = strings.TrimSpace(id)
		if id == "" || id == "operator" {
			return "operator", "操作员"
		}
		if name := taskAgentNames[id]; name != "" {
			return "agent", name
		}
		if name := agentNames[id]; name != "" {
			return "agent", name
		}
		return "system", id
	}
	issueFields := func(issueID string) (string, string) {
		issue := issueByID[issueID]
		return issue.Identifier, issue.Title
	}
	output := TaskTimeline{Task: task, IssueCount: len(issues), Events: []TaskTimelineEvent{}}
	appendEvent := func(event TaskTimelineEvent) {
		event.IssueIdentifier, event.IssueTitle = issueFields(event.IssueID)
		output.Events = append(output.Events, event)
	}

	for _, issue := range issues {
		actorType, actorName := actor(issue.CreatedBy)
		detail := strings.TrimSpace(issue.Objective)
		if detail == "" {
			detail = strings.TrimSpace(issue.Description)
		}
		appendEvent(TaskTimelineEvent{
			ID: "issue-created:" + issue.ID, Kind: "issue", IssueID: issue.ID,
			ActorType: actorType, ActorID: issue.CreatedBy, ActorName: actorName,
			Title: "创建 Issue " + issue.Identifier, Summary: issue.Title,
			Detail: detail, Status: issue.Status, CreatedAt: issue.CreatedAt,
		})
	}

	if len(issueIDs) == 0 {
		return output, nil
	}

	var executions []Execution
	if err := s.db.Where("issue_id IN ?", issueIDs).Order("started_at asc").Find(&executions).Error; err != nil {
		return TaskTimeline{}, err
	}
	executionByID := make(map[string]Execution, len(executions))
	for _, execution := range executions {
		executionByID[execution.ID] = execution
		actorID := execution.TaskAgentID
		if actorID == "" {
			actorID = execution.AgentID
		}
		actorType, actorName := actor(actorID)
		kind := executionKindLabel(execution.Kind)
		startTitle := actorName + " 开始" + kind
		startStatus := "running"
		if execution.Status == "queued" {
			startTitle = actorName + " 的" + kind + "进入队列"
			startStatus = "queued"
		}
		appendEvent(TaskTimelineEvent{
			ID: "execution-started:" + execution.ID, Kind: "execution",
			IssueID: execution.IssueID, ExecutionID: execution.ID,
			ActorType: actorType, ActorID: actorID, ActorName: actorName,
			Title: startTitle, Summary: issueByID[execution.IssueID].Title,
			Detail: strings.TrimSpace(strings.Join([]string{execution.Provider + "/" + execution.Model, "Session " + execution.SessionID}, " · ")),
			Status: startStatus, CreatedAt: execution.StartedAt,
		})
		if execution.FinishedAt == nil || execution.Kind == "validation" {
			continue
		}
		result := strings.TrimSpace(execution.Result)
		if result == "" {
			result = strings.TrimSpace(execution.Error)
		}
		title := actorName + " 完成" + kind
		if execution.Status == "failed" {
			title = actorName + " 执行失败"
		} else if execution.Status == "cancelled" {
			title = actorName + " 的执行已取消"
		}
		appendEvent(TaskTimelineEvent{
			ID: "execution-result:" + execution.ID, Kind: "result",
			IssueID: execution.IssueID, ExecutionID: execution.ID,
			ActorType: actorType, ActorID: actorID, ActorName: actorName,
			Title: title, Summary: truncate(result, 220), Detail: result,
			Status: execution.Status, CreatedAt: *execution.FinishedAt,
		})
	}

	var comments []IssueComment
	if err := s.db.Where("issue_id IN ?", issueIDs).Order("created_at asc").Find(&comments).Error; err != nil {
		return TaskTimeline{}, err
	}
	for _, comment := range comments {
		actorType, actorName := actor(comment.AuthorID)
		if comment.AuthorType != "" {
			actorType = comment.AuthorType
		}
		appendEvent(TaskTimelineEvent{
			ID: "comment:" + comment.ID, Kind: "comment", IssueID: comment.IssueID,
			ActorType: actorType, ActorID: comment.AuthorID, ActorName: actorName,
			Title: actorName + " 发起评论", Summary: truncate(strings.TrimSpace(comment.Body), 220),
			Detail: strings.TrimSpace(comment.Body), CreatedAt: comment.CreatedAt,
		})
	}

	var validations []IssueValidation
	if err := s.db.Where("issue_id IN ?", issueIDs).Order("created_at asc").Find(&validations).Error; err != nil {
		return TaskTimeline{}, err
	}
	for _, validation := range validations {
		appendEvent(TaskTimelineEvent{
			ID: "validation-started:" + validation.ID, Kind: "validation",
			IssueID: validation.IssueID, ExecutionID: validation.ValidationExecutionID,
			ActorType: "agent", ActorID: "acceptance-validator", ActorName: "验收 Agent",
			Title:   fmt.Sprintf("第 %d 次目标验收开始", validation.Attempt),
			Summary: truncate(validation.Objective, 220), Status: "running", CreatedAt: validation.CreatedAt,
		})
		if validation.CompletedAt == nil {
			continue
		}
		title := fmt.Sprintf("第 %d 次目标验收未通过", validation.Attempt)
		if validation.Passed {
			title = fmt.Sprintf("第 %d 次目标验收通过", validation.Attempt)
		} else if validation.Status == "abandoned" {
			title = fmt.Sprintf("第 %d 次验收放弃目标", validation.Attempt)
		} else if validation.Status == "error" {
			title = fmt.Sprintf("第 %d 次目标验收异常", validation.Attempt)
		}
		detail := strings.TrimSpace(validation.Feedback)
		if validation.Status == "abandoned" {
			detail = strings.TrimSpace(validation.AbandonmentProof)
		}
		if detail == "" {
			detail = strings.TrimSpace(validation.Error)
		}
		appendEvent(TaskTimelineEvent{
			ID: "validation-result:" + validation.ID, Kind: "validation",
			IssueID: validation.IssueID, ExecutionID: validation.ValidationExecutionID,
			ActorType: "agent", ActorID: "acceptance-validator", ActorName: "验收 Agent",
			Title: title, Summary: validation.Summary, Detail: detail,
			Status: validation.Status, CreatedAt: *validation.CompletedAt,
		})
	}

	var approvals []Approval
	if err := s.db.Where("issue_id IN ?", issueIDs).Order("created_at asc").Find(&approvals).Error; err != nil {
		return TaskTimeline{}, err
	}
	for _, approval := range approvals {
		execution := executionByID[approval.ExecutionID]
		actorID := execution.TaskAgentID
		if actorID == "" {
			actorID = execution.AgentID
		}
		actorType, actorName := actor(actorID)
		appendEvent(TaskTimelineEvent{
			ID: "approval-created:" + approval.ID, Kind: "approval",
			IssueID: approval.IssueID, ExecutionID: approval.ExecutionID,
			ActorType: actorType, ActorID: actorID, ActorName: actorName,
			Title: actorName + " 发起审批", Summary: approval.Title,
			Detail: approval.Detail, Status: approval.Status, CreatedAt: approval.CreatedAt,
		})
		if approval.ResolvedAt != nil {
			appendEvent(TaskTimelineEvent{
				ID: "approval-resolved:" + approval.ID, Kind: "approval",
				IssueID: approval.IssueID, ExecutionID: approval.ExecutionID,
				ActorType: "operator", ActorID: "operator", ActorName: "操作员",
				Title: "审批已处理", Summary: approval.Title,
				Status: approval.Status, CreatedAt: *approval.ResolvedAt,
			})
		}
	}

	var events []ExecutionEvent
	if err := s.db.Where("issue_id IN ?", issueIDs).Order("created_at asc").Find(&events).Error; err != nil {
		return TaskTimeline{}, err
	}
	for _, event := range events {
		kind, included := keyTimelineEventTypes[event.Type]
		if !included {
			continue
		}
		execution := executionByID[event.ExecutionID]
		actorID := execution.TaskAgentID
		if actorID == "" {
			actorID = execution.AgentID
		}
		actorType, actorName := actor(actorID)
		if execution.ID == "" {
			actorType, actorName = "system", "Aegis"
		}
		appendEvent(TaskTimelineEvent{
			ID: "event:" + event.ID, Kind: kind, IssueID: event.IssueID,
			ExecutionID: event.ExecutionID, ActorType: actorType,
			ActorID: actorID, ActorName: actorName,
			Title: event.Title, Summary: truncate(strings.TrimSpace(event.Detail), 220),
			Detail: strings.TrimSpace(event.Detail), Status: event.Status, CreatedAt: event.CreatedAt,
		})
	}

	sort.SliceStable(output.Events, func(i, j int) bool {
		if output.Events[i].CreatedAt.Equal(output.Events[j].CreatedAt) {
			return output.Events[i].ID > output.Events[j].ID
		}
		return output.Events[i].CreatedAt.After(output.Events[j].CreatedAt)
	})
	return output, nil
}

func executionKindLabel(kind string) string {
	switch kind {
	case "planning":
		return "规划任务"
	case "continuation":
		return "汇总父 Issue"
	case "validation":
		return "目标验收"
	case "rework":
		return "验收返工"
	case "wakeup":
		return "处理评论"
	default:
		return "执行 Issue"
	}
}

package control

import "time"

func (s *Store) Sessions() []SessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var e []Execution
	var issues []Issue
	s.db.Order("started_at desc").Find(&e)
	s.db.Find(&issues)
	for index := range e {
		e[index] = compactExecution(e[index])
	}
	for index := range issues {
		issues[index] = compactIssueForState(issues[index])
	}
	return s.sessionSummariesLocked(e, issues)
}
func (s *Store) sessionSummariesLocked(executions []Execution, issues []Issue) []SessionSummary {
	issueMap := map[string]Issue{}
	for _, i := range issues {
		issueMap[i.ID] = i
	}
	agentMap := map[string]string{}
	for _, a := range s.agents {
		agentMap[a.ID] = a.Name
	}
	out := make([]SessionSummary, 0, len(executions))
	seenSessions := make(map[string]bool, len(executions))
	for _, e := range executions {
		if e.SessionID != "" && seenSessions[e.SessionID] {
			continue
		}
		seenSessions[e.SessionID] = e.SessionID != ""
		i := issueMap[e.IssueID]
		out = append(out, SessionSummary{Execution: e, IssueIdentifier: i.Identifier, IssueTitle: i.Title, AgentName: agentMap[e.AgentID]})
	}
	return out
}
func (s *Store) GetSession(id string) (SessionDetail, error) {
	e, err := s.sessionExecution(id)
	if err != nil {
		return SessionDetail{}, err
	}
	issue, _ := s.GetIssue(e.IssueID)
	agentName := e.AgentID
	if a, err := s.GetAgent(e.AgentID); err == nil {
		agentName = a.Name
	}
	d := SessionDetail{Session: SessionSummary{Execution: e, IssueIdentifier: issue.Identifier, IssueTitle: issue.Title, AgentName: agentName}, Messages: []Message{}, Events: []ExecutionEvent{}, ProgressUpdates: []ExecutionProgress{}, Approvals: []Approval{}, Watermark: time.Now()}
	messages, err := s.SessionMessagesPage(e.ID, "", detailPageSize)
	if err != nil {
		return SessionDetail{}, err
	}
	d.Messages, d.MessagesPage = messages.Items, messages.Page
	events, err := s.SessionEventsPage(e.ID, "", detailPageSize)
	if err != nil {
		return SessionDetail{}, err
	}
	d.Events, d.EventsPage = events.Items, events.Page
	progress, err := s.SessionProgressPage(e.ID, "", detailPageSize)
	if err != nil {
		return SessionDetail{}, err
	}
	d.ProgressUpdates, d.ProgressPage = progress.Items, progress.Page
	s.db.Where("execution_id IN ?", s.executionIDsForPiSession(e)).Order("created_at desc").Find(&d.Approvals)
	return d, nil
}

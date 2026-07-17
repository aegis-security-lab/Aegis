package control

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

//go:embed aegis-guard.ts
var guardExtension []byte

type Manager struct {
	store      *Store
	knowledge  *KnowledgeRetrievalService
	guardPath  string
	controlURL string
	mu         sync.RWMutex
	scheduleMu sync.Mutex
	sessions   map[string]*PiSession
}
type PiSession struct {
	manager                                                          *Manager
	key, executionID, issueID, agentID, kind, wakeupID, controlToken string
	cmd                                                              *exec.Cmd
	stdin                                                            io.WriteCloser
	writeMu                                                          sync.Mutex
	currentMessageID                                                 string
	busy                                                             atomic.Bool
	closed                                                           atomic.Bool
}
type planPayload struct {
	Issues []struct {
		Title              string `json:"title"`
		Description        string `json:"description"`
		AcceptanceCriteria string `json:"acceptanceCriteria"`
		Priority           string `json:"priority"`
		DependsOn          []int  `json:"dependsOn"`
		AgentID            string `json:"agentId"`
	} `json:"issues"`
}

func NewManager(store *Store) (*Manager, error) {
	dir := filepath.Join(store.DataDir(), "runtime")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	path, err := filepath.Abs(filepath.Join(dir, "aegis-guard.ts"))
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, guardExtension, 0o600); err != nil {
		return nil, err
	}
	port := fallback(strings.TrimSpace(os.Getenv("PORT")), "8080")
	controlURL := fallback(strings.TrimSpace(os.Getenv("AEGIS_CONTROL_URL")), "http://127.0.0.1:"+port)
	m := &Manager{store: store, guardPath: path, controlURL: strings.TrimRight(controlURL, "/"), sessions: map[string]*PiSession{}}
	m.knowledge = NewKnowledgeRetrievalService(store, NewKeywordAIRetriever(NewPiKnowledgeRanker(store, path)))
	if store.Config().Configured {
		go m.resumeWork()
	}
	return m, nil
}
func (m *Manager) Close() {
	m.mu.RLock()
	items := make([]*PiSession, 0, len(m.sessions))
	for _, s := range m.sessions {
		items = append(items, s)
	}
	m.mu.RUnlock()
	for _, s := range items {
		s.Close()
	}
}

func DetectRuntime(nodePath, piPath string) RuntimeProbe {
	p := RuntimeProbe{}
	if nodePath == "" {
		nodePath, _ = exec.LookPath("node")
	}
	if piPath == "" {
		if home, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(home, "Code", "pi", "packages", "coding-agent", "dist", "cli.js")
			if _, err := os.Stat(candidate); err == nil {
				piPath = candidate
			}
			if _, err := os.Stat(filepath.Join(home, ".pi", "agent", "auth.json")); err == nil {
				p.AuthFound = true
			}
		}
	}
	p.NodePath = nodePath
	p.PiPath = piPath
	if nodePath == "" {
		p.Error = "未找到 Node.js"
		return p
	}
	if piPath == "" {
		p.Error = "未找到 Pi CLI"
		return p
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if out, err := exec.CommandContext(ctx, nodePath, "--version").CombinedOutput(); err == nil {
		p.NodeVersion = strings.TrimSpace(string(out))
	} else {
		p.Error = err.Error()
		return p
	}
	cmd, args := piCommand(Config{NodePath: nodePath, PiPath: piPath}, "--version")
	if out, err := exec.CommandContext(ctx, cmd, args...).CombinedOutput(); err == nil {
		p.PiVersion = strings.TrimSpace(string(out))
	} else {
		p.Error = string(out) + err.Error()
		return p
	}
	p.Ready = true
	return p
}

func (m *Manager) CreateIssue(input CreateIssueInput) (Issue, error) {
	issue, err := m.store.CreateIssue(input)
	if err != nil {
		return Issue{}, err
	}
	if issue.Status == "todo" {
		go func() { _ = m.DispatchIssue(issue.ID) }()
	}
	return issue, nil
}
func (m *Manager) DispatchIssue(id string) error {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	return m.dispatchIssue(id)
}

func (m *Manager) dispatchIssue(id string) error {
	issue, err := m.store.GetIssue(id)
	if err != nil {
		return err
	}
	if m.store.belongsToCancelledTask(issue) {
		return errors.New("所属任务已取消，不能继续调度")
	}
	if !slices.Contains([]string{"todo", "backlog"}, issue.Status) {
		return errors.New("Issue 当前不可调度")
	}
	agent, err := m.store.chooseAgent(issue)
	if err != nil {
		return err
	}
	kind := "work"
	if agent.Category == "orchestrator" {
		kind = "planning"
	}
	execution, err := m.store.createExecution(issue, agent.ID, kind)
	if err != nil {
		return err
	}
	if _, err = m.store.CheckoutIssue(issue.ID, CheckoutIssueInput{AgentID: agent.ID, ExecutionID: execution.ID, ExpectedStatuses: []string{"todo", "backlog"}}); err != nil {
		_ = m.store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": err.Error(), "finished_at": time.Now()})
		return err
	}
	prompt := workerPrompt(issue)
	if kind == "planning" {
		prompt = planningPrompt(issue, m.store.Agents())
	}
	_, err = m.startSession(issue, execution, agent, prompt)
	if err != nil {
		m.failExecution(issue, execution, err)
		return err
	}
	return nil
}

func (m *Manager) startSession(issue Issue, e Execution, agent AgentDefinition, prompt string) (*PiSession, error) {
	cfg := m.store.effectiveAgentConfig(agent)
	if !cfg.Configured {
		return nil, errors.New("Aegis 尚未配置")
	}
	knowledgeBases, err := m.store.knowledgeBasesByIDs(agent.KnowledgeBaseIDs)
	if err != nil {
		return nil, err
	}
	if len(knowledgeBases) != len(uniqueStrings(agent.KnowledgeBaseIDs)) {
		return nil, errors.New("Agent 关联的知识库不存在")
	}
	systemPrompt := agentKnowledgeSystemPrompt(agent.SystemPrompt, knowledgeBases)
	args := []string{"--mode", "rpc", "--provider", cfg.Provider, "--model", cfg.Model, "--thinking", cfg.Thinking, "--session-dir", filepath.Join(m.store.DataDir(), "sessions"), "--session-id", e.SessionID, "--name", issue.Identifier + " · " + agent.Name, "--no-extensions", "--extension", m.guardPath, "--no-approve", "--no-skills", "--system-prompt", systemPrompt}
	for _, p := range m.store.skillPaths(agent.SkillIDs) {
		args = append(args, "--skill", p)
	}
	activeTools := uniqueStrings(agent.Tools)
	if len(knowledgeBases) > 0 {
		activeTools = uniqueStrings(append(activeTools, "aegis_search_knowledge"))
	}
	if len(activeTools) == 0 {
		args = append(args, "--no-tools")
	} else {
		args = append(args, "--tools", strings.Join(activeTools, ","))
	}
	command, commandArgs := piCommand(cfg, args...)
	cmd := exec.Command(command, commandArgs...)
	cmd.Dir = issue.Workspace
	approval := cfg.ApprovalMode
	if agent.Permissions.ApprovalMode != "" {
		approval = agent.Permissions.ApprovalMode
	}
	if issue.WorkMode == "guided" && e.Kind == "work" {
		approval = "all"
	}
	controlToken, err := randomControlToken()
	if err != nil {
		return nil, err
	}
	cmd.Env = append(runtimeEnv(cfg, approval, agent.Permissions, issue.Workspace),
		"AEGIS_CONTROL_URL="+m.controlURL,
		"AEGIS_EXECUTION_ID="+e.ID,
		"AEGIS_CONTROL_TOKEN="+controlToken,
	)
	if len(knowledgeBases) > 0 {
		encodedKnowledgeBaseIDs, _ := json.Marshal(agent.KnowledgeBaseIDs)
		cmd.Env = append(cmd.Env, "AEGIS_KNOWLEDGE_BASE_IDS="+string(encodedKnowledgeBaseIDs))
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(m.store.DataDir(), "sessions"), 0o700); err != nil {
		return nil, err
	}
	if err := m.store.updateExecution(e.ID, map[string]any{
		"initial_prompt": prompt,
		"system_prompt":  systemPrompt,
		"tools_snapshot": snapshotTools(activeTools),
	}); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &PiSession{manager: m, key: e.ID, executionID: e.ID, issueID: issue.ID, agentID: agent.ID, kind: e.Kind, controlToken: controlToken, cmd: cmd, stdin: stdin}
	m.mu.Lock()
	m.sessions[s.key] = s
	m.mu.Unlock()
	_ = m.store.updateExecution(e.ID, map[string]any{"status": "starting", "pid": cmd.Process.Pid})
	m.store.addEvent(e.ID, issue.ID, "runtime", agent.Name+" 已连接 Pi", "RPC process PID "+strconv.Itoa(cmd.Process.Pid))
	go s.readLoop(stdout)
	go s.stderrLoop(stderr)
	go func() { err := cmd.Wait(); m.sessionExited(s, err) }()
	now := time.Now()
	userMessage := Message{ID: nextID("message"), ExecutionID: e.ID, IssueID: issue.ID, Role: "user", Content: prompt, CreatedAt: now, UpdatedAt: now}
	if err := m.store.db.Create(&userMessage).Error; err != nil {
		s.Close()
		return nil, err
	}
	_ = m.store.db.Model(&Execution{}).Where("id = ?", e.ID).UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
	m.store.notify()
	if err := s.Send(map[string]any{"id": nextID("rpc"), "type": "prompt", "message": prompt}); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *PiSession) Send(v map[string]any) error {
	if s.closed.Load() {
		return errors.New("Pi session is closed")
	}
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, err = s.stdin.Write(append(data, '\n'))
	return err
}
func (s *PiSession) Close() {
	if !s.closed.CompareAndSwap(false, true) {
		return
	}
	_ = s.stdin.Close()
	if s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
}
func (s *PiSession) readLoop(r io.Reader) {
	reader := bufio.NewReaderSize(r, 64*1024)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			s.manager.handleRPCLine(s, line)
		}
		if err != nil {
			return
		}
	}
}
func (s *PiSession) stderrLoop(r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	for scanner.Scan() {
		v := strings.TrimSpace(scanner.Text())
		if v != "" {
			s.manager.store.addEvent(s.executionID, s.issueID, "stderr", "Pi runtime", truncate(v, 1200))
		}
	}
}

func (m *Manager) handleRPCLine(s *PiSession, line []byte) {
	if s.closed.Load() {
		return
	}
	var event map[string]any
	if err := json.Unmarshal(line, &event); err != nil {
		m.store.addEvent(s.executionID, s.issueID, "error", "无法解析 Pi RPC 事件", truncate(string(line), 1000))
		return
	}
	switch stringValue(event["type"]) {
	case "agent_start":
		s.busy.Store(true)
		s.currentMessageID = ""
		_ = m.store.updateExecution(s.executionID, map[string]any{"status": "running", "current_tool": ""})
		m.store.notify()
	case "message_update":
		if a, ok := event["assistantMessageEvent"].(map[string]any); ok && a["type"] == "text_delta" {
			m.appendAssistantDelta(s, stringValue(a["delta"]))
		}
	case "tool_execution_start":
		tool := stringValue(event["toolName"])
		_ = m.store.updateExecution(s.executionID, map[string]any{"status": "running", "current_tool": tool})
		m.store.startToolEvent(s.executionID, s.issueID, stringValue(event["toolCallId"]), tool, event["args"])
	case "tool_execution_end":
		tool := stringValue(event["toolName"])
		_ = m.store.updateExecution(s.executionID, map[string]any{"current_tool": ""})
		isError, _ := event["isError"].(bool)
		m.store.finishToolEvent(s.executionID, s.issueID, stringValue(event["toolCallId"]), tool, event["result"], isError)
	case "extension_ui_request":
		m.handleApproval(s, event)
	case "agent_settled":
		s.busy.Store(false)
		if s.currentMessageID != "" {
			_ = m.store.db.Model(&Message{}).Where("id = ?", s.currentMessageID).Updates(map[string]any{"streaming": false, "updated_at": time.Now()}).Error
		}
		_ = s.Send(map[string]any{"id": nextID("stats"), "type": "get_session_stats"})
		m.handleSettled(s)
	case "response":
		if event["command"] == "get_session_stats" && event["success"] == true {
			m.updateStats(s, event["data"])
		}
		if event["success"] == false {
			m.store.addEvent(s.executionID, s.issueID, "error", "Pi RPC 请求失败", stringValue(event["error"]))
		}
	case "extension_error":
		m.store.addEvent(s.executionID, s.issueID, "error", "Pi extension 错误", compactJSON(event, 1400))
	}
}
func (m *Manager) appendAssistantDelta(s *PiSession, delta string) {
	if delta == "" {
		return
	}
	if s.currentMessageID == "" {
		s.currentMessageID = nextID("message")
		_ = m.store.db.Create(&Message{ID: s.currentMessageID, ExecutionID: s.executionID, IssueID: s.issueID, Role: "assistant", Streaming: true, CreatedAt: time.Now(), UpdatedAt: time.Now()}).Error
		_ = m.store.db.Model(&Execution{}).Where("id = ?", s.executionID).UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
	}
	_ = m.store.db.Model(&Message{}).Where("id = ?", s.currentMessageID).Updates(map[string]any{"content": gorm.Expr("content || ?", delta), "updated_at": time.Now()}).Error
	m.store.notify()
}

func (m *Manager) handleApproval(s *PiSession, event map[string]any) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	if stringValue(event["method"]) != "confirm" {
		_ = s.Send(map[string]any{"type": "extension_ui_response", "id": stringValue(event["id"]), "cancelled": true})
		return
	}
	var execution Execution
	issue, issueErr := m.store.GetIssue(s.issueID)
	if m.store.db.First(&execution, "id = ?", s.executionID).Error != nil || issueErr != nil || execution.Status == "cancelled" || issue.Status == "cancelled" {
		_ = s.Send(map[string]any{"type": "extension_ui_response", "id": stringValue(event["id"]), "cancelled": true})
		return
	}
	a := Approval{ID: nextID("approval"), ExecutionID: s.executionID, IssueID: s.issueID, RequestID: stringValue(event["id"]), Title: stringValue(event["title"]), Detail: stringValue(event["message"]), Status: "pending", CreatedAt: time.Now()}
	_ = m.store.db.Create(&a).Error
	_ = m.store.updateExecution(s.executionID, map[string]any{"status": "waiting_approval"})
	m.store.addEvent(s.executionID, s.issueID, "approval", "等待工具审批", a.Title)
}
func (m *Manager) ResolveApproval(id string, approved bool) (Approval, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	var a Approval
	if err := m.store.db.First(&a, "id = ? AND status = ?", id, "pending").Error; err != nil {
		return Approval{}, errors.New("pending approval not found")
	}
	s := m.getSession(a.ExecutionID)
	if s == nil {
		return Approval{}, errors.New("对应 Pi session 已断开")
	}
	if err := s.Send(map[string]any{"type": "extension_ui_response", "id": a.RequestID, "confirmed": approved}); err != nil {
		return Approval{}, err
	}
	status := "rejected"
	if approved {
		status = "approved"
	}
	now := time.Now()
	_ = m.store.db.Model(&a).Updates(map[string]any{"status": status, "resolved_at": now}).Error
	_ = m.store.updateExecution(a.ExecutionID, map[string]any{"status": "running"})
	m.store.notify()
	a.Status = status
	a.ResolvedAt = &now
	return a, nil
}

func (m *Manager) handleSettled(s *PiSession) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	issue, err := m.store.GetIssue(s.issueID)
	if err != nil {
		return
	}
	var execution Execution
	if m.store.db.First(&execution, "id = ?", s.executionID).Error != nil || issue.Status == "cancelled" || execution.Status == "cancelled" {
		return
	}
	result := m.latestAssistant(s.executionID)
	now := time.Now()
	if s.kind == "planning" {
		m.handlePlan(issue, s, result)
		return
	}
	_ = m.store.updateExecution(s.executionID, map[string]any{"status": "completed", "result": result, "current_tool": "", "finished_at": now, "pid": 0})
	m.collectExecutionAttachments(issue, s.executionID)
	if s.kind == "mention" {
		m.completeWakeup(s, result)
		return
	}
	issue, _ = m.store.GetIssue(s.issueID)
	if issue.ExecutionPhase == "waiting_children" {
		m.addAgentComment(issue.ID, s.agentID, fallback(result, "已拆分子 Issues，等待调度器完成子树。"), s.executionID)
		m.store.addEvent(s.executionID, issue.ID, "delegation", "父 Issue 正在等待子树", "当前 Execution 已结束；所有直属子 Issues 完成后将创建 continuation Execution。")
		m.store.notify()
		go m.scheduleChildren(issue.ID)
		return
	}
	status := "done"
	if issue.WorkMode == "guided" {
		status = "in_review"
	}
	_ = m.store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": status, "execution_phase": "completed", "result": result, "checkout_execution_id": "", "current_execution_id": s.executionID, "completed_at": func() any {
		if status == "done" {
			return now
		}
		return nil
	}(), "updated_at": now}).Error
	m.addAgentComment(issue.ID, s.agentID, result, s.executionID)
	m.store.addEvent(s.executionID, issue.ID, "worker", "Issue 执行完成", "结果已写入 Issue")
	m.store.notify()
	if issue.ParentID != "" {
		go m.scheduleChildren(issue.ParentID)
	}
}

func (m *Manager) handlePlan(parent Issue, s *PiSession, text string) {
	plan, err := parsePlan(text)
	if err != nil {
		m.failExecution(parent, Execution{ID: s.executionID}, fmt.Errorf("执行计划无法解析: %w", err))
		return
	}
	input := DecomposeIssueInput{RequestKey: "initial-plan:" + s.executionID, Summary: "Orchestrator initial plan", Children: make([]SubIssueSpec, len(plan.Issues))}
	for index, item := range plan.Issues {
		input.Children[index] = SubIssueSpec{Title: item.Title, Description: item.Description, AcceptanceCriteria: item.AcceptanceCriteria, Priority: item.Priority, AgentID: item.AgentID, DependsOn: item.DependsOn}
	}
	result, err := m.store.CreateSubIssues(parent.ID, s.executionID, s.agentID, input)
	if err != nil {
		m.failExecution(parent, Execution{ID: s.executionID}, err)
		return
	}
	now := time.Now()
	_ = m.store.updateExecution(s.executionID, map[string]any{"status": "completed", "result": text, "finished_at": now, "pid": 0})
	m.store.addEvent(s.executionID, parent.ID, "plan", "执行计划已生成", fmt.Sprintf("已创建 %d 个子 Issues", len(result.Children)))
	m.store.notify()
	go m.scheduleChildren(parent.ID)
}

func (m *Manager) scheduleChildren(parentID string) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	for {
		parent, err := m.store.GetIssue(parentID)
		if err != nil {
			return
		}
		var children []Issue
		m.store.db.Where("parent_id = ?", parent.ID).Order("number asc").Find(&children)
		if len(children) == 0 {
			return
		}
		allTerminal := true
		for _, c := range children {
			if !slices.Contains([]string{"done", "cancelled"}, c.Status) {
				allTerminal = false
				break
			}
		}
		if allTerminal {
			m.resumeParent(parent, children)
			return
		}
		var active int64
		m.store.db.Model(&Execution{}).Where("status IN ? AND issue_id IN (?)", []string{"queued", "starting", "running", "waiting_approval"}, m.store.db.Model(&Issue{}).Select("id").Where("parent_id = ?", parent.ID)).Count(&active)
		limit := m.store.Config().Concurrency
		if int(active) >= limit {
			return
		}
		var ready *Issue
		for i := range children {
			if children[i].Status != "todo" && children[i].Status != "backlog" {
				continue
			}
			n, _ := m.store.unresolvedBlockers(children[i].ID)
			if n == 0 {
				ready = &children[i]
				break
			}
		}
		if ready == nil {
			return
		}
		if err := m.dispatchIssue(ready.ID); err != nil {
			_ = m.store.db.Model(&Issue{}).Where("id = ?", ready.ID).Updates(map[string]any{"status": "blocked", "error": err.Error(), "updated_at": time.Now()}).Error
			m.store.notify()
			continue
		}
	}
}

func (m *Manager) resumeParent(parent Issue, children []Issue) {
	if parent.ExecutionPhase != "waiting_children" || parent.CheckoutExecutionID != "" {
		return
	}
	agent, err := m.store.chooseAgent(parent)
	if err != nil {
		m.blockIssue(parent, "父 Issue 无法恢复", err)
		return
	}
	execution, err := m.store.createExecution(parent, agent.ID, "continuation")
	if err != nil {
		m.blockIssue(parent, "无法创建 continuation Execution", err)
		return
	}
	if _, err = m.store.CheckoutIssue(parent.ID, CheckoutIssueInput{AgentID: agent.ID, ExecutionID: execution.ID, ExpectedStatuses: []string{"in_progress"}}); err != nil {
		_ = m.store.updateExecution(execution.ID, map[string]any{"status": "failed", "error": err.Error(), "finished_at": time.Now()})
		return
	}
	_ = m.store.db.Model(&Issue{}).Where("id = ?", parent.ID).Updates(map[string]any{"execution_phase": "resuming", "updated_at": time.Now()}).Error
	if _, err = m.startSession(parent, execution, agent, continuationPrompt(parent, children)); err != nil {
		m.failExecution(parent, execution, err)
		return
	}
	m.store.addEvent(execution.ID, parent.ID, "continuation", "子树已完成，恢复父 Issue", fmt.Sprintf("正在汇总 %d 个直属子 Issues 的结果。", len(children)))
}

// DecomposeExecution is called only by the authenticated Pi extension tool.
func (m *Manager) DecomposeExecution(executionID, token string, input DecomposeIssueInput) (DecompositionResult, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	session := m.getSession(executionID)
	if session == nil || token == "" || len(token) != len(session.controlToken) || subtle.ConstantTimeCompare([]byte(token), []byte(session.controlToken)) != 1 {
		return DecompositionResult{}, errors.New("invalid execution control token")
	}
	result, err := m.store.CreateSubIssues(session.issueID, session.executionID, session.agentID, input)
	if err != nil {
		return DecompositionResult{}, err
	}
	m.store.addEvent(session.executionID, session.issueID, "delegation", "Agent 已拆分子 Issues", fmt.Sprintf("%d 个子 Issues 已进入调度器。", len(result.Children)))
	go m.scheduleChildren(session.issueID)
	return result, nil
}

// SearchExecutionKnowledge is called only by the authenticated read-only knowledge tool.
func (m *Manager) SearchExecutionKnowledge(ctx context.Context, executionID, token string, input KnowledgeSearchInput) (KnowledgeSearchResult, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || len(token) != len(session.controlToken) || subtle.ConstantTimeCompare([]byte(token), []byte(session.controlToken)) != 1 {
		return KnowledgeSearchResult{}, errors.New("invalid execution control token")
	}
	agent, err := m.store.GetAgent(session.agentID)
	if err != nil {
		return KnowledgeSearchResult{}, err
	}
	return m.knowledge.Search(ctx, agent.KnowledgeBaseIDs, input)
}

func (m *Manager) ReconcileIssue(issue Issue) {
	if issue.ExecutionPhase == "waiting_children" {
		go m.scheduleChildren(issue.ID)
	}
	if issue.ParentID != "" && slices.Contains([]string{"done", "cancelled"}, issue.Status) {
		go m.scheduleChildren(issue.ParentID)
	}
}

func (m *Manager) failExecution(issue Issue, e Execution, cause error) {
	now := time.Now()
	if e.ID != "" {
		_ = m.store.updateExecution(e.ID, map[string]any{"status": "failed", "error": cause.Error(), "finished_at": now, "pid": 0})
	}
	_ = m.store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "blocked", "execution_phase": "blocked", "error": cause.Error(), "checkout_execution_id": "", "updated_at": now}).Error
	m.store.addEvent(e.ID, issue.ID, "error", "执行失败", cause.Error())
	m.store.notify()
}

func (m *Manager) blockIssue(issue Issue, title string, cause error) {
	now := time.Now()
	_ = m.store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{"status": "blocked", "execution_phase": "blocked", "error": cause.Error(), "checkout_execution_id": "", "updated_at": now}).Error
	m.store.addEvent("", issue.ID, "error", title, cause.Error())
	m.store.notify()
}
func (m *Manager) sessionExited(s *PiSession, waitErr error) {
	m.mu.Lock()
	if m.sessions[s.key] == s {
		delete(m.sessions, s.key)
	}
	m.mu.Unlock()
	if s.closed.Load() {
		return
	}
	var e Execution
	if m.store.db.First(&e, "id = ?", s.executionID).Error != nil || slices.Contains([]string{"completed", "failed", "stopped", "cancelled"}, e.Status) {
		return
	}
	message := "Pi 进程已退出"
	if waitErr != nil {
		message = waitErr.Error()
	}
	now := time.Now()
	_ = m.store.updateExecution(s.executionID, map[string]any{"status": "disconnected", "error": message, "finished_at": now, "pid": 0})
	_ = m.store.db.Model(&Issue{}).Where("id = ? AND checkout_execution_id = ?", s.issueID, s.executionID).Updates(map[string]any{"status": "blocked", "execution_phase": "blocked", "error": message, "checkout_execution_id": "", "updated_at": now}).Error
	m.store.addEvent(s.executionID, s.issueID, "error", "Pi session 已断开", message)
	m.store.notify()
}
func (m *Manager) latestAssistant(executionID string) string {
	var msg Message
	if m.store.db.Where("execution_id = ? AND role = ?", executionID, "assistant").Order("created_at desc").First(&msg).Error != nil {
		return ""
	}
	return strings.TrimSpace(msg.Content)
}
func (m *Manager) updateStats(s *PiSession, data any) {
	stats, ok := data.(map[string]any)
	if !ok {
		return
	}
	var execution Execution
	if err := m.store.db.First(&execution, "id = ?", s.executionID).Error; err != nil {
		return
	}
	input, output, cacheRead, cacheWrite, total := int64(0), int64(0), int64(0), int64(0), int64(0)
	if tokens, ok := stats["tokens"].(map[string]any); ok {
		input = tokenCount(tokens["input"])
		output = tokenCount(tokens["output"])
		cacheRead = tokenCount(tokens["cacheRead"])
		cacheWrite = tokenCount(tokens["cacheWrite"])
		total = tokenCount(tokens["total"])
	}
	if total == 0 {
		total = input + output + cacheRead + cacheWrite
	}
	cost := calculateModelCost(execution.Pricing, input, output, cacheRead, cacheWrite)
	_ = m.store.updateExecution(s.executionID, map[string]any{
		"cost":               cost,
		"tokens":             total,
		"input_tokens":       input,
		"output_tokens":      output,
		"cache_read_tokens":  cacheRead,
		"cache_write_tokens": cacheWrite,
	})
	m.store.notify()
}

func tokenCount(value any) int64 {
	switch number := value.(type) {
	case float64:
		return int64(number)
	case float32:
		return int64(number)
	case int:
		return int64(number)
	case int64:
		return number
	case json.Number:
		parsed, _ := number.Int64()
		return parsed
	default:
		return 0
	}
}

func (m *Manager) SendChat(issueID, executionID, message string) (Message, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return Message{}, errors.New("消息不能为空")
	}
	issue, err := m.store.GetIssue(issueID)
	if err != nil {
		return Message{}, err
	}
	var s *PiSession
	if executionID != "" {
		s = m.getSession(executionID)
	} else {
		m.mu.RLock()
		for _, candidate := range m.sessions {
			if candidate.issueID == issue.ID && candidate.busy.Load() {
				s = candidate
				break
			}
		}
		m.mu.RUnlock()
	}
	if s == nil {
		return Message{}, errors.New("该 Issue 当前没有连接中的 Pi session")
	}
	msg := Message{ID: nextID("message"), ExecutionID: s.executionID, IssueID: issue.ID, Role: "user", Content: message, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := m.store.db.Create(&msg).Error; err != nil {
		return Message{}, err
	}
	_ = m.store.db.Model(&Execution{}).Where("id = ?", s.executionID).UpdateColumn("message_count", gorm.Expr("message_count + 1")).Error
	command := "prompt"
	if s.busy.Load() {
		command = "steer"
	}
	if err := s.Send(map[string]any{"id": nextID("rpc"), "type": command, "message": message}); err != nil {
		return Message{}, err
	}
	m.store.notify()
	return msg, nil
}
func (m *Manager) StopExecution(id string) error {
	s := m.getSession(id)
	if s != nil {
		_ = s.Send(map[string]any{"type": "abort"})
		s.Close()
	}
	now := time.Now()
	if err := m.store.updateExecution(id, map[string]any{"status": "stopped", "finished_at": now, "pid": 0}); err != nil {
		return err
	}
	_ = m.store.db.Model(&Issue{}).Where("checkout_execution_id = ?", id).Updates(map[string]any{"status": "todo", "execution_phase": "active", "checkout_execution_id": "", "updated_at": now}).Error
	m.store.notify()
	return nil
}

func (m *Manager) CancelTask(id, reason string) (TaskCancellationResult, error) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()

	result, issueIDs, err := m.store.cancelTaskTree(id, reason)
	if err != nil {
		return TaskCancellationResult{}, err
	}
	issueSet := make(map[string]bool, len(issueIDs))
	for _, issueID := range issueIDs {
		issueSet[issueID] = true
	}
	m.mu.RLock()
	sessions := make([]*PiSession, 0)
	for _, session := range m.sessions {
		if issueSet[session.issueID] {
			sessions = append(sessions, session)
		}
	}
	m.mu.RUnlock()
	for _, session := range sessions {
		session.Close()
	}
	m.store.notify()
	return result, nil
}
func (m *Manager) getSession(id string) *PiSession {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.sessions[id]
}

func randomControlToken() (string, error) {
	data := make([]byte, 32)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

var mentionPattern = regexp.MustCompile(`\[@[^\]]+\]\(agent://([A-Za-z0-9._:-]+)\)`)

func (m *Manager) AddIssueComment(issueID, body string) (IssueComment, error) {
	body = strings.TrimSpace(body)
	if body == "" {
		return IssueComment{}, errors.New("评论内容不能为空")
	}
	if utf8.RuneCountInString(body) > 10000 {
		return IssueComment{}, errors.New("评论内容不能超过 10000 个字符")
	}
	if _, err := m.store.GetIssue(issueID); err != nil {
		return IssueComment{}, err
	}
	mentions := m.validMentions(body, "")
	c := IssueComment{ID: nextID("comment"), IssueID: issueID, AuthorType: "operator", AuthorID: "operator", Body: body, Mentions: mentions, Attachments: []IssueAttachment{}, CreatedAt: time.Now()}
	if err := m.store.db.Create(&c).Error; err != nil {
		return IssueComment{}, err
	}
	for _, agent := range mentions {
		w := AgentWakeup{ID: nextID("wakeup"), IssueID: issueID, CommentID: c.ID, AgentID: agent, Reason: "issue_comment_mentioned", Status: "queued", CreatedAt: time.Now()}
		_ = m.store.db.Create(&w).Error
		go m.dispatchWakeup(w.ID)
	}
	m.store.notify()
	return c, nil
}
func (m *Manager) validMentions(body, exclude string) []string {
	available := map[string]bool{}
	for _, a := range m.store.Agents() {
		available[a.ID] = a.Enabled
	}
	seen := map[string]bool{}
	var out []string
	for _, x := range mentionPattern.FindAllStringSubmatch(body, -1) {
		id := x[1]
		if id != exclude && available[id] && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func (m *Manager) dispatchWakeup(id string) {
	m.scheduleMu.Lock()
	defer m.scheduleMu.Unlock()
	var w AgentWakeup
	if m.store.db.First(&w, "id = ? AND status = ?", id, "queued").Error != nil {
		return
	}
	issue, err := m.store.GetIssue(w.IssueID)
	if err != nil {
		return
	}
	if m.store.belongsToCancelledTask(issue) {
		_ = m.store.db.Model(&w).Updates(map[string]any{"status": "cancelled", "error": "所属任务已取消", "completed_at": time.Now()}).Error
		m.store.notify()
		return
	}
	agent, err := m.store.executionAgent(w.AgentID)
	if err != nil {
		return
	}
	e, err := m.store.createExecution(issue, agent.ID, "mention")
	if err != nil {
		return
	}
	now := time.Now()
	_ = m.store.db.Model(&w).Updates(map[string]any{"status": "delivered", "execution_id": e.ID, "delivered_at": now}).Error
	prompt := mentionPrompt(issue, w, m.store.Agents(), m.store.db)
	s, err := m.startSession(issue, e, agent, prompt)
	if err != nil {
		_ = m.store.db.Model(&w).Updates(map[string]any{"status": "failed", "error": err.Error()}).Error
		return
	}
	s.wakeupID = w.ID
}
func (m *Manager) completeWakeup(s *PiSession, result string) {
	now := time.Now()
	_ = m.store.db.Model(&AgentWakeup{}).Where("id = ?", s.wakeupID).Updates(map[string]any{"status": "completed", "completed_at": now}).Error
	m.addAgentComment(s.issueID, s.agentID, result, s.executionID)
	m.store.notify()
}
func (m *Manager) addAgentComment(issueID, agentID, body, executionID string) {
	body = strings.TrimSpace(body)
	if body == "" {
		var attachmentCount int64
		if executionID != "" {
			_ = m.store.db.Model(&IssueAttachment{}).Where("execution_id = ? AND comment_id = ''", executionID).Count(&attachmentCount).Error
		}
		if attachmentCount == 0 {
			return
		}
		body = "已生成并附上交付物。"
	}
	mentions := m.validMentions(body, agentID)
	c := IssueComment{ID: nextID("comment"), IssueID: issueID, AuthorType: "agent", AuthorID: agentID, Body: body, Mentions: mentions, CreatedAt: time.Now()}
	if err := m.store.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&c).Error; err != nil {
			return err
		}
		return m.store.bindExecutionAttachments(tx, &c, executionID)
	}); err != nil {
		return
	}
	for _, id := range mentions {
		w := AgentWakeup{ID: nextID("wakeup"), IssueID: issueID, CommentID: c.ID, AgentID: id, Reason: "issue_comment_mentioned", Status: "queued", CreatedAt: time.Now()}
		_ = m.store.db.Create(&w).Error
		go m.dispatchWakeup(w.ID)
	}
}
func (m *Manager) resumeWork() {
	var wakeups []AgentWakeup
	m.store.db.Where("status = ?", "queued").Find(&wakeups)
	for _, w := range wakeups {
		go m.dispatchWakeup(w.ID)
	}
	var parents []Issue
	m.store.db.Where("status = ? AND execution_phase = ?", "in_progress", "waiting_children").Find(&parents)
	for _, p := range parents {
		go m.scheduleChildren(p.ID)
	}
}

func (s *Store) Sessions() []SessionSummary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var e []Execution
	var issues []Issue
	s.db.Order("started_at desc").Find(&e)
	s.db.Find(&issues)
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
	for _, e := range executions {
		i := issueMap[e.IssueID]
		out = append(out, SessionSummary{Execution: e, IssueIdentifier: i.Identifier, IssueTitle: i.Title, AgentName: agentMap[e.AgentID]})
	}
	return out
}
func (s *Store) GetSession(id string) (SessionDetail, error) {
	var e Execution
	if err := s.db.Where("id = ? OR session_id = ?", id, id).First(&e).Error; err != nil {
		return SessionDetail{}, errors.New("session not found")
	}
	issue, _ := s.GetIssue(e.IssueID)
	agentName := e.AgentID
	if a, err := s.GetAgent(e.AgentID); err == nil {
		agentName = a.Name
	}
	d := SessionDetail{Session: SessionSummary{Execution: e, IssueIdentifier: issue.Identifier, IssueTitle: issue.Title, AgentName: agentName}, Messages: []Message{}, Events: []ExecutionEvent{}, Approvals: []Approval{}}
	s.db.Where("execution_id = ?", e.ID).Order("created_at asc").Find(&d.Messages)
	s.db.Where("execution_id = ?", e.ID).Order("created_at desc").Find(&d.Events)
	s.db.Where("execution_id = ?", e.ID).Order("created_at desc").Find(&d.Approvals)
	return d, nil
}

func parsePlan(text string) (planPayload, error) {
	var p planPayload
	c := strings.TrimSpace(text)
	if start := strings.Index(c, "```json"); start >= 0 {
		c = c[start+7:]
		if end := strings.Index(c, "```"); end >= 0 {
			c = c[:end]
		}
	} else {
		a, b := strings.Index(c, "{"), strings.LastIndex(c, "}")
		if a >= 0 && b > a {
			c = c[a : b+1]
		}
	}
	if err := json.Unmarshal([]byte(c), &p); err != nil {
		return p, err
	}
	if len(p.Issues) < 2 || len(p.Issues) > maxChildrenPerRequest {
		return p, fmt.Errorf("plan must contain 2 to %d issues", maxChildrenPerRequest)
	}
	for i, x := range p.Issues {
		if strings.TrimSpace(x.Title) == "" || strings.TrimSpace(x.AcceptanceCriteria) == "" {
			return p, fmt.Errorf("issue %d is incomplete", i+1)
		}
	}
	return p, nil
}
func planningPrompt(i Issue, agents []AgentDefinition) string {
	var roster strings.Builder
	for _, a := range agents {
		if a.Enabled && a.Category != "orchestrator" && !a.Internal {
			fmt.Fprintf(&roster, "- %s: %s\n", a.ID, a.Description)
		}
	}
	return fmt.Sprintf(`Decompose this real software task into executable child Issues.
Objective: %s
Context: %s
Constraints: %s
Workspace: %s
Enabled agents:
%s
Return only JSON: {"issues":[{"title":"...","description":"...","acceptanceCriteria":"...","priority":"critical|high|medium|low","dependsOn":[1],"agentId":"backend-engineer"}]}. Dependencies use earlier 1-based indexes. Produce 2-8 independently verifiable implementation/test issues.`, i.Title, fallback(i.Context, i.Description), i.Constraints, i.Workspace, roster.String())
}
func workerPrompt(i Issue) string {
	return fmt.Sprintf(`Complete this Issue in the real workspace.
Issue %s: %s
Description: %s
Acceptance criteria: %s
Constraints: %s
Workspace: %s
Use tools to inspect and modify the project, run relevant validation, fix in-scope failures, and finish with a concise evidence-based report.
For every user-facing deliverable file you generate (reports, archives, images, documents, or datasets), call aegis_publish_attachment before ending the turn so Aegis can mount it on your completion comment. Source-code edits are collected separately and should not be published merely as attachments.

If this Issue is too broad for one reliable execution, call aegis_create_subissues once with 2-8 independently verifiable child Issues and explicit earlier-index dependencies. After the tool succeeds, stop implementation on the parent and end your turn. The scheduler will execute the children and later resume this Issue in a fresh continuation session. Do not create children for work you can safely complete yourself.`, i.Identifier, i.Title, i.Description, i.AcceptanceCriteria, i.Constraints, i.Workspace)
}

func continuationPrompt(parent Issue, children []Issue) string {
	var summaries strings.Builder
	for _, child := range children {
		fmt.Fprintf(&summaries, "- %s [%s] %s\n  Agent: %s\n  Result: %s\n", child.Identifier, child.Status, child.Title, child.AssigneeAgentID, fallback(truncate(strings.TrimSpace(child.Result), 1800), fallback(child.Error, "No result recorded.")))
	}
	return fmt.Sprintf(`Resume the parent Issue after all direct child Issues reached a terminal state.

Parent %s: %s
Description: %s
Acceptance criteria: %s
Workspace: %s

Direct child results:
%s
Inspect the actual workspace state, integrate or correct child work where needed, and run the parent-level validation. Publish every generated user-facing deliverable with aegis_publish_attachment before ending the turn. If material work is still too complex, you may call aegis_create_subissues again for a new bounded decomposition. Otherwise finish with a concise parent-level report covering integration, validation, and remaining risk.`, parent.Identifier, parent.Title, parent.Description, parent.AcceptanceCriteria, parent.Workspace, summaries.String())
}
func mentionPrompt(i Issue, w AgentWakeup, agents []AgentDefinition, db any) string {
	return fmt.Sprintf("You were mentioned on Issue %s: %s. Inspect the Issue context and provide a concrete response. You may perform scoped work using your tools. Finish with a response suitable for the Issue comment thread. Available agent IDs: %v", i.Identifier, i.Title, func() []string {
		var out []string
		for _, a := range agents {
			if a.Enabled && !a.Internal {
				out = append(out, a.ID)
			}
		}
		return out
	}())
}

func (m *Manager) TestConnection(ctx context.Context, input SaveConfigInput) ConnectionTestResult {
	started := time.Now()
	cfg := Config{NodePath: strings.TrimSpace(input.NodePath), PiPath: strings.TrimSpace(input.PiPath), Provider: strings.TrimSpace(input.Provider), Model: strings.TrimSpace(input.Model), BaseURL: strings.TrimSpace(input.BaseURL), Thinking: fallback(strings.TrimSpace(input.Thinking), "medium"), APIKey: strings.TrimSpace(input.APIKey)}
	args := []string{"--mode", "rpc", "--no-session", "--no-tools", "--no-extensions", "--extension", m.guardPath, "--no-approve", "--provider", cfg.Provider, "--model", cfg.Model, "--thinking", cfg.Thinking}
	command, commandArgs := piCommand(cfg, args...)
	cmd := exec.CommandContext(ctx, command, commandArgs...)
	cmd.Env = runtimeEnv(cfg, "none", PermissionBoundary{AllowNetwork: true, AllowShell: true, AllowWrite: true}, "")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return testResult(started, "", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return testResult(started, "", err)
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return testResult(started, "", err)
	}
	defer func() {
		_ = stdin.Close()
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
	}()
	data, _ := json.Marshal(map[string]any{"id": "connection-test", "type": "prompt", "message": "Reply with exactly AEGIS_OK and nothing else."})
	_, _ = stdin.Write(append(data, '\n'))
	reader := bufio.NewReader(stdout)
	var reply strings.Builder
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return testResult(started, reply.String(), fmt.Errorf("Pi exited: %w: %s", err, stderr.String()))
		}
		var event map[string]any
		if json.Unmarshal(line, &event) != nil {
			continue
		}
		if event["type"] == "message_update" {
			if d, ok := event["assistantMessageEvent"].(map[string]any); ok && d["type"] == "text_delta" {
				reply.WriteString(stringValue(d["delta"]))
			}
		}
		if event["type"] == "agent_settled" {
			return testResult(started, strings.TrimSpace(reply.String()), nil)
		}
	}
}
func testResult(start time.Time, reply string, err error) ConnectionTestResult {
	r := ConnectionTestResult{OK: err == nil, Reply: reply, Duration: time.Since(start).Milliseconds()}
	if err != nil {
		r.Error = err.Error()
	}
	return r
}
func piCommand(c Config, args ...string) (string, []string) {
	if strings.HasSuffix(strings.ToLower(c.PiPath), ".js") || strings.HasSuffix(strings.ToLower(c.PiPath), ".mjs") {
		return c.NodePath, append([]string{c.PiPath}, args...)
	}
	return c.PiPath, args
}
func runtimeEnv(c Config, approval string, b PermissionBoundary, workspace string) []string {
	env := append([]string(nil), os.Environ()...)
	env = append(env, "AEGIS_APPROVAL_MODE="+approval, "AEGIS_MANAGED=1", "AEGIS_PROVIDER="+c.Provider, "AEGIS_WORKSPACE="+workspace, "AEGIS_WORKSPACE_SCOPE="+b.WorkspaceScope, "AEGIS_ALLOW_NETWORK="+strconv.FormatBool(b.AllowNetwork), "AEGIS_ALLOW_SHELL="+strconv.FormatBool(b.AllowShell), "AEGIS_ALLOW_WRITE="+strconv.FormatBool(b.AllowWrite))
	if c.BaseURL != "" {
		env = append(env, "AEGIS_BASE_URL="+c.BaseURL)
	}
	if c.APIKey != "" {
		if key := providerEnv(c.Provider); key != "" {
			env = append(env, key+"="+c.APIKey)
		}
	}
	return env
}
func providerEnv(provider string) string {
	values := map[string]string{"anthropic": "ANTHROPIC_API_KEY", "openai": "OPENAI_API_KEY", "google": "GEMINI_API_KEY", "openrouter": "OPENROUTER_API_KEY", "deepseek": "DEEPSEEK_API_KEY", "groq": "GROQ_API_KEY", "xai": "XAI_API_KEY", "zai": "ZAI_API_KEY", "mistral": "MISTRAL_API_KEY", "opencode": "OPENCODE_API_KEY", "opencode-go": "OPENCODE_API_KEY"}
	return values[provider]
}

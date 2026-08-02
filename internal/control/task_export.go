package control

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"aegis/observability"
	observabilitysqlite "aegis/observability/sqlitestore"
	"gorm.io/gorm"
)

const taskEvidenceSchemaVersion = "aegis.task-evidence/v1"

var ErrTaskEvidenceNotFound = errors.New("task evidence: Task or Issue not found")

type TaskExportOptions struct {
	IncludeArtifacts bool
	RedactSecrets    bool
}

type TaskEvidenceManifest struct {
	SchemaVersion              string                     `json:"schemaVersion"`
	ExportID                   string                     `json:"exportId"`
	RequestedID                string                     `json:"requestedId"`
	TaskID                     string                     `json:"taskId,omitempty"`
	RootIssueIDs               []string                   `json:"rootIssueIds"`
	GeneratedAt                time.Time                  `json:"generatedAt"`
	SnapshotFrom               *time.Time                 `json:"snapshotFrom,omitempty"`
	SnapshotTo                 *time.Time                 `json:"snapshotTo,omitempty"`
	Complete                   bool                       `json:"complete"`
	ActiveAtExport             bool                       `json:"activeAtExport"`
	RedactionEnabled           bool                       `json:"redactionEnabled"`
	ArtifactsIncluded          bool                       `json:"artifactsIncluded"`
	ArtifactsMayContainSecrets bool                       `json:"artifactsMayContainSecrets"`
	Warnings                   []string                   `json:"warnings,omitempty"`
	Counts                     map[string]int             `json:"counts"`
	Files                      []TaskEvidenceManifestFile `json:"files"`
}

type TaskEvidenceManifestFile struct {
	Path        string `json:"path"`
	Category    string `json:"category"`
	MediaType   string `json:"mediaType"`
	Size        int64  `json:"size"`
	SHA256      string `json:"sha256"`
	RecordCount int    `json:"recordCount,omitempty"`
}

type TaskEvidenceScope struct {
	Task       *Task   `json:"task,omitempty"`
	RootIssues []Issue `json:"rootIssues"`
}

type TaskEvaluationContext struct {
	TaskID                 string            `json:"taskId,omitempty"`
	RootIssueIDs           []string          `json:"rootIssueIds"`
	Objectives             []string          `json:"objectives"`
	FinalResults           map[string]string `json:"finalResults"`
	IssueStatusCounts      map[string]int    `json:"issueStatusCounts"`
	ExecutionStatusCounts  map[string]int    `json:"executionStatusCounts"`
	ValidationStatusCounts map[string]int    `json:"validationStatusCounts"`
	TotalTokens            int64             `json:"totalTokens"`
	TotalCostUSD           float64           `json:"totalCostUsd"`
	StartedAt              *time.Time        `json:"startedAt,omitempty"`
	FinishedAt             *time.Time        `json:"finishedAt,omitempty"`
	DurationSeconds        float64           `json:"durationSeconds,omitempty"`
	FailedIssueIDs         []string          `json:"failedIssueIds,omitempty"`
	CancelledIssueIDs      []string          `json:"cancelledIssueIds,omitempty"`
	UnfinishedIssueIDs     []string          `json:"unfinishedIssueIds,omitempty"`
}

type TaskConversation struct {
	Execution Execution `json:"execution"`
	Messages  []Message `json:"messages"`
}

type TaskAttachmentEvidence struct {
	Kind         string     `json:"kind"`
	ID           string     `json:"id"`
	IssueID      string     `json:"issueId,omitempty"`
	ExecutionID  string     `json:"executionId,omitempty"`
	Name         string     `json:"name"`
	MimeType     string     `json:"mimeType"`
	DeclaredSize int64      `json:"declaredSize"`
	ArchivePath  string     `json:"archivePath,omitempty"`
	SHA256       string     `json:"sha256,omitempty"`
	ExportedSize int64      `json:"exportedSize,omitempty"`
	Available    bool       `json:"available"`
	CreatedAt    time.Time  `json:"createdAt"`
	BoundAt      *time.Time `json:"boundAt,omitempty"`
	SourcePath   string     `json:"sourcePath,omitempty"`
	Description  string     `json:"description,omitempty"`
	storagePath  string
}

type durableRuntimeRecord struct {
	ID             string          `json:"id"`
	Status         string          `json:"status,omitempty"`
	EventID        string          `json:"eventId,omitempty"`
	CoordinationID string          `json:"coordinationId,omitempty"`
	AvailableAt    *time.Time      `json:"availableAt,omitempty"`
	CreatedAt      *time.Time      `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time      `json:"updatedAt,omitempty"`
	Attempts       int             `json:"attempts,omitempty"`
	LeaseOwner     string          `json:"leaseOwner,omitempty"`
	LeaseExpiresAt *time.Time      `json:"leaseExpiresAt,omitempty"`
	LastError      string          `json:"lastError,omitempty"`
	Payload        json.RawMessage `json:"payload"`
}

type coordinationBindingEvidence struct {
	CoordinationID string          `json:"coordinationId"`
	Mode           string          `json:"mode"`
	Version        string          `json:"version"`
	Config         json.RawMessage `json:"config"`
	UpdatedAt      time.Time       `json:"updatedAt"`
}

type phoneSessionEvidence struct {
	ID          string          `json:"id"`
	TaskID      string          `json:"taskId,omitempty"`
	TaskAgentID string          `json:"taskAgentId,omitempty"`
	AgentID     string          `json:"agentId"`
	Actor       json.RawMessage `json:"actor"`
	Installed   json.RawMessage `json:"installed"`
	ActiveAppID string          `json:"activeAppId,omitempty"`
	Stacks      json.RawMessage `json:"stacks"`
	Drafts      json.RawMessage `json:"drafts"`
	Current     json.RawMessage `json:"current"`
	Results     json.RawMessage `json:"results"`
	CreatedAt   time.Time       `json:"createdAt"`
	UpdatedAt   time.Time       `json:"updatedAt"`
}

type phoneAuditEvidence struct {
	ID             string          `json:"id"`
	OccurredAt     time.Time       `json:"occurredAt"`
	AgentID        string          `json:"agentId"`
	PhoneSessionID string          `json:"phoneSessionId"`
	AppID          string          `json:"appId"`
	PageID         string          `json:"pageId"`
	PageRevision   string          `json:"pageRevision"`
	Ref            string          `json:"ref"`
	Action         string          `json:"action"`
	Target         string          `json:"target,omitempty"`
	Result         string          `json:"result"`
	Effect         string          `json:"effect,omitempty"`
	ErrorCode      string          `json:"errorCode,omitempty"`
	ErrorMessage   string          `json:"errorMessage,omitempty"`
	Arguments      json.RawMessage `json:"arguments,omitempty"`
}

type taskEvidence struct {
	scope                TaskEvidenceScope
	config               ConfigView
	issues               []Issue
	relations            []IssueRelation
	executions           []Execution
	conversations        []TaskConversation
	events               []ExecutionEvent
	runtimeEvents        []durableRuntimeRecord
	progress             []ExecutionProgress
	comments             []IssueComment
	approvals            []Approval
	validations          []IssueValidation
	wakeups              []AgentWakeup
	decompositions       []IssueDecomposition
	waits                []IssueChildWait
	taskAgents           []TaskAgent
	relayThreads         []RelayThread
	relayMessages        []RelayMessage
	relayReceipts        []RelayReceipt
	agents               []AgentDefinition
	skills               []SkillDefinition
	knowledgeBases       []KnowledgeBase
	knowledgeDocuments   []KnowledgeDocument
	coordinationBindings []coordinationBindingEvidence
	coordinationEvents   []durableRuntimeRecord
	coordinationEffects  []durableRuntimeRecord
	coordinationRuns     []durableRuntimeRecord
	phoneSessions        []phoneSessionEvidence
	phoneAudits          []phoneAuditEvidence
	logs                 []observabilitysqlite.Record
	attachments          []TaskAttachmentEvidence
	evaluation           TaskEvaluationContext
	from                 *time.Time
	to                   *time.Time
}

func (m *Manager) WriteTaskEvidence(ctx context.Context, requestedID string, output io.Writer, options TaskExportOptions) (manifest TaskEvidenceManifest, err error) {
	if m == nil || m.store == nil || output == nil {
		return TaskEvidenceManifest{}, errors.New("task export: manager, store and output are required")
	}
	requestedID = strings.TrimSpace(requestedID)
	if requestedID == "" {
		return TaskEvidenceManifest{}, errors.New("task export: task or Issue ID is required")
	}
	ctx = observability.WithScope(ctx, observability.Scope{TaskID: requestedID, Component: "control.task_export"})
	ctx, span := (&observability.Tracer{Logger: observability.Default(), Metrics: observability.DefaultMetrics()}).Start(ctx, "task.export", slog.String("requested_id", requestedID), slog.Bool("include_artifacts", options.IncludeArtifacts))
	defer func() { span.End(err, slog.Int("file_count", len(manifest.Files))) }()
	evidence, warnings, err := m.collectTaskEvidence(ctx, requestedID)
	if err != nil {
		return TaskEvidenceManifest{}, err
	}
	rootIDs := make([]string, len(evidence.scope.RootIssues))
	active := false
	for index, root := range evidence.scope.RootIssues {
		rootIDs[index] = root.ID
	}
	for _, issue := range evidence.issues {
		active = active || !issueStatusTerminal(issue.Status)
	}
	for _, execution := range evidence.executions {
		active = active || !slicesContains([]string{"completed", "failed", "budget_exceeded", "cancelled"}, execution.Status)
	}
	manifest = TaskEvidenceManifest{
		SchemaVersion: taskEvidenceSchemaVersion, ExportID: "task-export-" + observability.NewID(12), RequestedID: requestedID,
		RootIssueIDs: rootIDs, GeneratedAt: time.Now().UTC(), SnapshotFrom: evidence.from, SnapshotTo: evidence.to,
		Complete: len(warnings) == 0, ActiveAtExport: active, RedactionEnabled: options.RedactSecrets,
		ArtifactsIncluded: options.IncludeArtifacts, ArtifactsMayContainSecrets: options.IncludeArtifacts, Warnings: warnings, Counts: evidence.counts(), Files: []TaskEvidenceManifestFile{},
	}
	if evidence.scope.Task != nil {
		manifest.TaskID = evidence.scope.Task.ID
	}
	if active {
		manifest.Warnings = append(manifest.Warnings, "Task was active at export time; this is a consistent point-in-time snapshot, not a terminal record.")
		manifest.Complete = false
	}
	redactor := observability.NewRedactor()
	if options.RedactSecrets {
		config := m.store.Config()
		redactor.RegisterSecret(config.APIKey)
		redactor.RegisterSecret(config.WebSearch.APIKey)
	}
	archive := zip.NewWriter(output)
	defer func() {
		if closeErr := archive.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if options.IncludeArtifacts {
		for index := range evidence.attachments {
			file, addErr := addArtifactFile(archive, &evidence.attachments[index])
			if addErr != nil {
				manifest.Warnings = append(manifest.Warnings, addErr.Error())
				manifest.Complete = false
				continue
			}
			manifest.Files = append(manifest.Files, file)
		}
	}
	files := []struct {
		path, category string
		value          any
		count          int
	}{
		{"scope.json", "scope", evidence.scope, len(evidence.scope.RootIssues)},
		{"runtime_config.json", "configuration", evidence.config, 1},
		{"evaluation_context.json", "evaluation", evidence.evaluation, 1},
		{"issues.json", "domain", evidence.issues, len(evidence.issues)},
		{"relations.json", "domain", evidence.relations, len(evidence.relations)},
		{"agents.json", "configuration", evidence.agents, len(evidence.agents)},
		{"task_agents.json", "domain", evidence.taskAgents, len(evidence.taskAgents)},
		{"skills.json", "configuration", evidence.skills, len(evidence.skills)},
		{"knowledge_bases.json", "configuration", evidence.knowledgeBases, len(evidence.knowledgeBases)},
		{"knowledge_documents.json", "configuration", evidence.knowledgeDocuments, len(evidence.knowledgeDocuments)},
		{"executions.json", "execution", evidence.executions, len(evidence.executions)},
		{"conversations.json", "conversation", evidence.conversations, len(evidence.conversations)},
		{"execution_events.json", "event", evidence.events, len(evidence.events)},
		{"agent_runtime_events.json", "event", evidence.runtimeEvents, len(evidence.runtimeEvents)},
		{"progress.json", "event", evidence.progress, len(evidence.progress)},
		{"comments.json", "conversation", evidence.comments, len(evidence.comments)},
		{"approvals.json", "governance", evidence.approvals, len(evidence.approvals)},
		{"validations.json", "evaluation", evidence.validations, len(evidence.validations)},
		{"wakeups.json", "coordination", evidence.wakeups, len(evidence.wakeups)},
		{"decompositions.json", "coordination", evidence.decompositions, len(evidence.decompositions)},
		{"child_waits.json", "coordination", evidence.waits, len(evidence.waits)},
		{"relay_threads.json", "conversation", evidence.relayThreads, len(evidence.relayThreads)},
		{"relay_messages.json", "conversation", evidence.relayMessages, len(evidence.relayMessages)},
		{"relay_receipts.json", "conversation", evidence.relayReceipts, len(evidence.relayReceipts)},
		{"coordination/bindings.json", "coordination", evidence.coordinationBindings, len(evidence.coordinationBindings)},
		{"coordination/events.json", "coordination", evidence.coordinationEvents, len(evidence.coordinationEvents)},
		{"coordination/effects.json", "coordination", evidence.coordinationEffects, len(evidence.coordinationEffects)},
		{"coordination/executions.json", "coordination", evidence.coordinationRuns, len(evidence.coordinationRuns)},
		{"phone/sessions.json", "tool_audit", evidence.phoneSessions, len(evidence.phoneSessions)},
		{"phone/audit_events.json", "tool_audit", evidence.phoneAudits, len(evidence.phoneAudits)},
		{"observability/logs.json", "observability", evidence.logs, len(evidence.logs)},
		{"attachments.json", "artifact", evidence.attachments, len(evidence.attachments)},
	}
	for _, item := range files {
		encoded, encodeErr := exportJSON(item.value, redactor, options.RedactSecrets)
		if encodeErr != nil {
			return manifest, fmt.Errorf("task export: encode %s: %w", item.path, encodeErr)
		}
		entry, writeErr := addBytesFile(archive, item.path, item.category, "application/json", encoded, item.count)
		if writeErr != nil {
			return manifest, writeErr
		}
		manifest.Files = append(manifest.Files, entry)
	}
	manifest.Complete = manifest.Complete && len(manifest.Warnings) == 0
	manifestBytes, encodeErr := exportJSON(manifest, redactor, false)
	if encodeErr != nil {
		return manifest, encodeErr
	}
	if _, writeErr := addBytesFile(archive, "manifest.json", "manifest", "application/json", manifestBytes, 1); writeErr != nil {
		return manifest, writeErr
	}
	observability.Default().Info(ctx, "task.export.completed", slog.String("export_id", manifest.ExportID), slog.Int("file_count", len(manifest.Files)), slog.Bool("complete", manifest.Complete), slog.Int("warning_count", len(manifest.Warnings)))
	observability.DefaultMetrics().AddCounter("task_exports_total", 1, observability.Labels{"complete": fmt.Sprint(manifest.Complete)})
	return manifest, nil
}

func (m *Manager) collectTaskEvidence(ctx context.Context, requestedID string) (taskEvidence, []string, error) {
	var result taskEvidence
	var warnings []string
	err := m.store.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		scope, err := resolveTaskExportScope(tx, requestedID)
		if err != nil {
			return err
		}
		result.scope = scope
		result.config = configView(m.store.Config())
		result.issues, err = taskTreeIssues(tx, scope.RootIssues)
		if err != nil {
			return err
		}
		issueIDs := issueIDsOf(result.issues)
		if len(issueIDs) == 0 {
			warnings = append(warnings, "Task has no execution Issues.")
			return nil
		}
		if err = tx.Where("issue_id IN ? OR related_issue_id IN ?", issueIDs, issueIDs).Order("created_at asc").Find(&result.relations).Error; err != nil {
			return err
		}
		if err = tx.Where("issue_id IN ?", issueIDs).Order("started_at asc, id asc").Find(&result.executions).Error; err != nil {
			return err
		}
		executionIDs := executionIDsOf(result.executions)
		if err = queryByScope(tx, &result.events, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
			return err
		}
		if err = queryByScope(tx, &result.progress, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
			return err
		}
		if err = queryByScope(tx, &result.comments, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
			return err
		}
		if err = queryByScope(tx, &result.approvals, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
			return err
		}
		if err = tx.Where("issue_id IN ?", issueIDs).Order("created_at asc, id asc").Find(&result.validations).Error; err != nil {
			return err
		}
		if err = tx.Where("issue_id IN ?", issueIDs).Order("created_at asc, id asc").Find(&result.wakeups).Error; err != nil {
			return err
		}
		if err = tx.Where("parent_issue_id IN ?", issueIDs).Order("created_at asc, id asc").Find(&result.decompositions).Error; err != nil {
			return err
		}
		if err = tx.Where("parent_issue_id IN ?", issueIDs).Order("created_at asc, id asc").Find(&result.waits).Error; err != nil {
			return err
		}
		var messages []Message
		if err = queryByScope(tx, &messages, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
			return err
		}
		result.conversations = groupConversations(result.executions, messages)
		if err = collectRelay(tx, issueIDs, &result); err != nil {
			return err
		}
		if err = collectAttachments(tx, m.store.dataDir, scope, issueIDs, executionIDs, &result.attachments); err != nil {
			return err
		}
		if err = collectRegistryContext(tx, result.issues, result.executions, &result); err != nil {
			return err
		}
		rootIDs := make([]string, len(scope.RootIssues))
		for index, root := range scope.RootIssues {
			rootIDs[index] = root.ID
		}
		if err = tx.Where("task_id IN ?", rootIDs).Order("created_at asc, id asc").Find(&result.taskAgents).Error; err != nil {
			return err
		}
		if result.coordinationBindings, err = readCoordinationBindings(tx, rootIDs); err != nil {
			return err
		}
		if result.coordinationEvents, err = readDurableTable(tx, "coordination_events", "coordination_id IN ?", rootIDs); err != nil {
			return err
		}
		if result.coordinationEffects, err = readDurableTable(tx, "coordination_effects", "coordination_id IN ?", rootIDs); err != nil {
			return err
		}
		if result.coordinationRuns, err = readRelevantCoordinationExecutions(tx, append(append([]string(nil), issueIDs...), executionIDs...), rootIDs); err != nil {
			return err
		}
		if result.runtimeEvents, err = readAgentRuntimeEvents(tx, executionIDs); err != nil {
			return err
		}
		if result.phoneSessions, result.phoneAudits, err = readPhoneEvidence(tx, rootIDs, executionIDs); err != nil {
			return err
		}
		result.evaluation = buildEvaluationContext(scope, result.issues, result.executions, result.validations)
		result.from, result.to = evidenceBounds(result.issues, result.executions)
		return nil
	})
	if err != nil {
		return taskEvidence{}, nil, err
	}
	if system := m.store.Observability(); system != nil && system.Logs != nil {
		issueIDs := issueIDsOf(result.issues)
		executionIDs := executionIDsOf(result.executions)
		taskID := ""
		if result.scope.Task != nil {
			taskID = result.scope.Task.ID
		}
		rootCoordinationIDs := issueIDsOf(result.scope.RootIssues)
		logs, logErr := system.Logs.Query(ctx, observabilitysqlite.Filter{TaskID: taskID, IssueIDs: issueIDs, ExecutionIDs: executionIDs, CoordinationIDs: rootCoordinationIDs, MatchAnyScope: true, Limit: 100000})
		if logErr != nil {
			warnings = append(warnings, "Observability logs could not be loaded: "+logErr.Error())
		} else {
			result.logs = logs
		}
	} else {
		warnings = append(warnings, "Persistent observability logging was not enabled for this process.")
	}
	return result, warnings, nil
}

func resolveTaskExportScope(tx *gorm.DB, requestedID string) (TaskEvidenceScope, error) {
	var task Task
	if err := tx.First(&task, "id = ?", requestedID).Error; err == nil {
		var roots []Issue
		if err := tx.Where("task_source_id = ? AND parent_id = ?", task.ID, "").Order("created_at asc, number asc").Find(&roots).Error; err != nil {
			return TaskEvidenceScope{}, err
		}
		return TaskEvidenceScope{Task: &task, RootIssues: roots}, nil
	}
	var issue Issue
	if err := tx.First(&issue, "id = ?", requestedID).Error; err != nil {
		return TaskEvidenceScope{}, ErrTaskEvidenceNotFound
	}
	for issue.ParentID != "" {
		if err := tx.First(&issue, "id = ?", issue.ParentID).Error; err != nil {
			return TaskEvidenceScope{}, err
		}
	}
	scope := TaskEvidenceScope{RootIssues: []Issue{issue}}
	if issue.TaskSourceID != "" && tx.First(&task, "id = ?", issue.TaskSourceID).Error == nil {
		scope.Task = &task
	}
	return scope, nil
}

func taskTreeIssues(tx *gorm.DB, roots []Issue) ([]Issue, error) {
	result := append([]Issue(nil), roots...)
	frontier := issueIDsOf(roots)
	seen := map[string]bool{}
	for _, id := range frontier {
		seen[id] = true
	}
	for len(frontier) > 0 {
		var children []Issue
		if err := tx.Where("parent_id IN ?", frontier).Order("number asc").Find(&children).Error; err != nil {
			return nil, err
		}
		frontier = frontier[:0]
		for _, child := range children {
			if seen[child.ID] {
				continue
			}
			seen[child.ID] = true
			result = append(result, child)
			frontier = append(frontier, child.ID)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Number < result[j].Number })
	return result, nil
}

func queryByScope(tx *gorm.DB, destination any, issueIDs, executionIDs []string, order string) error {
	query := tx.Where("issue_id IN ?", issueIDs)
	if len(executionIDs) > 0 {
		query = query.Or("execution_id IN ?", executionIDs)
	}
	return query.Order(order).Find(destination).Error
}

func collectRelay(tx *gorm.DB, issueIDs []string, result *taskEvidence) error {
	if err := tx.Where("issue_id IN ?", issueIDs).Order("created_at asc, id asc").Find(&result.relayMessages).Error; err != nil {
		return err
	}
	threadIDs := make([]string, 0, len(result.relayMessages))
	messageIDs := make([]string, 0, len(result.relayMessages))
	for _, message := range result.relayMessages {
		threadIDs = append(threadIDs, message.ThreadID)
		messageIDs = append(messageIDs, message.ID)
	}
	threadIDs = uniqueStrings(threadIDs)
	messageIDs = uniqueStrings(messageIDs)
	if len(threadIDs) > 0 {
		if err := tx.Where("id IN ?", threadIDs).Order("created_at asc").Find(&result.relayThreads).Error; err != nil {
			return err
		}
	}
	if len(messageIDs) > 0 {
		if err := tx.Where("message_id IN ?", messageIDs).Order("created_at asc").Find(&result.relayReceipts).Error; err != nil {
			return err
		}
	}
	return nil
}

func collectAttachments(tx *gorm.DB, dataDir string, scope TaskEvidenceScope, issueIDs, executionIDs []string, destination *[]TaskAttachmentEvidence) error {
	var outputs []IssueAttachment
	if err := queryByScope(tx, &outputs, issueIDs, executionIDs, "created_at asc, id asc"); err != nil {
		return err
	}
	for _, item := range outputs {
		storagePath, _ := evidenceStoragePath(dataDir, item.StoragePath)
		*destination = append(*destination, TaskAttachmentEvidence{Kind: "output", ID: item.ID, IssueID: item.IssueID, ExecutionID: item.ExecutionID, Name: item.Name, MimeType: item.MimeType, DeclaredSize: item.Size, CreatedAt: item.CreatedAt, SourcePath: item.SourcePath, Description: item.Description, storagePath: storagePath})
	}
	var inputs []InputAttachment
	query := tx.Where("issue_id IN ?", issueIDs)
	if len(executionIDs) > 0 {
		query = query.Or("execution_id IN ?", executionIDs)
	}
	if scope.Task != nil {
		query = query.Or("task_id = ?", scope.Task.ID)
	}
	if err := query.Order("created_at asc, id asc").Find(&inputs).Error; err != nil {
		return err
	}
	for _, item := range inputs {
		storagePath, _ := evidenceStoragePath(dataDir, item.StoragePath)
		*destination = append(*destination, TaskAttachmentEvidence{Kind: "input", ID: item.ID, IssueID: item.IssueID, ExecutionID: item.ExecutionID, Name: item.Name, MimeType: item.MimeType, DeclaredSize: item.Size, CreatedAt: item.CreatedAt, BoundAt: item.BoundAt, storagePath: storagePath})
	}
	return nil
}

func evidenceStoragePath(dataDir, stored string) (string, error) {
	base, err := filepath.Abs(dataDir)
	if err != nil {
		return "", err
	}
	candidate := strings.TrimSpace(stored)
	if candidate == "" {
		return "", errors.New("empty storage path")
	}
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, candidate)
	}
	candidate, err = filepath.Abs(candidate)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(base, candidate)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("storage path escapes data directory")
	}
	return candidate, nil
}

func collectRegistryContext(tx *gorm.DB, issues []Issue, executions []Execution, result *taskEvidence) error {
	agentIDs := map[string]bool{}
	for _, issue := range issues {
		if issue.AssigneeAgentID != "" {
			agentIDs[issue.AssigneeAgentID] = true
		}
	}
	for _, execution := range executions {
		if execution.AgentID != "" {
			agentIDs[execution.AgentID] = true
		}
	}
	skillIDs, knowledgeIDs := map[string]bool{}, map[string]bool{}
	var agentRows []agentRecord
	if err := tx.Order("created_at asc, id asc").Find(&agentRows).Error; err != nil {
		return err
	}
	for _, row := range agentRows {
		agent := row.Definition
		if !agentIDs[agent.ID] {
			continue
		}
		result.agents = append(result.agents, agent)
		for _, id := range agent.SkillIDs {
			skillIDs[id] = true
		}
		for _, id := range agent.KnowledgeBaseIDs {
			knowledgeIDs[id] = true
		}
	}
	var skillRows []skillRecord
	if err := tx.Order("created_at asc, id asc").Find(&skillRows).Error; err != nil {
		return err
	}
	for _, row := range skillRows {
		skill := row.Definition
		if skillIDs[skill.ID] {
			result.skills = append(result.skills, skill)
		}
	}
	if len(knowledgeIDs) > 0 {
		ids := mapKeys(knowledgeIDs)
		if err := tx.Where("id IN ?", ids).Order("created_at asc").Find(&result.knowledgeBases).Error; err != nil {
			return err
		}
		if err := tx.Where("knowledge_base_id IN ?", ids).Order("created_at asc").Find(&result.knowledgeDocuments).Error; err != nil {
			return err
		}
	}
	return nil
}

func readCoordinationBindings(tx *gorm.DB, coordinationIDs []string) ([]coordinationBindingEvidence, error) {
	if len(coordinationIDs) == 0 || !tx.Migrator().HasTable("coordination_bindings") {
		return nil, nil
	}
	type row struct {
		CoordinationID string
		Mode           string
		Version        string
		Config         string
		UpdatedAt      time.Time
	}
	var rows []row
	if err := tx.Table("coordination_bindings").Where("coordination_id IN ?", coordinationIDs).Order("updated_at asc, coordination_id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]coordinationBindingEvidence, len(rows))
	for index, item := range rows {
		result[index] = coordinationBindingEvidence{CoordinationID: item.CoordinationID, Mode: item.Mode, Version: item.Version, Config: validRawJSON(item.Config), UpdatedAt: item.UpdatedAt}
	}
	return result, nil
}

func readDurableTable(tx *gorm.DB, table, condition string, values any) ([]durableRuntimeRecord, error) {
	if !tx.Migrator().HasTable(table) {
		return nil, nil
	}
	type row struct {
		ID, Status, EventID, CoordinationID, LastError, Payload string
		LeaseOwner                                              string
		AvailableAt, CreatedAt, UpdatedAt, LeaseExpiresAt       *time.Time
		Attempts                                                int
	}
	var rows []row
	query := tx.Table(table)
	if condition != "" {
		query = query.Where(condition, values)
	}
	if err := query.Order("created_at asc, id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]durableRuntimeRecord, len(rows))
	for index, item := range rows {
		result[index] = durableRuntimeRecord{ID: item.ID, Status: item.Status, EventID: item.EventID, CoordinationID: item.CoordinationID, AvailableAt: item.AvailableAt, CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt, Attempts: item.Attempts, LeaseOwner: item.LeaseOwner, LeaseExpiresAt: item.LeaseExpiresAt, LastError: item.LastError, Payload: validRawJSON(item.Payload)}
	}
	return result, nil
}

func readRelevantCoordinationExecutions(tx *gorm.DB, ids, rootIDs []string) ([]durableRuntimeRecord, error) {
	items, err := readDurableTable(tx, "coordination_executions", "", nil)
	if err != nil {
		return nil, err
	}
	needles := append(append([]string(nil), ids...), rootIDs...)
	result := make([]durableRuntimeRecord, 0)
	for _, item := range items {
		if containsJSONIdentity(item.Payload, item.ID, needles) {
			result = append(result, item)
		}
	}
	return result, nil
}

func readAgentRuntimeEvents(tx *gorm.DB, executionIDs []string) ([]durableRuntimeRecord, error) {
	if len(executionIDs) == 0 || !tx.Migrator().HasTable("agent_runtime_events") {
		return nil, nil
	}
	type row struct {
		ExecutionID string
		Sequence    uint64
		Attempt     int
		CreatedAt   time.Time
		Payload     string
	}
	var rows []row
	if err := tx.Table("agent_runtime_events").Where("execution_id IN ?", executionIDs).Order("execution_id asc, sequence asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	result := make([]durableRuntimeRecord, len(rows))
	for index, item := range rows {
		created := item.CreatedAt
		result[index] = durableRuntimeRecord{ID: fmt.Sprintf("%s:%d", item.ExecutionID, item.Sequence), Status: fmt.Sprintf("attempt-%d", item.Attempt), CreatedAt: &created, Payload: validRawJSON(item.Payload)}
	}
	return result, nil
}

func readPhoneEvidence(tx *gorm.DB, taskIDs, executionIDs []string) ([]phoneSessionEvidence, []phoneAuditEvidence, error) {
	if len(taskIDs) == 0 && len(executionIDs) == 0 || !tx.Migrator().HasTable("agent_app_phone_sessions") {
		return nil, nil, nil
	}
	type sessionRow struct {
		ID, TaskID, TaskAgentID, AgentID, ActorJSON, InstalledJSON, ActiveAppID, StacksJSON, DraftsJSON, CurrentJSON, ResultsJSON string
		CreatedAt, UpdatedAt                                                                                                      time.Time
	}
	var rows []sessionRow
	if err := tx.Table("agent_app_phone_sessions").Order("created_at asc").Find(&rows).Error; err != nil {
		return nil, nil, err
	}
	sessions := make([]phoneSessionEvidence, 0)
	ids := make([]string, 0)
	taskSet := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		taskSet[strings.TrimSpace(taskID)] = struct{}{}
	}
	for _, item := range rows {
		_, taskMatch := taskSet[strings.TrimSpace(item.TaskID)]
		if !taskMatch && !containsJSONIdentity(json.RawMessage(item.ActorJSON), "", executionIDs) {
			continue
		}
		sessions = append(sessions, phoneSessionEvidence{ID: item.ID, TaskID: item.TaskID, TaskAgentID: item.TaskAgentID, AgentID: item.AgentID, Actor: validRawJSON(item.ActorJSON), Installed: validRawJSON(item.InstalledJSON), ActiveAppID: item.ActiveAppID, Stacks: validRawJSON(item.StacksJSON), Drafts: validRawJSON(item.DraftsJSON), Current: validRawJSON(item.CurrentJSON), Results: validRawJSON(item.ResultsJSON), CreatedAt: item.CreatedAt, UpdatedAt: item.UpdatedAt})
		ids = append(ids, item.ID)
	}
	if len(ids) == 0 || !tx.Migrator().HasTable("agent_app_audit_events") {
		return sessions, nil, nil
	}
	type auditRow struct {
		ID                                                                                                                                string
		OccurredAt                                                                                                                        time.Time
		AgentID, PhoneSessionID, AppID, PageID, PageRevision, Ref, Action, Target, Result, Effect, ErrorCode, ErrorMessage, ArgumentsJSON string
	}
	var auditRows []auditRow
	if err := tx.Table("agent_app_audit_events").Where("phone_session_id IN ?", ids).Order("occurred_at asc, id asc").Find(&auditRows).Error; err != nil {
		return nil, nil, err
	}
	audits := make([]phoneAuditEvidence, len(auditRows))
	for index, item := range auditRows {
		audits[index] = phoneAuditEvidence{ID: item.ID, OccurredAt: item.OccurredAt, AgentID: item.AgentID, PhoneSessionID: item.PhoneSessionID, AppID: item.AppID, PageID: item.PageID, PageRevision: item.PageRevision, Ref: item.Ref, Action: item.Action, Target: item.Target, Result: item.Result, Effect: item.Effect, ErrorCode: item.ErrorCode, ErrorMessage: item.ErrorMessage, Arguments: validRawJSON(item.ArgumentsJSON)}
	}
	return sessions, audits, nil
}

func buildEvaluationContext(scope TaskEvidenceScope, issues []Issue, executions []Execution, validations []IssueValidation) TaskEvaluationContext {
	result := TaskEvaluationContext{RootIssueIDs: make([]string, len(scope.RootIssues)), FinalResults: map[string]string{}, IssueStatusCounts: map[string]int{}, ExecutionStatusCounts: map[string]int{}, ValidationStatusCounts: map[string]int{}}
	if scope.Task != nil {
		result.TaskID = scope.Task.ID
		if scope.Task.Objective != "" {
			result.Objectives = append(result.Objectives, scope.Task.Objective)
		}
	}
	for index, root := range scope.RootIssues {
		result.RootIssueIDs[index] = root.ID
		if root.Objective != "" && !slicesContains(result.Objectives, root.Objective) {
			result.Objectives = append(result.Objectives, root.Objective)
		}
		if root.Result != "" {
			result.FinalResults[root.ID] = root.Result
		}
	}
	for _, issue := range issues {
		result.IssueStatusCounts[issue.Status]++
		switch issue.Status {
		case "failed":
			result.FailedIssueIDs = append(result.FailedIssueIDs, issue.ID)
		case "cancelled":
			result.CancelledIssueIDs = append(result.CancelledIssueIDs, issue.ID)
		case "done":
		default:
			result.UnfinishedIssueIDs = append(result.UnfinishedIssueIDs, issue.ID)
		}
	}
	for _, execution := range executions {
		result.ExecutionStatusCounts[execution.Status]++
		result.TotalTokens += execution.Tokens
		result.TotalCostUSD += execution.Cost
	}
	for _, validation := range validations {
		result.ValidationStatusCounts[validation.Status]++
	}
	result.StartedAt, result.FinishedAt = evidenceBounds(issues, executions)
	if result.StartedAt != nil && result.FinishedAt != nil {
		result.DurationSeconds = result.FinishedAt.Sub(*result.StartedAt).Seconds()
	}
	return result
}

func evidenceBounds(issues []Issue, executions []Execution) (*time.Time, *time.Time) {
	var from, to time.Time
	consider := func(value time.Time) {
		if value.IsZero() {
			return
		}
		if from.IsZero() || value.Before(from) {
			from = value
		}
		if to.IsZero() || value.After(to) {
			to = value
		}
	}
	for _, issue := range issues {
		consider(issue.CreatedAt)
		consider(issue.UpdatedAt)
		if issue.CompletedAt != nil {
			consider(*issue.CompletedAt)
		}
		if issue.CancelledAt != nil {
			consider(*issue.CancelledAt)
		}
	}
	for _, execution := range executions {
		consider(execution.StartedAt)
		consider(execution.UpdatedAt)
		if execution.FinishedAt != nil {
			consider(*execution.FinishedAt)
		}
	}
	if from.IsZero() {
		return nil, nil
	}
	return &from, &to
}

func groupConversations(executions []Execution, messages []Message) []TaskConversation {
	byExecution := map[string][]Message{}
	for _, message := range messages {
		byExecution[message.ExecutionID] = append(byExecution[message.ExecutionID], message)
	}
	result := make([]TaskConversation, len(executions))
	for index, execution := range executions {
		result[index] = TaskConversation{Execution: execution, Messages: byExecution[execution.ID]}
	}
	return result
}

func (e taskEvidence) counts() map[string]int {
	return map[string]int{"rootIssues": len(e.scope.RootIssues), "issues": len(e.issues), "taskAgents": len(e.taskAgents), "relations": len(e.relations), "executions": len(e.executions), "messages": countConversationMessages(e.conversations), "executionEvents": len(e.events), "runtimeEvents": len(e.runtimeEvents), "progress": len(e.progress), "comments": len(e.comments), "approvals": len(e.approvals), "validations": len(e.validations), "wakeups": len(e.wakeups), "decompositions": len(e.decompositions), "childWaits": len(e.waits), "relayMessages": len(e.relayMessages), "coordinationEvents": len(e.coordinationEvents), "coordinationEffects": len(e.coordinationEffects), "coordinationExecutions": len(e.coordinationRuns), "phoneAuditEvents": len(e.phoneAudits), "observabilityLogs": len(e.logs), "attachments": len(e.attachments)}
}

func addArtifactFile(archive *zip.Writer, attachment *TaskAttachmentEvidence) (TaskEvidenceManifestFile, error) {
	attachment.Available = false
	path := strings.TrimSpace(attachment.storagePath)
	if path == "" {
		return TaskEvidenceManifestFile{}, fmt.Errorf("attachment %s (%s) has no storage path", attachment.ID, attachment.Name)
	}
	file, err := os.Open(path)
	if err != nil {
		return TaskEvidenceManifestFile{}, fmt.Errorf("attachment %s (%s) is unavailable: %w", attachment.ID, attachment.Name, err)
	}
	defer file.Close()
	name := safeArchiveName(attachment.Name)
	archivePath := fmt.Sprintf("artifacts/%s/%s/%s", attachment.Kind, safeArchiveName(attachment.ID), name)
	header := &zip.FileHeader{Name: archivePath, Method: zip.Deflate, Modified: attachment.CreatedAt}
	header.SetMode(0o600)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return TaskEvidenceManifestFile{}, err
	}
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(writer, hash), file)
	if err != nil {
		return TaskEvidenceManifestFile{}, err
	}
	attachment.ArchivePath, attachment.SHA256, attachment.ExportedSize, attachment.Available = archivePath, hex.EncodeToString(hash.Sum(nil)), size, true
	return TaskEvidenceManifestFile{Path: archivePath, Category: "artifact", MediaType: fallback(attachment.MimeType, "application/octet-stream"), Size: size, SHA256: attachment.SHA256, RecordCount: 1}, nil
}

func addBytesFile(archive *zip.Writer, path, category, mediaType string, data []byte, count int) (TaskEvidenceManifestFile, error) {
	header := &zip.FileHeader{Name: path, Method: zip.Deflate, Modified: time.Now().UTC()}
	header.SetMode(0o600)
	writer, err := archive.CreateHeader(header)
	if err != nil {
		return TaskEvidenceManifestFile{}, err
	}
	if _, err := writer.Write(data); err != nil {
		return TaskEvidenceManifestFile{}, err
	}
	digest := sha256.Sum256(data)
	return TaskEvidenceManifestFile{Path: path, Category: category, MediaType: mediaType, Size: int64(len(data)), SHA256: hex.EncodeToString(digest[:]), RecordCount: count}, nil
}

func exportJSON(value any, redactor *observability.Redactor, redact bool) ([]byte, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	if !redact {
		var pretty bytes.Buffer
		if err := json.Indent(&pretty, encoded, "", "  "); err != nil {
			return nil, err
		}
		pretty.WriteByte('\n')
		return pretty.Bytes(), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return nil, err
	}
	document = redactDocument(redactor, "", document)
	result, err := json.MarshalIndent(document, "", "  ")
	return append(result, '\n'), err
}

func redactDocument(redactor *observability.Redactor, key string, value any) any {
	switch typed := value.(type) {
	case string:
		return redactor.String(key, typed)
	case []any:
		for index := range typed {
			typed[index] = redactDocument(redactor, key, typed[index])
		}
		return typed
	case map[string]any:
		for childKey, child := range typed {
			typed[childKey] = redactDocument(redactor, childKey, child)
		}
		return typed
	default:
		return value
	}
}

func validRawJSON(value string) json.RawMessage {
	value = strings.TrimSpace(value)
	if json.Valid([]byte(value)) {
		return json.RawMessage(value)
	}
	encoded, _ := json.Marshal(value)
	return encoded
}

func containsJSONIdentity(payload json.RawMessage, id string, identities []string) bool {
	if id != "" {
		for _, identity := range identities {
			if id == identity {
				return true
			}
		}
	}
	text := string(payload)
	for _, identity := range identities {
		if identity != "" && strings.Contains(text, `"`+identity+`"`) {
			return true
		}
	}
	return false
}

func issueIDsOf(issues []Issue) []string {
	result := make([]string, len(issues))
	for i := range issues {
		result[i] = issues[i].ID
	}
	return result
}
func executionIDsOf(items []Execution) []string {
	result := make([]string, len(items))
	for i := range items {
		result[i] = items[i].ID
	}
	return result
}
func mapKeys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for key := range values {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
func slicesContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
func countConversationMessages(items []TaskConversation) int {
	total := 0
	for _, item := range items {
		total += len(item.Messages)
	}
	return total
}
func safeArchiveName(value string) string {
	value = filepath.Base(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == 0 {
			return '_'
		}
		return r
	}, value)
	if value == "" || value == "." {
		return "unnamed"
	}
	return value
}

package control

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

const maxAttachmentSize = 100 << 20

func (m *Manager) PublishExecutionAttachment(executionID, token string, input PublishAttachmentInput) (IssueAttachment, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || len(token) != len(session.controlToken) || !secureEqual(token, session.controlToken) {
		return IssueAttachment{}, errors.New("invalid execution control token")
	}
	issue, err := m.store.GetIssue(session.issueID)
	if err != nil {
		return IssueAttachment{}, err
	}
	attachment, err := m.store.captureAttachment(issue, executionID, input)
	if err != nil {
		return IssueAttachment{}, err
	}
	m.store.addEvent(executionID, issue.ID, "attachment", "Agent 已发布附件", attachment.Name)
	m.store.notify()
	return attachment, nil
}

func secureEqual(left, right string) bool {
	if len(left) != len(right) {
		return false
	}
	var mismatch byte
	for index := range left {
		mismatch |= left[index] ^ right[index]
	}
	return mismatch == 0
}

func (m *Manager) collectExecutionAttachments(issue Issue, executionID string) {
	var events []ExecutionEvent
	if err := m.store.db.Where(
		"execution_id = ? AND type = ? AND tool_name = ? AND status = ? AND is_error = ?",
		executionID, "tool", "write", "completed", false,
	).Order("created_at asc").Find(&events).Error; err != nil {
		return
	}
	for _, event := range events {
		var input map[string]any
		if json.Unmarshal([]byte(event.InputJSON), &input) != nil {
			continue
		}
		path := firstString(input, "path", "filePath", "file_path")
		if path == "" || !autoPublishableAttachment(path) {
			continue
		}
		_, _ = m.store.captureAttachment(issue, executionID, PublishAttachmentInput{Path: path})
	}
}

func autoPublishableAttachment(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".tsv", ".ppt", ".pptx", ".zip", ".tar", ".gz", ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
}

func firstString(values map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func (s *Store) captureAttachment(issue Issue, executionID string, input PublishAttachmentInput) (IssueAttachment, error) {
	if strings.TrimSpace(executionID) == "" {
		return IssueAttachment{}, errors.New("execution id is required")
	}
	source, relative, info, err := resolveWorkspaceAttachment(issue.Workspace, input.Path)
	if err != nil {
		return IssueAttachment{}, err
	}
	if info.Size() > maxAttachmentSize {
		return IssueAttachment{}, fmt.Errorf("附件不能超过 %d MB", maxAttachmentSize>>20)
	}
	var existing IssueAttachment
	if err := s.db.Where("execution_id = ? AND source_path = ?", executionID, relative).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return IssueAttachment{}, err
	}

	name := strings.TrimSpace(input.Name)
	if name == "" {
		name = filepath.Base(source)
	} else {
		name = filepath.Base(name)
	}
	if name == "." || name == string(filepath.Separator) || name == "" {
		return IssueAttachment{}, errors.New("附件名称无效")
	}
	id := nextID("attachment")
	storageRelative := filepath.Join("artifacts", id, name)
	storageAbsolute := filepath.Join(s.dataDir, storageRelative)
	if err := os.MkdirAll(filepath.Dir(storageAbsolute), 0o700); err != nil {
		return IssueAttachment{}, err
	}
	if err := copyAttachmentFile(source, storageAbsolute); err != nil {
		_ = os.RemoveAll(filepath.Dir(storageAbsolute))
		return IssueAttachment{}, err
	}
	attachment := IssueAttachment{
		ID: id, IssueID: issue.ID, ExecutionID: executionID, Name: name,
		Description: strings.TrimSpace(input.Description), SourcePath: relative,
		StoragePath: storageRelative, MimeType: attachmentMimeType(name), Size: info.Size(), CreatedAt: time.Now(),
	}
	if err := s.db.Create(&attachment).Error; err != nil {
		_ = os.RemoveAll(filepath.Dir(storageAbsolute))
		return IssueAttachment{}, err
	}
	return attachment, nil
}

func resolveWorkspaceAttachment(workspace, candidate string) (string, string, os.FileInfo, error) {
	workspace = strings.TrimSpace(workspace)
	candidate = strings.TrimSpace(candidate)
	if workspace == "" || candidate == "" {
		return "", "", nil, errors.New("附件路径不能为空")
	}
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", nil, err
	}
	source := candidate
	if !filepath.IsAbs(source) {
		source = filepath.Join(workspaceAbs, source)
	}
	source, err = filepath.Abs(source)
	if err != nil {
		return "", "", nil, err
	}
	if !pathWithin(workspaceAbs, source) {
		return "", "", nil, errors.New("附件必须位于 Issue 工作目录中")
	}
	resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs)
	if err != nil {
		return "", "", nil, err
	}
	resolvedSource, err := filepath.EvalSymlinks(source)
	if err != nil {
		return "", "", nil, fmt.Errorf("附件文件不存在: %w", err)
	}
	if !pathWithin(resolvedWorkspace, resolvedSource) {
		return "", "", nil, errors.New("附件符号链接不能指向工作目录之外")
	}
	info, err := os.Stat(resolvedSource)
	if err != nil {
		return "", "", nil, err
	}
	if !info.Mode().IsRegular() {
		return "", "", nil, errors.New("附件必须是普通文件")
	}
	relative, err := filepath.Rel(workspaceAbs, source)
	if err != nil {
		return "", "", nil, err
	}
	return resolvedSource, filepath.ToSlash(relative), info, nil
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func copyAttachmentFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(out, io.LimitReader(in, maxAttachmentSize+1))
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if written > maxAttachmentSize {
		return fmt.Errorf("附件不能超过 %d MB", maxAttachmentSize>>20)
	}
	return closeErr
}

func attachmentMimeType(name string) string {
	if value := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); value != "" {
		return value
	}
	return "application/octet-stream"
}

func (s *Store) bindExecutionAttachments(tx *gorm.DB, comment *IssueComment, executionID string) error {
	if executionID == "" {
		comment.Attachments = []IssueAttachment{}
		return nil
	}
	if err := tx.Model(&IssueAttachment{}).
		Where("execution_id = ? AND comment_id = ''", executionID).
		Update("comment_id", comment.ID).Error; err != nil {
		return err
	}
	return tx.Where("comment_id = ?", comment.ID).Order("created_at asc").Find(&comment.Attachments).Error
}

func (s *Store) AttachmentFile(id string) (IssueAttachment, *os.File, error) {
	var attachment IssueAttachment
	if err := s.db.First(&attachment, "id = ?", id).Error; err != nil {
		return IssueAttachment{}, nil, errors.New("attachment not found")
	}
	root := filepath.Join(s.dataDir, "artifacts")
	path := filepath.Join(s.dataDir, attachment.StoragePath)
	if !pathWithin(root, path) {
		return IssueAttachment{}, nil, errors.New("invalid attachment storage path")
	}
	file, err := os.Open(path)
	if err != nil {
		return IssueAttachment{}, nil, err
	}
	return attachment, file, nil
}

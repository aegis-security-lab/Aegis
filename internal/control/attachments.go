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
const maxValidationAttachmentChunk = 32 << 10

func (m *Manager) PublishExecutionAttachment(executionID, token string, input PublishAttachmentInput) (IssueAttachment, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || len(token) != len(session.controlToken) || !secureEqual(token, session.controlToken) {
		return IssueAttachment{}, errors.New("invalid execution control token")
	}
	issue, err := m.store.GetIssue(session.issueID)
	if err != nil {
		return IssueAttachment{}, err
	}
	input.Path = m.hostWorkspacePath(issue, input.Path)
	attachment, err := m.store.captureAttachment(issue, executionID, input)
	if err != nil {
		return IssueAttachment{}, err
	}
	m.store.addEvent(executionID, issue.ID, "attachment", "Agent 已发布附件", attachment.Name)
	m.store.notify()
	return attachment, nil
}

func (m *Manager) ValidationAttachments(executionID, token string) ([]ValidationAttachmentInfo, error) {
	validation, err := m.validationAttachmentContext(executionID, token)
	if err != nil {
		return nil, err
	}
	return m.validationAttachmentInfos(validation.SourceExecutionID)
}

func (m *Manager) ReadValidationAttachment(executionID, token, attachmentID string, offset int64, limit int) (ValidationAttachmentChunk, error) {
	validation, err := m.validationAttachmentContext(executionID, token)
	if err != nil {
		return ValidationAttachmentChunk{}, err
	}
	attachmentID = strings.TrimSpace(attachmentID)
	var attachment IssueAttachment
	if attachmentID == "" || m.store.db.First(&attachment, "id = ? AND issue_id = ? AND execution_id = ?", attachmentID, validation.IssueID, validation.SourceExecutionID).Error != nil {
		return ValidationAttachmentChunk{}, errors.New("validation attachment not found")
	}
	info := m.validationAttachmentInfo(attachment)
	if !info.Readable {
		return ValidationAttachmentChunk{}, errors.New("该附件不是可按文本读取的格式，请根据附件元信息判断或要求 Worker 提供可读取版本")
	}
	if offset < 0 || offset > attachment.Size {
		return ValidationAttachmentChunk{}, errors.New("附件读取 offset 超出范围")
	}
	if limit <= 0 {
		limit = 16 << 10
	}
	if limit > maxValidationAttachmentChunk {
		limit = maxValidationAttachmentChunk
	}
	_, file, err := m.store.AttachmentFile(attachment.ID)
	if err != nil {
		return ValidationAttachmentChunk{}, err
	}
	defer file.Close()
	if _, err = file.Seek(offset, io.SeekStart); err != nil {
		return ValidationAttachmentChunk{}, err
	}
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)))
	if err != nil {
		return ValidationAttachmentChunk{}, err
	}
	next := offset + int64(len(data))
	return ValidationAttachmentChunk{
		Attachment: info, Offset: offset, NextOffset: next,
		Content: strings.ToValidUTF8(string(data), "�"), EOF: next >= attachment.Size,
	}, nil
}

func (m *Manager) validationAttachmentContext(executionID, token string) (IssueValidation, error) {
	session := m.getSession(executionID)
	if session == nil || session.kind != "validation" || token == "" || !secureEqual(token, session.controlToken) {
		return IssueValidation{}, errors.New("invalid validation execution control token")
	}
	var validation IssueValidation
	if err := m.store.db.First(&validation, "validation_execution_id = ? AND status = ?", executionID, "running").Error; err != nil {
		return IssueValidation{}, errors.New("active validation not found")
	}
	return validation, nil
}

func (m *Manager) validationAttachmentInfos(sourceExecutionID string) ([]ValidationAttachmentInfo, error) {
	var attachments []IssueAttachment
	if err := m.store.db.Where("execution_id = ?", sourceExecutionID).Order("created_at asc").Find(&attachments).Error; err != nil {
		return nil, err
	}
	infos := make([]ValidationAttachmentInfo, len(attachments))
	for index, attachment := range attachments {
		infos[index] = m.validationAttachmentInfo(attachment)
	}
	return infos, nil
}

func (m *Manager) validationAttachmentInfo(attachment IssueAttachment) ValidationAttachmentInfo {
	return ValidationAttachmentInfo{
		ID: attachment.ID, Name: attachment.Name, Description: attachment.Description,
		MimeType: attachment.MimeType, Size: attachment.Size,
		DownloadURL: strings.TrimRight(m.controlURL, "/") + "/api/attachments/" + attachment.ID,
		Readable:    validationAttachmentReadable(attachment),
	}
}

func validationAttachmentReadable(attachment IssueAttachment) bool {
	if strings.HasPrefix(strings.ToLower(attachment.MimeType), "text/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(attachment.Name)) {
	case ".md", ".markdown", ".txt", ".json", ".jsonl", ".csv", ".tsv", ".xml", ".html", ".yaml", ".yml", ".log", ".sql":
		return true
	default:
		return false
	}
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
		path, _ := input["path"].(string)
		path = strings.TrimSpace(path)
		if path == "" || !autoPublishableAttachment(path) {
			continue
		}
		path = m.hostWorkspacePath(issue, path)
		_, _ = m.store.captureAttachment(issue, executionID, PublishAttachmentInput{Path: path})
	}
}

func (m *Manager) hostWorkspacePath(issue Issue, path string) string {
	if issue.ContainerProfileID == "" || !filepath.IsAbs(path) {
		return path
	}
	profile, err := m.store.GetContainerProfile(issue.ContainerProfileID)
	if err != nil {
		return path
	}
	relative, err := filepath.Rel(profile.WorkspacePath, filepath.Clean(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(issue.Workspace, relative)
}

func autoPublishableAttachment(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".csv", ".tsv", ".ppt", ".pptx", ".zip", ".tar", ".gz", ".png", ".jpg", ".jpeg", ".webp", ".gif":
		return true
	default:
		return false
	}
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

// captureGeneratedAttachment persists a control-plane generated artifact without
// pretending that it came from the Issue workspace. sourcePath is a stable,
// non-secret logical path used for idempotency within one Execution.
func (s *Store) captureGeneratedAttachment(issue Issue, executionID, sourcePath, name, description string, data []byte) (IssueAttachment, error) {
	executionID = strings.TrimSpace(executionID)
	sourcePath = strings.TrimSpace(filepath.ToSlash(sourcePath))
	name = filepath.Base(strings.TrimSpace(name))
	if executionID == "" || sourcePath == "" {
		return IssueAttachment{}, errors.New("execution id and generated attachment source are required")
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return IssueAttachment{}, errors.New("附件名称无效")
	}
	if len(data) > maxAttachmentSize {
		return IssueAttachment{}, fmt.Errorf("附件不能超过 %d MB", maxAttachmentSize>>20)
	}
	var existing IssueAttachment
	if err := s.db.Where("execution_id = ? AND source_path = ?", executionID, sourcePath).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return IssueAttachment{}, err
	}
	id := nextID("attachment")
	storageRelative := filepath.Join("artifacts", id, name)
	storageAbsolute := filepath.Join(s.dataDir, storageRelative)
	if err := os.MkdirAll(filepath.Dir(storageAbsolute), 0o700); err != nil {
		return IssueAttachment{}, err
	}
	if err := os.WriteFile(storageAbsolute, data, 0o600); err != nil {
		_ = os.RemoveAll(filepath.Dir(storageAbsolute))
		return IssueAttachment{}, err
	}
	attachment := IssueAttachment{
		ID: id, IssueID: issue.ID, ExecutionID: executionID, Name: name,
		Description: strings.TrimSpace(description), SourcePath: sourcePath,
		StoragePath: storageRelative, MimeType: attachmentMimeType(name), Size: int64(len(data)), CreatedAt: time.Now(),
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

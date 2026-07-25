package control

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gorm.io/gorm"
)

type OperatorAttachment struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
}

// UploadOperatorAttachment streams an operator-supplied file directly into
// the task container. No host workspace path is exposed to the Agent.
func (m *Manager) UploadOperatorAttachment(issueID, executionID, name string, source io.Reader, size int64) (OperatorAttachment, error) {
	if source == nil || size < 0 || size > MaxAttachmentSize {
		return OperatorAttachment{}, fmt.Errorf("附件不能超过 %d MB", MaxAttachmentSize>>20)
	}
	issue, err := m.store.GetIssue(issueID)
	if err != nil {
		return OperatorAttachment{}, err
	}
	session := m.getSession(executionID)
	if session == nil || session.issueID != issue.ID {
		return OperatorAttachment{}, errors.New("该 Agent 当前没有连接中的 Pi session")
	}
	if issue.ContainerID == "" {
		return OperatorAttachment{}, errors.New("当前任务没有绑定容器，无法上传附件")
	}
	container, err := m.store.StartContainer(issue.ContainerID)
	if err != nil {
		return OperatorAttachment{}, err
	}
	cleanName := filepath.Base(strings.TrimSpace(name))
	if cleanName == "" || cleanName == "." {
		return OperatorAttachment{}, errors.New("附件名称无效")
	}
	dir := filepath.ToSlash(filepath.Join(container.WorkspacePath, ".aegis", "operator-attachments", nextID("upload")))
	destination := filepath.ToSlash(filepath.Join(dir, cleanName))
	data, err := io.ReadAll(io.LimitReader(source, MaxAttachmentSize+1))
	if err != nil || int64(len(data)) != size {
		return OperatorAttachment{}, errors.New("附件上传内容不完整")
	}
	cmd := exec.Command("docker", "exec", "-i", container.Name, "sh", "-c", `mkdir -p "$1" && cat > "$2"`, "aegis-upload", dir, destination)
	cmd.Stdin = bytes.NewReader(data)
	if output, runErr := cmd.CombinedOutput(); runErr != nil {
		return OperatorAttachment{}, fmt.Errorf("写入任务容器失败: %s", strings.TrimSpace(string(output)))
	}
	return OperatorAttachment{Name: cleanName, Path: destination, Size: size}, nil
}

const MaxAttachmentSize = 100 << 20
const maxValidationAttachmentChunk = 32 << 10

func (m *Manager) SubmitFinalResult(executionID, token string, input SubmitFinalResultInput) (Execution, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || !secureEqual(token, session.controlToken) {
		return Execution{}, errors.New("invalid execution control token")
	}
	input.Body = strings.TrimSpace(input.Body)
	if input.Body == "" {
		return Execution{}, errors.New("最终结果正文不能为空")
	}
	if len([]rune(input.Body)) > 50000 {
		return Execution{}, errors.New("最终结果正文不能超过 50000 个字符")
	}
	issue, err := m.store.GetIssue(session.issueID)
	if err != nil {
		return Execution{}, err
	}
	if strings.TrimSpace(input.Path) != "" {
		return Execution{}, errors.New("附件必须先通过附件工具直传服务端，再提交最终结果")
	}
	if err := m.store.db.Model(&Execution{}).Where("id = ?", executionID).Updates(map[string]any{"final_result": input.Body, "final_result_submitted": true, "result": input.Body}).Error; err != nil {
		return Execution{}, err
	}
	m.store.addEvent(executionID, issue.ID, "delivery", "Agent 已提交最终结果", "最终结果将作为独立交付内容进入验收，不再使用验收回复作为交付正文。")
	m.store.notify()
	var execution Execution
	err = m.store.db.First(&execution, "id = ?", executionID).Error
	return execution, err
}

func (m *Manager) UploadExecutionAttachment(executionID, token string, input PublishAttachmentInput, source io.Reader, size int64) (IssueAttachment, error) {
	session := m.getSession(executionID)
	if session == nil || token == "" || len(token) != len(session.controlToken) || !secureEqual(token, session.controlToken) {
		return IssueAttachment{}, errors.New("invalid execution control token")
	}
	if source == nil {
		return IssueAttachment{}, errors.New("附件内容不能为空")
	}
	issue, err := m.store.GetIssue(session.issueID)
	if err != nil {
		return IssueAttachment{}, err
	}
	attachment, err := m.store.captureUploadedAttachment(issue, executionID, input, source, size)
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

func (s *Store) captureUploadedAttachment(issue Issue, executionID string, input PublishAttachmentInput, source io.Reader, size int64) (IssueAttachment, error) {
	executionID = strings.TrimSpace(executionID)
	sourcePath := strings.TrimSpace(filepath.ToSlash(input.Path))
	if executionID == "" || sourcePath == "" {
		return IssueAttachment{}, errors.New("execution id and attachment source path are required")
	}
	if len(sourcePath) > 4096 {
		return IssueAttachment{}, errors.New("附件来源路径过长")
	}
	if size < 0 || size > MaxAttachmentSize {
		return IssueAttachment{}, fmt.Errorf("附件不能超过 %d MB", MaxAttachmentSize>>20)
	}
	var existing IssueAttachment
	if err := s.db.Where("execution_id = ? AND source_path = ?", executionID, sourcePath).First(&existing).Error; err == nil {
		return existing, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return IssueAttachment{}, err
	}

	name := filepath.Base(strings.TrimSpace(input.Name))
	if name == "" || name == "." || name == string(filepath.Separator) {
		name = filepath.Base(sourcePath)
	}
	if name == "" || name == "." || name == string(filepath.Separator) {
		return IssueAttachment{}, errors.New("附件名称无效")
	}
	id := nextID("attachment")
	storageRelative := filepath.Join("artifacts", id, name)
	storageAbsolute := filepath.Join(s.dataDir, storageRelative)
	if err := os.MkdirAll(filepath.Dir(storageAbsolute), 0o700); err != nil {
		return IssueAttachment{}, err
	}
	out, err := os.OpenFile(storageAbsolute, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		_ = os.RemoveAll(filepath.Dir(storageAbsolute))
		return IssueAttachment{}, err
	}
	written, copyErr := io.Copy(out, io.LimitReader(source, MaxAttachmentSize+1))
	closeErr := out.Close()
	if copyErr != nil || closeErr != nil || written > MaxAttachmentSize || written != size {
		_ = os.RemoveAll(filepath.Dir(storageAbsolute))
		switch {
		case copyErr != nil:
			return IssueAttachment{}, copyErr
		case closeErr != nil:
			return IssueAttachment{}, closeErr
		case written > MaxAttachmentSize:
			return IssueAttachment{}, fmt.Errorf("附件不能超过 %d MB", MaxAttachmentSize>>20)
		default:
			return IssueAttachment{}, errors.New("附件上传内容不完整")
		}
	}
	attachment := IssueAttachment{
		ID: id, IssueID: issue.ID, ExecutionID: executionID, Name: name,
		Description: strings.TrimSpace(input.Description), SourcePath: sourcePath,
		StoragePath: storageRelative, MimeType: attachmentMimeType(name), Size: written, CreatedAt: time.Now(),
	}
	if err = s.db.Create(&attachment).Error; err != nil {
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
	if len(data) > MaxAttachmentSize {
		return IssueAttachment{}, fmt.Errorf("附件不能超过 %d MB", MaxAttachmentSize>>20)
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

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
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

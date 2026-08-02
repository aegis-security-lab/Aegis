package control

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
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
const maxValidationArchiveEntries = 1000
const maxValidationArchiveEntrySize = 10 << 20

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
	var unfinishedChildren int64
	if err := m.store.db.Model(&Issue{}).Where("parent_id = ? AND status NOT IN ?", issue.ID, terminalIssueStatuses).Count(&unfinishedChildren).Error; err != nil {
		return Execution{}, err
	}
	if unfinishedChildren > 0 {
		return Execution{}, fmt.Errorf("仍有 %d 个直属子 Issue 未结束，不能提交最终结果", unfinishedChildren)
	}
	if err := m.store.db.Model(&Execution{}).Where("id = ?", executionID).Updates(map[string]any{"final_result": input.Body, "final_result_submitted": true, "result": input.Body}).Error; err != nil {
		return Execution{}, err
	}
	var execution Execution
	if err = m.store.db.First(&execution, "id = ?", executionID).Error; err != nil {
		return Execution{}, err
	}
	var existing int64
	_ = m.store.db.Model(&IssueComment{}).Where("issue_id = ? AND execution_id = ? AND type = ?", issue.ID, execution.ID, "delivery").Count(&existing).Error
	if existing == 0 {
		if issue.Status == "todo" || issue.Status == "backlog" {
			_ = m.store.db.Model(&Issue{}).Where("id = ?", issue.ID).Updates(map[string]any{
				"status": "in_progress", "execution_phase": "active", "current_execution_id": execution.ID,
				"checkout_execution_id": execution.ID, "started_at": time.Now(), "updated_at": time.Now(),
			}).Error
			issue, _ = m.store.GetIssue(issue.ID)
		}
		if issue.ValidationDisabled || strings.TrimSpace(issue.Objective) == "" {
			m.addTypedAgentComment(issue.ID, execution.AgentID, "delivery", input.Body, execution.ID, []commentWakeupTarget{})
			m.completeIssueWithoutValidation(issue, execution.ID, input.Body, time.Now())
		} else if err = m.publishDeliveryForValidation(issue, execution, execution.AgentID, input.Body); err != nil {
			return Execution{}, err
		}
	}
	m.store.addEvent(executionID, issue.ID, "delivery", "Agent 通过 Board 提交最终结果", "Agent 主动调用交付工具，结果已立即发布到 Issue 并进入后续验收。")
	m.store.notify()
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

func (m *Manager) nativeValidationAttachments(executionID string) ([]ValidationAttachmentInfo, error) {
	validation, err := m.activeValidationForExecution(executionID)
	if err != nil {
		return nil, err
	}
	return m.validationAttachmentInfos(validation.SourceExecutionID)
}

func (m *Manager) ReadValidationAttachment(executionID, token, attachmentID, archivePath string, offset int64, limit int) (ValidationAttachmentChunk, error) {
	validation, err := m.validationAttachmentContext(executionID, token)
	if err != nil {
		return ValidationAttachmentChunk{}, err
	}
	return m.readValidationAttachment(validation, attachmentID, archivePath, offset, limit)
}

func (m *Manager) nativeReadValidationAttachment(executionID, attachmentID, archivePath string, offset int64, limit int) (ValidationAttachmentChunk, error) {
	validation, err := m.activeValidationForExecution(executionID)
	if err != nil {
		return ValidationAttachmentChunk{}, err
	}
	return m.readValidationAttachment(validation, attachmentID, archivePath, offset, limit)
}

func (m *Manager) readValidationAttachment(validation IssueValidation, attachmentID, archivePath string, offset int64, limit int) (ValidationAttachmentChunk, error) {
	attachmentID = strings.TrimSpace(attachmentID)
	var attachment IssueAttachment
	if attachmentID == "" || m.store.db.First(&attachment, "id = ? AND issue_id = ? AND execution_id = ?", attachmentID, validation.IssueID, validation.SourceExecutionID).Error != nil {
		return ValidationAttachmentChunk{}, errors.New("validation attachment not found")
	}
	info := m.validationAttachmentInfo(attachment)
	archivePath = strings.TrimSpace(filepath.ToSlash(archivePath))
	if !info.Readable && archivePath == "" {
		return ValidationAttachmentChunk{}, errors.New("该附件不是可按文本读取的格式，请根据附件元信息判断或要求 Worker 提供可读取版本")
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
	if archivePath != "" {
		return readValidationArchiveEntry(file, attachment.Size, info, archivePath, offset, limit)
	}
	if offset < 0 || offset > attachment.Size {
		return ValidationAttachmentChunk{}, errors.New("附件读取 offset 超出范围")
	}
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
	return m.activeValidationForExecution(executionID)
}

func (m *Manager) activeValidationForExecution(executionID string) (IssueValidation, error) {
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

func (m *Manager) materializeValidationAttachments(sourceExecutionID string, container ContainerInstance) ([]ValidationAttachmentInfo, error) {
	var attachments []IssueAttachment
	if err := m.store.db.Where("execution_id = ?", sourceExecutionID).Order("created_at asc").Find(&attachments).Error; err != nil {
		return nil, err
	}
	infos := make([]ValidationAttachmentInfo, 0, len(attachments))
	for _, attachment := range attachments {
		if info, err := os.Stat(attachment.StoragePath); err != nil || info.Size() != attachment.Size {
			return nil, fmt.Errorf("验收附件 %s 在服务端不存在或不完整", attachment.Name)
		}
		destination := validationAttachmentRuntimePath(attachment)
		if err := materializeContainerValidationAttachment(attachment, container, destination); err != nil {
			return nil, fmt.Errorf("准备验收附件 %s 失败: %w", attachment.Name, err)
		}
		info := m.validationAttachmentInfo(attachment)
		info.Path = destination
		infos = append(infos, info)
	}
	return infos, nil
}

func validationAttachmentRuntimePath(attachment IssueAttachment) string {
	return path.Join(TaskWorkspacePath, ".aegis", "validation-evidence", attachment.ExecutionID, attachment.ID, attachment.Name)
}

func materializeContainerValidationAttachment(attachment IssueAttachment, container ContainerInstance, destination string) error {
	check := exec.Command("docker", "exec", container.Name, "sh", "-c", `test -f "$1" && test "$(wc -c < "$1")" -eq "$2"`, "aegis-validation-check", destination, strconv.FormatInt(attachment.Size, 10))
	if check.Run() == nil {
		return nil
	}
	dir := path.Dir(destination)
	if output, err := exec.Command("docker", "exec", container.Name, "mkdir", "-p", dir).CombinedOutput(); err != nil {
		return fmt.Errorf("创建容器验收附件目录失败: %s", strings.TrimSpace(string(output)))
	}
	temporary := destination + ".partial"
	if output, err := exec.Command("docker", "cp", attachment.StoragePath, container.Name+":"+temporary).CombinedOutput(); err != nil {
		return fmt.Errorf("复制验收附件到容器失败: %s", strings.TrimSpace(string(output)))
	}
	if output, err := exec.Command("docker", "exec", container.Name, "sh", "-c", `mv "$1" "$2" && chmod 0444 "$2"`, "aegis-validation-copy", temporary, destination).CombinedOutput(); err != nil {
		_ = exec.Command("docker", "exec", container.Name, "rm", "-f", temporary).Run()
		return fmt.Errorf("完成容器验收附件写入失败: %s", strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) validationAttachmentInfo(attachment IssueAttachment) ValidationAttachmentInfo {
	info := ValidationAttachmentInfo{
		ID: attachment.ID, Name: attachment.Name, Description: attachment.Description,
		MimeType: attachment.MimeType, Size: attachment.Size,
		Path:        validationAttachmentRuntimePath(attachment),
		DownloadURL: strings.TrimRight(m.controlURL, "/") + "/api/attachments/" + attachment.ID,
		Readable:    validationAttachmentReadable(attachment),
	}
	if strings.EqualFold(filepath.Ext(attachment.Name), ".zip") {
		info.ArchiveEntries = m.validationArchiveEntries(attachment)
	}
	return info
}

func (m *Manager) validationArchiveEntries(attachment IssueAttachment) []ValidationArchiveEntry {
	_, file, err := m.store.AttachmentFile(attachment.ID)
	if err != nil {
		return nil
	}
	defer file.Close()
	reader, err := zip.NewReader(file, attachment.Size)
	if err != nil || len(reader.File) > maxValidationArchiveEntries {
		return nil
	}
	entries := make([]ValidationArchiveEntry, 0, len(reader.File))
	for _, item := range reader.File {
		if item.FileInfo().IsDir() || !safeValidationArchivePath(item.Name) {
			continue
		}
		entries = append(entries, ValidationArchiveEntry{Path: item.Name, Size: int64(item.UncompressedSize64), Readable: validationArchiveEntryReadable(item.Name, item.UncompressedSize64)})
	}
	return entries
}

func readValidationArchiveEntry(file *os.File, archiveSize int64, info ValidationAttachmentInfo, archivePath string, offset int64, limit int) (ValidationAttachmentChunk, error) {
	if !strings.EqualFold(filepath.Ext(info.Name), ".zip") || !safeValidationArchivePath(archivePath) {
		return ValidationAttachmentChunk{}, errors.New("无效的 ZIP 内文件路径")
	}
	reader, err := zip.NewReader(file, archiveSize)
	if err != nil || len(reader.File) > maxValidationArchiveEntries {
		return ValidationAttachmentChunk{}, errors.New("ZIP 附件无效或文件数量超过限制")
	}
	for _, item := range reader.File {
		if item.Name != archivePath {
			continue
		}
		if !validationArchiveEntryReadable(item.Name, item.UncompressedSize64) {
			return ValidationAttachmentChunk{}, errors.New("ZIP 内文件不是受支持的文本格式或解压后过大")
		}
		if offset < 0 || uint64(offset) > item.UncompressedSize64 {
			return ValidationAttachmentChunk{}, errors.New("ZIP 内文件读取 offset 超出范围")
		}
		entry, openErr := item.Open()
		if openErr != nil {
			return ValidationAttachmentChunk{}, openErr
		}
		defer entry.Close()
		if _, err = io.CopyN(io.Discard, entry, offset); err != nil && !errors.Is(err, io.EOF) {
			return ValidationAttachmentChunk{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(entry, int64(limit)))
		if readErr != nil {
			return ValidationAttachmentChunk{}, readErr
		}
		next := offset + int64(len(data))
		return ValidationAttachmentChunk{Attachment: info, ArchivePath: archivePath, Offset: offset, NextOffset: next, Content: strings.ToValidUTF8(string(data), "�"), EOF: uint64(next) >= item.UncompressedSize64}, nil
	}
	return ValidationAttachmentChunk{}, errors.New("ZIP 内文件不存在")
}

func safeValidationArchivePath(name string) bool {
	clean := filepath.ToSlash(filepath.Clean(name))
	return name != "" && clean == filepath.ToSlash(name) && clean != "." && !strings.HasPrefix(clean, "../") && !strings.HasPrefix(clean, "/")
}

func validationArchiveEntryReadable(name string, size uint64) bool {
	if size > maxValidationArchiveEntrySize {
		return false
	}
	return validationAttachmentReadable(IssueAttachment{Name: name})
}

func validationAttachmentReadable(attachment IssueAttachment) bool {
	if strings.HasPrefix(strings.ToLower(attachment.MimeType), "text/") {
		return true
	}
	switch strings.ToLower(filepath.Ext(attachment.Name)) {
	case ".md", ".markdown", ".txt", ".json", ".jsonl", ".csv", ".tsv", ".xml", ".html", ".yaml", ".yml", ".log", ".sql",
		".py", ".js", ".jsx", ".ts", ".tsx", ".go", ".rs", ".java", ".kt", ".kts", ".c", ".h", ".cc", ".cpp", ".cs", ".php", ".rb", ".sh", ".bash", ".zsh", ".fish", ".ps1", ".toml", ".ini", ".conf", ".env":
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

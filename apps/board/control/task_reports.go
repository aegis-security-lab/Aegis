package control

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type taskReportManifest struct {
	SchemaVersion     string                       `json:"schemaVersion"`
	TaskID            string                       `json:"taskId"`
	RootIssueID       string                       `json:"rootIssueId"`
	RootIdentifier    string                       `json:"rootIdentifier"`
	RootTitle         string                       `json:"rootTitle"`
	SourceExecutionID string                       `json:"sourceExecutionId"`
	GeneratedAt       time.Time                    `json:"generatedAt"`
	Attachments       []taskReportManifestArtifact `json:"attachments"`
}

type taskReportManifestArtifact struct {
	Name        string `json:"name"`
	ArchivePath string `json:"archivePath"`
	Description string `json:"description,omitempty"`
	MimeType    string `json:"mimeType"`
	Size        int64  `json:"size"`
}

// publishRootIssueTaskReport appends an immutable lifecycle report made
// exclusively from one successful Worker Execution.
func (m *Manager) publishRootIssueTaskReport(issue Issue, sourceExecutionID string, now time.Time) error {
	if issue.ParentID != "" || strings.TrimSpace(issue.TaskSourceID) == "" {
		return nil
	}
	sourceExecutionID = strings.TrimSpace(sourceExecutionID)
	if sourceExecutionID == "" {
		return errors.New("根 Issue 最终报告缺少来源 Execution")
	}
	var existing TaskReport
	if err := m.store.db.First(&existing, "root_issue_id = ? AND source_execution_id = ?", issue.ID, sourceExecutionID).Error; err == nil {
		return nil
	}
	var attachments []IssueAttachment
	if err := m.store.db.Where("issue_id = ? AND execution_id = ?", issue.ID, sourceExecutionID).Order("created_at asc, id asc").Find(&attachments).Error; err != nil {
		return err
	}
	if len(attachments) == 0 {
		return errors.New("根 Issue 最后一次提交没有可打包的附件")
	}

	reportID := nextID("task-report")
	directory := filepath.Join(m.store.dataDir, "task-reports", reportID)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("创建任务报告目录失败: %w", err)
	}
	title := m.taskReportTitle(issue, sourceExecutionID, now)
	name := safeTaskReportFilename(issue.Identifier, title)
	finalPath := filepath.Join(directory, name)
	temporary, err := os.CreateTemp(directory, ".report-*.zip")
	if err != nil {
		_ = os.RemoveAll(directory)
		return fmt.Errorf("创建任务报告临时文件失败: %w", err)
	}
	temporaryPath := temporary.Name()
	cleanup := func() {
		_ = temporary.Close()
		_ = os.Remove(temporaryPath)
		_ = os.RemoveAll(directory)
	}

	manifest := taskReportManifest{
		SchemaVersion: "aegis.task-report/v1", TaskID: issue.TaskSourceID,
		RootIssueID: issue.ID, RootIdentifier: issue.Identifier, RootTitle: issue.Title,
		SourceExecutionID: sourceExecutionID, GeneratedAt: now,
		Attachments: make([]taskReportManifestArtifact, 0, len(attachments)),
	}
	archive := zip.NewWriter(temporary)
	usedNames := map[string]int{"manifest.json": 1}
	for _, attachment := range attachments {
		storagePath, pathErr := m.store.attachmentStoragePath(attachment)
		if pathErr != nil {
			_ = archive.Close()
			cleanup()
			return pathErr
		}
		archiveName := uniqueArchiveName(attachment.Name, usedNames)
		if err = copyTaskReportAttachment(archive, storagePath, archiveName, attachment.Size); err != nil {
			_ = archive.Close()
			cleanup()
			return fmt.Errorf("打包附件 %s 失败: %w", attachment.Name, err)
		}
		manifest.Attachments = append(manifest.Attachments, taskReportManifestArtifact{
			Name: attachment.Name, ArchivePath: archiveName, Description: attachment.Description,
			MimeType: attachment.MimeType, Size: attachment.Size,
		})
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err == nil {
		var writer io.Writer
		writer, err = archive.Create("manifest.json")
		if err == nil {
			_, err = writer.Write(manifestBytes)
		}
	}
	if closeErr := archive.Close(); err == nil {
		err = closeErr
	}
	if syncErr := temporary.Sync(); err == nil {
		err = syncErr
	}
	if closeErr := temporary.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		cleanup()
		return fmt.Errorf("完成任务报告 ZIP 失败: %w", err)
	}
	if err = os.Rename(temporaryPath, finalPath); err != nil {
		cleanup()
		return fmt.Errorf("发布任务报告 ZIP 失败: %w", err)
	}
	if err = os.Chmod(finalPath, 0o600); err != nil {
		cleanup()
		return fmt.Errorf("保护任务报告 ZIP 失败: %w", err)
	}
	info, err := os.Stat(finalPath)
	if err != nil {
		cleanup()
		return err
	}

	var maxVersion int
	if err = m.store.db.Model(&TaskReport{}).Where("root_issue_id = ?", issue.ID).Select("coalesce(max(version), 0)").Scan(&maxVersion).Error; err != nil {
		cleanup()
		return err
	}
	report := TaskReport{
		ID: reportID, TaskID: issue.TaskSourceID, RootIssueID: issue.ID,
		SourceExecutionID: sourceExecutionID, Version: maxVersion + 1, Title: title,
		Name: name, StoragePath: filepath.Join("task-reports", reportID, name),
		MimeType: "application/zip", Size: info.Size(), AttachmentCount: len(attachments),
		CreatedAt: now, UpdatedAt: now,
	}
	if err = m.store.db.Create(&report).Error; err != nil {
		cleanup()
		return err
	}
	m.store.addEvent(sourceExecutionID, issue.ID, "delivery", "任务报告已生成", fmt.Sprintf("第 %d 版报告已将本次提交的 %d 个附件打包为 %s。", report.Version, len(attachments), name))
	return nil
}

// backfillRootIssueTaskReports upgrades historical completed roots at service
// startup. Generation is lifecycle-driven and never triggered by opening Home.
func (m *Manager) backfillRootIssueTaskReports() {
	var roots []Issue
	if err := m.store.db.Raw(`
		SELECT issues.* FROM issues
		LEFT JOIN task_reports ON task_reports.root_issue_id = issues.id
		WHERE issues.parent_id = ''
		  AND issues.task_source_id <> ''
		  AND issues.status IN ('done', 'in_review')
		  AND issues.current_execution_id <> ''
		  AND task_reports.id IS NULL
		ORDER BY issues.completed_at ASC, issues.id ASC`).Scan(&roots).Error; err != nil {
		return
	}
	for _, root := range roots {
		var attachmentCount int64
		if err := m.store.db.Model(&IssueAttachment{}).Where("issue_id = ? AND execution_id = ?", root.ID, root.CurrentExecutionID).Count(&attachmentCount).Error; err != nil || attachmentCount == 0 {
			continue
		}
		_ = m.publishRootIssueTaskReport(root, root.CurrentExecutionID, time.Now())
	}
}

func copyTaskReportAttachment(archive *zip.Writer, sourcePath, archiveName string, expectedSize int64) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	info, err := source.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() != expectedSize {
		return errors.New("附件不存在、不是普通文件或大小不完整")
	}
	writer, err := archive.CreateHeader(&zip.FileHeader{Name: archiveName, Method: zip.Deflate})
	if err != nil {
		return err
	}
	written, err := io.Copy(writer, source)
	if err != nil {
		return err
	}
	if written != expectedSize {
		return errors.New("附件复制不完整")
	}
	return nil
}

func (m *Manager) taskReportTitle(issue Issue, sourceExecutionID string, generatedAt time.Time) string {
	var wakeup AgentWakeup
	if err := m.store.db.Where("issue_id = ? AND execution_id = ? AND comment_id <> ''", issue.ID, sourceExecutionID).Order("created_at desc, id desc").First(&wakeup).Error; err == nil {
		var comment IssueComment
		if err := m.store.db.First(&comment, "id = ?", wakeup.CommentID).Error; err == nil {
			if title := compactReportTitle(comment.Body); title != "" {
				return title
			}
		}
	}
	var execution Execution
	if err := m.store.db.First(&execution, "id = ?", sourceExecutionID).Error; err == nil && !execution.StartedAt.IsZero() {
		var comment IssueComment
		query := m.store.db.Where("issue_id = ? AND author_type = ? AND created_at >= ?", issue.ID, "operator", execution.StartedAt)
		if !generatedAt.IsZero() {
			query = query.Where("created_at <= ?", generatedAt)
		}
		if err := query.Order("created_at desc, id desc").First(&comment).Error; err == nil {
			if title := compactReportTitle(comment.Body); title != "" {
				return title
			}
		}
	}
	return fallback(strings.TrimSpace(issue.Title), issue.Identifier+" 报告")
}

func compactReportTitle(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\n", " "))
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, "#*-_ `。.!！?？")
	if len([]rune(value)) > 48 {
		value = string([]rune(value)[:48]) + "…"
	}
	return value
}

func safeTaskReportFilename(identifier, title string) string {
	base := strings.Trim(strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r < 32 {
			return '-'
		}
		return r
	}, strings.TrimSpace(identifier)+"-"+strings.TrimSpace(title)), " .-")
	if base == "" {
		base = "task-report"
	}
	if len([]rune(base)) > 100 {
		base = string([]rune(base)[:100])
	}
	return base + ".zip"
}

func uniqueArchiveName(name string, used map[string]int) string {
	base := filepath.Base(strings.TrimSpace(name))
	if base == "." || base == "" {
		base = "attachment"
	}
	if used[base] == 0 {
		used[base] = 1
		return base
	}
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	for index := used[base] + 1; ; index++ {
		candidate := fmt.Sprintf("%s-%d%s", stem, index, ext)
		if used[candidate] == 0 {
			used[base], used[candidate] = index, 1
			return candidate
		}
	}
}

func (s *Store) TaskReportFile(id string) (TaskReport, *os.File, error) {
	var report TaskReport
	if err := s.db.First(&report, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return TaskReport{}, nil, errors.New("task report not found")
	}
	path, err := s.taskReportStoragePath(report)
	if err != nil {
		return TaskReport{}, nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return TaskReport{}, nil, err
	}
	return report, file, nil
}

func (s *Store) taskReportStoragePath(report TaskReport) (string, error) {
	root := filepath.Join(s.dataDir, "task-reports")
	storagePath := filepath.Clean(strings.TrimSpace(report.StoragePath))
	if storagePath == "." || storagePath == "" {
		return "", errors.New("invalid task report storage path")
	}
	if !filepath.IsAbs(storagePath) {
		storagePath = filepath.Join(s.dataDir, storagePath)
	}
	if !pathWithin(root, storagePath) {
		return "", errors.New("invalid task report storage path")
	}
	return storagePath, nil
}

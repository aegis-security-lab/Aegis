package control

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

const maxKnowledgeDocumentBytes = 512 * 1024

var knowledgeProviderIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

func (s *Store) ListKnowledgeBases() ([]KnowledgeBase, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.listKnowledgeBases()
}

func (s *Store) listKnowledgeBases() ([]KnowledgeBase, error) {
	var bases []KnowledgeBase
	if err := s.db.Order("updated_at DESC").Find(&bases).Error; err != nil {
		return nil, err
	}
	if len(bases) == 0 {
		return []KnowledgeBase{}, nil
	}
	type countRow struct {
		KnowledgeBaseID string
		Count           int64
	}
	var counts []countRow
	if err := s.db.Model(&KnowledgeDocument{}).
		Select("knowledge_base_id, count(*) AS count").
		Group("knowledge_base_id").Scan(&counts).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]int64, len(counts))
	for _, row := range counts {
		byID[row.KnowledgeBaseID] = row.Count
	}
	for index := range bases {
		bases[index].DocumentCount = byID[bases[index].ID]
	}
	return bases, nil
}

func (s *Store) GetKnowledgeBase(id string) (KnowledgeBaseDetail, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var base KnowledgeBase
	if err := s.db.First(&base, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return KnowledgeBaseDetail{}, errors.New("knowledge base not found")
	}
	var documents []KnowledgeDocument
	if err := s.db.Where("knowledge_base_id = ?", base.ID).Order("name ASC").Find(&documents).Error; err != nil {
		return KnowledgeBaseDetail{}, err
	}
	if documents == nil {
		documents = []KnowledgeDocument{}
	}
	base.DocumentCount = int64(len(documents))
	return KnowledgeBaseDetail{KnowledgeBase: base, Documents: documents}, nil
}

func (s *Store) CreateKnowledgeBase(input SaveKnowledgeBaseInput) (KnowledgeBase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	name, description, provider, err := validateKnowledgeBaseInput(input)
	if err != nil {
		return KnowledgeBase{}, err
	}
	var count int64
	if err := s.db.Model(&KnowledgeBase{}).Where("name = ?", name).Count(&count).Error; err != nil {
		return KnowledgeBase{}, err
	}
	if count > 0 {
		return KnowledgeBase{}, errors.New("知识库名称已存在")
	}
	now := time.Now()
	base := KnowledgeBase{ID: nextID("knowledge"), Name: name, Description: description, RetrievalProvider: provider, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&base).Error; err != nil {
		return KnowledgeBase{}, err
	}
	s.changedLocked()
	return base, nil
}

func (s *Store) UpdateKnowledgeBase(id string, input SaveKnowledgeBaseInput) (KnowledgeBase, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var base KnowledgeBase
	if err := s.db.First(&base, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return KnowledgeBase{}, errors.New("knowledge base not found")
	}
	name, description, provider, err := validateKnowledgeBaseInput(input)
	if err != nil {
		return KnowledgeBase{}, err
	}
	var count int64
	if err := s.db.Model(&KnowledgeBase{}).Where("name = ? AND id <> ?", name, base.ID).Count(&count).Error; err != nil {
		return KnowledgeBase{}, err
	}
	if count > 0 {
		return KnowledgeBase{}, errors.New("知识库名称已存在")
	}
	base.Name = name
	base.Description = description
	base.RetrievalProvider = provider
	base.UpdatedAt = time.Now()
	if err := s.db.Save(&base).Error; err != nil {
		return KnowledgeBase{}, err
	}
	s.changedLocked()
	return base, nil
}

func (s *Store) DeleteKnowledgeBase(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	var base KnowledgeBase
	if err := s.db.First(&base, "id = ?", id).Error; err != nil {
		return errors.New("knowledge base not found")
	}
	for _, agent := range s.agents {
		if slices.Contains(agent.KnowledgeBaseIDs, id) {
			return fmt.Errorf("知识库已关联 Agent「%s」，请先解除关联", agent.Name)
		}
	}
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Delete(&KnowledgeDocument{}, "knowledge_base_id = ?", id).Error; err != nil {
			return err
		}
		return tx.Delete(&KnowledgeBase{}, "id = ?", id).Error
	}); err != nil {
		return err
	}
	s.changedLocked()
	return nil
}

func (s *Store) CreateKnowledgeDocument(knowledgeBaseID string, input SaveKnowledgeDocumentInput) (KnowledgeDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var base KnowledgeBase
	if err := s.db.First(&base, "id = ?", strings.TrimSpace(knowledgeBaseID)).Error; err != nil {
		return KnowledgeDocument{}, errors.New("knowledge base not found")
	}
	name, content, err := validateKnowledgeDocumentInput(input)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	if err := s.ensureKnowledgeDocumentNameAvailable(base.ID, name, ""); err != nil {
		return KnowledgeDocument{}, err
	}
	now := time.Now()
	document := KnowledgeDocument{ID: nextID("knowledge-document"), KnowledgeBaseID: base.ID, Name: name, Content: content, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&document).Error; err != nil {
		return KnowledgeDocument{}, err
	}
	_ = s.db.Model(&KnowledgeBase{}).Where("id = ?", base.ID).Update("updated_at", now).Error
	s.changedLocked()
	return document, nil
}

func (s *Store) UpdateKnowledgeDocument(id string, input SaveKnowledgeDocumentInput) (KnowledgeDocument, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var document KnowledgeDocument
	if err := s.db.First(&document, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return KnowledgeDocument{}, errors.New("knowledge document not found")
	}
	name, content, err := validateKnowledgeDocumentInput(input)
	if err != nil {
		return KnowledgeDocument{}, err
	}
	if err := s.ensureKnowledgeDocumentNameAvailable(document.KnowledgeBaseID, name, document.ID); err != nil {
		return KnowledgeDocument{}, err
	}
	now := time.Now()
	document.Name = name
	document.Content = content
	document.UpdatedAt = now
	if err := s.db.Save(&document).Error; err != nil {
		return KnowledgeDocument{}, err
	}
	_ = s.db.Model(&KnowledgeBase{}).Where("id = ?", document.KnowledgeBaseID).Update("updated_at", now).Error
	s.changedLocked()
	return document, nil
}

func (s *Store) DeleteKnowledgeDocument(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var document KnowledgeDocument
	if err := s.db.First(&document, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return errors.New("knowledge document not found")
	}
	if err := s.db.Delete(&KnowledgeDocument{}, "id = ?", document.ID).Error; err != nil {
		return err
	}
	now := time.Now()
	_ = s.db.Model(&KnowledgeBase{}).Where("id = ?", document.KnowledgeBaseID).Update("updated_at", now).Error
	s.changedLocked()
	return nil
}

func (s *Store) knowledgeBaseIDSet() (map[string]struct{}, error) {
	var ids []string
	if err := s.db.Model(&KnowledgeBase{}).Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	result := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		result[id] = struct{}{}
	}
	return result, nil
}

func (s *Store) knowledgeBasesByIDs(ids []string) ([]KnowledgeBase, error) {
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return []KnowledgeBase{}, nil
	}
	var bases []KnowledgeBase
	if err := s.db.Where("id IN ?", ids).Find(&bases).Error; err != nil {
		return nil, err
	}
	byID := make(map[string]KnowledgeBase, len(bases))
	for _, base := range bases {
		byID[base.ID] = base
	}
	ordered := make([]KnowledgeBase, 0, len(ids))
	for _, id := range ids {
		if base, ok := byID[id]; ok {
			ordered = append(ordered, base)
		}
	}
	return ordered, nil
}

func (s *Store) knowledgeDocuments(baseIDs []string) ([]KnowledgeDocument, error) {
	baseIDs = uniqueStrings(baseIDs)
	if len(baseIDs) == 0 {
		return []KnowledgeDocument{}, nil
	}
	var documents []KnowledgeDocument
	if err := s.db.Where("knowledge_base_id IN ?", baseIDs).Order("name ASC").Find(&documents).Error; err != nil {
		return nil, err
	}
	return documents, nil
}

func (s *Store) ensureKnowledgeDocumentNameAvailable(knowledgeBaseID, name, excludedID string) error {
	query := s.db.Model(&KnowledgeDocument{}).Where("knowledge_base_id = ? AND name = ?", knowledgeBaseID, name)
	if excludedID != "" {
		query = query.Where("id <> ?", excludedID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return errors.New("该知识库中已存在同名文档")
	}
	return nil
}

func validateKnowledgeBaseInput(input SaveKnowledgeBaseInput) (string, string, string, error) {
	name := strings.TrimSpace(input.Name)
	description := strings.TrimSpace(input.Description)
	provider := strings.TrimSpace(input.RetrievalProvider)
	if name == "" {
		return "", "", "", errors.New("知识库名称不能为空")
	}
	if utf8.RuneCountInString(name) > 100 {
		return "", "", "", errors.New("知识库名称不能超过 100 个字符")
	}
	if description == "" {
		return "", "", "", errors.New("知识库介绍不能为空")
	}
	if utf8.RuneCountInString(description) > 5000 {
		return "", "", "", errors.New("知识库介绍不能超过 5000 个字符")
	}
	if provider == "" {
		provider = KnowledgeProviderKeywordAI
	}
	if len(provider) > 64 || !knowledgeProviderIDPattern.MatchString(provider) {
		return "", "", "", errors.New("知识库检索 Provider 必须是小写字母、数字、下划线或连字符组成的标识")
	}
	return name, description, provider, nil
}

func validateKnowledgeDocumentInput(input SaveKnowledgeDocumentInput) (string, string, error) {
	name := strings.TrimSpace(input.Name)
	content := strings.TrimSpace(input.Content)
	if name == "" || filepath.Base(name) != name || !strings.EqualFold(filepath.Ext(name), ".md") {
		return "", "", errors.New("文档名称必须是单个 .md 文件名")
	}
	if utf8.RuneCountInString(name) > 180 {
		return "", "", errors.New("文档名称不能超过 180 个字符")
	}
	if content == "" {
		return "", "", errors.New("Markdown 内容不能为空")
	}
	if !utf8.ValidString(content) {
		return "", "", errors.New("Markdown 内容必须是有效的 UTF-8 文本")
	}
	if len(content) > maxKnowledgeDocumentBytes {
		return "", "", errors.New("单个 Markdown 文档不能超过 512 KiB")
	}
	return name, content, nil
}

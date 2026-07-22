package control

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"
)

var ErrAgentTemplateIDConflict = errors.New("人才 ID 已存在")

func AgentTemplateID(provider, model, prompt string) string {
	identity := strings.Join([]string{
		strings.TrimSpace(provider),
		strings.TrimSpace(model),
		strings.TrimSpace(prompt),
	}, "\x00")
	digest := sha256.Sum256([]byte(identity))
	return hex.EncodeToString(digest[:])
}

func (s *Store) AgentTemplates() []AgentTemplate {
	var templates []AgentTemplate
	s.db.Order("metadata_chinese_name asc, metadata_english_name asc, id asc").Find(&templates)
	return templates
}

func (s *Store) GetAgentTemplate(id string) (AgentTemplate, error) {
	var template AgentTemplate
	if err := s.db.First(&template, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return AgentTemplate{}, errors.New("agent template not found")
	}
	return template, nil
}

func (s *Store) SaveAgentTemplate(input SaveAgentTemplateInput) (AgentTemplate, error) {
	input.Provider = strings.TrimSpace(input.Provider)
	input.Model = strings.TrimSpace(input.Model)
	input.SystemPrompt = strings.TrimSpace(input.SystemPrompt)
	input.Note = strings.TrimSpace(input.Note)
	input.Metadata.EnglishName = strings.TrimSpace(input.Metadata.EnglishName)
	input.Metadata.ChineseName = strings.TrimSpace(input.Metadata.ChineseName)
	input.Metadata.Introduction = strings.TrimSpace(input.Metadata.Introduction)
	input.Metadata.Positions = uniqueStrings(input.Metadata.Positions)
	if input.Provider == "" || input.Model == "" || input.SystemPrompt == "" {
		return AgentTemplate{}, errors.New("厂商、模型和系统提示词不能为空")
	}
	if input.Metadata.EnglishName == "" || input.Metadata.ChineseName == "" || input.Metadata.Introduction == "" || len(input.Metadata.Positions) == 0 {
		return AgentTemplate{}, errors.New("英文名、中文名、个人介绍和至少一个职位不能为空")
	}
	if utf8.RuneCountInString(input.Note) > 10000 {
		return AgentTemplate{}, errors.New("人才备注不能超过 10000 个字符")
	}
	id := AgentTemplateID(input.Provider, input.Model, input.SystemPrompt)
	var count int64
	if err := s.db.Model(&AgentTemplate{}).Where("id = ?", id).Count(&count).Error; err != nil {
		return AgentTemplate{}, err
	}
	if count > 0 {
		return AgentTemplate{}, fmt.Errorf("%w：%s", ErrAgentTemplateIDConflict, id)
	}
	now := time.Now()
	template := AgentTemplate{ID: id, Provider: input.Provider, Model: input.Model, SystemPrompt: input.SystemPrompt, Note: input.Note, Metadata: input.Metadata, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&template).Error; err != nil {
		return AgentTemplate{}, err
	}
	s.notify()
	return template, nil
}

func (s *Store) UpdateAgentTemplateNote(id, note string) (AgentTemplate, error) {
	template, err := s.GetAgentTemplate(id)
	if err != nil {
		return AgentTemplate{}, err
	}
	note = strings.TrimSpace(note)
	if utf8.RuneCountInString(note) > 10000 {
		return AgentTemplate{}, errors.New("人才备注不能超过 10000 个字符")
	}
	template.Note = note
	template.UpdatedAt = time.Now()
	if err := s.db.Save(&template).Error; err != nil {
		return AgentTemplate{}, err
	}
	s.notify()
	return template, nil
}

func (s *Store) SetAgentTemplateHidden(id string, hidden bool) (AgentTemplate, error) {
	template, err := s.GetAgentTemplate(id)
	if err != nil {
		return AgentTemplate{}, err
	}
	template.Hidden = hidden
	template.UpdatedAt = time.Now()
	if err := s.db.Save(&template).Error; err != nil {
		return AgentTemplate{}, err
	}
	s.notify()
	return template, nil
}

func (s *Store) seedAgentTemplates(now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.normalizeAgentTemplateIDs(now); err != nil {
		return err
	}
	for index := range s.agents {
		agent := &s.agents[index]
		provider := fallback(strings.TrimSpace(agent.Model.Provider), s.config.Provider)
		model := fallback(strings.TrimSpace(agent.Model.Model), s.config.Model)
		id := AgentTemplateID(provider, model, agent.SystemPrompt)
		var count int64
		if err := s.db.Model(&AgentTemplate{}).Where("id = ?", id).Count(&count).Error; err != nil {
			return err
		}
		if count == 0 {
			englishName := agent.ID
			chineseName := agent.Name
			metadata := AgentTemplateMetadata{EnglishName: englishName, ChineseName: chineseName, Introduction: fallback(agent.Description, agent.Name), Positions: []string{fallback(agent.Category, "general")}}
			template := AgentTemplate{ID: id, Provider: provider, Model: model, SystemPrompt: agent.SystemPrompt, Metadata: metadata, Builtin: agent.Builtin, CreatedAt: now, UpdatedAt: now}
			if err := s.db.Create(&template).Error; err != nil {
				return err
			}
		}
		if agent.TemplateID != id || agent.Model.Provider != provider || agent.Model.Model != model {
			agent.TemplateID = id
			agent.Model.Provider, agent.Model.Model = provider, model
			record := agentRecord{ID: agent.ID, Definition: *agent, CreatedAt: agent.CreatedAt, UpdatedAt: now}
			if err := s.db.Save(&record).Error; err != nil {
				return err
			}
		}
	}
	referencedIDs := make([]string, 0, len(s.agents))
	for _, agent := range s.agents {
		referencedIDs = append(referencedIDs, agent.TemplateID)
	}
	if err := s.db.Where("builtin = ? AND id NOT IN ?", true, referencedIDs).Delete(&AgentTemplate{}).Error; err != nil {
		return err
	}
	return nil
}

// normalizeAgentTemplateIDs enforces the content-addressed template identity
// invariant for every stored talent and employee reference.
func (s *Store) normalizeAgentTemplateIDs(now time.Time) error {
	var templates []AgentTemplate
	if err := s.db.Find(&templates).Error; err != nil {
		return err
	}
	replacements := make(map[string]string)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		for _, template := range templates {
			expectedID := AgentTemplateID(template.Provider, template.Model, template.SystemPrompt)
			if template.ID == expectedID {
				continue
			}
			var existing AgentTemplate
			err := tx.First(&existing, "id = ?", expectedID).Error
			if errors.Is(err, gorm.ErrRecordNotFound) {
				migrated := template
				migrated.ID = expectedID
				migrated.UpdatedAt = now
				if err := tx.Create(&migrated).Error; err != nil {
					return err
				}
			} else if err != nil {
				return err
			}
			replacements[template.ID] = expectedID
		}
		for index := range s.agents {
			replacement, ok := replacements[s.agents[index].TemplateID]
			if !ok {
				continue
			}
			s.agents[index].TemplateID = replacement
			s.agents[index].UpdatedAt = now
			record := agentRecord{
				ID: s.agents[index].ID, Definition: s.agents[index],
				CreatedAt: s.agents[index].CreatedAt, UpdatedAt: now,
			}
			if err := tx.Save(&record).Error; err != nil {
				return err
			}
		}
		for oldID := range replacements {
			if err := tx.Delete(&AgentTemplate{}, "id = ?", oldID).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

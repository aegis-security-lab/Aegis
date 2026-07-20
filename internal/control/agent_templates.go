package control

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"time"
)

func AgentTemplateID(provider, model, prompt string) string {
	provider = normalizeRegistryID(fallback(strings.TrimSpace(provider), "global"))
	model = normalizeRegistryID(fallback(strings.TrimSpace(model), "default"))
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(strings.TrimSpace(prompt))))[:6]
	return provider + "-" + model + "-" + hash
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
	id := AgentTemplateID(input.Provider, input.Model, input.SystemPrompt)
	if existing, err := s.GetAgentTemplate(id); err == nil {
		return existing, nil
	}
	now := time.Now()
	template := AgentTemplate{ID: id, Provider: input.Provider, Model: input.Model, SystemPrompt: input.SystemPrompt, Metadata: input.Metadata, CreatedAt: now, UpdatedAt: now}
	if err := s.db.Create(&template).Error; err != nil {
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
		if agent.TemplateID == "" {
			agent.TemplateID = id
			agent.Model.Provider, agent.Model.Model = provider, model
			record := agentRecord{ID: agent.ID, Definition: *agent, CreatedAt: agent.CreatedAt, UpdatedAt: now}
			if err := s.db.Save(&record).Error; err != nil {
				return err
			}
		}
	}
	return nil
}

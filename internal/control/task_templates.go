package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type taskTemplateSeedRecord struct {
	ID        string `gorm:"primaryKey"`
	CreatedAt time.Time
}

func ensureDefaultTaskTemplates(db *gorm.DB, now time.Time) error {
	const seedID = "default-task-templates-v1"
	var seed taskTemplateSeedRecord
	if err := db.First(&seed, "id = ?", seedID).Error; err == nil {
		return nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("inspect default task template seed: %w", err)
	}
	defaults := []TaskTemplate{
		{
			ID: "task-template-code-audit", Title: "全面代码审计", Priority: "high", AssigneeAgentID: "aegis-orchestrator",
			Description: `对目标项目开展全面、系统、可复现的代码审计。先完整理解项目的业务目标、架构、信任边界、数据流、部署方式、依赖关系和关键资产，再制定覆盖清单并拆分任务；禁止在尚未摸清整体结构前盲目分派。

审计必须覆盖所有模块、目录、入口、接口、权限路径、数据处理流程、配置、构建与部署脚本、第三方依赖和安全边界，逐项确认是否已经审查，不得因代码量大而跳过低关注区域。重点验证认证、授权、输入验证、注入、文件处理、反序列化、并发、竞态、密钥、加密、网络边界、供应链和业务逻辑风险。

对每个潜在问题进行源码追踪、调用链分析、可达性判断和实际验证，区分理论风险、不可利用问题与真实漏洞。必要时编写最小 PoC、测试或复现脚本。持续维护覆盖矩阵和证据索引；发现遗漏时主动补充分工。

最终交付必须是可独立阅读、可复现的完整报告，包含范围、方法、架构理解、覆盖情况、发现列表、风险等级、根因、证据、复现步骤、影响、修复建议、验证结果、未覆盖项及其原因，并附上所有支持文件。`,
			Objective:               "达到 S 级代码审计目标：完整理解项目并覆盖全部可审计模块与关键代码路径；所有结论均有源码链路和可复现证据；最终报告完整、准确、可独立验收，不依赖聊天记录或历史增量附件。",
			HumanValidationFallback: true, CreatedAt: now, UpdatedAt: now,
		},
		{
			ID: "task-template-red-team", Title: "红队渗透测试", Priority: "high", AssigneeAgentID: "aegis-orchestrator",
			Description: `对授权目标开展全面、深入、证据驱动的红队渗透测试。首先确认授权范围、禁止事项、测试窗口和数据处理要求；随后建立完整资产清单，持续收集域名、子域名、IP、端口、服务、证书、DNS、云资源、公开代码、第三方入口、身份系统、管理后台、API、移动端与供应链暴露面，并对资产归属和存活状态进行交叉验证。

完成资产画像后再制定攻击路径和并行分工。测试必须覆盖外部暴露面、认证与会话、权限与越权、输入与协议、文件和对象存储、业务逻辑、配置错误、云与容器、横向移动、权限提升、持久化及敏感数据路径。不得只依赖自动扫描；每条线索都要人工复核、组合分析并追踪到可验证结论，不遗漏低显著度但可串联的细节。

所有操作必须严格处于授权边界内，优先使用低影响验证方式，避免破坏、持久化或接触非必要真实数据。对真实漏洞提供最小化 PoC、完整请求响应、时间线、影响分析和修复建议；对误报和不可利用线索说明排除依据。

最终交付必须包含执行摘要、范围与约束、资产清单、攻击面地图、测试矩阵、攻击路径、漏洞详情、证据、复现步骤、风险评级、修复优先级、未覆盖项和清理说明，并附上完整支持材料。`,
			Objective:               "达到 S 级红队渗透目标：在授权边界内形成尽可能完整且经验证的资产与攻击面覆盖，深入验证所有高价值攻击路径和细节线索，最终交付可独立复现、证据充分、无关键遗漏的完整报告与附件包。",
			HumanValidationFallback: true, CreatedAt: now, UpdatedAt: now,
		},
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for _, template := range defaults {
			var count int64
			if err := tx.Model(&TaskTemplate{}).Where("title = ?", template.Title).Count(&count).Error; err != nil {
				return err
			}
			if count == 0 {
				if err := tx.Create(&template).Error; err != nil {
					return err
				}
			}
		}
		return tx.Create(&taskTemplateSeedRecord{ID: seedID, CreatedAt: now}).Error
	})
}

func (s *Store) TaskTemplates() ([]TaskTemplate, error) {
	var templates []TaskTemplate
	if err := s.db.Order("updated_at desc, title asc").Find(&templates).Error; err != nil {
		return nil, err
	}
	return templates, nil
}

func (s *Store) SaveTaskTemplate(input TaskTemplate) (TaskTemplate, error) {
	input.Title, input.Description, input.Objective = strings.TrimSpace(input.Title), strings.TrimSpace(input.Description), strings.TrimSpace(input.Objective)
	if input.Title == "" || input.Description == "" {
		return TaskTemplate{}, errors.New("模板标题和任务说明不能为空")
	}
	if len([]rune(input.Title)) > 120 {
		return TaskTemplate{}, errors.New("模板标题不能超过 120 个字符")
	}
	input.Priority = normalizeIssuePriority(input.Priority)
	switch input.Priority {
	case "":
		input.Priority = "high"
	case "high", "middle", "low":
	default:
		return TaskTemplate{}, errors.New("模板优先级无效")
	}
	if input.TimeBudgetMinutes != nil && *input.TimeBudgetMinutes <= 0 {
		return TaskTemplate{}, errors.New("模板时间预算必须大于 0 分钟")
	}
	now := time.Now()
	var existing TaskTemplate
	if err := s.db.First(&existing, "title = ?", input.Title).Error; err == nil {
		updates := map[string]any{"description": input.Description, "objective": input.Objective, "priority": input.Priority, "assignee_agent_id": input.AssigneeAgentID, "time_budget_minutes": input.TimeBudgetMinutes, "human_validation_fallback": input.HumanValidationFallback, "updated_at": now}
		if err := s.db.Model(&existing).Updates(updates).Error; err != nil {
			return TaskTemplate{}, err
		}
		return s.GetTaskTemplate(existing.ID)
	}
	input.ID, input.CreatedAt, input.UpdatedAt = nextID("task-template"), now, now
	if err := s.db.Create(&input).Error; err != nil {
		return TaskTemplate{}, err
	}
	return input, nil
}

func (s *Store) GetTaskTemplate(id string) (TaskTemplate, error) {
	var template TaskTemplate
	if err := s.db.First(&template, "id = ?", strings.TrimSpace(id)).Error; err != nil {
		return TaskTemplate{}, errors.New("task template not found")
	}
	return template, nil
}

func (s *Store) DeleteTaskTemplate(id string) error {
	result := s.db.Delete(&TaskTemplate{}, "id = ?", strings.TrimSpace(id))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errors.New("task template not found")
	}
	return nil
}

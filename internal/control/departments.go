package control

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// seedOrganization creates the initial organization once and assigns existing
// employees to the closest functional department. It is intentionally
// idempotent so restarts never overwrite an operator's later assignments.
func (s *Store) seedOrganization() error {
	defaults := []Department{
		{ID: "department-engineering", Name: "技术研发部", Code: "engineering", Description: "负责后端、前端与工程交付。", Enabled: true},
		{ID: "department-security", Name: "安全攻防部", Code: "security", Description: "负责红队测试、安全研究与验证。", Enabled: true},
		{ID: "department-product", Name: "产品与项目部", Code: "product", Description: "负责市场调研、产品规划与项目统筹。", Enabled: true},
	}
	for _, department := range defaults {
		var existing Department
		err := s.db.First(&existing, "id = ?", department.ID).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			now := time.Now()
			department.CreatedAt, department.UpdatedAt = now, now
			if err := s.db.Create(&department).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	for index, agent := range s.agents {
		if agent.Internal || agent.DepartmentID != "" {
			continue
		}
		departmentID := "department-engineering"
		switch agent.Category {
		case "security":
			departmentID = "department-security"
		case "orchestrator", "development", "concierge":
			departmentID = "department-product"
		}
		agent.DepartmentID = departmentID
		if err := s.db.Save(&agentRecord{ID: agent.ID, Definition: agent, CreatedAt: agent.CreatedAt, UpdatedAt: time.Now()}).Error; err != nil {
			return err
		}
		s.agents[index] = agent
	}
	return nil
}

func (s *Store) Departments() []Department {
	var items []Department
	s.db.Order("name asc").Find(&items)
	return items
}

func (s *Store) SaveDepartment(input SaveDepartmentInput) (Department, error) {
	input.Name = strings.TrimSpace(input.Name)
	input.Code = strings.ToLower(strings.TrimSpace(input.Code))
	input.ParentID = strings.TrimSpace(input.ParentID)
	input.LeaderAgentID = strings.TrimSpace(input.LeaderAgentID)
	if input.Name == "" || input.Code == "" {
		return Department{}, errors.New("部门名称和编码不能为空")
	}
	if len([]rune(input.Name)) > 80 || len(input.Code) > 60 {
		return Department{}, errors.New("部门名称或编码过长")
	}
	if input.ParentID != "" && input.ParentID == input.ID {
		return Department{}, errors.New("部门不能将自己设为上级")
	}
	if input.ParentID != "" {
		var parent Department
		if s.db.First(&parent, "id = ?", input.ParentID).Error != nil {
			return Department{}, errors.New("上级部门不存在")
		}
	}
	if input.ID != "" && input.ParentID != "" {
		seen := map[string]bool{input.ID: true}
		current := input.ParentID
		for current != "" {
			if seen[current] {
				return Department{}, errors.New("部门层级不能形成循环")
			}
			seen[current] = true
			var parent Department
			if s.db.First(&parent, "id = ?", current).Error != nil {
				break
			}
			current = parent.ParentID
		}
	}
	if input.LeaderAgentID != "" {
		agent, err := s.GetAgent(input.LeaderAgentID)
		if err != nil {
			return Department{}, fmt.Errorf("部门负责人不存在: %w", err)
		}
		if agent.Internal || !agent.Enabled {
			return Department{}, errors.New("部门负责人必须是已启用的外部员工")
		}
	}
	now := time.Now()
	d := Department{ID: input.ID, ParentID: input.ParentID, Name: input.Name, Code: input.Code, Description: strings.TrimSpace(input.Description), LeaderAgentID: input.LeaderAgentID, Enabled: true, CreatedAt: now, UpdatedAt: now}
	if d.ID == "" {
		d.ID = nextID("department")
	} else {
		var old Department
		if s.db.First(&old, "id = ?", d.ID).Error != nil {
			return Department{}, errors.New("部门不存在")
		}
		d.CreatedAt = old.CreatedAt
		d.Enabled = old.Enabled
	}
	if input.Enabled != nil {
		d.Enabled = *input.Enabled
	}
	if err := s.db.Save(&d).Error; err != nil {
		return Department{}, err
	}
	s.notify()
	return d, nil
}

func (s *Store) DeleteDepartment(id string) error {
	var d Department
	if s.db.First(&d, "id = ?", id).Error != nil {
		return errors.New("部门不存在")
	}
	for _, agent := range s.Agents() {
		if agent.DepartmentID == id {
			return errors.New("部门仍有员工归属，不能删除")
		}
	}
	var count int64
	s.db.Model(&Department{}).Where("parent_id = ?", id).Count(&count)
	if count > 0 {
		return errors.New("部门仍有下级部门，不能删除")
	}
	if err := s.db.Delete(&d).Error; err != nil {
		return err
	}
	s.notify()
	return nil
}

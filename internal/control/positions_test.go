package control

import (
	"strings"
	"testing"
)

func TestOrganizationSeedsRealEmployeesAndPositions(t *testing.T) {
	store := configuredStore(t)
	backend, err := store.GetAgent("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if backend.Name != "陈默" || backend.EnglishName != "Michael Chen" {
		t.Fatalf("backend identity=%+v", backend)
	}
	if backend.PositionID != "backend-engineer" || backend.ManagerAgentID != "development-lead" {
		t.Fatalf("backend organization=%+v", backend)
	}
	position, err := store.GetPosition(backend.PositionID)
	if err != nil {
		t.Fatal(err)
	}
	if position.Name != "后端工程师" || position.DepartmentID != "department-engineering" {
		t.Fatalf("position=%+v", position)
	}
	if err := store.ValidateDelegation("development-lead", backend.ID); err != nil {
		t.Fatalf("direct-report delegation rejected: %v", err)
	}
	if err := store.ValidateDelegation("backend-engineer", "frontend-engineer"); err == nil {
		t.Fatal("peer delegation should be rejected")
	}
}

func TestOrganizationSeedsRequestedHeadcountAndUIDesigner(t *testing.T) {
	store := configuredStore(t)
	want := map[string]int{
		"backend-engineer":              10,
		"frontend-engineer":             10,
		"red-team-engineer":             8,
		"recon-engineer":                5,
		"vulnerability-report-engineer": 2,
		"ui-design-engineer":            1,
	}
	got := make(map[string]int)
	for _, employee := range store.Agents() {
		got[employee.PositionID]++
	}
	for positionID, count := range want {
		if got[positionID] != count {
			t.Fatalf("position %s headcount=%d, want %d", positionID, got[positionID], count)
		}
	}
	designer, err := store.GetAgent("ui-design-engineer-001")
	if err != nil {
		t.Fatal(err)
	}
	if designer.Name != "夏知微" || designer.EnglishName != "Zoey Xia" || designer.ManagerAgentID != "development-lead" {
		t.Fatalf("designer=%+v", designer)
	}
	if !strings.Contains(designer.SystemPrompt, "PRD") {
		t.Fatalf("designer prompt does not define PRD delivery: %q", designer.SystemPrompt)
	}
	if err := store.ValidateDelegation("development-lead", designer.ID); err != nil {
		t.Fatalf("development lead cannot delegate to UI designer: %v", err)
	}
}

func TestHireEmployeeClonesPositionDefaultsAndGeneratesID(t *testing.T) {
	store := configuredStore(t)
	position, err := store.GetPosition("backend-engineer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.GetAgentTemplate(position.TemplateID); err != nil {
		t.Fatalf("position template %q is unavailable (%+v): %v", position.TemplateID, store.AgentTemplates(), err)
	}
	employee, err := store.HireEmployee(position.ID, HireEmployeeInput{
		Name: "高屿", EnglishName: "Ian Gao", ManagerAgentID: "development-lead",
	})
	if err != nil {
		t.Fatalf("hire from position %+v: %v", position, err)
	}
	if employee.ID != "backend-engineer-011" {
		t.Fatalf("employee id=%q", employee.ID)
	}
	if employee.PositionID != position.ID || employee.DepartmentID != position.DepartmentID {
		t.Fatalf("employee organization=%+v", employee)
	}
	if employee.SystemPrompt != position.SystemPrompt || len(employee.Tools) != len(position.Tools) {
		t.Fatal("employee did not inherit the position runtime blueprint")
	}
}

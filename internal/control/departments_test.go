package control

import "testing"

func TestDepartmentCRUDAndEmployeeGuard(t *testing.T) {
	store := configuredStore(t)
	lead := store.Agents()[0]
	d, err := store.SaveDepartment(SaveDepartmentInput{Name: "平台工程", Code: "platform", Description: "平台研发", LeaderAgentID: lead.ID})
	if err != nil {
		t.Fatal(err)
	}
	if d.LeaderAgentID != lead.ID || !d.Enabled {
		t.Fatalf("unexpected department: %+v", d)
	}
	if _, err = store.SaveDepartment(SaveDepartmentInput{ID: d.ID, Name: "平台工程", Code: "platform", LeaderAgentID: lead.ID}); err != nil {
		t.Fatal(err)
	}
	agent, err := store.CreateAgent(SaveAgentInput{ID: "dept-test-agent", Name: "部门员工", Description: "test", Category: "backend", Enabled: true, SystemPrompt: "work", DepartmentID: d.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.DeleteDepartment(d.ID); err == nil {
		t.Fatal("department with employees should not be deleted")
	}
	if agent.DepartmentID != d.ID {
		t.Fatalf("department was not persisted: %+v", agent)
	}
}

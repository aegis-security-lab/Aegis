package control

import (
	"testing"
	"time"
)

func TestDefaultTaskTemplatesAreSeededOnce(t *testing.T) {
	store := configuredStore(t)
	templates, err := store.TaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 2 {
		t.Fatalf("expected two default templates, got %d", len(templates))
	}
	for _, template := range templates {
		if template.Description == "" || template.Objective == "" {
			t.Fatalf("default template %q is incomplete", template.Title)
		}
	}

	if err := store.DeleteTaskTemplate(templates[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := ensureDefaultTaskTemplates(store.db, time.Now()); err != nil {
		t.Fatal(err)
	}
	templates, err = store.TaskTemplates()
	if err != nil {
		t.Fatal(err)
	}
	if len(templates) != 1 {
		t.Fatalf("deleted default template was recreated; got %d templates", len(templates))
	}
}

func TestSaveTaskTemplateUpsertsByTitle(t *testing.T) {
	store := configuredStore(t)
	budget := 45
	created, err := store.SaveTaskTemplate(TaskTemplate{
		Title: "自定义模板", Description: "第一版说明", Objective: "第一版目标",
		Priority: "middle", TimeBudgetMinutes: &budget,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := store.SaveTaskTemplate(TaskTemplate{
		Title: "自定义模板", Description: "完整的新说明", Objective: "新目标",
		Priority: "high", HumanValidationFallback: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ID != created.ID {
		t.Fatalf("same title created a second template: %s != %s", updated.ID, created.ID)
	}
	if updated.Description != "完整的新说明" || updated.Objective != "新目标" || !updated.HumanValidationFallback {
		t.Fatalf("template was not updated: %#v", updated)
	}
	if updated.TimeBudgetMinutes != nil {
		t.Fatalf("expected omitted budget to clear existing value, got %v", *updated.TimeBudgetMinutes)
	}
}

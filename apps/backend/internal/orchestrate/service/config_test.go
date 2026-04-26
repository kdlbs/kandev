package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestExportBundle_ProducesValidBundle(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	wsID := "ws-export"

	// Seed data.
	agent := &models.AgentInstance{
		WorkspaceID:        wsID,
		Name:               "ceo",
		Role:               models.AgentRoleCEO,
		BudgetMonthlyCents: 5000,
	}
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	skill := &models.Skill{
		WorkspaceID: wsID,
		Name:        "Code Review",
		Slug:        "code-review",
		Description: "Reviews code",
		SourceType:  "inline",
		Content:     "Review all PRs.",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	project := &models.Project{
		WorkspaceID:  wsID,
		Name:         "api-v2",
		Description:  "API rewrite",
		Repositories: "[]",
	}
	if err := svc.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	bundle, err := svc.ExportBundle(ctx, wsID)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if len(bundle.Agents) != 1 {
		t.Errorf("agents = %d, want 1", len(bundle.Agents))
	}
	if bundle.Agents[0].Name != "ceo" {
		t.Errorf("agent name = %q, want ceo", bundle.Agents[0].Name)
	}
	if len(bundle.Skills) != 1 {
		t.Errorf("skills = %d, want 1", len(bundle.Skills))
	}
	if len(bundle.Projects) != 1 {
		t.Errorf("projects = %d, want 1", len(bundle.Projects))
	}
}

func TestExportZip_ReturnsReader(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	reader, err := svc.ExportZip(ctx, "ws-empty")
	if err != nil {
		t.Fatalf("export zip: %v", err)
	}
	if reader == nil {
		t.Fatal("reader is nil")
	}
}

func TestImportPreview_ShowsCorrectDiff(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	wsID := "ws-preview"

	// Create an existing agent.
	existing := &models.AgentInstance{
		WorkspaceID: wsID,
		Name:        "existing-agent",
		Role:        models.AgentRoleWorker,
	}
	if err := svc.CreateAgentInstance(ctx, existing); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	bundle := &service.ConfigBundle{
		Agents: []service.AgentConfig{
			{Name: "existing-agent", Role: "worker"},
			{Name: "new-agent", Role: "specialist"},
		},
		Skills: []service.SkillConfig{
			{Name: "New Skill", Slug: "new-skill", SourceType: "inline"},
		},
	}
	preview, err := svc.PreviewImport(ctx, wsID, bundle)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if len(preview.Agents.Created) != 1 || preview.Agents.Created[0] != "new-agent" {
		t.Errorf("agents created = %v, want [new-agent]", preview.Agents.Created)
	}
	if len(preview.Agents.Updated) != 1 || preview.Agents.Updated[0] != "existing-agent" {
		t.Errorf("agents updated = %v, want [existing-agent]", preview.Agents.Updated)
	}
	if len(preview.Skills.Created) != 1 {
		t.Errorf("skills created = %d, want 1", len(preview.Skills.Created))
	}
}

func TestApplyImport_CreatesEntities(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()
	wsID := "ws-import"

	bundle := &service.ConfigBundle{
		Agents: []service.AgentConfig{
			{Name: "imported-ceo", Role: "ceo", BudgetMonthlyCents: 1000},
			{Name: "imported-worker", Role: "worker"},
		},
		Skills: []service.SkillConfig{
			{Name: "Imported Skill", Slug: "imported-skill", SourceType: "inline", Content: "test"},
		},
	}
	result, err := svc.ApplyImport(ctx, wsID, bundle)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if result.CreatedCount != 3 {
		t.Errorf("created = %d, want 3", result.CreatedCount)
	}

	// Apply again -- should be updates, not creates.
	result2, err := svc.ApplyImport(ctx, wsID, bundle)
	if err != nil {
		t.Fatalf("apply again: %v", err)
	}
	if result2.UpdatedCount != 3 {
		t.Errorf("updated = %d, want 3", result2.UpdatedCount)
	}
	if result2.CreatedCount != 0 {
		t.Errorf("created on re-import = %d, want 0", result2.CreatedCount)
	}
}

package configloader

import (
	"testing"
)

func TestLoadRoleTemplates_CEO(t *testing.T) {
	templates, err := LoadRoleTemplates("ceo")
	if err != nil {
		t.Fatalf("load CEO templates: %v", err)
	}
	if len(templates) < 2 {
		t.Fatalf("expected at least 2 CEO templates, got %d", len(templates))
	}

	found := map[string]bool{}
	for _, tmpl := range templates {
		found[tmpl.Filename] = true
		if tmpl.Content == "" {
			t.Errorf("template %s has empty content", tmpl.Filename)
		}
	}
	if !found["AGENTS.md"] {
		t.Error("missing AGENTS.md template for CEO")
	}
	if !found["HEARTBEAT.md"] {
		t.Error("missing HEARTBEAT.md template for CEO")
	}
}

func TestLoadRoleTemplates_Worker(t *testing.T) {
	templates, err := LoadRoleTemplates("worker")
	if err != nil {
		t.Fatalf("load worker templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 worker template, got %d", len(templates))
	}
	if templates[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", templates[0].Filename)
	}
}

func TestLoadRoleTemplates_Reviewer(t *testing.T) {
	templates, err := LoadRoleTemplates("reviewer")
	if err != nil {
		t.Fatalf("load reviewer templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 reviewer template, got %d", len(templates))
	}
	if templates[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", templates[0].Filename)
	}
}

func TestLoadRoleTemplates_UnknownRole(t *testing.T) {
	templates, err := LoadRoleTemplates("nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for unknown role: %v", err)
	}
	if templates != nil {
		t.Errorf("expected nil for unknown role, got %d templates", len(templates))
	}
}

func TestLoadRoleTemplates_Security(t *testing.T) {
	templates, err := LoadRoleTemplates("security")
	if err != nil {
		t.Fatalf("load security templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 security template, got %d", len(templates))
	}
	if templates[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", templates[0].Filename)
	}
}

func TestLoadRoleTemplates_QA(t *testing.T) {
	templates, err := LoadRoleTemplates("qa")
	if err != nil {
		t.Fatalf("load qa templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 qa template, got %d", len(templates))
	}
	if templates[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", templates[0].Filename)
	}
}

func TestLoadRoleTemplates_DevOps(t *testing.T) {
	templates, err := LoadRoleTemplates("devops")
	if err != nil {
		t.Fatalf("load devops templates: %v", err)
	}
	if len(templates) != 1 {
		t.Fatalf("expected 1 devops template, got %d", len(templates))
	}
	if templates[0].Filename != "AGENTS.md" {
		t.Errorf("filename = %q, want AGENTS.md", templates[0].Filename)
	}
}

func TestAvailableInstructionRoles(t *testing.T) {
	roles, err := AvailableInstructionRoles()
	if err != nil {
		t.Fatalf("available roles: %v", err)
	}
	if len(roles) < 6 {
		t.Errorf("expected at least 6 roles, got %d: %v", len(roles), roles)
	}

	roleSet := map[string]bool{}
	for _, r := range roles {
		roleSet[r] = true
	}
	for _, expected := range []string{"ceo", "worker", "reviewer", "security", "qa", "devops"} {
		if !roleSet[expected] {
			t.Errorf("missing expected role %q", expected)
		}
	}
}

package service

import (
	"strings"
	"testing"
)

func TestConfiguredRoleIsIncludedInThePrompt(t *testing.T) {
	si := &SchedulerIntegration{svc: &Service{}}
	manifest := &SkillManifest{WorkspaceSlug: "ws", AgentID: "chief", Instructions: []ManifestInstruction{{Filename: "AGENTS.md", Content: "Coordinate existing work."}, {Filename: "ROLE.md", Content: "Reuse the existing Jira task for follow-ups. Read ./POLICY.md."}}}
	dir, prompt := si.resolveInstructionsForPrompt(manifest, "local")
	if !strings.Contains(prompt, "Reuse the existing Jira task for follow-ups.") || !strings.Contains(prompt, dir+"/POLICY.md") {
		t.Fatalf("role configuration was not delivered: %s", prompt)
	}
}

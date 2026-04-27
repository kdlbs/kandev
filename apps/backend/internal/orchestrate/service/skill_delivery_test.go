package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/orchestrate/models"
	"github.com/kandev/kandev/internal/orchestrate/service"
)

func TestBuildSkillManifest_ContainsSkillsAndInstructions(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	// Create an agent with desired skills.
	agent := makeAgent("worker-manifest", models.AgentRoleWorker)
	agent.DesiredSkills = `["kandev-protocol"]`
	agent.AgentProfileID = "profile-1"
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Create a skill in the DB.
	skill := &models.Skill{
		ID:          "skill-1",
		WorkspaceID: "ws-1",
		Name:        "Kandev Protocol",
		Slug:        "kandev-protocol",
		Content:     "# Protocol\nFollow the heartbeat.",
		SourceType:  "inline",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	// Update the default instruction file.
	svc.ExecSQL(t, `UPDATE orchestrate_agent_instructions SET content = ?
		WHERE agent_instance_id = ? AND filename = ?`,
		"# You are the manifest agent.", agent.ID, "AGENTS.md")

	// Set up agent type resolver.
	svc.SetAgentTypeResolver(func(profileID string) string {
		if profileID == "profile-1" {
			return "claude-acp"
		}
		return ""
	})

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	// Verify skills.
	if len(manifest.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(manifest.Skills))
	}
	if manifest.Skills[0].Slug != "kandev-protocol" {
		t.Errorf("skill slug = %q, want %q", manifest.Skills[0].Slug, "kandev-protocol")
	}
	if manifest.Skills[0].Content != "# Protocol\nFollow the heartbeat." {
		t.Errorf("skill content = %q", manifest.Skills[0].Content)
	}

	// Verify instructions.
	if len(manifest.Instructions) == 0 {
		t.Fatal("expected at least one instruction file")
	}
	found := false
	for _, instr := range manifest.Instructions {
		if instr.Filename == "AGENTS.md" {
			found = true
			if instr.Content != "# You are the manifest agent." {
				t.Errorf("AGENTS.md content = %q", instr.Content)
			}
		}
	}
	if !found {
		t.Error("expected AGENTS.md instruction file in manifest")
	}

	// Verify agent type.
	if manifest.AgentTypeID != "claude-acp" {
		t.Errorf("agent type = %q, want %q", manifest.AgentTypeID, "claude-acp")
	}
	if manifest.AgentID != agent.ID {
		t.Errorf("agent ID = %q, want %q", manifest.AgentID, agent.ID)
	}
	if manifest.WorkspaceSlug != "default" {
		t.Errorf("workspace slug = %q, want %q", manifest.WorkspaceSlug, "default")
	}
}

func TestBuildSkillManifest_NoSkills(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-no-skills", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	if len(manifest.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(manifest.Skills))
	}
	// Instructions should still be present (default AGENTS.md from CreateAgentInstance).
	if len(manifest.Instructions) == 0 {
		t.Error("expected default instructions even with no skills")
	}
}

func TestDeliverSkillsLocal_WritesFilesAndReturnsPath(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-local-deliver", models.AgentRoleWorker)
	agent.DesiredSkills = `["test-skill"]`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	// Create skill.
	skill := &models.Skill{
		ID:          "skill-local",
		WorkspaceID: "ws-1",
		Name:        "Test Skill",
		Slug:        "test-skill",
		Content:     "# Test\nDo the test.",
		SourceType:  "inline",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	// Update instruction content.
	svc.ExecSQL(t, `UPDATE orchestrate_agent_instructions SET content = ?
		WHERE agent_instance_id = ? AND filename = ?`,
		"# Local delivery test", agent.ID, "AGENTS.md")

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	// Use local_pc executor type.
	execCfg := &service.ExecutorConfig{Type: "local_pc"}
	instructionsDir := service.DeliverSkillsForTest(si, ctx, manifest, execCfg)

	// Verify instructions dir is non-empty and contains AGENTS.md.
	if instructionsDir == "" {
		t.Fatal("expected non-empty instructions dir")
	}
	data, err := os.ReadFile(filepath.Join(instructionsDir, "AGENTS.md"))
	if err != nil {
		t.Fatalf("read AGENTS.md: %v", err)
	}
	if string(data) != "# Local delivery test" {
		t.Errorf("AGENTS.md content = %q", string(data))
	}

	// Verify skill file was written.
	basePath := svc.ConfigLoader().BasePath()
	skillFile := filepath.Join(basePath, "runtime", "default", "skills", "test-skill", "SKILL.md")
	skillData, err := os.ReadFile(skillFile)
	if err != nil {
		t.Fatalf("read skill SKILL.md: %v", err)
	}
	if string(skillData) != "# Test\nDo the test." {
		t.Errorf("SKILL.md content = %q", string(skillData))
	}
}

func TestDeliverSkillsDocker_SetsRuntimeDir(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-docker-deliver", models.AgentRoleWorker)
	agent.DesiredSkills = `["docker-skill"]`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	skill := &models.Skill{
		ID:          "skill-docker",
		WorkspaceID: "ws-1",
		Name:        "Docker Skill",
		Slug:        "docker-skill",
		Content:     "# Docker\nRun in container.",
		SourceType:  "inline",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	execCfg := &service.ExecutorConfig{Type: "local_docker"}
	instructionsDir := service.DeliverSkillsForTest(si, ctx, manifest, execCfg)

	// Instructions dir should be a host path.
	if instructionsDir == "" {
		t.Fatal("expected non-empty instructions dir")
	}

	// runtimeDir should be set for Docker bind mount propagation.
	runtimeDir := service.RuntimeDirForTest(si)
	if runtimeDir == "" {
		t.Fatal("expected runtimeDir to be set for Docker")
	}
	if !strings.HasSuffix(runtimeDir, "runtime") {
		t.Errorf("runtimeDir = %q, expected to end with 'runtime'", runtimeDir)
	}

	// Verify skill file exists on host.
	skillFile := filepath.Join(runtimeDir, "default", "skills", "docker-skill", "SKILL.md")
	if _, err := os.Stat(skillFile); err != nil {
		t.Fatalf("skill file should exist on host: %v", err)
	}
}

func TestDeliverSkillsSprites_SetsManifestJSON(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-sprites-deliver", models.AgentRoleWorker)
	agent.DesiredSkills = `["sprite-skill"]`
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	skill := &models.Skill{
		ID:          "skill-sprite",
		WorkspaceID: "ws-1",
		Name:        "Sprite Skill",
		Slug:        "sprite-skill",
		Content:     "# Sprite\nRun in sprite.",
		SourceType:  "inline",
	}
	if err := svc.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	svc.ExecSQL(t, `UPDATE orchestrate_agent_instructions SET content = ?
		WHERE agent_instance_id = ? AND filename = ?`,
		"# Sprite instructions", agent.ID, "AGENTS.md")

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	execCfg := &service.ExecutorConfig{Type: "sprites"}
	instructionsDir := service.DeliverSkillsForTest(si, ctx, manifest, execCfg)

	// Path should be sprite-side (not a host path).
	if !strings.HasPrefix(instructionsDir, "/root/.kandev/runtime/") {
		t.Errorf("instructionsDir = %q, expected sprite-side path", instructionsDir)
	}

	// Manifest JSON should be set for Sprites upload.
	manifestJSON := service.SkillManifestJSONForTest(si)
	if manifestJSON == "" {
		t.Fatal("expected skill manifest JSON to be set for Sprites")
	}

	// Verify it deserializes correctly.
	var decoded service.SkillManifest
	if err := json.Unmarshal([]byte(manifestJSON), &decoded); err != nil {
		t.Fatalf("unmarshal manifest: %v", err)
	}
	if len(decoded.Skills) != 1 {
		t.Errorf("expected 1 skill in manifest, got %d", len(decoded.Skills))
	}
	if decoded.Skills[0].Slug != "sprite-skill" {
		t.Errorf("skill slug = %q", decoded.Skills[0].Slug)
	}
	if len(decoded.Instructions) == 0 {
		t.Error("expected instructions in manifest")
	}
}

func TestBuildSymlinkScript_CorrectCommands(t *testing.T) {
	manifest := &service.SkillManifest{
		AgentTypeID:   "claude-acp",
		WorkspaceSlug: "myws",
		AgentID:       "agent-1",
		Skills: []service.ManifestSkill{
			{Slug: "review", Content: "# Review"},
			{Slug: "tdd", Content: "# TDD"},
		},
	}

	script := service.BuildSymlinkScriptForTest(manifest, "/root/.kandev/runtime")

	// Should create directories and symlinks.
	if !strings.Contains(script, "mkdir -p") {
		t.Error("expected mkdir command")
	}
	if !strings.Contains(script, "ln -sf") {
		t.Error("expected symlink command")
	}

	// Should reference both skills.
	if !strings.Contains(script, "/root/.kandev/runtime/myws/skills/review") {
		t.Error("expected review skill path")
	}
	if !strings.Contains(script, "/root/.kandev/runtime/myws/skills/tdd") {
		t.Error("expected tdd skill path")
	}

	// Should target Claude skill dirs (contains .claude/skills).
	if !strings.Contains(script, ".claude/skills") {
		t.Error("expected .claude/skills target dir for claude-acp")
	}
}

func TestBuildSymlinkScript_EmptyForUnknownAgent(t *testing.T) {
	manifest := &service.SkillManifest{
		AgentTypeID: "unknown-agent",
		Skills:      []service.ManifestSkill{{Slug: "test"}},
	}
	script := service.BuildSymlinkScriptForTest(manifest, "/tmp")
	if script != "" {
		t.Errorf("expected empty script for unknown agent type, got %q", script)
	}
}

func TestBuildSymlinkScript_EmptyForNoSkills(t *testing.T) {
	manifest := &service.SkillManifest{
		AgentTypeID: "claude-acp",
	}
	script := service.BuildSymlinkScriptForTest(manifest, "/tmp")
	if script != "" {
		t.Errorf("expected empty script for no skills, got %q", script)
	}
}

func TestBuildSymlinkScript_NilManifest(t *testing.T) {
	script := service.BuildSymlinkScriptForTest(nil, "/tmp")
	if script != "" {
		t.Errorf("expected empty script for nil manifest, got %q", script)
	}
}

func TestDeliverSkills_NilManifest(t *testing.T) {
	svc := newTestService(t)
	si := service.NewSchedulerIntegration(svc, 0)
	result := service.DeliverSkillsForTest(si, context.Background(), nil, nil)
	if result != "" {
		t.Errorf("expected empty string for nil manifest, got %q", result)
	}
}

func TestDeliverSkills_NilExecConfig_FallsBackToLocal(t *testing.T) {
	svc := newTestService(t)
	ctx := context.Background()

	agent := makeAgent("worker-nil-exec", models.AgentRoleWorker)
	if err := svc.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	si := service.NewSchedulerIntegration(svc, 0)
	manifest := service.BuildSkillManifestForTest(si, ctx, agent, "default")

	// nil exec config should fall back to local delivery without panic.
	result := service.DeliverSkillsForTest(si, ctx, manifest, nil)
	// Should be a host path (local delivery fallback).
	if result == "" && len(manifest.Instructions) > 0 {
		t.Error("expected non-empty instructions dir for local fallback with instructions")
	}
}

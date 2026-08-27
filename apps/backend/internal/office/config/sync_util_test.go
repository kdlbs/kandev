package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
)

func TestMissingNames(t *testing.T) {
	tests := []struct {
		name     string
		existing []string
		incoming []string
		want     []string
	}{
		{
			name:     "all present leaves nothing missing",
			existing: []string{"a", "b"},
			incoming: []string{"b", "a"},
			want:     nil,
		},
		{
			name:     "missing entries preserve existing order",
			existing: []string{"a", "b", "c"},
			incoming: []string{"b"},
			want:     []string{"a", "c"},
		},
		{
			name:     "empty incoming means everything is missing",
			existing: []string{"a", "b"},
			incoming: nil,
			want:     []string{"a", "b"},
		},
		{
			name:     "empty existing is never missing",
			existing: nil,
			incoming: []string{"a"},
			want:     nil,
		},
		{
			name:     "extra incoming names are not reported",
			existing: []string{"a"},
			incoming: []string{"a", "b", "c"},
			want:     nil,
		},
		{
			name:     "duplicate existing names each report",
			existing: []string{"a", "a"},
			incoming: []string{"b"},
			want:     []string{"a", "a"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assertStrings(t, "missingNames", missingNames(tc.existing, tc.incoming), tc.want)
		})
	}
}

func TestModelNameExtractors(t *testing.T) {
	agents := []*models.AgentInstance{{Name: "ada"}, {Name: "grace"}}
	assertStrings(t, "agentNames", agentNames(agents), []string{"ada", "grace"})
	assertStrings(t, "agentNames empty", agentNames(nil), []string{})

	skills := []*models.Skill{{Slug: "code-review"}, {Slug: "triage"}}
	assertStrings(t, "skillSlugs", skillSlugs(skills), []string{"code-review", "triage"})
	assertStrings(t, "skillSlugs empty", skillSlugs(nil), []string{})

	routines := []*models.Routine{{Name: "standup"}}
	assertStrings(t, "routineNames", routineNames(routines), []string{"standup"})
	assertStrings(t, "routineNames empty", routineNames(nil), []string{})

	projects := []*models.Project{{Name: "apollo"}, {Name: "gemini"}}
	assertStrings(t, "projectNames", projectNames(projects), []string{"apollo", "gemini"})
	assertStrings(t, "projectNames empty", projectNames(nil), []string{})
}

func TestBundleNameExtractors(t *testing.T) {
	bundle := fullBundle()
	bundle.Agents = append(bundle.Agents, AgentConfig{Name: "grace"})
	bundle.Skills = append(bundle.Skills, SkillConfig{Slug: "triage"})
	bundle.Routines = append(bundle.Routines, RoutineConfig{Name: "retro"})
	bundle.Projects = append(bundle.Projects, ProjectConfig{Name: "gemini"})

	assertStrings(t, "agentBundleNames", agentBundleNames(bundle.Agents), []string{"ada", "grace"})
	assertStrings(t, "skillBundleSlugs", skillBundleSlugs(bundle.Skills), []string{"code-review", "triage"})
	assertStrings(t, "routineBundleNames", routineBundleNames(bundle.Routines), []string{"standup", "retro"})
	assertStrings(t, "projectBundleNames", projectBundleNames(bundle.Projects), []string{"apollo", "gemini"})
}

func TestBundleSets(t *testing.T) {
	bundle := fullBundle()

	// Agents key on Name, skills on Slug — a set keyed on the wrong field
	// would make every diff report spurious creations.
	agentSet := bundleAgentSet(bundle.Agents)
	assertEqual(t, "agent set size", len(agentSet), 1)
	assertEqual(t, "agent set has ada", agentSet["ada"], true)
	assertEqual(t, "agent set lacks cto role", agentSet["cto"], false)

	skillSet := bundleSkillSet(bundle.Skills)
	assertEqual(t, "skill set has slug", skillSet["code-review"], true)
	assertEqual(t, "skill set does not key on name", skillSet["Code Review"], false)

	routineSet := bundleRoutineSet(bundle.Routines)
	assertEqual(t, "routine set has standup", routineSet["standup"], true)

	projectSet := bundleProjectSet(bundle.Projects)
	assertEqual(t, "project set has apollo", projectSet["apollo"], true)

	assertEqual(t, "empty agent set", len(bundleAgentSet(nil)), 0)
	assertEqual(t, "empty skill set", len(bundleSkillSet(nil)), 0)
	assertEqual(t, "empty routine set", len(bundleRoutineSet(nil)), 0)
	assertEqual(t, "empty project set", len(bundleProjectSet(nil)), 0)
}

// TestWriteBundleToFS_AllFieldsReachDisk writes a fully-populated bundle and
// reads it back through the ConfigLoader. Every field that survives the
// YAML round trip is asserted; a field dropped by writeBundleEntities would
// come back zero-valued instead of failing loudly at write time.
func TestWriteBundleToFS_AllFieldsReachDisk(t *testing.T) {
	env := newTestEnv(t)
	bundle := fullBundle()

	if err := writeBundleToFS(env.writer, bundle); err != nil {
		t.Fatalf("writeBundleToFS: %v", err)
	}

	// The writer always targets the "default" FS workspace, not the
	// bundle's settings name.
	if _, err := os.Stat(env.wsPath("agents", "ada.yml")); err != nil {
		t.Fatalf("agent file not written to default workspace: %v", err)
	}

	agents := env.loader.GetAgents(defaultWorkspaceName)
	if len(agents) != 1 {
		t.Fatalf("agents on disk: got %d, want 1", len(agents))
	}
	got := agents[0]
	assertEqual(t, "agent name", got.Name, "ada")
	assertEqual(t, "agent role", string(got.Role), "cto")
	assertEqual(t, "agent icon", got.Icon, "🤖")
	assertEqual(t, "agent reports_to", got.ReportsTo, "grace")
	assertEqual(t, "agent budget", got.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", got.MaxConcurrentSessions, 3)
	assertEqual(t, "agent desired skills", got.DesiredSkills, `["go","sql"]`)
	assertEqual(t, "agent executor preference", got.ExecutorPreference, "local_docker")

	skills := env.loader.GetSkills(defaultWorkspaceName)
	if len(skills) != 1 {
		t.Fatalf("skills on disk: got %d, want 1", len(skills))
	}
	assertEqual(t, "skill slug", skills[0].Slug, "code-review")
	assertEqual(t, "skill content", skills[0].Content, "# Code Review\n\nread the diff\n")

	routines := env.loader.GetRoutines(defaultWorkspaceName)
	if len(routines) != 1 {
		t.Fatalf("routines on disk: got %d, want 1", len(routines))
	}
	assertEqual(t, "routine name", routines[0].Name, "standup")
	assertEqual(t, "routine description", routines[0].Description, "daily standup")
	assertEqual(t, "routine task template", routines[0].TaskTemplate, "summarise yesterday")
	assertEqual(t, "routine concurrency", string(routines[0].ConcurrencyPolicy), "skip")

	projects := env.loader.GetProjects(defaultWorkspaceName)
	if len(projects) != 1 {
		t.Fatalf("projects on disk: got %d, want 1", len(projects))
	}
	assertEqual(t, "project name", projects[0].Name, "apollo")
	assertEqual(t, "project description", projects[0].Description, "moon shot")
	assertEqual(t, "project status", string(projects[0].Status), "paused")
	assertEqual(t, "project color", projects[0].Color, "#ff00ff")
	assertEqual(t, "project budget", projects[0].BudgetCents, 99000)
	assertEqual(t, "project repositories", projects[0].Repositories, `["kdlbs/kandev"]`)
	assertEqual(t, "project executor config", projects[0].ExecutorConfig, `{"type":"local_docker"}`)
}

// TestWriteBundleToFS_EmptySkillContentGetsHeading covers the one value
// writeBundleEntities synthesises rather than copies.
func TestWriteBundleToFS_EmptySkillContentGetsHeading(t *testing.T) {
	env := newTestEnv(t)
	bundle := &ConfigBundle{Skills: []SkillConfig{{Name: "Triage Bugs", Slug: "triage"}}}

	if err := writeBundleToFS(env.writer, bundle); err != nil {
		t.Fatalf("writeBundleToFS: %v", err)
	}

	data, err := os.ReadFile(env.wsPath("skills", "triage", "SKILL.md"))
	if err != nil {
		t.Fatalf("read SKILL.md: %v", err)
	}
	assertEqual(t, "synthesised skill content", string(data), "# Triage Bugs\n")
}

func TestWriteBundleToFS_EmptyBundleWritesNothing(t *testing.T) {
	env := newTestEnv(t)

	if err := writeBundleToFS(env.writer, &ConfigBundle{}); err != nil {
		t.Fatalf("writeBundleToFS: %v", err)
	}

	for _, dir := range []string{"agents", "skills", "routines", "projects"} {
		if _, err := os.Stat(env.wsPath(dir)); !os.IsNotExist(err) {
			t.Errorf("%s dir should not exist for an empty bundle (err=%v)", dir, err)
		}
	}
}

// TestWriteBundleToFS_RejectsUnsafeNames pins the FileWriter's path-component
// validation reaching back through writeBundleEntities: a traversal name must
// abort the write rather than escape the workspace directory.
func TestWriteBundleToFS_RejectsUnsafeNames(t *testing.T) {
	// escapedPath is where the write would land if the name were joined
	// unchecked: "../escape" climbs out of the per-entity subdirectory into
	// the workspace root. Asserting on that exact path matters — an
	// arbitrary non-existent path would pass no matter what the writer did.
	tests := []struct {
		name        string
		bundle      *ConfigBundle
		escapedPath []string
	}{
		{
			"agent traversal",
			&ConfigBundle{Agents: []AgentConfig{{Name: "../escape"}}},
			[]string{"escape.yml"},
		},
		{
			"skill traversal",
			&ConfigBundle{Skills: []SkillConfig{{Slug: "../escape"}}},
			[]string{"escape", "SKILL.md"},
		},
		{
			"routine traversal",
			&ConfigBundle{Routines: []RoutineConfig{{Name: "../escape"}}},
			[]string{"escape.yml"},
		},
		{
			"project traversal",
			&ConfigBundle{Projects: []ProjectConfig{{Name: "../escape"}}},
			[]string{"escape.yml"},
		},
		{
			"empty agent name",
			&ConfigBundle{Agents: []AgentConfig{{Name: ""}}},
			nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			err := writeBundleToFS(env.writer, tc.bundle)
			if err == nil {
				t.Fatal("expected an error for an unsafe name, got nil")
			}
			if tc.escapedPath == nil {
				return
			}
			escaped := env.wsPath(tc.escapedPath...)
			if _, statErr := os.Stat(escaped); !os.IsNotExist(statErr) {
				t.Errorf("write escaped into %s: %v", escaped, statErr)
			}
		})
	}
}

// TestWriteBundleToFS_StopsAtFirstError proves the write is not best-effort:
// the bad entity aborts the loop, so the entity after it is never written.
func TestWriteBundleToFS_StopsAtFirstError(t *testing.T) {
	env := newTestEnv(t)
	bundle := &ConfigBundle{Agents: []AgentConfig{
		{Name: "ada"},
		{Name: "../escape"},
		{Name: "grace"},
	}}

	if err := writeBundleToFS(env.writer, bundle); err == nil {
		t.Fatal("expected an error, got nil")
	}
	if _, err := os.Stat(env.wsPath("agents", "ada.yml")); err != nil {
		t.Errorf("agent before the failure should have been written: %v", err)
	}
	if _, err := os.Stat(env.wsPath("agents", "grace.yml")); !os.IsNotExist(err) {
		t.Errorf("agent after the failure should not have been written: %v", err)
	}
}

// TestWriteBundleToFS_UsesDefaultWorkspaceOnly documents that the on-disk
// target is always the "default" workspace directory: the bundle's settings
// name does not steer the writer.
func TestWriteBundleToFS_UsesDefaultWorkspaceOnly(t *testing.T) {
	env := newTestEnv(t)
	bundle := &ConfigBundle{
		Settings: SettingsConfig{Name: "some-other-workspace"},
		Agents:   []AgentConfig{{Name: "ada", Role: "cto"}},
	}

	if err := writeBundleToFS(env.writer, bundle); err != nil {
		t.Fatalf("writeBundleToFS: %v", err)
	}

	if _, err := os.Stat(env.wsPath("agents", "ada.yml")); err != nil {
		t.Errorf("agent not written under %q: %v", defaultWorkspaceName, err)
	}
	other := filepath.Join(env.base, "workspaces", "some-other-workspace")
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Errorf("settings name should not create a workspace dir: %v", err)
	}
}

// TestWriteBundleEntities_EmptyIsNoOp guards the direct helper entry point used
// by ApplyOutgoing after its nil check: an empty bundle creates no directories.
func TestWriteBundleEntities_EmptyIsNoOp(t *testing.T) {
	base := t.TempDir()
	loader := configloader.NewConfigLoader(base)
	writer := configloader.NewFileWriter(base, loader)

	if err := writeBundleEntities(writer, &ConfigBundle{}); err != nil {
		t.Fatalf("writeBundleEntities: %v", err)
	}
	if _, err := os.Stat(filepath.Join(base, "workspaces")); !os.IsNotExist(err) {
		t.Errorf("no workspace dir should be created: %v", err)
	}
}

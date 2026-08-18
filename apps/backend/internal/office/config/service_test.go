package config

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"sort"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/kandev/kandev/internal/office/models"
)

func TestNewConfigService_WiresDependencies(t *testing.T) {
	env := newTestEnv(t)

	if env.svc.repo != env.repo {
		t.Error("repo not wired")
	}
	if env.svc.cfgLoader != env.loader {
		t.Error("config loader not wired")
	}
	if env.svc.cfgWriter != env.writer {
		t.Error("config writer not wired")
	}
	if env.svc.logger == nil {
		t.Error("logger not wired")
	}
	if env.svc.activity == nil {
		t.Error("activity logger not wired")
	}
}

// seedFullDBState writes one of each entity with every exportable column set,
// plus a second agent used as the target of the three name references.
func seedFullDBState(t *testing.T, env *testEnv, wsID string) *models.AgentInstance {
	t.Helper()
	ctx := context.Background()

	lead := seedAgent(t, env, wsID, "grace")

	agent := &models.AgentInstance{
		WorkspaceID:           wsID,
		Name:                  "ada",
		Role:                  models.AgentRole("cto"),
		Icon:                  "🤖",
		Status:                models.AgentStatusIdle,
		ReportsTo:             lead.ID,
		BudgetMonthlyCents:    4200,
		MaxConcurrentSessions: 3,
		DesiredSkills:         `["go","sql"]`,
		ExecutorPreference:    "local_docker",
	}
	if err := env.repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	skill := &models.Skill{
		WorkspaceID: wsID,
		Name:        "Code Review",
		Slug:        "code-review",
		Description: "reviews diffs",
		SourceType:  models.SkillSourceType("inline"),
		Content:     "# Code Review\n",
	}
	if err := env.repo.CreateSkill(ctx, skill); err != nil {
		t.Fatalf("create skill: %v", err)
	}

	routine := &models.Routine{
		WorkspaceID:            wsID,
		Name:                   "standup",
		Description:            "daily standup",
		TaskTemplate:           "summarise yesterday",
		AssigneeAgentProfileID: lead.ID,
		Status:                 "active",
		ConcurrencyPolicy:      models.RoutineConcurrencyPolicy("skip"),
	}
	if err := env.repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}

	project := &models.Project{
		WorkspaceID:        wsID,
		Name:               "apollo",
		Description:        "moon shot",
		Status:             models.ProjectStatus("paused"),
		LeadAgentProfileID: lead.ID,
		Color:              "#ff00ff",
		BudgetCents:        99000,
		Repositories:       `["kdlbs/kandev"]`,
		ExecutorConfig:     `{"type":"local_docker"}`,
	}
	if err := env.repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}
	return lead
}

// TestExportBundle_CarriesEveryField exports fully-populated DB rows and
// asserts each column reaches the bundle, including the three ID→name
// references the exporter resolves.
func TestExportBundle_CarriesEveryField(t *testing.T) {
	env := newTestEnv(t)
	seedFullDBState(t, env, testWorkspaceID)

	bundle, err := env.svc.ExportBundle(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	assertEqual(t, "settings name", bundle.Settings.Name, testWorkspaceID)

	var ada *AgentConfig
	for i := range bundle.Agents {
		if bundle.Agents[i].Name == "ada" {
			ada = &bundle.Agents[i]
		}
	}
	if ada == nil {
		t.Fatalf("agent ada missing from export (%d agents)", len(bundle.Agents))
	}
	assertEqual(t, "agent role", ada.Role, "cto")
	assertEqual(t, "agent icon", ada.Icon, "🤖")
	assertEqual(t, "agent reports_to resolved to name", ada.ReportsTo, "grace")
	assertEqual(t, "agent budget", ada.BudgetMonthlyCents, 4200)
	assertEqual(t, "agent max sessions", ada.MaxConcurrentSessions, 3)
	assertEqual(t, "agent desired skills", ada.DesiredSkills, `["go","sql"]`)
	assertEqual(t, "agent executor preference", ada.ExecutorPreference, "local_docker")

	if len(bundle.Skills) != 1 {
		t.Fatalf("skills: got %d, want 1", len(bundle.Skills))
	}
	assertEqual(t, "skill name", bundle.Skills[0].Name, "Code Review")
	assertEqual(t, "skill slug", bundle.Skills[0].Slug, "code-review")
	assertEqual(t, "skill description", bundle.Skills[0].Description, "reviews diffs")
	assertEqual(t, "skill source type", bundle.Skills[0].SourceType, "inline")
	assertEqual(t, "skill content", bundle.Skills[0].Content, "# Code Review\n")

	if len(bundle.Routines) != 1 {
		t.Fatalf("routines: got %d, want 1", len(bundle.Routines))
	}
	assertEqual(t, "routine name", bundle.Routines[0].Name, "standup")
	assertEqual(t, "routine description", bundle.Routines[0].Description, "daily standup")
	assertEqual(t, "routine task template", bundle.Routines[0].TaskTemplate, "summarise yesterday")
	assertEqual(t, "routine assignee resolved to name", bundle.Routines[0].AssigneeName, "grace")
	assertEqual(t, "routine concurrency", bundle.Routines[0].ConcurrencyPolicy, "skip")

	if len(bundle.Projects) != 1 {
		t.Fatalf("projects: got %d, want 1", len(bundle.Projects))
	}
	assertEqual(t, "project name", bundle.Projects[0].Name, "apollo")
	assertEqual(t, "project description", bundle.Projects[0].Description, "moon shot")
	assertEqual(t, "project status", bundle.Projects[0].Status, "paused")
	assertEqual(t, "project color", bundle.Projects[0].Color, "#ff00ff")
	assertEqual(t, "project budget", bundle.Projects[0].BudgetCents, 99000)
	assertEqual(t, "project repositories", bundle.Projects[0].Repositories, `["kdlbs/kandev"]`)
	assertEqual(t, "project executor config", bundle.Projects[0].ExecutorConfig,
		`{"type":"local_docker"}`)
	assertEqual(t, "project lead resolved to name", bundle.Projects[0].LeadAgentName, "grace")
}

// TestExportBundle_UnresolvableReferencesBecomeEmpty covers the other branch of
// the ID→name lookup: an ID with no matching agent exports as an empty string
// rather than leaking the raw ID.
func TestExportBundle_UnresolvableReferencesBecomeEmpty(t *testing.T) {
	env := newTestEnv(t)
	ctx := context.Background()

	agent := &models.AgentInstance{
		WorkspaceID: testWorkspaceID, Name: "ada", ReportsTo: "no-such-agent",
	}
	if err := env.repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}
	routine := &models.Routine{
		WorkspaceID: testWorkspaceID, Name: "standup", AssigneeAgentProfileID: "no-such-agent",
	}
	if err := env.repo.CreateRoutine(ctx, routine); err != nil {
		t.Fatalf("create routine: %v", err)
	}
	project := &models.Project{
		WorkspaceID: testWorkspaceID, Name: "apollo", LeadAgentProfileID: "no-such-agent",
	}
	if err := env.repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("create project: %v", err)
	}

	bundle, err := env.svc.ExportBundle(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	assertEqual(t, "agent reports_to", bundle.Agents[0].ReportsTo, "")
	assertEqual(t, "routine assignee", bundle.Routines[0].AssigneeName, "")
	assertEqual(t, "project lead", bundle.Projects[0].LeadAgentName, "")
}

// TestExportBundle_LeaksOtherWorkspaces_KnownDefect records a suspected bug,
// not an endorsed contract. Every export helper calls ListX(ctx, "") and so
// lists across all workspaces; the workspace ID argument only labels the
// settings block. PreviewImport on the same service *is* scoped (see
// TestPreviewImport_ScopesToWorkspace), so the two disagree, and under
// enabled auth a config export or zip for one workspace can carry another
// user's agents, routines, projects and skill content.
//
// This is reported separately rather than fixed here: this package's tests
// were added under a test-only mandate. Scoping the export is the fix, and it
// must delete or invert this test — that failure is the intended signal.
func TestExportBundle_LeaksOtherWorkspaces_KnownDefect(t *testing.T) {
	env := newTestEnv(t)
	seedAgent(t, env, testWorkspaceID, "ada")
	seedAgent(t, env, "ws-other", "grace")
	seedSkill(t, env, "ws-other", "triage")
	seedRoutine(t, env, "ws-other", "retro")
	seedProject(t, env, "ws-other", "gemini")

	bundle, err := env.svc.ExportBundle(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}

	names := agentBundleNames(bundle.Agents)
	sort.Strings(names)
	assertStrings(t, "agents across workspaces", names, []string{"ada", "grace"})
	assertStrings(t, "skills across workspaces", skillBundleSlugs(bundle.Skills), []string{"triage"})
	assertStrings(t, "routines across workspaces",
		routineBundleNames(bundle.Routines), []string{"retro"})
	assertStrings(t, "projects across workspaces",
		projectBundleNames(bundle.Projects), []string{"gemini"})
}

func TestExportBundle_EmptyDatabase(t *testing.T) {
	env := newTestEnv(t)

	bundle, err := env.svc.ExportBundle(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportBundle: %v", err)
	}
	assertEqual(t, "settings name", bundle.Settings.Name, testWorkspaceID)
	assertEqual(t, "agents", len(bundle.Agents), 0)
	assertEqual(t, "skills", len(bundle.Skills), 0)
	assertEqual(t, "routines", len(bundle.Routines), 0)
	assertEqual(t, "projects", len(bundle.Projects), 0)
}

// TestExportBundle_WrapsRepositoryErrors drops one table at a time and asserts
// each export stage names its own failure.
func TestExportBundle_WrapsRepositoryErrors(t *testing.T) {
	tests := []struct {
		name      string
		dropTable string
		wantIn    string
	}{
		{"agents", "agent_profiles", "list agents:"},
		{"skills", "office_skills", "list skills:"},
		{"routines", "office_routines", "list routines:"},
		{"projects", "office_projects", "list projects:"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t)
			env.dropTable(t, tc.dropTable)

			bundle, err := env.svc.ExportBundle(context.Background(), testWorkspaceID)
			if err == nil {
				t.Fatalf("expected an error after dropping %s, got nil", tc.dropTable)
			}
			if bundle != nil {
				t.Errorf("bundle should be nil on error, got %+v", bundle)
			}
			if !bytes.Contains([]byte(err.Error()), []byte(tc.wantIn)) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantIn)
			}
		})
	}
}

// readZip returns every entry of a zip archive keyed by name.
func readZip(t *testing.T, r io.Reader) map[string][]byte {
	t.Helper()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read zip bytes: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, openErr := f.Open()
		if openErr != nil {
			t.Fatalf("open zip entry %s: %v", f.Name, openErr)
		}
		body, readErr := io.ReadAll(rc)
		_ = rc.Close()
		if readErr != nil {
			t.Fatalf("read zip entry %s: %v", f.Name, readErr)
		}
		out[f.Name] = body
	}
	return out
}

// TestBundleToZip_EveryFieldSurvivesTheArchive marshals a fully-populated
// bundle and unmarshals each entry back into its DTO, so a field lost in the
// YAML layer fails here.
func TestBundleToZip_EveryFieldSurvivesTheArchive(t *testing.T) {
	want := fullBundle()

	r, err := bundleToZip(want)
	if err != nil {
		t.Fatalf("bundleToZip: %v", err)
	}
	entries := readZip(t, r)

	if len(entries) != 5 {
		t.Fatalf("zip entries: got %d (%v), want 5", len(entries), entries)
	}

	var settings SettingsConfig
	unmarshalEntry(t, entries, ".kandev/kandev.yml", &settings)
	assertEqual(t, "settings name", settings.Name, want.Settings.Name)
	assertEqual(t, "settings description", settings.Description, want.Settings.Description)

	var agent AgentConfig
	unmarshalEntry(t, entries, ".kandev/agents/ada.yml", &agent)
	assertEqual(t, "agent", agent, want.Agents[0])

	var skill SkillConfig
	unmarshalEntry(t, entries, ".kandev/skills/code-review.yml", &skill)
	assertEqual(t, "skill", skill, want.Skills[0])

	var routine RoutineConfig
	unmarshalEntry(t, entries, ".kandev/routines/standup.yml", &routine)
	assertEqual(t, "routine", routine, want.Routines[0])

	var project ProjectConfig
	unmarshalEntry(t, entries, ".kandev/projects/apollo.yml", &project)
	assertEqual(t, "project", project, want.Projects[0])
}

func unmarshalEntry(t *testing.T, entries map[string][]byte, name string, out interface{}) {
	t.Helper()
	body, ok := entries[name]
	if !ok {
		t.Fatalf("zip entry %q missing", name)
	}
	if err := yaml.Unmarshal(body, out); err != nil {
		t.Fatalf("unmarshal %s: %v", name, err)
	}
}

func TestBundleToZip_EmptyBundleStillCarriesSettings(t *testing.T) {
	r, err := bundleToZip(&ConfigBundle{})
	if err != nil {
		t.Fatalf("bundleToZip: %v", err)
	}
	entries := readZip(t, r)
	assertEqual(t, "zip entries", len(entries), 1)
	if _, ok := entries[".kandev/kandev.yml"]; !ok {
		t.Errorf("settings entry missing, got %v", entries)
	}
}

// TestExportZip_ReflectsDatabaseState wires ExportBundle and bundleToZip
// together over real rows.
func TestExportZip_ReflectsDatabaseState(t *testing.T) {
	env := newTestEnv(t)
	seedFullDBState(t, env, testWorkspaceID)

	r, err := env.svc.ExportZip(context.Background(), testWorkspaceID)
	if err != nil {
		t.Fatalf("ExportZip: %v", err)
	}
	entries := readZip(t, r)

	for _, name := range []string{
		".kandev/kandev.yml",
		".kandev/agents/ada.yml",
		".kandev/agents/grace.yml",
		".kandev/skills/code-review.yml",
		".kandev/routines/standup.yml",
		".kandev/projects/apollo.yml",
	} {
		if _, ok := entries[name]; !ok {
			t.Errorf("zip entry %q missing", name)
		}
	}

	var agent AgentConfig
	unmarshalEntry(t, entries, ".kandev/agents/ada.yml", &agent)
	assertEqual(t, "agent role", agent.Role, "cto")
	assertEqual(t, "agent reports_to", agent.ReportsTo, "grace")
	assertEqual(t, "agent budget", agent.BudgetMonthlyCents, 4200)
}

func TestExportZip_PropagatesExportError(t *testing.T) {
	env := newTestEnv(t)
	env.closeDB(t)

	r, err := env.svc.ExportZip(context.Background(), testWorkspaceID)
	if err == nil {
		t.Fatal("expected an error from a closed database, got nil")
	}
	if r != nil {
		t.Error("reader should be nil on error")
	}
}

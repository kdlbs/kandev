package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/configloader"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// testWorkspaceID is the DB workspace every fixture is written under. It is
// deliberately different from defaultWorkspaceName ("default"), which is the
// *filesystem* workspace directory the ConfigLoader reads — the two are
// separate namespaces and several tests depend on telling them apart.
const testWorkspaceID = "ws-test"

// activityEntry records one shared.ActivityLogger.LogActivity call.
type activityEntry struct {
	wsID       string
	actorType  string
	actorID    string
	action     string
	targetType string
	targetID   string
	details    string
}

// recordingActivityLogger captures activity calls made during an import.
type recordingActivityLogger struct {
	entries []activityEntry
}

func (r *recordingActivityLogger) LogActivity(
	_ context.Context, wsID, actorType, actorID, action, targetType, targetID, details string,
) {
	r.entries = append(r.entries, activityEntry{
		wsID: wsID, actorType: actorType, actorID: actorID,
		action: action, targetType: targetType, targetID: targetID, details: details,
	})
}

func (r *recordingActivityLogger) LogActivityWithRun(
	ctx context.Context, wsID, actorType, actorID, action, targetType, targetID, details, _, _ string,
) {
	r.LogActivity(ctx, wsID, actorType, actorID, action, targetType, targetID, details)
}

// testEnv bundles a ConfigService with everything a test needs to reach
// behind it: the DB it writes to, the on-disk config root the loader and
// writer share, and the activity log it emits to.
type testEnv struct {
	svc  *ConfigService
	repo *sqlite.Repository
	// db is the handle repository reads go through; wdb is the handle writes
	// go through. newTestEnv points both at one in-memory database;
	// newSplitTestEnv gives them separate handles so writes can be broken
	// while reads keep working.
	db       *sqlx.DB
	wdb      *sqlx.DB
	base     string
	loader   *configloader.ConfigLoader
	writer   *configloader.FileWriter
	activity *recordingActivityLogger
}

// newTestEnv builds a ConfigService over an in-memory SQLite repo and a
// temp-dir config root containing an empty "default" FS workspace.
//
// Office agent CRUD reads/writes the unified agent_profiles table (ADR 0005
// Wave C), so the settings-store schema is brought up on the same handles
// before the office repo initialises.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	db := newTestDB(t)

	repo, err := sqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	return newEnvAround(t, repo, db, db)
}

// newSplitTestEnv builds the same service over a file-backed database opened
// twice: once for reads and once for writes. Closing the write handle with
// breakWrites leaves every List working while every Create/Update/Delete
// fails, which is the only way to reach the write-error branches of the
// import and prune paths.
func newSplitTestEnv(t *testing.T) *testEnv {
	t.Helper()
	dsn := filepath.Join(t.TempDir(), "office.db")

	writerDB := openTestDB(t, dsn)
	if _, _, err := settingsstore.Provide(writerDB, writerDB, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	readerDB := openTestDB(t, dsn)

	repo, err := sqlite.NewWithDB(writerDB, readerDB, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return newEnvAround(t, repo, readerDB, writerDB)
}

func openTestDB(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite %s: %v", dsn, err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// newEnvAround wires a ConfigService around an already-built repository plus a
// fresh temp-dir config root holding an empty "default" FS workspace.
func newEnvAround(t *testing.T, repo *sqlite.Repository, readDB, writeDB *sqlx.DB) *testEnv {
	t.Helper()
	base := t.TempDir()
	wsDir := filepath.Join(base, "workspaces", defaultWorkspaceName)
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatalf("mkdir fs workspace: %v", err)
	}
	writeFile(t, filepath.Join(wsDir, "kandev.yml"), "name: default\n")

	loader := configloader.NewConfigLoader(base)
	if err := loader.Load(); err != nil {
		t.Fatalf("load config: %v", err)
	}
	writer := configloader.NewFileWriter(base, loader)
	activity := &recordingActivityLogger{}

	return &testEnv{
		svc:      NewConfigService(repo, loader, writer, logger.Default(), activity),
		repo:     repo,
		db:       readDB,
		wdb:      writeDB,
		base:     base,
		loader:   loader,
		writer:   writer,
		activity: activity,
	}
}

// breakWrites closes the write handle of a split environment.
func (e *testEnv) breakWrites(t *testing.T) {
	t.Helper()
	if e.wdb == e.db {
		t.Fatal("breakWrites requires newSplitTestEnv")
	}
	if err := e.wdb.Close(); err != nil {
		t.Fatalf("close write handle: %v", err)
	}
}

// newTestDB opens an in-memory SQLite database with the settings-store
// schema installed.
func newTestDB(t *testing.T) *sqlx.DB {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })

	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store init: %v", err)
	}
	return db
}

// closeDB closes the underlying database so every repository call fails.
// Used to exercise error propagation without a mock repository (the service
// holds a concrete *sqlite.Repository).
func (e *testEnv) closeDB(t *testing.T) {
	t.Helper()
	if err := e.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
}

// dropTable removes one table so calls touching that entity fail while the
// rest of the repository keeps working. This isolates a single stage of a
// multi-stage import, which closing the whole database cannot do.
func (e *testEnv) dropTable(t *testing.T, table string) {
	t.Helper()
	if _, err := e.db.Exec("DROP TABLE " + table); err != nil {
		t.Fatalf("drop table %s: %v", table, err)
	}
}

// wsPath returns a path inside the on-disk "default" workspace.
func (e *testEnv) wsPath(parts ...string) string {
	return filepath.Join(append([]string{e.base, "workspaces", defaultWorkspaceName}, parts...)...)
}

// writeFSAgent writes an agent YAML file into the on-disk workspace.
func (e *testEnv) writeFSAgent(t *testing.T, name, body string) {
	t.Helper()
	writeFileIn(t, e.wsPath("agents"), name+".yml", body)
}

// writeFSSkill writes a skills/<slug>/SKILL.md file into the on-disk workspace.
func (e *testEnv) writeFSSkill(t *testing.T, slug, content string) {
	t.Helper()
	writeFileIn(t, e.wsPath("skills", slug), "SKILL.md", content)
}

// writeFSRoutine writes a routine YAML file into the on-disk workspace.
func (e *testEnv) writeFSRoutine(t *testing.T, name, body string) {
	t.Helper()
	writeFileIn(t, e.wsPath("routines"), name+".yml", body)
}

// writeFSProject writes a project YAML file into the on-disk workspace.
func (e *testEnv) writeFSProject(t *testing.T, name, body string) {
	t.Helper()
	writeFileIn(t, e.wsPath("projects"), name+".yml", body)
}

func writeFileIn(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	writeFile(t, filepath.Join(dir, name), body)
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// fullBundle returns a ConfigBundle with every field of every entity set to a
// distinct non-zero value. Tests that import or export it assert field-by-field,
// so a field dropped anywhere in the pipeline fails loudly instead of silently
// arriving empty.
func fullBundle() *ConfigBundle {
	return &ConfigBundle{
		Settings: SettingsConfig{Name: "Acme Workspace", Description: "the whole office"},
		Agents: []AgentConfig{{
			Name:                  "ada",
			Role:                  "cto",
			Icon:                  "🤖",
			ReportsTo:             "grace",
			BudgetMonthlyCents:    4200,
			MaxConcurrentSessions: 3,
			DesiredSkills:         `["go","sql"]`,
			ExecutorPreference:    "local_docker",
		}},
		Skills: []SkillConfig{{
			Name:        "Code Review",
			Slug:        "code-review",
			Description: "reviews diffs",
			SourceType:  "inline",
			Content:     "# Code Review\n\nread the diff\n",
		}},
		Routines: []RoutineConfig{{
			Name:              "standup",
			Description:       "daily standup",
			TaskTemplate:      "summarise yesterday",
			AssigneeName:      "ada",
			ConcurrencyPolicy: "skip",
		}},
		Projects: []ProjectConfig{{
			Name:           "apollo",
			Description:    "moon shot",
			Status:         "paused",
			Color:          "#ff00ff",
			BudgetCents:    99000,
			Repositories:   `["kdlbs/kandev"]`,
			ExecutorConfig: `{"type":"local_docker"}`,
			LeadAgentName:  "ada",
		}},
	}
}

// hierarchyAgent returns a minimal AgentConfig for reports_to resolution
// tests. Role and counts are irrelevant to hierarchy resolution but keep the
// fixture realistic.
func hierarchyAgent(name, reportsTo string) AgentConfig {
	return AgentConfig{
		Name:                  name,
		Role:                  "engineer",
		ReportsTo:             reportsTo,
		BudgetMonthlyCents:    100,
		MaxConcurrentSessions: 1,
	}
}

// seedAgent inserts an office agent row and returns it.
func seedAgent(t *testing.T, e *testEnv, wsID, name string) *models.AgentInstance {
	t.Helper()
	agent := &models.AgentInstance{
		WorkspaceID:           wsID,
		Name:                  name,
		Role:                  models.AgentRole("engineer"),
		Icon:                  "🛠",
		Status:                models.AgentStatusIdle,
		BudgetMonthlyCents:    100,
		MaxConcurrentSessions: 1,
	}
	if err := e.repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("seed agent %s: %v", name, err)
	}
	return agent
}

// seedSkill inserts an office skill row and returns it.
func seedSkill(t *testing.T, e *testEnv, wsID, slug string) *models.Skill {
	t.Helper()
	skill := &models.Skill{
		WorkspaceID: wsID,
		Name:        slug,
		Slug:        slug,
		SourceType:  models.SkillSourceType("inline"),
		Content:     "seeded",
	}
	if err := e.repo.CreateSkill(context.Background(), skill); err != nil {
		t.Fatalf("seed skill %s: %v", slug, err)
	}
	return skill
}

// seedRoutine inserts an office routine row and returns it.
func seedRoutine(t *testing.T, e *testEnv, wsID, name string) *models.Routine {
	t.Helper()
	routine := &models.Routine{
		WorkspaceID: wsID,
		Name:        name,
		Status:      "active",
	}
	if err := e.repo.CreateRoutine(context.Background(), routine); err != nil {
		t.Fatalf("seed routine %s: %v", name, err)
	}
	return routine
}

// seedProject inserts an office project row and returns it.
func seedProject(t *testing.T, e *testEnv, wsID, name string) *models.Project {
	t.Helper()
	project := &models.Project{
		WorkspaceID: wsID,
		Name:        name,
		Status:      models.ProjectStatusActive,
	}
	if err := e.repo.CreateProject(context.Background(), project); err != nil {
		t.Fatalf("seed project %s: %v", name, err)
	}
	return project
}

// agentByName finds a seeded/imported agent by name, failing if absent.
func agentByName(t *testing.T, e *testEnv, wsID, name string) *models.AgentInstance {
	t.Helper()
	rows, err := e.repo.ListAgentInstances(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	for _, a := range rows {
		if a.Name == name {
			return a
		}
	}
	t.Fatalf("agent %q not found in workspace %q", name, wsID)
	return nil
}

// skillBySlug finds a seeded/imported skill by slug, failing if absent.
func skillBySlug(t *testing.T, e *testEnv, wsID, slug string) *models.Skill {
	t.Helper()
	rows, err := e.repo.ListSkills(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list skills: %v", err)
	}
	for _, sk := range rows {
		if sk.Slug == slug {
			return sk
		}
	}
	t.Fatalf("skill %q not found in workspace %q", slug, wsID)
	return nil
}

// routineByName finds a seeded/imported routine by name, failing if absent.
func routineByName(t *testing.T, e *testEnv, wsID, name string) *models.Routine {
	t.Helper()
	rows, err := e.repo.ListRoutines(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list routines: %v", err)
	}
	for _, r := range rows {
		if r.Name == name {
			return r
		}
	}
	t.Fatalf("routine %q not found in workspace %q", name, wsID)
	return nil
}

// projectByName finds a seeded/imported project by name, failing if absent.
func projectByName(t *testing.T, e *testEnv, wsID, name string) *models.Project {
	t.Helper()
	rows, err := e.repo.ListProjects(context.Background(), wsID)
	if err != nil {
		t.Fatalf("list projects: %v", err)
	}
	for _, p := range rows {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("project %q not found in workspace %q", name, wsID)
	return nil
}

// assertStrings compares a []string against want by length and elements. A nil
// slice and an empty slice are equivalent here; pass want=nil to mean "no
// entries" regardless of which the production code returns.
func assertStrings(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s: got %v (len %d), want %v (len %d)", label, got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("%s[%d]: got %q, want %q", label, i, got[i], want[i])
		}
	}
}

// assertEqual fails with a labelled message when got != want.
func assertEqual[T comparable](t *testing.T, label string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s: got %v, want %v", label, got, want)
	}
}

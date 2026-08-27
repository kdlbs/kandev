package sqlite_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// fullProject builds a Project with every persisted column set to a
// distinct non-zero value. Every field is asserted on read-back so a
// column dropped from the INSERT or the SELECT projection fails loudly
// instead of silently defaulting.
func fullProject(id, workspaceID, name string) *models.Project {
	return &models.Project{
		ID:                 id,
		WorkspaceID:        workspaceID,
		Name:               name,
		Description:        "ship the thing",
		Status:             models.ProjectStatus("archived"),
		LeadAgentProfileID: "agent-lead-7",
		Color:              "#ff00aa",
		BudgetCents:        123456,
		Repositories:       `["kdlbs/kandev","kdlbs/other"]`,
		ExecutorConfig:     `{"type":"local_docker"}`,
	}
}

// assertProjectEquals compares every persisted field of a project.
func assertProjectEquals(t *testing.T, got, want *models.Project) {
	t.Helper()
	if got.ID != want.ID {
		t.Errorf("ID = %q, want %q", got.ID, want.ID)
	}
	if got.WorkspaceID != want.WorkspaceID {
		t.Errorf("WorkspaceID = %q, want %q", got.WorkspaceID, want.WorkspaceID)
	}
	if got.Name != want.Name {
		t.Errorf("Name = %q, want %q", got.Name, want.Name)
	}
	if got.Description != want.Description {
		t.Errorf("Description = %q, want %q", got.Description, want.Description)
	}
	if got.Status != want.Status {
		t.Errorf("Status = %q, want %q", got.Status, want.Status)
	}
	if got.LeadAgentProfileID != want.LeadAgentProfileID {
		t.Errorf("LeadAgentProfileID = %q, want %q", got.LeadAgentProfileID, want.LeadAgentProfileID)
	}
	if got.Color != want.Color {
		t.Errorf("Color = %q, want %q", got.Color, want.Color)
	}
	if got.BudgetCents != want.BudgetCents {
		t.Errorf("BudgetCents = %d, want %d", got.BudgetCents, want.BudgetCents)
	}
	if got.Repositories != want.Repositories {
		t.Errorf("Repositories = %q, want %q", got.Repositories, want.Repositories)
	}
	if got.ExecutorConfig != want.ExecutorConfig {
		t.Errorf("ExecutorConfig = %q, want %q", got.ExecutorConfig, want.ExecutorConfig)
	}
	if !got.CreatedAt.Equal(want.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, want.CreatedAt)
	}
	if !got.UpdatedAt.Equal(want.UpdatedAt) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, want.UpdatedAt)
	}
}

func TestCreateProject_RoundTripsEveryField(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullProject("proj-full", "ws-1", "Apollo")
	before := time.Now().UTC().Add(-time.Second)
	if err := repo.CreateProject(ctx, want); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if want.CreatedAt.Before(before) || want.UpdatedAt.Before(before) {
		t.Fatalf("CreateProject did not stamp timestamps: %v / %v", want.CreatedAt, want.UpdatedAt)
	}

	got, err := repo.GetProject(ctx, "proj-full")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	assertProjectEquals(t, got, want)
}

func TestCreateProject_GeneratesIDWhenEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := fullProject("", "ws-1", "Generated")
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	if project.ID == "" {
		t.Fatal("CreateProject left ID empty")
	}
	if _, err := repo.GetProject(ctx, project.ID); err != nil {
		t.Fatalf("GetProject(generated id): %v", err)
	}
}

func TestGetProject_NotFound(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.GetProject(context.Background(), "missing")
	if err == nil {
		t.Fatal("GetProject(missing) = nil error, want not-found error")
	}
	if got != nil {
		t.Errorf("GetProject(missing) returned %+v, want nil", got)
	}
	if !strings.Contains(err.Error(), "project not found: missing") {
		t.Errorf("error = %q, want it to name the missing project", err)
	}
}

func TestGetProjectByName_ScopedToWorkspace(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	want := fullProject("proj-a", "ws-a", "Shared Name")
	if err := repo.CreateProject(ctx, want); err != nil {
		t.Fatalf("create ws-a project: %v", err)
	}
	other := fullProject("proj-b", "ws-b", "Shared Name")
	if err := repo.CreateProject(ctx, other); err != nil {
		t.Fatalf("create ws-b project: %v", err)
	}

	got, err := repo.GetProjectByName(ctx, "ws-a", "Shared Name")
	if err != nil {
		t.Fatalf("GetProjectByName: %v", err)
	}
	assertProjectEquals(t, got, want)
}

func TestGetProjectByName_NotFound(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateProject(ctx, fullProject("proj-a", "ws-a", "Apollo")); err != nil {
		t.Fatalf("create project: %v", err)
	}

	got, err := repo.GetProjectByName(ctx, "ws-other", "Apollo")
	if err == nil {
		t.Fatal("GetProjectByName(wrong workspace) = nil error, want not-found")
	}
	if got != nil {
		t.Errorf("returned %+v, want nil", got)
	}
	if !strings.Contains(err.Error(), "project not found: Apollo") {
		t.Errorf("error = %q, want it to name the project", err)
	}
}

func TestListProjects_ScopesAndOrdersByName(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, p := range []*models.Project{
		fullProject("p-zulu", "ws-1", "Zulu"),
		fullProject("p-alpha", "ws-1", "Alpha"),
		fullProject("p-mike", "ws-1", "Mike"),
		fullProject("p-other", "ws-2", "Bravo"),
	} {
		if err := repo.CreateProject(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.ID, err)
		}
	}

	scoped, err := repo.ListProjects(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListProjects(ws-1): %v", err)
	}
	gotNames := make([]string, 0, len(scoped))
	for _, p := range scoped {
		gotNames = append(gotNames, p.Name)
	}
	if strings.Join(gotNames, ",") != "Alpha,Mike,Zulu" {
		t.Errorf("ListProjects(ws-1) names = %v, want Alpha,Mike,Zulu in that order", gotNames)
	}

	all, err := repo.ListProjects(ctx, "")
	if err != nil {
		t.Fatalf("ListProjects(all): %v", err)
	}
	if len(all) != 4 {
		t.Fatalf("ListProjects(\"\") returned %d rows, want 4 (all workspaces)", len(all))
	}
	if all[0].Name != "Alpha" || all[1].Name != "Bravo" {
		t.Errorf("ListProjects(\"\") not ordered by name: %q, %q", all[0].Name, all[1].Name)
	}
}

func TestListProjects_EmptyIsNonNilSlice(t *testing.T) {
	repo := newTestRepo(t)

	got, err := repo.ListProjects(context.Background(), "ws-empty")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if got == nil {
		t.Fatal("ListProjects returned nil slice, want empty non-nil")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

func TestUpdateProject_PersistsEveryMutableField(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	project := &models.Project{
		ID:          "proj-upd",
		WorkspaceID: "ws-1",
		Name:        "Before",
		Status:      models.ProjectStatus("active"),
	}
	if err := repo.CreateProject(ctx, project); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}
	createdAt := project.CreatedAt

	project.Name = "After"
	project.Description = "new description"
	project.Status = models.ProjectStatus("paused")
	project.LeadAgentProfileID = "agent-new"
	project.Color = "#00ff00"
	project.BudgetCents = 999
	project.Repositories = `["kdlbs/updated"]`
	project.ExecutorConfig = `{"type":"local_pc"}`
	if err := repo.UpdateProject(ctx, project); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	got, err := repo.GetProject(ctx, "proj-upd")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	assertProjectEquals(t, got, project)
	if !got.CreatedAt.Equal(createdAt) {
		t.Errorf("UpdateProject moved CreatedAt: %v, want %v", got.CreatedAt, createdAt)
	}
	if !got.UpdatedAt.After(createdAt) && !got.UpdatedAt.Equal(createdAt) {
		t.Errorf("UpdatedAt = %v, want >= CreatedAt %v", got.UpdatedAt, createdAt)
	}
}

func TestUpdateProject_DoesNotTouchOtherRows(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	target := fullProject("proj-target", "ws-1", "Target")
	bystander := fullProject("proj-bystander", "ws-1", "Bystander")
	for _, p := range []*models.Project{target, bystander} {
		if err := repo.CreateProject(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.ID, err)
		}
	}

	target.Name = "Renamed"
	if err := repo.UpdateProject(ctx, target); err != nil {
		t.Fatalf("UpdateProject: %v", err)
	}

	got, err := repo.GetProject(ctx, "proj-bystander")
	if err != nil {
		t.Fatalf("GetProject(bystander): %v", err)
	}
	assertProjectEquals(t, got, bystander)
}

func TestDeleteProject_RemovesOnlyTargetRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, p := range []*models.Project{
		fullProject("p-del", "ws-1", "Doomed"),
		fullProject("p-keep", "ws-1", "Kept"),
	} {
		if err := repo.CreateProject(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.ID, err)
		}
	}

	if err := repo.DeleteProject(ctx, "p-del"); err != nil {
		t.Fatalf("DeleteProject: %v", err)
	}

	remaining, err := repo.ListProjects(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("remaining projects = %d, want 1: %+v", len(remaining), remaining)
	}
	if remaining[0].ID != "p-keep" {
		t.Errorf("survivor = %q, want p-keep", remaining[0].ID)
	}
	if _, err := repo.GetProject(ctx, "p-del"); err == nil {
		t.Error("GetProject(deleted) = nil error, want not-found")
	}
}

func TestDeleteProject_MissingRowIsNotAnError(t *testing.T) {
	repo := newTestRepo(t)

	if err := repo.DeleteProject(context.Background(), "never-existed"); err != nil {
		t.Fatalf("DeleteProject(missing) = %v, want nil", err)
	}
}

func TestListProjectsWithCounts_BucketsTaskStates(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()

	busy := fullProject("p-busy", "ws-1", "Busy")
	idle := fullProject("p-idle", "ws-1", "Idle")
	elsewhere := fullProject("p-elsewhere", "ws-2", "Elsewhere")
	for _, p := range []*models.Project{busy, idle, elsewhere} {
		if err := repo.CreateProject(ctx, p); err != nil {
			t.Fatalf("create %s: %v", p.ID, err)
		}
	}

	// 6 tasks on p-busy: 2 in-progress buckets, 1 done, 1 blocked, 2 other.
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO tasks (id, workspace_id, project_id, state, title, created_at, updated_at) VALUES
			('t-inprog',   'ws-1', 'p-busy', 'IN_PROGRESS', 'a', datetime('now'), datetime('now')),
			('t-sched',    'ws-1', 'p-busy', 'SCHEDULING',  'b', datetime('now'), datetime('now')),
			('t-done',     'ws-1', 'p-busy', 'COMPLETED',   'c', datetime('now'), datetime('now')),
			('t-blocked',  'ws-1', 'p-busy', 'BLOCKED',     'd', datetime('now'), datetime('now')),
			('t-todo',     'ws-1', 'p-busy', 'TODO',        'e', datetime('now'), datetime('now')),
			('t-review',   'ws-1', 'p-busy', 'REVIEW',      'f', datetime('now'), datetime('now')),
			('t-noproject','ws-1', '',       'TODO',        'g', datetime('now'), datetime('now'))
	`); err != nil {
		t.Fatalf("seed tasks: %v", err)
	}

	got, err := repo.ListProjectsWithCounts(ctx, "ws-1")
	if err != nil {
		t.Fatalf("ListProjectsWithCounts: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("projects = %d, want 2 (ws-1 only): %+v", len(got), got)
	}

	byID := map[string]*models.ProjectWithCounts{}
	for _, p := range got {
		byID[p.ID] = p
	}
	busyCounts := byID["p-busy"].TaskCounts
	want := models.TaskCounts{Total: 6, InProgress: 2, Done: 1, Blocked: 1}
	if busyCounts != want {
		t.Errorf("p-busy counts = %+v, want %+v", busyCounts, want)
	}
	// A project with zero tasks must still appear, with all counts at 0
	// (that is the point of the LEFT JOIN).
	if idleCounts := byID["p-idle"].TaskCounts; idleCounts != (models.TaskCounts{}) {
		t.Errorf("p-idle counts = %+v, want all zero", idleCounts)
	}
	// The embedded Project must be fully hydrated, not just the id.
	assertProjectEquals(t, &byID["p-busy"].Project, busy)
}

func TestListProjectsWithCounts_EmptyWorkspace(t *testing.T) {
	repo := newSearchTestRepo(t)

	got, err := repo.ListProjectsWithCounts(context.Background(), "ws-nothing")
	if err != nil {
		t.Fatalf("ListProjectsWithCounts: %v", err)
	}
	if got == nil {
		t.Fatal("returned nil slice, want empty non-nil")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// GetProjectWorkspaceID lives in tasks.go but reads office_projects, so it
// is exercised alongside the project CRUD it depends on.
func TestGetProjectWorkspaceID(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	if err := repo.CreateProject(ctx, fullProject("p-ws", "ws-lookup", "Lookup")); err != nil {
		t.Fatalf("CreateProject: %v", err)
	}

	cases := []struct {
		name      string
		projectID string
		want      string
	}{
		{"existing project", "p-ws", "ws-lookup"},
		{"missing project", "p-nope", ""},
		{"empty id short-circuits", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.GetProjectWorkspaceID(ctx, tc.projectID)
			if err != nil {
				t.Fatalf("GetProjectWorkspaceID: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

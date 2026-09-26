package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seedPendingTitleTask creates a task whose generated title is claimed by
// ownerSessionID (empty means pending but unclaimed).
func seedPendingTitleTask(t *testing.T, repo *sqliterepo.Repository, taskID, ownerSessionID string, pending bool) {
	t.Helper()
	metadata := map[string]interface{}{}
	if pending {
		metadata[models.MetaKeyAgentTitlePending] = true
	}
	if ownerSessionID != "" {
		metadata[models.MetaKeyAgentTitleOwnerSessionID] = ownerSessionID
	}
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: taskID, WorkspaceID: "ws-title", WorkflowID: "wf-title", WorkflowStepID: "step-1",
		Title: "Untitled", State: v1.TaskStateCreated, Priority: "medium", Metadata: metadata,
	}); err != nil {
		t.Fatalf("create task %s: %v", taskID, err)
	}
}

func newTitleTestService(t *testing.T) (*Service, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	svc, bus, repo := createTestService(t)
	ctx := context.Background()
	if err := repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-title", Name: "Titles"}); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if err := repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf-title", WorkspaceID: "ws-title", Name: "flow"}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	bus.ClearEvents()
	return svc, bus, repo
}

func TestSetPendingAgentTitleAppliesClaimedTitle(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-title", "sess-owner", true)

	task, accepted, reason, err := svc.SetPendingAgentTitle(ctx, "task-title", "sess-owner", "  Fix the login bug  ")
	if err != nil {
		t.Fatalf("SetPendingAgentTitle: %v", err)
	}
	if !accepted || reason != "" {
		t.Fatalf("accepted = %v, reason = %q; want an accepted write", accepted, reason)
	}
	if task.Title != "Fix the login bug" {
		t.Fatalf("title = %q, want the trimmed title", task.Title)
	}
	if models.IsAgentTitlePending(task.Metadata) {
		t.Fatalf("metadata = %v, want the pending marker cleared", task.Metadata)
	}
	if models.AgentTitleOwnerSessionID(task.Metadata) != "" {
		t.Fatalf("metadata = %v, want the owner claim cleared", task.Metadata)
	}

	stored, err := repo.GetTask(ctx, "task-title")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != "Fix the login bug" {
		t.Fatalf("persisted title = %q", stored.Title)
	}
	if types := eventTypes(bus.GetPublishedEvents()); len(types) != 1 || types[0] != events.TaskUpdated {
		t.Fatalf("published %v, want exactly one %s", types, events.TaskUpdated)
	}
}

func TestSetPendingAgentTitleRefusesUnclaimedAndForeignSessions(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-not-pending", "sess-owner", false)
	seedPendingTitleTask(t, repo, "task-other-owner", "sess-owner", true)

	task, accepted, reason, err := svc.SetPendingAgentTitle(ctx, "task-not-pending", "sess-owner", "New title")
	if err != nil {
		t.Fatalf("SetPendingAgentTitle: %v", err)
	}
	if accepted || reason != titleNotPendingReason {
		t.Fatalf("accepted = %v, reason = %q; want %q", accepted, reason, titleNotPendingReason)
	}
	if task.Title != "Untitled" {
		t.Fatalf("title = %q, want it untouched", task.Title)
	}

	task, accepted, reason, err = svc.SetPendingAgentTitle(ctx, "task-other-owner", "sess-intruder", "New title")
	if err != nil {
		t.Fatalf("SetPendingAgentTitle: %v", err)
	}
	if accepted || reason != titleNotOwnerReason {
		t.Fatalf("accepted = %v, reason = %q; want %q", accepted, reason, titleNotOwnerReason)
	}
	if task.Title != "Untitled" {
		t.Fatalf("title = %q, want it untouched", task.Title)
	}

	// An empty session id can never own the claim.
	_, accepted, reason, err = svc.SetPendingAgentTitle(ctx, "task-other-owner", "", "New title")
	if err != nil {
		t.Fatalf("SetPendingAgentTitle: %v", err)
	}
	if accepted || reason != titleNotOwnerReason {
		t.Fatalf("accepted = %v, reason = %q; want %q", accepted, reason, titleNotOwnerReason)
	}

	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("refused title writes published %v, want nothing", eventTypes(published))
	}
}

func TestSetPendingAgentTitleValidatesTitle(t *testing.T) {
	svc, bus, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-validate", "sess-owner", true)

	if _, _, _, err := svc.SetPendingAgentTitle(ctx, "task-validate", "sess-owner", "   "); err == nil {
		t.Fatal("a blank title must be rejected")
	}
	stored, err := repo.GetTask(ctx, "task-validate")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != "Untitled" {
		t.Fatalf("title = %q, want it untouched after a rejected write", stored.Title)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("rejected title published %v, want nothing", eventTypes(published))
	}

	if _, _, _, err := svc.SetPendingAgentTitle(ctx, "task-missing", "sess-owner", "Title"); err == nil {
		t.Fatal("setting a title on an unknown task must fail")
	}
}

func TestSetPendingAgentTitleIsWorkspaceScoped(t *testing.T) {
	svc, bus, repo := createTestService(t)
	seedScopedWorkspaces(t, repo)
	ctx := context.Background()
	// UpdateTask deliberately strips the pending-title keys, so claim them the
	// way production does: a targeted metadata write.
	if err := repo.SetTaskMetadataKey(ctx, "task-b", models.MetaKeyAgentTitlePending, true); err != nil {
		t.Fatalf("mark task title pending: %v", err)
	}
	if err := repo.SetTaskMetadataKey(ctx, "task-b", models.MetaKeyAgentTitleOwnerSessionID, "sess-b"); err != nil {
		t.Fatalf("claim task title: %v", err)
	}
	if reloaded, err := repo.GetTask(ctx, "task-b"); err != nil {
		t.Fatalf("GetTask: %v", err)
	} else if !models.IsAgentTitleOwner(reloaded.Metadata, "sess-b") {
		t.Fatalf("fixture metadata = %v, want a pending title claimed by sess-b", reloaded.Metadata)
	}
	bus.ClearEvents()

	_, _, _, err := svc.SetPendingAgentTitle(ctxAs("user-a"), "task-b", "sess-b", "Hijacked")
	if !errors.Is(err, repoerrors.ErrTaskNotFound) {
		t.Fatalf("foreign SetPendingAgentTitle = %v, want ErrTaskNotFound", err)
	}
	stored, err := repo.GetTask(ctx, "task-b")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if stored.Title != "B's task" {
		t.Fatalf("title = %q, want it untouched by a denied caller", stored.Title)
	}
	if published := bus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("denied title write published %v, want nothing", eventTypes(published))
	}

	if _, accepted, _, err := svc.SetPendingAgentTitle(ctxAs("user-b"), "task-b", "sess-b", "Owner title"); err != nil || !accepted {
		t.Fatalf("owner SetPendingAgentTitle accepted = %v, err = %v", accepted, err)
	}
}

func TestReplaceTaskRepositoriesSwapsAssociations(t *testing.T) {
	svc, _, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-repos", "", false)
	for _, id := range []string{"repo-a", "repo-b"} {
		if err := repo.CreateRepository(ctx, &models.Repository{
			ID: id, WorkspaceID: "ws-title", Name: id, DefaultBranch: "main",
		}); err != nil {
			t.Fatalf("create repository %s: %v", id, err)
		}
	}

	if err := svc.ReplaceTaskRepositories(ctx, "task-repos", "ws-title", []TaskRepositoryInput{
		{RepositoryID: "repo-a", BaseBranch: "main"},
	}); err != nil {
		t.Fatalf("first ReplaceTaskRepositories: %v", err)
	}
	linked, err := repo.ListTaskRepositories(ctx, "task-repos")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(linked) != 1 || linked[0].RepositoryID != "repo-a" {
		t.Fatalf("task repositories = %+v, want just repo-a", linked)
	}

	if err := svc.ReplaceTaskRepositories(ctx, "task-repos", "ws-title", []TaskRepositoryInput{
		{RepositoryID: "repo-b", BaseBranch: "develop"},
	}); err != nil {
		t.Fatalf("second ReplaceTaskRepositories: %v", err)
	}
	linked, err = repo.ListTaskRepositories(ctx, "task-repos")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(linked) != 1 || linked[0].RepositoryID != "repo-b" {
		t.Fatalf("task repositories = %+v, want the previous link replaced by repo-b", linked)
	}
	if linked[0].BaseBranch != "develop" {
		t.Fatalf("base branch = %q, want develop", linked[0].BaseBranch)
	}

	// Replacing with an empty list clears every association.
	if err := svc.ReplaceTaskRepositories(ctx, "task-repos", "ws-title", nil); err != nil {
		t.Fatalf("clearing ReplaceTaskRepositories: %v", err)
	}
	linked, err = repo.ListTaskRepositories(ctx, "task-repos")
	if err != nil {
		t.Fatalf("ListTaskRepositories: %v", err)
	}
	if len(linked) != 0 {
		t.Fatalf("task repositories = %+v, want none", linked)
	}
}

func TestIsConfigTaskAcceptsNumericAndBooleanMarkers(t *testing.T) {
	cases := []struct {
		name string
		meta map[string]interface{}
		want bool
	}{
		{"no metadata", nil, false},
		{"unrelated key", map[string]interface{}{"other": 1}, false},
		{"json number", map[string]interface{}{"config_mode": float64(1)}, true},
		{"json number zero", map[string]interface{}{"config_mode": float64(0)}, false},
		{"int", map[string]interface{}{"config_mode": 1}, true},
		{"int other", map[string]interface{}{"config_mode": 2}, false},
		{"bool true", map[string]interface{}{"config_mode": true}, true},
		{"bool false", map[string]interface{}{"config_mode": false}, false},
		{"string is not a marker", map[string]interface{}{"config_mode": "1"}, false},
	}
	for _, tc := range cases {
		if got := isConfigTask(&models.Task{Metadata: tc.meta}); got != tc.want {
			t.Fatalf("%s: isConfigTask = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestListTasksByWorkspaceWithArchiveModeSeparatesArchived(t *testing.T) {
	svc, _, repo := newTitleTestService(t)
	ctx := context.Background()
	seedPendingTitleTask(t, repo, "task-live", "", false)
	seedPendingTitleTask(t, repo, "task-archived", "", false)
	if err := svc.ArchiveTask(ctx, "task-archived"); err != nil {
		t.Fatalf("ArchiveTask: %v", err)
	}

	live, total, err := svc.ListTasksByWorkspace(ctx, "ws-title", "", "", "", 1, 50, "", false, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace: %v", err)
	}
	if total != 1 || len(live) != 1 || live[0].ID != "task-live" {
		t.Fatalf("live listing = %d tasks (total %d), want just task-live", len(live), total)
	}

	archived, total, err := svc.ListTasksByWorkspaceWithArchiveMode(ctx, "ws-title", "", "", "", 1, 50, "", false, false, false, false, true)
	if err != nil {
		t.Fatalf("ListTasksByWorkspaceWithArchiveMode: %v", err)
	}
	if total != 1 || len(archived) != 1 || archived[0].ID != "task-archived" {
		t.Fatalf("archived listing = %+v (total %d), want just task-archived", archived, total)
	}

	both, total, err := svc.ListTasksByWorkspace(ctx, "ws-title", "", "", "", 1, 50, "", true, false, false, false)
	if err != nil {
		t.Fatalf("ListTasksByWorkspace including archived: %v", err)
	}
	if total != 2 || len(both) != 2 {
		t.Fatalf("combined listing = %d tasks (total %d), want 2", len(both), total)
	}
}

func TestParsePRQueryAcceptsOptionalHash(t *testing.T) {
	cases := []struct {
		query string
		want  int
		ok    bool
	}{
		{"123", 123, true},
		{"#123", 123, true},
		{"  #42  ", 42, true},
		{"", 0, false},
		{"#", 0, false},
		{"0", 0, false},
		{"-5", 0, false},
		{"login bug", 0, false},
	}
	for _, tc := range cases {
		got, ok := parsePRQuery(tc.query)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("parsePRQuery(%q) = %d, %v; want %d, %v", tc.query, got, ok, tc.want, tc.ok)
		}
	}
}

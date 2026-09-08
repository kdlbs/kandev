package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// countingTaskRepo records whether the quick-chat listing ever reached the task
// store. A denial must be decided before the query runs, so the counter staying
// at zero is what proves the guard is an authorization check and not a filter
// applied to rows we already read.
type countingTaskRepo struct {
	repository.TaskRepository
	listCalls int
}

func (r *countingTaskRepo) ListTasksByWorkspace(
	ctx context.Context,
	workspaceID, workflowID, repositoryID, query string,
	page, pageSize int,
	sort string,
	includeArchived, includeEphemeral, onlyEphemeral, excludeConfig bool,
) ([]*models.Task, int, error) {
	r.listCalls++
	return r.TaskRepository.ListTasksByWorkspace(
		ctx, workspaceID, workflowID, repositoryID, query,
		page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig,
	)
}

// seedScopedQuickChats gives user-a and user-b one owned workspace each, with a
// single restorable quick chat apiece.
func seedScopedQuickChats(t *testing.T, svc *Service, sqlxDB *sqlx.DB) {
	t.Helper()
	ctx := context.Background()
	for _, ws := range []*models.Workspace{
		{ID: "ws-qc-a", Name: "A's", OwnerID: "user-a"},
		{ID: "ws-qc-b", Name: "B's", OwnerID: "user-b"},
	} {
		if err := svc.workspaces.CreateWorkspace(ctx, ws); err != nil {
			t.Fatalf("CreateWorkspace(%s): %v", ws.ID, err)
		}
	}
	base := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "qc-task-a", workspaceID: "ws-qc-a", title: "A's secret chat",
		agentProfileID: "agent-a", updatedAt: base,
	})
	seedQuickChatTask(t, svc, sqlxDB, quickChatFixture{
		id: "qc-task-b", workspaceID: "ws-qc-b", title: "B's secret chat",
		agentProfileID: "agent-b", updatedAt: base,
	})
}

// TestListQuickChatSessionsDeniesForeignWorkspace is the core regression: quick
// chat names are user-authored text, so a foreign listing must fail closed and
// must fail before the task query runs.
func TestListQuickChatSessionsDeniesForeignWorkspace(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	seedScopedQuickChats(t, svc, sqlxDB)

	counter := &countingTaskRepo{TaskRepository: svc.tasks}
	svc.tasks = counter

	items, err := svc.ListQuickChatSessions(ctxAs("user-b"), "ws-qc-a")
	if !errors.Is(err, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("foreign quick-chat listing: got (%v, %v); want ErrWorkspaceNotFound", items, err)
	}
	if len(items) != 0 {
		t.Fatalf("denied listing leaked %d tabs", len(items))
	}
	if counter.listCalls != 0 {
		t.Fatalf("quick-chat query ran %d time(s) for a denied caller; want 0", counter.listCalls)
	}
}

// TestListQuickChatSessionsForeignMatchesNonexistent pins the no-existence-leak
// guarantee at the service boundary: both cases resolve to the same sentinel,
// which is what makes them collapse into one response. The sentinels carry
// different detail text (the repository names the missing ID), so the byte-level
// identity of the two responses is pinned at the HTTP boundary instead, in
// TestHTTPListQuickChatSessionsDeniesForeignWorkspace.
func TestListQuickChatSessionsForeignMatchesNonexistent(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	seedScopedQuickChats(t, svc, sqlxDB)

	_, foreignErr := svc.ListQuickChatSessions(ctxAs("user-b"), "ws-qc-a")
	_, missingErr := svc.ListQuickChatSessions(ctxAs("user-b"), "ws-does-not-exist")

	if !errors.Is(foreignErr, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("foreign workspace: got %v; want ErrWorkspaceNotFound", foreignErr)
	}
	if !errors.Is(missingErr, repoerrors.ErrWorkspaceNotFound) {
		t.Fatalf("nonexistent workspace: got %v; want ErrWorkspaceNotFound", missingErr)
	}
}

// TestListQuickChatSessionsOwnerStillSeesTabs guards against the fix turning
// into a denial for the legitimate owner.
func TestListQuickChatSessionsOwnerStillSeesTabs(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	seedScopedQuickChats(t, svc, sqlxDB)

	items, err := svc.ListQuickChatSessions(ctxAs("user-a"), "ws-qc-a")
	if err != nil {
		t.Fatalf("owner listing: %v", err)
	}
	if len(items) != 1 || items[0].Name != "A's secret chat" {
		t.Fatalf("owner listing = %+v; want one tab named %q", items, "A's secret chat")
	}
}

// TestListQuickChatSessionsUnscopedCallersUnchanged pins single-user behavior:
// the synthetic identity (auth disabled) and identity-less internal callers see
// every workspace's tabs exactly as before.
func TestListQuickChatSessionsUnscopedCallersUnchanged(t *testing.T) {
	svc, sqlxDB := createOfficeIntegrationServiceWithDB(t)
	seedScopedQuickChats(t, svc, sqlxDB)

	for name, ctx := range map[string]context.Context{
		"auth disabled": ctxSynthetic(),
		"internal":      context.Background(),
	} {
		for _, ws := range []struct{ id, want string }{
			{"ws-qc-a", "A's secret chat"},
			{"ws-qc-b", "B's secret chat"},
		} {
			items, err := svc.ListQuickChatSessions(ctx, ws.id)
			if err != nil {
				t.Fatalf("%s caller listing %s: %v", name, ws.id, err)
			}
			if len(items) != 1 || items[0].Name != ws.want {
				t.Fatalf("%s caller listing %s = %+v; want one tab named %q", name, ws.id, items, ws.want)
			}
		}
	}
}

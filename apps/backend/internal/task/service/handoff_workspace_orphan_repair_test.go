package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

// repairFakeRepo layers the workspaceOrphanRepairRepository selections on
// top of phase4TaskRepo (which already satisfies the full
// repository.TaskRepository surface plus the guarded CAS write). Real
// production wiring only ever has all three methods together (they're one
// concrete *sqlite.Repository), so a single fake covering all three keeps
// these tests close to that shape.
type repairFakeRepo struct {
	*phase4TaskRepo
	candidates    []models.OrphanRepairCandidate
	candidatesErr error
	markers       []models.StaleOrphanMarker
	markersErr    error
	malformed     int
	malformedErr  error
}

func (r *repairFakeRepo) ListOrphanRepairCandidates(context.Context) ([]models.OrphanRepairCandidate, error) {
	return r.candidates, r.candidatesErr
}

func (r *repairFakeRepo) ListStaleOrphanMarkers(context.Context) ([]models.StaleOrphanMarker, error) {
	return r.markers, r.markersErr
}

func (r *repairFakeRepo) CountMalformedTaskMetadata(context.Context) (int, error) {
	return r.malformed, r.malformedErr
}

func newRepairFakeRepo(fakeTasks *fakeTaskRepo) *repairFakeRepo {
	return &repairFakeRepo{phase4TaskRepo: &phase4TaskRepo{base: fakeTasks}}
}

func newRepairService(t *testing.T, repo *repairFakeRepo, pub *repairCapturingPublisher) *HandoffService {
	t.Helper()
	svc := NewHandoffService(repo, nil, nil, nil, nil, nil)
	if pub != nil {
		svc.SetTaskEventPublisher(pub)
	}
	return svc
}

// repairCapturingPublisher records the full task snapshot passed to
// PublishTaskUpdated, unlike the shared fakeEventPublisher (which only
// records IDs) - AC-003.5 needs to assert on the published metadata
// content, not just that a publish happened.
type repairCapturingPublisher struct {
	updated []*models.Task
}

func (p *repairCapturingPublisher) PublishTaskUpdated(_ context.Context, task *models.Task, _ ...string) {
	p.updated = append(p.updated, task)
}

func (p *repairCapturingPublisher) PublishTaskDeleted(context.Context, *models.Task) {}

// TestRepairOrphanedWorkspaceMarkers_UnsupportedRepositorySkips covers the
// "never true in production, but must not panic wiring without it" path:
// a repository that doesn't implement workspaceOrphanRepairRepository (the
// same phase4TaskRepo other handoff tests use) is skipped outright.
func TestRepairOrphanedWorkspaceMarkers_UnsupportedRepositorySkips(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("child", "parent", "")
	svc := newPhase4Service(t, fakeTasks, nil, nil)

	// Must not panic despite the repository lacking the repair methods.
	svc.RepairOrphanedWorkspaceMarkers(context.Background())
}

// TestRepairOrphanedWorkspaceMarkers_StampsCandidate covers AC-003.1/.7: a
// selected candidate whose guard still holds gets stamped and the
// post-write snapshot is published (AC-003.5).
func TestRepairOrphanedWorkspaceMarkers_StampsCandidate(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("parent", "", "")
	fakeTasks.tasks["parent"].ArchivedAt = timePtr()
	fakeTasks.addTask("child", "parent", "")
	fakeTasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent"},
	}

	repo := newRepairFakeRepo(fakeTasks)
	repo.candidates = []models.OrphanRepairCandidate{
		{TaskID: "child", ParentID: "parent", Workspace: map[string]interface{}{"mode": "inherit_parent"}},
	}
	pub := &repairCapturingPublisher{}
	svc := newRepairService(t, repo, pub)

	svc.RepairOrphanedWorkspaceMarkers(context.Background())

	ws, _ := fakeTasks.tasks["child"].Metadata["workspace"].(map[string]interface{})
	if orphaned, _ := ws["orphaned"].(bool); !orphaned {
		t.Fatalf("expected child to be stamped orphaned, got workspace=%v", ws)
	}
	if parentID, _ := ws["orphaned_parent_id"].(string); parentID != "parent" {
		t.Errorf("orphaned_parent_id = %q, want %q", parentID, "parent")
	}
	if len(pub.updated) != 1 || pub.updated[0].ID != "child" {
		t.Fatalf("expected one PublishTaskUpdated for child, got %v", pub.updated)
	}
	// AC-003.5: the published snapshot must carry the just-written metadata,
	// not the pre-write selection row.
	if orphaned, _ := pub.updated[0].Metadata["workspace"].(map[string]interface{})["orphaned"].(bool); !orphaned {
		t.Errorf("published snapshot missing stamped marker: %v", pub.updated[0].Metadata)
	}
}

// TestRepairOrphanedWorkspaceMarkers_WarnAndContinueOnLostGuard covers
// AC-003.6: one candidate's guard fails (its named parent isn't actually
// archived - the window-race case) while a second candidate succeeds. The
// failure must not abort the pass.
func TestRepairOrphanedWorkspaceMarkers_WarnAndContinueOnLostGuard(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("parent-live", "", "") // never archived: guard will fail
	fakeTasks.addTask("stale-child", "parent-live", "")
	fakeTasks.tasks["stale-child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent"},
	}

	fakeTasks.addTask("parent-archived", "", "")
	fakeTasks.tasks["parent-archived"].ArchivedAt = timePtr()
	fakeTasks.addTask("good-child", "parent-archived", "")
	fakeTasks.tasks["good-child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent"},
	}

	repo := newRepairFakeRepo(fakeTasks)
	repo.candidates = []models.OrphanRepairCandidate{
		{TaskID: "stale-child", ParentID: "parent-live", Workspace: map[string]interface{}{"mode": "inherit_parent"}},
		{TaskID: "good-child", ParentID: "parent-archived", Workspace: map[string]interface{}{"mode": "inherit_parent"}},
	}
	pub := &repairCapturingPublisher{}
	svc := newRepairService(t, repo, pub)

	svc.RepairOrphanedWorkspaceMarkers(context.Background())

	if ws, _ := fakeTasks.tasks["stale-child"].Metadata["workspace"].(map[string]interface{}); ws["orphaned"] != nil {
		t.Errorf("stale-child guard should have failed (parent not archived), got workspace=%v", ws)
	}
	if ws, _ := fakeTasks.tasks["good-child"].Metadata["workspace"].(map[string]interface{}); ws["orphaned"] != true {
		t.Errorf("good-child should still be stamped despite the other candidate's lost guard, got workspace=%v", ws)
	}
	if len(pub.updated) != 1 || pub.updated[0].ID != "good-child" {
		t.Fatalf("expected exactly one publish for good-child, got %v", pub.updated)
	}
}

// TestRepairOrphanedWorkspaceMarkers_ClearsStaleMarker covers AC-003.9: a
// marker naming a parent that is now unarchived (the parent was
// unarchived after the marker was written) is retracted.
func TestRepairOrphanedWorkspaceMarkers_ClearsStaleMarker(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("parent", "", "") // unarchived - the claim is stale
	fakeTasks.addTask("child", "parent", "")
	fakeTasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{
			"mode":               "inherit_parent",
			"orphaned":           true,
			"orphaned_parent_id": "parent",
			"orphaned_reason":    "parent_archived",
			"orphaned_at":        "2026-01-01T00:00:00Z",
		},
	}

	repo := newRepairFakeRepo(fakeTasks)
	repo.markers = []models.StaleOrphanMarker{
		{TaskID: "child", Workspace: map[string]interface{}{
			"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "parent",
			"orphaned_reason": "parent_archived", "orphaned_at": "2026-01-01T00:00:00Z",
		}},
	}
	pub := &repairCapturingPublisher{}
	svc := newRepairService(t, repo, pub)

	svc.RepairOrphanedWorkspaceMarkers(context.Background())

	ws, _ := fakeTasks.tasks["child"].Metadata["workspace"].(map[string]interface{})
	if _, present := ws["orphaned"]; present {
		t.Errorf("expected stale marker cleared, got workspace=%v", ws)
	}
	if len(pub.updated) != 1 || pub.updated[0].ID != "child" {
		t.Fatalf("expected one PublishTaskUpdated for child, got %v", pub.updated)
	}
}

// TestRepairOrphanedWorkspaceMarkers_SelectionFailureDoesNotAbortOtherPass
// covers AC-003.11: a failed selection (e.g. candidates query error) must
// not prevent the other pass (clearing) from running.
func TestRepairOrphanedWorkspaceMarkers_SelectionFailureDoesNotAbortOtherPass(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("parent", "", "")
	fakeTasks.addTask("child", "parent", "")
	fakeTasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{
			"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "parent",
		},
	}

	repo := newRepairFakeRepo(fakeTasks)
	repo.candidatesErr = errors.New("boom")
	repo.markers = []models.StaleOrphanMarker{
		{TaskID: "child", Workspace: map[string]interface{}{
			"mode": "inherit_parent", "orphaned": true, "orphaned_parent_id": "parent",
		}},
	}
	pub := &repairCapturingPublisher{}
	svc := newRepairService(t, repo, pub)

	svc.RepairOrphanedWorkspaceMarkers(context.Background())

	ws, _ := fakeTasks.tasks["child"].Metadata["workspace"].(map[string]interface{})
	if _, present := ws["orphaned"]; present {
		t.Errorf("clearing pass should still have run despite the candidates error, got workspace=%v", ws)
	}
}

// TestRepairOrphanedWorkspaceMarkers_IdempotentSecondPass covers AC-003.3:
// running the repair again once nothing is left to fix does nothing and
// does not publish spuriously.
func TestRepairOrphanedWorkspaceMarkers_IdempotentSecondPass(t *testing.T) {
	fakeTasks := newFakeTaskRepo()
	fakeTasks.addTask("parent", "", "")
	fakeTasks.tasks["parent"].ArchivedAt = timePtr()
	fakeTasks.addTask("child", "parent", "")
	fakeTasks.tasks["child"].Metadata = map[string]interface{}{
		"workspace": map[string]interface{}{"mode": "inherit_parent"},
	}

	repo := newRepairFakeRepo(fakeTasks)
	repo.candidates = []models.OrphanRepairCandidate{
		{TaskID: "child", ParentID: "parent", Workspace: map[string]interface{}{"mode": "inherit_parent"}},
	}
	pub := &repairCapturingPublisher{}
	svc := newRepairService(t, repo, pub)
	svc.RepairOrphanedWorkspaceMarkers(context.Background())
	if len(pub.updated) != 1 {
		t.Fatalf("setup: expected one publish from the first pass, got %v", pub.updated)
	}

	// A real repository's SELECT would no longer return this row once
	// marked; model that here by clearing the candidate list, mirroring
	// what the second boot's selection would see.
	repo.candidates = nil
	svc.RepairOrphanedWorkspaceMarkers(context.Background())

	if len(pub.updated) != 1 {
		t.Errorf("expected no additional publish on the idempotent second pass, got %v", pub.updated)
	}
}

func timePtr() *time.Time {
	now := time.Now().UTC()
	return &now
}

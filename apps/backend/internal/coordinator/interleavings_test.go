package coordinator

import (
	"context"
	"errors"
	"sync"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// The tests in this file are the rows of the Interleavings table in
// docs/specs/coordinator/system-design/permissions.md, one per row.

func createWorkflowsTable(t *testing.T, store *Store) {
	t.Helper()
	if _, err := store.db.Exec(`CREATE TABLE IF NOT EXISTS workflows (id TEXT PRIMARY KEY, workspace_id TEXT NOT NULL, name TEXT)`); err != nil {
		t.Fatal(err)
	}
}

func addWorkflow(t *testing.T, store *Store, id, workspaceID string) {
	t.Helper()
	if _, err := store.db.Exec(store.db.Rebind(`INSERT INTO workflows (id, workspace_id, name) VALUES (?, ?, ?)`), id, workspaceID, id); err != nil {
		t.Fatal(err)
	}
}

func dropWorkflow(t *testing.T, store *Store, id string) {
	t.Helper()
	if _, err := store.db.Exec(store.db.Rebind(`DELETE FROM workflows WHERE id = ?`), id); err != nil {
		t.Fatal(err)
	}
}

func policyBody(overrides map[string]string) string {
	actions := map[string]string{"create_task": "requires_approval", "start_agent": "denied", "message": "denied", "move": "denied", "resume": "denied", "stop": "denied"}
	for k, v := range overrides {
		actions[k] = v
	}
	out := `{"policy":{"actions":{`
	first := true
	for _, a := range AllActions {
		if !first {
			out += ","
		}
		first = false
		out += `"` + string(a) + `":"` + actions[string(a)] + `"`
	}
	return out + `}}}`
}

func mustSave(t *testing.T, svc *Service, workspaceID, coordinatorID, body string) *CoordinatorPhase2 {
	t.Helper()
	got, err := svc.SaveSettings(context.Background(), workspaceID, coordinatorID, []byte(body))
	if err != nil {
		t.Fatalf("SaveSettings(%s): %v", body, err)
	}
	return got
}

func settingsCode(t *testing.T, err error) string {
	t.Helper()
	var se *SettingsError
	if !errors.As(err, &se) {
		t.Fatalf("err = %v, want *SettingsError", err)
	}
	return se.Code
}

func phase2ApproveFixture(t *testing.T) (*Store, *Coordinator, *fakeDecisionTaskService, *Service) {
	t.Helper()
	store, c, tasks, svc := approveFixture(t)
	svc.phase2 = true
	svc.SetConversationDeps(newFakeConversationTasks(), newFakeSessionEnsurer())
	return store, c, tasks, svc
}

func assertPolicyDenied(t *testing.T, err error, want Action) {
	t.Helper()
	var denied *PolicyDeniedError
	if !errors.As(err, &denied) || denied.Action != want {
		t.Fatalf("err = %v, want PolicyDeniedError(%s)", err, want)
	}
}

// Row 2: the guard passed before S, S tightens, the insert follows. The
// proposal exists, approve is 409 policy_denied, Reject succeeds.
func TestInterleaving2_TightenAfterGuardLeavesPendingProposalThatCannotApprove(t *testing.T) {
	store, c, tasks, svc := phase2ApproveFixture(t)
	ctx := context.Background()
	mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"create_task": "denied"}))
	p := insertProposal(t, store, c, sampleSpec())

	if got := statusOf(t, store, p.ID); got != string(ProposalStatusPending) {
		t.Fatalf("status = %q, want pending", got)
	}
	_, err := svc.ApproveProposal(ctx, "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	assertPolicyDenied(t, err, ActionCreateTask)
	if len(tasks.createCalls) != 0 {
		t.Fatalf("create calls = %d, want 0", len(tasks.createCalls))
	}
	if _, err := svc.RejectProposal(ctx, "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("reject: %v", err)
	}
}

// Row 5: the re-check read allowed, S tightens before the claim; the approval
// proceeds and executes.
func TestInterleaving5_TightenBetweenRecheckAndClaimStillExecutes(t *testing.T) {
	store, c, tasks, svc := phase2ApproveFixture(t)
	tasks.createResult = createdResult("task-new")
	tasks.settled = true
	p := insertProposal(t, store, c, sampleSpec())
	svc.afterApproveRecheck = func() {
		mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"create_task": "denied"}))
	}

	got, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if got.Status != ProposalStatusApproved || len(tasks.createCalls) != 1 {
		t.Fatalf("status=%q creates=%d, want approved and 1", got.Status, len(tasks.createCalls))
	}
	// The next approve sees the tightened setting.
	p2 := insertProposal(t, store, c, sampleSpec())
	svc.afterApproveRecheck = nil
	_, err = svc.ApproveProposal(context.Background(), "ws-1", c.ID, p2.ID, ApproveProposalRequest{})
	assertPolicyDenied(t, err, ActionCreateTask)
}

// Row 6: the re-check reads after S committed; 409, no claim, Reject works.
func TestInterleaving6_RecheckAfterTightenIsPolicyDeniedWithNoClaim(t *testing.T) {
	store, c, tasks, svc := phase2ApproveFixture(t)
	p := insertProposal(t, store, c, sampleSpec())
	mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"create_task": "denied"}))

	_, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{})
	assertPolicyDenied(t, err, ActionCreateTask)
	if got := statusOf(t, store, p.ID); got != string(ProposalStatusPending) {
		t.Fatalf("status = %q, want pending (no claim)", got)
	}
	if len(tasks.createCalls) != 0 {
		t.Fatalf("create calls = %d, want 0", len(tasks.createCalls))
	}
	if _, err := svc.RejectProposal(context.Background(), "ws-1", c.ID, p.ID, RejectProposalRequest{}); err != nil {
		t.Fatalf("reject: %v", err)
	}
}

// Row 7: the route reads config_revision, S commits (reset bumps it), the
// conditional update matches nothing; the new task is deleted and the route
// is a 409.
func TestInterleaving7_SaveBetweenRevisionReadAndCommitConflicts(t *testing.T) {
	deps := newConversationTestDeps(t)
	deps.svc.phase2 = true
	c := deps.coordinator
	deps.tasks.onCreate = func(*taskmodels.Task) {
		mustSave(t, deps.svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"message": "requires_approval"}))
	}

	_, err := deps.svc.OpenConversation(context.Background(), c.WorkspaceID, c.ID)
	if !errors.Is(err, ErrConversationConflict) {
		t.Fatalf("err = %v, want ErrConversationConflict", err)
	}
	if len(deps.tasks.createdIDs) != 1 || len(deps.tasks.deletedIDs) != 1 || deps.tasks.deletedIDs[0] != deps.tasks.createdIDs[0] {
		t.Fatalf("created=%v deleted=%v, want the new task deleted", deps.tasks.createdIDs, deps.tasks.deletedIDs)
	}
}

// Row 8: the route's update commits, then S commits; the route is 200 and S
// clears and archives the just-opened conversation.
func TestInterleaving8_SaveAfterOpenArchivesTheOpenedConversation(t *testing.T) {
	deps := newConversationTestDeps(t)
	deps.svc.phase2 = true
	c := deps.coordinator
	ctx := context.Background()

	res, err := deps.svc.OpenConversation(ctx, c.WorkspaceID, c.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	before, _ := deps.svc.store.GetCoordinatorByID(ctx, c.ID)
	mustSave(t, deps.svc, c.WorkspaceID, c.ID, policyBody(map[string]string{"message": "requires_approval"}))
	after, err := deps.svc.store.GetCoordinatorByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.ConversationTaskID != nil || after.ConfigRevision != before.ConfigRevision+1 {
		t.Fatalf("after = %+v (before revision %d), want cleared task and revision+1", after, before.ConfigRevision)
	}
	if len(deps.tasks.archivedIDs) != 1 || deps.tasks.archivedIDs[0] != res.TaskID {
		t.Fatalf("archived = %v, want [%s]", deps.tasks.archivedIDs, res.TaskID)
	}
}

// Row 9: two saves are serialised; each change raises the revision once; a
// save equal to the stored value is a no-op.
func TestInterleaving9_ConcurrentSavesBumpRevisionOnceEach(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	var wg sync.WaitGroup
	for _, o := range []map[string]string{{"message": "requires_approval"}, {"move": "requires_approval"}} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(o))
		}()
	}
	wg.Wait()
	got, err := store.GetCoordinatorByID(context.Background(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.PolicyRevision != 2 {
		t.Fatalf("policy_revision = %d, want 2", got.PolicyRevision)
	}
	last := mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(nil)) // sends all six: last commit wins
	if last.PolicyRevision != 3 {
		t.Fatalf("revision = %d, want 3", last.PolicyRevision)
	}
	again := mustSave(t, svc, c.WorkspaceID, c.ID, policyBody(nil))
	if again.PolicyRevision != 3 {
		t.Fatalf("equal save revision = %d, want 3 (no-op)", again.PolicyRevision)
	}
}

// Row 10a: W is deleted before S reads workflows.
func TestInterleaving10a_DeletedWorkflowBeforeSave(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	addWorkflow(t, store, "wf-w", "ws-1")
	mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-w"]}}`)
	dropWorkflow(t, store, "wf-w")

	// Not stored and gone: foreign.
	_, err := svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(`{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-x"]}}`))
	if got := settingsCode(t, err); got != "watches_foreign_workflow" {
		t.Fatalf("code = %q", got)
	}
	// Stored and gone: dropped. Dropping it leaves the effective set, so the
	// request is a no-op 200 with no revision bump.
	rev := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`).PolicyRevision
	got := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-w"]}}`)
	if got.PolicyRevision != rev || len(got.Watches.WorkflowIDs) != 1 || got.Watches.WorkflowIDs[0] != "wf-a" {
		t.Fatalf("drop-only save = %+v, want no-op over [wf-a] at revision %d", got, rev)
	}
	// Dropping it so nothing is left is watches_empty.
	_, err = svc.SaveSettings(context.Background(), c.WorkspaceID, c.ID, []byte(`{"watches":{"scope":"selected","workflow_ids":["wf-w"]}}`))
	if got := settingsCode(t, err); got != "watches_empty" {
		t.Fatalf("code = %q, want watches_empty", got)
	}
}

// Row 10b: W is deleted after S committed. Its row stays stored, the effective
// set omits it, an equal-to-effective save is a no-op, and the subscriber
// tidies the row without touching revision or scope.
func TestInterleaving10b_DeletedWorkflowAfterSave(t *testing.T) {
	store, c, _, svc := phase2ApproveFixture(t)
	createWorkflowsTable(t, store)
	addWorkflow(t, store, "wf-a", "ws-1")
	addWorkflow(t, store, "wf-w", "ws-1")
	ctx := context.Background()
	saved := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a","wf-w"]}}`)
	dropWorkflow(t, store, "wf-w")

	view, err := svc.GetSettings(ctx, c.WorkspaceID, c.ID)
	if err != nil || len(view.Watches.WorkflowIDs) != 1 || view.Watches.WorkflowIDs[0] != "wf-a" {
		t.Fatalf("GET = %+v, %v, want effective [wf-a]", view, err)
	}
	eq := mustSave(t, svc, c.WorkspaceID, c.ID, `{"watches":{"scope":"selected","workflow_ids":["wf-a"]}}`)
	if eq.PolicyRevision != saved.PolicyRevision {
		t.Fatalf("equal-to-effective save revision = %d, want %d", eq.PolicyRevision, saved.PolicyRevision)
	}
	var stored int
	_ = store.db.Get(&stored, `SELECT COUNT(*) FROM coordinator_watches WHERE workflow_id = 'wf-w'`)
	if stored != 1 {
		t.Fatalf("stale rows = %d, want 1 until the subscriber runs", stored)
	}
	if err := svc.WorkflowDeleted(ctx, "wf-w"); err != nil {
		t.Fatalf("WorkflowDeleted: %v", err)
	}
	_ = store.db.Get(&stored, `SELECT COUNT(*) FROM coordinator_watches WHERE workflow_id = 'wf-w'`)
	after, _ := store.GetCoordinatorByID(ctx, c.ID)
	if stored != 0 || after.PolicyRevision != saved.PolicyRevision || after.WatchScope != "selected" {
		t.Fatalf("after subscriber: rows=%d revision=%d scope=%q", stored, after.PolicyRevision, after.WatchScope)
	}
}

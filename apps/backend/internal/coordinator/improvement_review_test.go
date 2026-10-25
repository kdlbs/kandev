package coordinator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// wireNoTurnProbes replaces the fixture's kind dependencies with recorders so a
// test can assert no prompt was delivered and no session was resumed.
func (f *improvementFixture) wireNoTurnProbes() (*fakeMessenger, *fakeResumer) {
	m, r := &fakeMessenger{}, &fakeResumer{}
	f.svc.SetKindDeps(KindDeps{Tasks: f.tasks, Messenger: m, Resumer: r})
	return m, r
}

func (f *improvementFixture) bindConversation(t *testing.T, taskID string) {
	t.Helper()
	mustExec(t, f.store, `UPDATE coordinators SET conversation_task_id = ? WHERE id = ?`, taskID, f.c.ID)
}

func (f *improvementFixture) conversationTaskID(t *testing.T) *string {
	t.Helper()
	c, err := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return c.ConversationTaskID
}

func noTurnStartImprovementApprove(t *testing.T) {
	f := newImprovementFixture(t)
	messenger, resumer := f.wireNoTurnProbes()
	f.bindConversation(t, "conv-old")
	f.approved(t)
	if len(messenger.prompts) != 0 || resumer.calls != 0 {
		t.Fatalf("approve started a turn: prompts=%v resumes=%d", messenger.prompts, resumer.calls)
	}
	if len(f.cleared) != 0 {
		t.Fatalf("approve replaced the conversation: %v", f.cleared)
	}
}

func noTurnStartImprovementApply(t *testing.T) {
	f := newImprovementFixture(t)
	messenger, resumer := f.wireNoTurnProbes()
	f.bindConversation(t, "conv-old")
	_, change := f.approved(t)
	if _, err := f.apply(change.ID); err != nil {
		t.Fatal(err)
	}
	if len(messenger.prompts) != 0 || resumer.calls != 0 {
		t.Fatalf("apply started a turn: prompts=%v resumes=%d", messenger.prompts, resumer.calls)
	}
}

func TestImprovementApplyReplacesConversationWithoutStartingATurn(t *testing.T) {
	f := newImprovementFixture(t)
	messenger, resumer := f.wireNoTurnProbes()
	f.bindConversation(t, "conv-old")
	_, change := f.approved(t)
	if _, err := f.apply(change.ID); err != nil {
		t.Fatal(err)
	}
	if len(f.cleared) != 1 || f.cleared[0] != "conv-old" {
		t.Fatalf("cleared = %v, want [conv-old]", f.cleared)
	}
	if got := f.conversationTaskID(t); got != nil {
		t.Fatalf("conversation_task_id = %q, want cleared", *got)
	}
	if len(messenger.prompts) != 0 || resumer.calls != 0 {
		t.Fatalf("apply started a turn: prompts=%v resumes=%d", messenger.prompts, resumer.calls)
	}
}

func TestImprovementApplyRefusedWhenAProfileNoLongerResolves(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	f.svc.validator = newValidatorForTest(
		map[string]*settingsmodels.AgentProfile{},
		map[string]*taskmodels.ExecutorProfile{"e": {ID: "e"}})
	f.bindConversation(t, "conv-old")
	if _, err := f.apply(change.ID); err == nil {
		t.Fatal("apply succeeded although the agent profile no longer resolves")
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("context changed: %q", f.context(t))
	}
	if len(f.cleared) != 0 {
		t.Fatalf("conversation replaced by a refused apply: %v", f.cleared)
	}
	changes, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
	if err != nil || len(changes) != 1 || changes[0].Status != ChangeStatusPending {
		t.Fatalf("change not left pending: %+v err=%v", changes, err)
	}
}

func TestImprovementChangeInDifferentWorkspaceIsNotFound(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	ctx := context.Background()
	if _, err := f.svc.ApplyPendingChange(ctx, "ws-other", f.c.ID, change.ID); err == nil {
		t.Fatal("apply through another workspace succeeded")
	}
	if _, err := f.svc.DiscardPendingChange(ctx, "ws-other", f.c.ID, change.ID); err == nil {
		t.Fatal("discard through another workspace succeeded")
	}
	if _, err := f.svc.ListPendingChanges(ctx, "ws-other", f.c.ID); err == nil {
		t.Fatal("list through another workspace succeeded")
	}
	if f.context(t) != "old instructions" {
		t.Fatalf("context changed")
	}
}

func TestImprovementListOmitsChangesWhoseProposalIsNotApproved(t *testing.T) {
	f := newImprovementFixture(t)
	_, change := f.approved(t)
	for _, status := range []string{"failed", "rejected", "approving"} {
		mustExec(t, f.store, `UPDATE coordinator_proposals SET status = ? WHERE id = ?`, status, change.ProposalID)
		changes, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
		if err != nil || len(changes) != 0 {
			t.Fatalf("proposal %s: changes = %+v err=%v, want none", status, changes, err)
		}
	}
	mustExec(t, f.store, `UPDATE coordinator_proposals SET status = 'approved' WHERE id = ?`, change.ProposalID)
	changes, err := f.svc.ListPendingChanges(context.Background(), "ws-1", f.c.ID)
	if err != nil || len(changes) != 1 {
		t.Fatalf("approved proposal: changes = %+v err=%v", changes, err)
	}
}

func TestImprovementPhase3OffDoesNotCountTowardTheOpenLimit(t *testing.T) {
	f := newImprovementFixture(t)
	for i := 0; i < maxOpenProposals; i++ {
		f.mustPropose(t)
	}
	f.svc.phase3 = false
	target := idleSession()
	target.WorkflowID, target.WorkflowStepID = "wf-1", "step-1"
	f.tasks.target = target
	_, _, err := f.kindsFixture.propose(ProposalKindMessage, `{"task_id":"task-0","text":"hi"}`)
	if errors.Is(err, ErrCoordinatorProposalCapReached) {
		t.Fatalf("hidden improvements counted toward the limit: %v", err)
	}
	if err != nil {
		t.Fatalf("propose message: %v", err)
	}
}

func TestImprovementReappearsWithStoredStatusWhenPhase3ReturnsOn(t *testing.T) {
	f := newImprovementFixture(t)
	p, change := f.approved(t)
	f.svc.phase3 = false
	ctx := context.Background()
	if _, err := f.svc.GetProposal(ctx, "ws-1", f.c.ID, p.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get while off err = %v", err)
	}
	f.svc.phase3 = true
	got, err := f.svc.GetProposal(ctx, "ws-1", f.c.ID, p.ID)
	if err != nil || got.Status != ProposalStatusApproved {
		t.Fatalf("after re-enable: %+v err=%v", got, err)
	}
	changes, err := f.svc.ListPendingChanges(ctx, "ws-1", f.c.ID)
	if err != nil || len(changes) != 1 || changes[0].ID != change.ID || changes[0].Status != ChangeStatusPending {
		t.Fatalf("change after re-enable: %+v err=%v", changes, err)
	}
}

func TestImprovementApproveOnHeldClaimWhilePhase3OffStillSettles(t *testing.T) {
	ctx := context.Background()
	t.Run("fresh claim answers a conflict and stays approving", func(t *testing.T) {
		f := newImprovementFixture(t)
		p := f.mustPropose(t)
		claimDirectly(t, f.store, p, "tok", ProposalSpec{}, time.Now())
		f.svc.phase3 = false
		_, err := f.approve(p, nil)
		_ = assertConflict(t, err, ProposalStatusApproving)
		if statusOf(t, f.store, p.ID) != string(ProposalStatusApproving) || f.changeRows(t) != 0 {
			t.Fatal("fresh claim was disturbed")
		}
	})
	t.Run("stale claim settles failed without a change", func(t *testing.T) {
		f := newImprovementFixture(t)
		p := f.mustPropose(t)
		claimDirectly(t, f.store, p, "tok", ProposalSpec{}, time.Now().Add(-10*time.Minute))
		f.svc.phase3 = false
		got, err := f.svc.ApproveProposal(ctx, "ws-1", f.c.ID, p.ID, nil)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status != ProposalStatusFailed || f.changeRows(t) != 0 {
			t.Fatalf("status = %q changes = %d, want failed and none", got.Status, f.changeRows(t))
		}
	})
}

func TestStampToolPolicyBindsTheImprovementToolByPhase3(t *testing.T) {
	cases := []struct {
		name     string
		phase3   bool
		wantTool bool
	}{
		{name: "phase 3 on", phase3: true, wantTool: true},
		{name: "phase 3 off", phase3: false, wantTool: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newImprovementFixture(t)
			f.svc.phase2, f.svc.phase3 = true, tc.phase3
			meta := map[string]interface{}{}
			if err := f.svc.stampToolPolicy(meta, f.c, "conv-1"); err != nil {
				t.Fatal(err)
			}
			raw := ""
			for _, v := range meta {
				if s, ok := v.(string); ok {
					raw = s
				}
			}
			if got := strings.Contains(raw, improvementTool); got != tc.wantTool {
				t.Fatalf("binding contains %s = %v, want %v: %s", improvementTool, got, tc.wantTool, raw)
			}
		})
	}
}

func TestToolNamesImprovementRequiresPhase3AndPhase2(t *testing.T) {
	has := func(names []string) bool {
		for _, n := range names {
			if n == improvementTool {
				return true
			}
		}
		return false
	}
	p := PhaseOnePolicy()
	if !has(ToolNames(p, true, true)) {
		t.Fatal("phase 3 on must bind the improvement tool")
	}
	if has(ToolNames(p, true, false)) {
		t.Fatal("phase 3 off must not bind the improvement tool")
	}
	if has(ToolNames(p, false, true)) {
		t.Fatal("phase 2 off must not bind the improvement tool")
	}
}

package coordinator

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSettleDecision_LosingClaimerWritesNoRow(t *testing.T) {
	ctx := context.Background()
	t.Run("complete with a stale token", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p, _ := claimedPhase2(t, store, c)
		m, err := svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, "stale", "task-x")
		if err != nil || m {
			t.Fatalf("matched=%v err=%v, want false/nil", m, err)
		}
		if rows := listActivity(t, store, c.ID); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})
	t.Run("fail after the proposal settled", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p, tok := claimedPhase2(t, store, c)
		if m, err := svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "task-x"); err != nil || !m {
			t.Fatalf("matched=%v err=%v", m, err)
		}
		m, err := svc.failProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, tok, "late")
		if err != nil || m {
			t.Fatalf("matched=%v err=%v, want false/nil", m, err)
		}
		if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].Outcome != ActivityApproved {
			t.Fatalf("rows = %+v, want only the approved row", rows)
		}
	})
	t.Run("fail with a stale token", func(t *testing.T) {
		store, c, svc := phase2Fixture(t, true)
		p, _ := claimedPhase2(t, store, c)
		m, err := svc.failProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, "stale", "boom")
		if err != nil || m {
			t.Fatalf("matched=%v err=%v, want false/nil", m, err)
		}
		if rows := listActivity(t, store, c.ID); len(rows) != 0 {
			t.Fatalf("rows = %+v, want none", rows)
		}
	})
}

func TestApprove_RetryAfterFailureWritesNewRow(t *testing.T) {
	store, c, svc := phase2Fixture(t, true)
	tasks := svc.decisionTasks.(*fakeDecisionTaskService)
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createErr = errors.New("create boom")
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	tasks.createErr = nil
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 2 || rows[0].Outcome != ActivityFailed || rows[0].Detail != "create boom" || rows[1].Outcome != ActivityApproved {
		t.Fatalf("rows = %+v, want [failed(create boom), approved]", rows)
	}
}

func TestApprove_CreateErrorWritesFailedRow(t *testing.T) {
	store, c, tasks, svc := approveFixture(t)
	svc.phase2 = true
	p := insertProposal(t, store, c, sampleSpec())
	tasks.createErr = errors.New("create boom")
	if _, err := svc.ApproveProposal(context.Background(), "ws-1", c.ID, p.ID, ApproveProposalRequest{}); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, store, c.ID)
	if len(rows) != 1 || rows[0].Outcome != ActivityFailed || rows[0].Detail != "create boom" {
		t.Fatalf("rows = %+v, want one failed row with the create error", rows)
	}
}

func TestSettleDecision_RowContentMatchesSpec(t *testing.T) {
	ctx := context.Background()
	store, c, svc := phase2Fixture(t, true)
	p := insertKind(t, store, c, ProposalKindCreateTask)
	edited := sampleSpec()
	edited.Title = "A different title"
	claimDirectly(t, store, p, "tok", edited, time.Now())
	if m, err := svc.completeProposalStore(ctx, c.WorkspaceID, c.ID, p.ID, "tok", "task-x"); err != nil || !m {
		t.Fatalf("matched=%v err=%v", m, err)
	}
	if rows := listActivity(t, store, c.ID); len(rows) != 1 || rows[0].Detail != "A different title" {
		t.Fatalf("rows = %+v, want detail = the edited title", rows)
	}

	store2, c2, svc2 := phase2Fixture(t, true)
	p2 := insertKind(t, store2, c2, ProposalKindCreateTask)
	if m, err := svc2.rejectProposalStore(ctx, p2, "no", "u1"); err != nil || !m {
		t.Fatalf("matched=%v err=%v", m, err)
	}
	if rows := listActivity(t, store2, c2.ID); len(rows) != 1 || rows[0].ActorUserID == nil || *rows[0].ActorUserID != "u1" {
		t.Fatalf("rows = %+v, want actor u1", rows)
	}
}

func TestProposeTask_ProposedActivityDetailIsTitle(t *testing.T) {
	f := newProposalTestFixture(t)
	f.svc.phase2 = true
	req := f.baseRequest()
	if _, _, err := f.svc.ProposeTask(context.Background(), f.coordinator.ID, req); err != nil {
		t.Fatal(err)
	}
	rows := listActivity(t, f.svc.store, f.coordinator.ID)
	if len(rows) != 1 || rows[0].Detail != req.Title {
		t.Fatalf("rows = %+v, want detail = the proposal title", rows)
	}
}

// Only the proposal decision paths write activity rows; a manager's direct
// Resume or Send it back never does.
func TestActivityWriters_OnlyProposalPaths(t *testing.T) {
	for _, name := range []string{"stalls.go", "recovery.go", "reject.go"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		for _, call := range []string{".Record(", ".RecordRefusal(", "InsertActivity("} {
			if strings.Contains(string(src), call) {
				t.Errorf("%s must not write activity rows directly (%s)", name, call)
			}
		}
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	allowed := map[string]bool{"decision_phase2.go": true, "proposals.go": true, "activity.go": true, "undo.go": true, "activity_store.go": true}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") || allowed[n] {
			continue
		}
		src, _ := os.ReadFile(n)
		if strings.Contains(string(src), ".Record(") || strings.Contains(string(src), "InsertActivity(") {
			t.Errorf("%s writes activity rows outside the allowed writers", n)
		}
	}
}

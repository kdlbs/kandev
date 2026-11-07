package coordinator

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

func (f *admitFixture) admitAction() Admission {
	return f.env.svc.AdmitAction(context.Background(), f.env.c.ID)
}

func TestAdmitAction_PassesWithoutConversationOrCooldown(t *testing.T) {
	f := newAdmitFixture(t)
	f.setState(taskmodels.TaskSessionStateRunning) // a busy conversation holds Admit, not AdmitAction
	mustExec(t, f.env.store, `UPDATE coordinators SET conversation_task_id = NULL WHERE id = ?`, f.env.c.ID)
	if got := f.admitAction(); !got.OK {
		t.Fatalf("admission = %+v, want ok", got)
	}
	wantHeld(t, f.admit(AdmitReadOnly), "no_conversation", "")
}

func TestAdmitAction_HoldsInOrder(t *testing.T) {
	f := newAdmitFixture(t)
	wantHeld(t, f.env.svc.AdmitAction(context.Background(), "nope"), "autonomy_off", "coordinator_not_found")
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, false, f.env.c.ID)
	wantHeld(t, f.admitAction(), "autonomy_off", "")
	mustExec(t, f.env.store, `UPDATE coordinators SET autonomy_enabled = ? WHERE id = ?`, true, f.env.c.ID)
	f.fc.mode = "disabled"
	if got := f.admitAction(); got.OK || got.Reason != "containment" || got.Detail == "" {
		t.Fatalf("admission = %+v", got)
	}
	f.fc.mode = auth.ModeEnabled
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{}, errors.New("boom") }
	wantHeld(t, f.admitAction(), "spend_unmeasured", "")
	f.ledger.sumFn = func(spendCall) (taskmodels.UsageSum, error) { return taskmodels.UsageSum{CostSubcents: 1000}, nil }
	wantHeld(t, f.admitAction(), "ceiling_reached", "")
}

func TestAdmitAction_CountsAndWritesNothing(t *testing.T) {
	f := newAdmitFixture(t)
	f.fc.mode = "disabled"
	before := containmentFailedCount("auth_enabled")
	f.admitAction()
	if got := containmentFailedCount("auth_enabled"); got != before {
		t.Fatalf("AdmitAction moved the containment counter %d -> %d", before, got)
	}
}

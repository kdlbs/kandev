package backendapp

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
)

type fakePendingApproverReader struct {
	pending []dashboard.PendingApproverDTO
	err     error
}

func (f fakePendingApproverReader) PendingApproversForCompletion(
	context.Context, string, string,
) ([]dashboard.PendingApproverDTO, error) {
	return f.pending, f.err
}

func TestOfficeApprovalGuardAdapter_MapsPendingApprovers(t *testing.T) {
	adapter := officeApprovalGuardAdapter{reader: fakePendingApproverReader{
		pending: []dashboard.PendingApproverDTO{{AgentProfileID: "agent-ceo", Name: "CEO"}},
	}}

	got, err := adapter.PendingApproversForCompletion(context.Background(), "task-1", "step-done")
	if err != nil {
		t.Fatalf("PendingApproversForCompletion: %v", err)
	}
	if len(got) != 1 || got[0].AgentProfileID != "agent-ceo" || got[0].Name != "CEO" {
		t.Fatalf("got = %+v, want CEO", got)
	}
}

func TestOfficeApprovalGuardAdapter_PropagatesReadError(t *testing.T) {
	readErr := errors.New("read failed")
	adapter := officeApprovalGuardAdapter{reader: fakePendingApproverReader{err: readErr}}

	if _, err := adapter.PendingApproversForCompletion(context.Background(), "task-1", "step-done"); !errors.Is(err, readErr) {
		t.Fatalf("err = %v, want read error", err)
	}
}

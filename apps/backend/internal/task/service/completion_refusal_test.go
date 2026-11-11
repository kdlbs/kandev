package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/completion"
)

type fakeApproverNameResolver struct{ names map[string]string }

func (r fakeApproverNameResolver) ResolvePendingApproverNames(_ context.Context, ids []string) []PendingApprover {
	pending := make([]PendingApprover, 0, len(ids))
	for _, id := range ids {
		name := r.names[id]
		if name == "" {
			name = id
		}
		pending = append(pending, PendingApprover{AgentProfileID: id, Name: name})
	}
	return pending
}

func TestTranslateCompletionRefusalResolvesSubjects(t *testing.T) {
	svc, _, _ := createTestService(t)
	svc.SetApproverNameResolver(fakeApproverNameResolver{names: map[string]string{"agent-ceo": "CEO"}})
	err := svc.translateCompletionError(context.Background(), &completion.RefusedError{
		Code: completion.CodeApprovalsPending, Subjects: []string{"agent-ceo", "agent-cto"},
	})
	pending, ok := err.(*ApprovalsPendingMoveError)
	if !ok {
		t.Fatalf("error = %T (%v), want ApprovalsPendingMoveError", err, err)
	}
	if len(pending.Pending) != 2 || pending.Pending[0].Name != "CEO" || pending.Pending[1].Name != "agent-cto" {
		t.Fatalf("pending = %+v, want resolved CEO and stable fallback ID", pending.Pending)
	}
}

package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
)

type fakeApproverNameReader struct{ names map[string]string }

func (f fakeApproverNameReader) ResolvePendingApproverNames(_ context.Context, ids []string) []dashboard.PendingApproverDTO {
	resolved := make([]dashboard.PendingApproverDTO, 0, len(ids))
	for _, id := range ids {
		resolved = append(resolved, dashboard.PendingApproverDTO{AgentProfileID: id, Name: f.names[id]})
	}
	return resolved
}

func TestOfficeApproverNameResolverAdapterMapsNames(t *testing.T) {
	adapter := officeApproverNameResolverAdapter{reader: fakeApproverNameReader{names: map[string]string{"agent-ceo": "CEO"}}}
	got := adapter.ResolvePendingApproverNames(context.Background(), []string{"agent-ceo"})
	if len(got) != 1 || got[0].AgentProfileID != "agent-ceo" || got[0].Name != "CEO" {
		t.Fatalf("got = %+v, want CEO", got)
	}
}

func TestOfficeApproverNameResolverAdapterFallsBackToStableIDs(t *testing.T) {
	adapter := officeApproverNameResolverAdapter{}
	got := adapter.ResolvePendingApproverNames(context.Background(), []string{"agent-cto"})
	if len(got) != 1 || got[0].AgentProfileID != "agent-cto" || got[0].Name != "agent-cto" {
		t.Fatalf("got = %+v, want stable ID fallback", got)
	}
}

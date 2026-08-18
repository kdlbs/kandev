package clarification

import (
	"context"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// TestResolveBundle_R6_ClearedInMemoryStoreDoesNotWinASecondClaim proves R6:
// the durable resolution row, not the in-memory Store, is the source of
// truth for who won. A cleared/restarted Store (simulated here by handing
// the second ResolveBundle call a brand-new, empty Store while keeping the
// same durable resolutions/messages/authorizer/event-bus) must still see the
// pending_id as already claimed and lose.
func TestResolveBundle_R6_ClearedInMemoryStoreDoesNotWinASecondClaim(t *testing.T) {
	msgs := map[string][]*taskmodels.Message{"p1": {questionMessage("m1", "s1", "p1", "q1", 0)}}
	f := newResolverFixture(t, msgs)

	first, claimed, err := f.resolver.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: "winner"}},
	})
	if err != nil {
		t.Fatalf("first ResolveBundle: %v", err)
	}
	if !claimed {
		t.Fatalf("expected the first call to win the claim")
	}

	// Simulate a backend restart: a fresh, empty in-memory Store, wired into
	// a new Resolver that shares the same durable resolutions store, message
	// repo, message updater, authorizer and event bus as before the restart.
	restarted := NewResolver(NewStore(0), f.resolutions, f.repo, f.messages, f.authorizer, f.eventBus, logger.Default())

	second, claimed, err := restarted.ResolveBundle(context.Background(), "p1", Outcome{
		Answers: []Answer{{QuestionID: "q1", CustomText: "impostor"}},
	})
	if err != nil {
		t.Fatalf("second ResolveBundle: %v", err)
	}
	if claimed {
		t.Fatalf("expected the second call, after a simulated restart, to lose the claim")
	}
	if second.Status != first.Status || second.Resume != first.Resume {
		t.Fatalf("expected the loser to carry the winner's recorded status/resume verbatim, got %+v want %+v", second, first)
	}
	// Strip the monotonic clock reading before comparing: first.Response
	// never left process memory, but second.Response went through a
	// serialize/deserialize round trip (mirroring the real durable-row
	// path), which drops it — reflect.DeepEqual would otherwise treat two
	// wall-clock-identical times as different.
	wantResponse, gotResponse := *first.Response, *second.Response
	wantResponse.RespondedAt = wantResponse.RespondedAt.Round(0)
	gotResponse.RespondedAt = gotResponse.RespondedAt.Round(0)
	if !reflect.DeepEqual(gotResponse, wantResponse) {
		t.Fatalf("expected the loser to carry the winner's recorded response verbatim, got %+v want %+v", gotResponse, wantResponse)
	}
}

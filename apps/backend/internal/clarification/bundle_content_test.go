package clarification

import (
	"context"
	"errors"
	"testing"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

type fakeBundleMessages struct {
	byPending map[string][]*taskmodels.Message
	err       error
	asked     []string
}

func (f *fakeBundleMessages) FindMessagesByPendingIDs(_ context.Context, ids []string) (map[string][]*taskmodels.Message, error) {
	f.asked = ids
	return f.byPending, f.err
}

func bundleMsg(id, qid string, idx int, extra map[string]any) *taskmodels.Message {
	meta := map[string]any{"question_id": qid, "question_index": idx}
	for k, v := range extra {
		meta[k] = v
	}
	return &taskmodels.Message{ID: id, Metadata: meta}
}

func TestHydrateBundle_OrdersAndRewritesIndexAndTakesFirstContext(t *testing.T) {
	store := &fakeBundleMessages{byPending: map[string][]*taskmodels.Message{
		"p1": {
			bundleMsg("m2", "b", 5, nil),
			bundleMsg("m1", "a", 2, map[string]any{"context": "  shared  "}),
		},
	}}
	got, err := HydrateBundle(context.Background(), store, taskmodels.ClarificationBundleSummary{PendingID: "p1"})
	if err != nil || got == nil {
		t.Fatalf("HydrateBundle = %v, %v", got, err)
	}
	if len(store.asked) != 1 || store.asked[0] != "p1" {
		t.Fatalf("asked %v, want [p1]", store.asked)
	}
	if len(got.Messages) != 2 || got.Messages[0].ID != "m1" || got.Messages[1].ID != "m2" {
		t.Fatalf("messages out of order: %+v", got.Messages)
	}
	if got.Messages[0].Metadata["question_index"] != 0 || got.Messages[1].Metadata["question_index"] != 1 {
		t.Fatalf("question_index not rewritten to rank: %+v", got.Messages)
	}
	if got.Context != "  shared  " {
		t.Fatalf("context = %q, want the first message's context verbatim", got.Context)
	}
}

func TestHydrateBundle_NoMessagesIsNil(t *testing.T) {
	store := &fakeBundleMessages{byPending: map[string][]*taskmodels.Message{}}
	got, err := HydrateBundle(context.Background(), store, taskmodels.ClarificationBundleSummary{PendingID: "p1"})
	if err != nil || got != nil {
		t.Fatalf("HydrateBundle = %v, %v; want nil, nil", got, err)
	}
}

func TestHydrateBundle_StoreErrorPropagates(t *testing.T) {
	boom := errors.New("boom")
	store := &fakeBundleMessages{err: boom}
	got, err := HydrateBundle(context.Background(), store, taskmodels.ClarificationBundleSummary{PendingID: "p1"})
	if !errors.Is(err, boom) || got != nil {
		t.Fatalf("HydrateBundle = %v, %v; want nil, boom", got, err)
	}
}

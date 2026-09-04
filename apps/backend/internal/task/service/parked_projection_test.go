package service

import (
	"context"
	"testing"
)

// fakeTaskParkedProvider is a fixed-value TaskParkedProvider test double,
// mirroring fakeActivityProvider's role for ForegroundActivityProvider.
type fakeTaskParkedProvider struct {
	parked   bool
	revision uint64
	epoch    uint64
}

func (f fakeTaskParkedProvider) TaskParkedSnapshot(string) (bool, uint64) {
	return f.parked, f.revision
}

func (f fakeTaskParkedProvider) ParkedEpoch() uint64 {
	return f.epoch
}

// AC-22/AC-62: a wired provider stamps parked_on_background_work,
// parked_revision, and parked_epoch onto every task.updated payload.
func TestPublishTaskUpdated_IncludesParkedFieldsWhenProviderWired(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	svc.SetTaskParkedProvider(fakeTaskParkedProvider{parked: true, revision: 3, epoch: 42})

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if got, ok := data["parked_on_background_work"].(bool); !ok || !got {
		t.Fatalf("parked_on_background_work = %#v, want true", data["parked_on_background_work"])
	}
	if got, ok := data["parked_revision"].(uint64); !ok || got != 3 {
		t.Fatalf("parked_revision = %#v, want 3", data["parked_revision"])
	}
	if got, ok := data["parked_epoch"].(uint64); !ok || got != 42 {
		t.Fatalf("parked_epoch = %#v, want 42", data["parked_epoch"])
	}
}

// AC-50: an unwired provider (the pre-feature default, and every test that
// does not call SetTaskParkedProvider) omits the parked fields entirely
// rather than asserting a false value the backend cannot vouch for.
func TestPublishTaskUpdated_OmitsParkedFieldsWhenProviderUnwired(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	if _, ok := data["parked_on_background_work"]; ok {
		t.Fatalf("parked_on_background_work should be absent when unwired: %#v", data["parked_on_background_work"])
	}
	if _, ok := data["parked_revision"]; ok {
		t.Fatalf("parked_revision should be absent when unwired: %#v", data["parked_revision"])
	}
}

// A settled projection (false) must still be serialized explicitly so a
// client's stale "parked" reading is cleared, mirroring foreground_activity's
// explicit-nil-vs-omitted contract but for a plain boolean.
func TestPublishTaskUpdated_ParkedFalseIsSerializedExplicitly(t *testing.T) {
	svc, eventBus, repo := createTestService(t)
	ctx := context.Background()
	createTaskWithoutRepositories(t, ctx, repo)
	svc.SetTaskParkedProvider(fakeTaskParkedProvider{parked: false, revision: 2, epoch: 42})

	task, err := repo.GetTask(ctx, "task-1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	svc.PublishTaskUpdated(ctx, task)

	data := singlePublishedEventData(t, eventBus)
	got, ok := data["parked_on_background_work"].(bool)
	if !ok {
		t.Fatalf("parked_on_background_work missing: %#v", data["parked_on_background_work"])
	}
	if got {
		t.Fatal("expected parked_on_background_work = false to be serialized explicitly, not omitted")
	}
}

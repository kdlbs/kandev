package orchestrator

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// TestDeferCeilingRefusalCreatesRecordFromAbsent covers the first refusal for a
// task with no deferred_launch value at all: AC-11's create case.
func TestDeferCeilingRefusalCreatesRecordFromAbsent(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "defer-absent", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	payload := map[string]interface{}{"prompt": "hello"}
	if err := svc.deferCeilingRefusal(ctx, "defer-absent", "", models.CeilingLaunchStart, payload, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal: %v", err)
	}

	record := deferredLaunchOf(t, svc, "defer-absent")
	if record == nil {
		t.Fatal("no deferred_launch record was written")
	}
	if record[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling_deferred not set: %+v", record)
	}
	if record[models.CeilingLaunchKindKey] != string(models.CeilingLaunchStart) {
		t.Fatalf("ceiling_launch_kind = %v, want %q", record[models.CeilingLaunchKindKey], models.CeilingLaunchStart)
	}
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "hello" {
		t.Fatalf("ceiling_launch_payload did not carry the payload: %+v", record)
	}
	if record[models.CeilingQueuedAtKey] == nil || record[models.CeilingQueuedAtKey] == "" {
		t.Fatalf("ceiling_queued_at was not stamped: %+v", record)
	}
}

// TestDeferCeilingRefusalMergesOntoExistingWIPIntent pins AC-46a/AC-12b: a
// ceiling refusal must not clobber a pre-existing start_when_unblocked intent.
func TestDeferCeilingRefusalMergesOntoExistingWIPIntent(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "defer-merge", Title: "T",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: map[string]interface{}{
			models.DeferredLaunchStartWhenUnblockedKey: true,
			"user_id": "u1",
		}},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.deferCeilingRefusal(ctx, "defer-merge", "", models.CeilingLaunchStart, map[string]interface{}{"prompt": "p"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal: %v", err)
	}

	record := deferredLaunchOf(t, svc, "defer-merge")
	if record[models.DeferredLaunchStartWhenUnblockedKey] != true {
		t.Fatalf("start_when_unblocked was clobbered: %+v", record)
	}
	if record["user_id"] != "u1" {
		t.Fatalf("user_id was clobbered: %+v", record)
	}
	if record[models.CeilingDeferredKey] != true {
		t.Fatalf("ceiling_deferred was not added: %+v", record)
	}
}

// TestDeferCeilingRefusalIsANoOpOnByteIdenticalDuplicate pins AC-12a/AC-32: a
// second refusal that is the same launch must not disturb the stored record's
// queued_at.
func TestDeferCeilingRefusalIsANoOpOnByteIdenticalDuplicate(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "defer-dup", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	payload := map[string]interface{}{"prompt": "same"}
	if err := svc.deferCeilingRefusal(ctx, "defer-dup", "", models.CeilingLaunchStart, payload, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal (1): %v", err)
	}
	first := deferredLaunchOf(t, svc, "defer-dup")
	firstQueuedAt := first[models.CeilingQueuedAtKey]

	if err := svc.deferCeilingRefusal(ctx, "defer-dup", "", models.CeilingLaunchStart, map[string]interface{}{"prompt": "same"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal (2): %v", err)
	}
	second := deferredLaunchOf(t, svc, "defer-dup")
	if second[models.CeilingQueuedAtKey] != firstQueuedAt {
		t.Fatalf("queued_at changed on a byte-identical duplicate: %v -> %v", firstQueuedAt, second[models.CeilingQueuedAtKey])
	}
}

// TestDeferCeilingRefusalRetainsEarlierRecordOnDifferingDuplicate pins AC-12d:
// a second, differing automatic refusal for the same task must not overwrite the
// first launch's payload.
func TestDeferCeilingRefusalRetainsEarlierRecordOnDifferingDuplicate(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "defer-collide", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if err := svc.deferCeilingRefusal(ctx, "defer-collide", "", models.CeilingLaunchStart, map[string]interface{}{"prompt": "first"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal (1): %v", err)
	}
	if err := svc.deferCeilingRefusal(ctx, "defer-collide", "", models.CeilingLaunchStart, map[string]interface{}{"prompt": "second"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal (2): %v", err)
	}

	record := deferredLaunchOf(t, svc, "defer-collide")
	nested, _ := record[models.CeilingLaunchPayloadKey].(map[string]interface{})
	if nested["prompt"] != "first" {
		t.Fatalf("the earlier record was not retained: %+v", record)
	}
}

// TestDeferCeilingRefusalReplacesNonObjectValue pins AC-46: a non-object
// deferred_launch value is replaced wholesale by the ceiling record.
func TestDeferCeilingRefusalReplacesNonObjectValue(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{
		ID: "defer-nonobject", Title: "T",
		Metadata: map[string]interface{}{models.MetaKeyDeferredLaunch: "garbage"},
	}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}

	if err := svc.deferCeilingRefusal(ctx, "defer-nonobject", "", models.CeilingLaunchStart, map[string]interface{}{"prompt": "p"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal: %v", err)
	}

	record := deferredLaunchOf(t, svc, "defer-nonobject")
	if record[models.CeilingDeferredKey] != true {
		t.Fatalf("non-object value was not replaced with a ceiling record: %+v", record)
	}
}

// TestDeferCeilingRefusalPublishesTaskUpdated pins the fix for a ceiling
// deferral write not publishing task.updated: without it, the WS-driven UI
// never learns a launch was queued behind the session ceiling.
func TestDeferCeilingRefusalPublishesTaskUpdated(t *testing.T) {
	svc, repo := newServiceWithRealRepo(t)
	ctx := context.Background()
	if err := repo.CreateTask(ctx, &models.Task{ID: "defer-publish", Title: "T"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	events := &capturingTaskEvents{}
	svc.SetTaskEventPublisher(events)

	if err := svc.deferCeilingRefusal(ctx, "defer-publish", "s1", models.CeilingLaunchStart,
		map[string]interface{}{"prompt": "hello"}, ceilingReasonRefused, 0, false, 0); err != nil {
		t.Fatalf("deferCeilingRefusal: %v", err)
	}

	published := events.last()
	if published == nil {
		t.Fatal("deferCeilingRefusal did not publish a task.updated event")
	}
	if published.ID != "defer-publish" {
		t.Fatalf("published task id = %q, want %q", published.ID, "defer-publish")
	}
	deferred, ok := published.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	if !ok || deferred[models.CeilingDeferredKey] != true {
		t.Fatalf("published task's deferred_launch is missing the new ceiling record: %#v",
			published.Metadata[models.MetaKeyDeferredLaunch])
	}
}

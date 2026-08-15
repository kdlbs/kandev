package github

import (
	"context"
	"errors"
	"testing"
	"time"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// TestSetTaskPRDisposition_PublishesOnChangeNotOnIdenticalRePatch closes the
// gap flagged during PR review: no test previously exercised the event-bus
// side of SetTaskPRDisposition end to end (the controller test harness wires
// a nil event bus). AC-29 requires a publish when the disposition changes
// and no publish — nor an advance of disposition_recorded_at — on an
// identical re-PATCH.
func TestSetTaskPRDisposition_PublishesOnChangeNotOnIdenticalRePatch(t *testing.T) {
	svc, store, eb := setupSyncTest(t)
	ctx := context.Background()

	tp := &TaskPR{
		WorkspaceID: "ws-1", TaskID: "task-1", Owner: "acme", Repo: "demo",
		PRNumber: 1, PRURL: "https://github.com/acme/demo/pull/1", PRTitle: "closed pr",
		State: "closed", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, tp); err != nil {
		t.Fatalf("create task PR: %v", err)
	}
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error { return nil })

	disposition := TaskPRDispositionDuplicate
	patch := DispositionPatch{DispositionSet: true, Disposition: &disposition}
	updated, err := svc.SetTaskPRDisposition(ctx, "ws-1", tp.ID, patch)
	if err != nil {
		t.Fatalf("SetTaskPRDisposition: %v", err)
	}
	if updated == nil || updated.Disposition == nil || *updated.Disposition != TaskPRDispositionDuplicate {
		t.Fatalf("updated disposition = %+v, want %q", updated, TaskPRDispositionDuplicate)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("published events after first PATCH = %d, want 1", got)
	}
	firstRecordedAt := updated.DispositionRecordedAt
	if firstRecordedAt == nil {
		t.Fatal("DispositionRecordedAt = nil, want set after first PATCH")
	}

	updated, err = svc.SetTaskPRDisposition(ctx, "ws-1", tp.ID, patch)
	if err != nil {
		t.Fatalf("SetTaskPRDisposition (identical re-PATCH): %v", err)
	}
	if got := eb.publishedCount(); got != 1 {
		t.Fatalf("published events after identical re-PATCH = %d, want still 1 (no republish)", got)
	}
	if updated.DispositionRecordedAt == nil || !updated.DispositionRecordedAt.Equal(*firstRecordedAt) {
		t.Fatalf("DispositionRecordedAt after identical re-PATCH = %v, want unchanged %v",
			updated.DispositionRecordedAt, firstRecordedAt)
	}
}

func TestSetTaskPRDispositionLegacyWorkspaceMismatchFailsClosed(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 10,
		PRURL: "https://github.com/acme/demo/pull/10", PRTitle: "closed pr",
		State: "closed", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"},
	})
	authorizerCalls := 0
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		authorizerCalls++
		return nil
	})
	disposition := TaskPRDispositionDuplicate
	patch := DispositionPatch{DispositionSet: true, Disposition: &disposition}
	beforeMetric := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, outcomeMetricLabel("action", "set"))

	_, err := svc.SetTaskPRDisposition(ctx, "ws-other", pr.ID, patch)
	if !errors.Is(err, ErrTaskPRNotFound) {
		t.Fatalf("error = %v, want ErrTaskPRNotFound", err)
	}
	stored, err := store.GetTaskPRByID(ctx, pr.ID)
	if err != nil {
		t.Fatalf("reload legacy PR: %v", err)
	}
	if stored == nil || stored.Disposition != nil || stored.DispositionRecordedAt != nil {
		t.Fatalf("stored legacy PR = %+v, want no disposition write", stored)
	}
	if authorizerCalls != 0 {
		t.Fatalf("workspace authorizer calls = %d, want 0 on mismatch", authorizerCalls)
	}
	if got := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, outcomeMetricLabel("action", "set")); got != beforeMetric {
		t.Fatalf("set disposition metric = %d, want unchanged %d", got, beforeMetric)
	}
	if eventBus.publishedCount() != 0 {
		t.Fatalf("published events = %d, want 0 on rejected disposition", eventBus.publishedCount())
	}
}

func TestSetTaskPRDispositionLegacyWorkspaceUsesTaskWorkspace(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 13,
		PRURL: "https://github.com/acme/demo/pull/13", PRTitle: "closed pr",
		State: "closed", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	svc.SetTaskIssueStore(&fakeTaskIssueStore{
		task: &taskmodels.Task{ID: "task-legacy", WorkspaceID: "ws-owner"},
	})
	var authorizedWorkspace string
	svc.SetWorkspaceAuthorizer(func(_ context.Context, workspaceID string) error {
		authorizedWorkspace = workspaceID
		return nil
	})
	disposition := TaskPRDispositionDuplicate
	updated, err := svc.SetTaskPRDisposition(ctx, "ws-owner", pr.ID, DispositionPatch{
		DispositionSet: true,
		Disposition:    &disposition,
	})
	if err != nil {
		t.Fatalf("SetTaskPRDisposition: %v", err)
	}
	if updated == nil || updated.Disposition == nil || *updated.Disposition != disposition {
		t.Fatalf("updated disposition = %+v, want %q", updated, disposition)
	}
	if authorizedWorkspace != "ws-owner" {
		t.Fatalf("authorized workspace = %q, want ws-owner", authorizedWorkspace)
	}
	if updated.WorkspaceID != "" {
		t.Fatalf("updated association workspace = %q, want legacy blank value", updated.WorkspaceID)
	}
	if eventBus.publishedCount() != 1 {
		t.Fatalf("published events = %d, want one update event", eventBus.publishedCount())
	}
	eventBus.mu.Lock()
	event := eventBus.events[0]
	eventBus.mu.Unlock()
	eventPayload, ok := event.Data.(*TaskPR)
	if !ok {
		t.Fatalf("event payload type = %T, want *TaskPR", event.Data)
	}
	if eventPayload.WorkspaceID != "ws-owner" {
		t.Fatalf("event workspace = %q, want ws-owner", eventPayload.WorkspaceID)
	}
	if eventPayload == updated {
		t.Fatal("event payload aliases stored legacy association")
	}
}

func TestSetTaskPRDispositionLegacyWorkspaceRequiresTaskResolver(t *testing.T) {
	svc, store, eventBus := setupSyncTest(t)
	ctx := context.Background()
	pr := &TaskPR{
		TaskID: "task-legacy", Owner: "acme", Repo: "demo", PRNumber: 11,
		PRURL: "https://github.com/acme/demo/pull/11", PRTitle: "closed pr",
		State: "closed", CreatedAt: time.Now().UTC(),
	}
	if err := store.CreateTaskPR(ctx, pr); err != nil {
		t.Fatalf("create legacy PR: %v", err)
	}
	authorizerCalls := 0
	svc.SetWorkspaceAuthorizer(func(context.Context, string) error {
		authorizerCalls++
		return nil
	})
	disposition := TaskPRDispositionDuplicate
	patch := DispositionPatch{DispositionSet: true, Disposition: &disposition}
	beforeMetric := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, outcomeMetricLabel("action", "set"))

	_, err := svc.SetTaskPRDisposition(ctx, "ws-supplied", pr.ID, patch)
	if !errors.Is(err, ErrTaskPRNotFound) {
		t.Fatalf("error = %v, want ErrTaskPRNotFound", err)
	}
	stored, err := store.GetTaskPRByID(ctx, pr.ID)
	if err != nil {
		t.Fatalf("reload legacy PR: %v", err)
	}
	if stored == nil || stored.Disposition != nil || stored.DispositionRecordedAt != nil {
		t.Fatalf("stored legacy PR = %+v, want no disposition write", stored)
	}
	if authorizerCalls != 0 {
		t.Fatalf("workspace authorizer calls = %d, want 0 without resolver", authorizerCalls)
	}
	if got := readOutcomeCounter(t, taskPROutcomeDispositionsTotal, outcomeMetricLabel("action", "set")); got != beforeMetric {
		t.Fatalf("set disposition metric = %d, want unchanged %d", got, beforeMetric)
	}
	if eventBus.publishedCount() != 0 {
		t.Fatalf("published events = %d, want 0 on rejected disposition", eventBus.publishedCount())
	}
}

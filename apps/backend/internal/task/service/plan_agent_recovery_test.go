package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func TestPlanAgentRecovery(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery"
	seedTask(t, ctx, repo, taskID)
	first, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Title: "Original", Content: "complete plan",
	})
	if err != nil {
		t.Fatalf("CreatePlan(first): %v", err)
	}
	damaged, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Title: "Damaged", Content: "short fragment", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("CreatePlan(damaged): %v", err)
	}
	history, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions: %v", err)
	}
	if len(history) != 2 {
		t.Fatalf("history length = %d, want 2", len(history))
	}
	var source *models.TaskPlanRevision
	for _, revision := range history {
		if revision.Content == first.Plan.Content {
			source = revision
		}
	}
	if source == nil {
		t.Fatal("could not identify the preserved source revision")
	}
	_, sourceVersion, err := svc.GetAgentPlanRevision(ctx, taskID, source.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision: %v", err)
	}

	restored, err := svc.RestoreAgentPlanRevision(ctx, RestorePlanRequest{
		TaskID: taskID, RevisionID: source.ID,
		ExpectedVersion: damaged.Plan.WriteVersion, ExpectedRevisionVersion: sourceVersion,
	})
	if err != nil {
		t.Fatalf("RestoreAgentPlanRevision: %v", err)
	}
	if restored.AlreadyCurrent || restored.Plan.Content != first.Plan.Content || restored.Plan.Title != first.Plan.Title {
		t.Fatalf("restore result = %#v, want original snapshot", restored)
	}
	if restored.Plan.WriteVersion == damaged.Plan.WriteVersion {
		t.Fatal("restore did not rotate the HEAD version")
	}
	if restored.Revision.RevertOfRevisionID == nil || *restored.Revision.RevertOfRevisionID != source.ID {
		t.Fatalf("restore revision source = %v, want %q", restored.Revision.RevertOfRevisionID, source.ID)
	}
	if restored.Revision.AuthorKind != createdByAgent {
		t.Fatalf("restore author kind = %q, want agent", restored.Revision.AuthorKind)
	}

	finalHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions after restore: %v", err)
	}
	if len(finalHistory) != 3 || finalHistory[1].Content != damaged.Plan.Content || finalHistory[2].Content != source.Content {
		t.Fatalf("history after restore = %#v, want damaged and source preserved", finalHistory)
	}
}

func TestPlanAgentRecoveryRequiresBothStableVersions(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-versions"
	seedTask(t, ctx, repo, taskID)
	first, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "first"})
	if err != nil {
		t.Fatalf("CreatePlan(first): %v", err)
	}
	second, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "second", ForceNewRevision: true})
	if err != nil {
		t.Fatalf("CreatePlan(second): %v", err)
	}
	revision, err := svc.GetLatestRevision(ctx, taskID)
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	_, revisionVersion, err := svc.GetAgentPlanRevision(ctx, taskID, revision.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision: %v", err)
	}

	cases := []struct {
		name string
		req  RestorePlanRequest
		code string
	}{
		{
			name: "missing head version", code: PlanErrorVersionRequired,
			req: RestorePlanRequest{TaskID: taskID, RevisionID: revision.ID, ExpectedRevisionVersion: revisionVersion},
		},
		{
			name: "missing source version", code: PlanErrorRevisionVersionRequired,
			req: RestorePlanRequest{TaskID: taskID, RevisionID: revision.ID, ExpectedVersion: second.Plan.WriteVersion},
		},
		{
			name: "stale head version", code: PlanErrorVersionConflict,
			req: RestorePlanRequest{TaskID: taskID, RevisionID: revision.ID, ExpectedVersion: first.Plan.WriteVersion, ExpectedRevisionVersion: revisionVersion},
		},
		{
			name: "stale source version", code: PlanErrorRevisionChanged,
			req: RestorePlanRequest{TaskID: taskID, RevisionID: revision.ID, ExpectedVersion: second.Plan.WriteVersion, ExpectedRevisionVersion: "stale"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.RestoreAgentPlanRevision(ctx, tc.req)
			var safety *PlanSafetyError
			if !errors.As(err, &safety) || safety.Code != tc.code {
				t.Fatalf("error = %T %v, want safety code %q", err, err, tc.code)
			}
		})
	}
	got, err := svc.GetPlan(ctx, taskID)
	if err != nil || got.Content != "second" || got.WriteVersion != second.Plan.WriteVersion {
		t.Fatalf("version failures changed plan = %#v, %v", got, err)
	}
}

// TestPlanAgentRecoveryRejectsCoalescedSourceSnapshot covers
// AC-TASKS-PLAN-SAFE-004.2 and AC-TASKS-PLAN-SAFE-005.1: a source token that
// was valid before coalescing must not restore the source after its bytes
// changed in place.
func TestPlanAgentRecoveryRejectsCoalescedSourceSnapshot(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-coalesced-source"
	seedTask(t, ctx, repo, taskID)

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "source-v1"})
	if err != nil {
		t.Fatalf("CreatePlan(first): %v", err)
	}
	firstRevision, err := svc.GetLatestRevision(ctx, taskID)
	if err != nil {
		t.Fatalf("GetLatestRevision(first): %v", err)
	}
	source, sourceVersion, err := svc.GetAgentPlanRevision(ctx, taskID, firstRevision.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision(source): %v", err)
	}

	_, err = svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "source-v2"})
	if err != nil {
		t.Fatalf("CreatePlan(coalesced source): %v", err)
	}
	coalescedRevision, err := svc.GetLatestRevision(ctx, taskID)
	if err != nil {
		t.Fatalf("GetLatestRevision(coalesced source): %v", err)
	}
	if coalescedRevision.ID != source.ID || coalescedRevision.RevisionNumber != source.RevisionNumber || coalescedRevision.Content != "source-v2" {
		t.Fatalf("coalesced revision = %#v, want source ID/number with updated bytes", coalescedRevision)
	}
	mutatedSource, mutatedSourceVersion, err := svc.GetAgentPlanRevision(ctx, taskID, source.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision(mutated source): %v", err)
	}
	if mutatedSourceVersion == sourceVersion || mutatedSource.Content != "source-v2" {
		t.Fatalf("source snapshot did not change in place: content=%q version=%q", mutatedSource.Content, mutatedSourceVersion)
	}

	damaged, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Content: "damaged", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("CreatePlan(damaged): %v", err)
	}
	beforePlan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan before restore: %v", err)
	}
	beforeHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions before restore: %v", err)
	}
	eventBus.ClearEvents()

	_, err = svc.RestoreAgentPlanRevision(ctx, RestorePlanRequest{
		TaskID: taskID, RevisionID: source.ID,
		ExpectedVersion: damaged.Plan.WriteVersion, ExpectedRevisionVersion: sourceVersion,
	})
	var safety *PlanSafetyError
	if !errors.As(err, &safety) || safety.Code != PlanErrorRevisionChanged {
		t.Fatalf("restore error = %T %v, want %q", err, err, PlanErrorRevisionChanged)
	}
	if safety.CurrentRevisionVersion != mutatedSourceVersion {
		t.Fatalf("error source version = %q, want current %q", safety.CurrentRevisionVersion, mutatedSourceVersion)
	}

	gotPlan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan after rejected restore: %v", err)
	}
	if !reflect.DeepEqual(gotPlan, beforePlan) {
		t.Fatalf("plan changed after rejected restore: got %#v, want %#v", gotPlan, beforePlan)
	}
	gotHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions after rejected restore: %v", err)
	}
	if !reflect.DeepEqual(gotHistory, beforeHistory) {
		t.Fatalf("history changed after rejected restore: got %#v, want %#v", gotHistory, beforeHistory)
	}
	if published := eventBus.GetPublishedEvents(); len(published) != 0 {
		t.Fatalf("rejected restore published %d events", len(published))
	}
}

// TestPlanAgentRecoveryRestoresOversizedHistoryAndPreservesPlanMetadata covers
// AC-TASKS-PLAN-SAFE-002.5 and AC-TASKS-PLAN-SAFE-004.7: historical content is
// exempt from the current-write size limit, while comments and implementation
// markers survive the restore.
func TestPlanAgentRecoveryRestoresOversizedHistoryAndPreservesPlanMetadata(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-oversized"
	seedTask(t, ctx, repo, taskID)
	largeContent := strings.Repeat("x", MaxPlanContentBytes+1)
	head := &models.TaskPlan{
		ID: taskID + "-plan", TaskID: taskID, Title: "Historical source",
		Content: largeContent, CreatedBy: createdByAgent,
	}
	sourceRevision := &models.TaskPlanRevision{
		ID: taskID + "-source", TaskID: taskID, RevisionNumber: 1,
		Title: "Historical source", Content: largeContent,
		AuthorKind: createdByAgent, AuthorName: "Agent",
	}
	if err := repo.WritePlanRevision(ctx, head, sourceRevision, nil, false, false); err != nil {
		t.Fatalf("WritePlanRevision(source): %v", err)
	}
	if len(sourceRevision.Content) <= MaxPlanContentBytes {
		t.Fatal("test source did not exceed the current plan size limit")
	}
	if _, err := repo.MarkTaskPlanImplementationStarted(ctx, taskID, "session-preserve", "user"); err != nil {
		t.Fatalf("MarkTaskPlanImplementationStarted: %v", err)
	}
	comment := &models.TaskPlanComment{
		ID: taskID + "-comment", TaskID: taskID, PlanID: head.ID,
		Body: "Keep this review note", SelectedText: "historical",
		AnchorFrom: 0, AnchorTo: len("historical"),
	}
	if _, err := repo.CreateTaskPlanComment(ctx, comment); err != nil {
		t.Fatalf("CreateTaskPlanComment: %v", err)
	}

	damaged, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Title: "Current", Content: "current", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("CreatePlan(damaged): %v", err)
	}
	beforeRestore, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan before restore: %v", err)
	}
	if beforeRestore.ImplementationStartedAt == nil || beforeRestore.ImplementationStartedSessionID == nil || beforeRestore.ImplementationStartedBy == nil {
		t.Fatalf("metadata before restore = %#v, want implementation marker", beforeRestore)
	}
	startedAt := *beforeRestore.ImplementationStartedAt
	startedSessionID := *beforeRestore.ImplementationStartedSessionID
	startedBy := *beforeRestore.ImplementationStartedBy
	beforeComments, err := repo.ListTaskPlanComments(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskPlanComments before restore: %v", err)
	}
	beforeHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions before restore: %v", err)
	}
	_, sourceVersion, err := svc.GetAgentPlanRevision(ctx, taskID, sourceRevision.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision(source): %v", err)
	}
	eventBus.ClearEvents()

	result, err := svc.RestoreAgentPlanRevision(ctx, RestorePlanRequest{
		TaskID: taskID, RevisionID: sourceRevision.ID,
		ExpectedVersion: damaged.Plan.WriteVersion, ExpectedRevisionVersion: sourceVersion,
	})
	if err != nil {
		t.Fatalf("RestoreAgentPlanRevision(oversized source): %v", err)
	}
	if result.Plan == nil || result.Plan.Title != sourceRevision.Title || result.Plan.Content != largeContent {
		t.Fatalf("restore result = %#v, want the oversized historical source", result)
	}
	if result.Plan.WriteVersion == damaged.Plan.WriteVersion {
		t.Fatal("restore did not rotate the HEAD version")
	}
	if result.Plan.ImplementationStartedAt == nil || !result.Plan.ImplementationStartedAt.Equal(startedAt) {
		t.Fatalf("restored marker time = %v, want %v", result.Plan.ImplementationStartedAt, startedAt)
	}
	if result.Plan.ImplementationStartedSessionID == nil || *result.Plan.ImplementationStartedSessionID != startedSessionID {
		t.Fatalf("restored session marker = %v, want %s", result.Plan.ImplementationStartedSessionID, startedSessionID)
	}
	if result.Plan.ImplementationStartedBy == nil || *result.Plan.ImplementationStartedBy != startedBy {
		t.Fatalf("restored marker actor = %v, want %s", result.Plan.ImplementationStartedBy, startedBy)
	}
	if result.Plan.CommentsRevision != beforeRestore.CommentsRevision {
		t.Fatalf("restored comments revision = %d, want %d", result.Plan.CommentsRevision, beforeRestore.CommentsRevision)
	}
	afterComments, err := repo.ListTaskPlanComments(ctx, taskID)
	if err != nil {
		t.Fatalf("ListTaskPlanComments after restore: %v", err)
	}
	if !reflect.DeepEqual(afterComments, beforeComments) {
		t.Fatalf("comments changed after restore: got %#v, want %#v", afterComments, beforeComments)
	}
	afterHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions after restore: %v", err)
	}
	if len(afterHistory) != len(beforeHistory)+1 || afterHistory[len(afterHistory)-1].Content != largeContent {
		t.Fatalf("history after restore = %#v, want the oversized source plus one revert", afterHistory)
	}
	if published := eventBus.GetPublishedEvents(); len(published) == 0 {
		t.Fatal("successful restore did not publish plan events")
	}
}

// TestPlanAgentRecoveryConflictsWithInterveningBrowserWrite covers
// AC-TASKS-PLAN-SAFE-002.4, AC-TASKS-PLAN-SAFE-004.2, and
// AC-TASKS-PLAN-SAFE-005.4: a browser write admitted while an agent restore is
// waiting must win, and the restore must not overwrite it.
func TestPlanAgentRecoveryConflictsWithInterveningBrowserWrite(t *testing.T) {
	svc, eventBus, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-browser-race"
	seedTask(t, ctx, repo, taskID)

	_, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "source"})
	if err != nil {
		t.Fatalf("CreatePlan(source): %v", err)
	}
	source, sourceVersion, err := svc.GetAgentPlanRevision(ctx, taskID, mustGetLatestRevisionID(t, svc, ctx, taskID))
	if err != nil {
		t.Fatalf("GetAgentPlanRevision(source): %v", err)
	}
	_ = source
	damaged, err := svc.CreatePlan(ctx, CreatePlanRequest{
		TaskID: taskID, Content: "agent damage", ForceNewRevision: true,
	})
	if err != nil {
		t.Fatalf("CreatePlan(damaged): %v", err)
	}

	authorizationEntered := make(chan struct{})
	releaseAuthorization := make(chan struct{})
	var firstAuthorization sync.Once
	svc.SetTaskAuthorizer(func(context.Context, string) error {
		blocked := false
		firstAuthorization.Do(func() {
			close(authorizationEntered)
			blocked = true
		})
		if blocked {
			<-releaseAuthorization
		}
		return nil
	})

	type restoreOutcome struct {
		result RestorePlanResult
		err    error
	}
	restoreDone := make(chan restoreOutcome, 1)
	go func() {
		result, restoreErr := svc.RestoreAgentPlanRevision(ctx, RestorePlanRequest{
			TaskID: taskID, RevisionID: source.ID,
			ExpectedVersion: damaged.Plan.WriteVersion, ExpectedRevisionVersion: sourceVersion,
		})
		restoreDone <- restoreOutcome{result: result, err: restoreErr}
	}()
	select {
	case <-authorizationEntered:
	case <-time.After(time.Second):
		t.Fatal("restore did not reach authorization barrier")
	}

	browser, err := svc.UpdatePlan(ctx, UpdatePlanRequest{
		TaskID: taskID, Title: "Browser", Content: "browser edit", CreatedBy: createdByUser,
	})
	if err != nil {
		t.Fatalf("UpdatePlan(browser): %v", err)
	}
	beforeRestorePlan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan before releasing restore: %v", err)
	}
	if beforeRestorePlan.Content != browser.Plan.Content || beforeRestorePlan.WriteVersion == damaged.Plan.WriteVersion {
		t.Fatalf("browser write = %#v, stored = %#v", browser.Plan, beforeRestorePlan)
	}
	beforeRestoreHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions before releasing restore: %v", err)
	}
	eventsAfterBrowser := len(eventBus.GetPublishedEvents())
	if eventsAfterBrowser == 0 {
		t.Fatal("browser write did not publish an event")
	}

	close(releaseAuthorization)
	var outcome restoreOutcome
	select {
	case outcome = <-restoreDone:
	case <-time.After(time.Second):
		t.Fatal("restore did not finish after authorization barrier release")
	}
	var safety *PlanSafetyError
	if !errors.As(outcome.err, &safety) || safety.Code != PlanErrorVersionConflict {
		t.Fatalf("restore error = %T %v, want %q", outcome.err, outcome.err, PlanErrorVersionConflict)
	}
	if outcome.result.Plan != nil {
		t.Fatalf("rejected restore returned a plan: %#v", outcome.result.Plan)
	}

	gotPlan, err := svc.GetPlan(ctx, taskID)
	if err != nil {
		t.Fatalf("GetPlan after rejected restore: %v", err)
	}
	if !reflect.DeepEqual(gotPlan, beforeRestorePlan) {
		t.Fatalf("plan changed after rejected restore: got %#v, want %#v", gotPlan, beforeRestorePlan)
	}
	gotHistory, err := svc.ListRevisions(ctx, taskID)
	if err != nil {
		t.Fatalf("ListRevisions after rejected restore: %v", err)
	}
	if !reflect.DeepEqual(gotHistory, beforeRestoreHistory) {
		t.Fatalf("history changed after rejected restore: got %#v, want %#v", gotHistory, beforeRestoreHistory)
	}
	if published := len(eventBus.GetPublishedEvents()); published != eventsAfterBrowser {
		t.Fatalf("rejected restore published events: before=%d after=%d", eventsAfterBrowser, published)
	}
}

func mustGetLatestRevisionID(t *testing.T, svc *PlanService, ctx context.Context, taskID string) string {
	t.Helper()
	revision, err := svc.GetLatestRevision(ctx, taskID)
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	return revision.ID
}

func TestPlanAgentRecoveryAlreadyCurrentDoesNotWrite(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-current"
	seedTask(t, ctx, repo, taskID)
	created, err := svc.CreatePlan(ctx, CreatePlanRequest{TaskID: taskID, Content: "current"})
	if err != nil {
		t.Fatalf("CreatePlan: %v", err)
	}
	latest, err := svc.GetLatestRevision(ctx, taskID)
	if err != nil {
		t.Fatalf("GetLatestRevision: %v", err)
	}
	_, revisionVersion, err := svc.GetAgentPlanRevision(ctx, taskID, latest.ID)
	if err != nil {
		t.Fatalf("GetAgentPlanRevision: %v", err)
	}
	result, err := svc.RestoreAgentPlanRevision(ctx, RestorePlanRequest{
		TaskID: taskID, RevisionID: latest.ID,
		ExpectedVersion: created.Plan.WriteVersion, ExpectedRevisionVersion: revisionVersion,
	})
	if err != nil {
		t.Fatalf("RestoreAgentPlanRevision: %v", err)
	}
	if !result.AlreadyCurrent || result.Plan.WriteVersion != created.Plan.WriteVersion {
		t.Fatalf("already-current result = %#v, want unchanged version", result)
	}
	history, err := svc.ListRevisions(ctx, taskID)
	if err != nil || len(history) != 1 {
		t.Fatalf("already-current history = %d, %v; want one row", len(history), err)
	}
}

func TestPlanAgentRevisionMetadataIsBoundedAndContentFree(t *testing.T) {
	svc, _, repo := createTestPlanService(t)
	ctx := context.Background()
	taskID := "task-plan-agent-recovery-metadata"
	seedTask(t, ctx, repo, taskID)
	for i := 0; i < 3; i++ {
		if _, err := svc.CreatePlan(ctx, CreatePlanRequest{
			TaskID: taskID, Content: strings.Repeat(string(rune('a'+i)), 10), ForceNewRevision: true,
		}); err != nil {
			t.Fatalf("CreatePlan(%d): %v", i, err)
		}
	}
	page, err := svc.ListAgentPlanRevisionMetadata(ctx, taskID, 0, 2)
	if err != nil {
		t.Fatalf("ListAgentPlanRevisionMetadata: %v", err)
	}
	if len(page.Revisions) != 2 || page.NextBeforeRevisionNum == 0 {
		t.Fatalf("page = %#v, want two rows and a cursor", page)
	}
	for _, revision := range page.Revisions {
		if revision.Content != "" || revision.ContentBytes != 10 {
			t.Fatalf("metadata revision = %#v, want no content and byte size 10", revision)
		}
	}
	next, err := svc.ListAgentPlanRevisionMetadata(ctx, taskID, page.NextBeforeRevisionNum, 2)
	if err != nil {
		t.Fatalf("next metadata page: %v", err)
	}
	if len(next.Revisions) != 1 || next.Revisions[0].RevisionNumber >= page.NextBeforeRevisionNum {
		t.Fatalf("next page = %#v, want the older revision", next)
	}
}

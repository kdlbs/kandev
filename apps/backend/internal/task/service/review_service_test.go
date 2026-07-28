package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

func createTestReviewService(t *testing.T) (*ReviewService, *MockEventBus, *sqliterepo.Repository) {
	t.Helper()
	_, eventBus, repo := createTestService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	return NewReviewService(repo, eventBus, log), eventBus, repo
}

func validFindingInput() ReviewFindingInput {
	return ReviewFindingInput{
		FilePath:     "apps/web/a.ts",
		StartLine:    12,
		EndLine:      14,
		Severity:     "blocker",
		Category:     "correctness",
		Title:        "Nil dereference",
		Body:         "`x` can be nil here.",
		FileDiffHash: "deadbeef",
	}
}

func eventTypes(events []*bus.Event) []string {
	types := make([]string, 0, len(events))
	for _, e := range events {
		types = append(types, e.Type)
	}
	return types
}

func countEvents(published []*bus.Event, eventType string) int {
	n := 0
	for _, e := range published {
		if e.Type == eventType {
			n++
		}
	}
	return n
}

func TestReviewService_CreateRunPublishesPendingRun(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-1")

	run, err := svc.CreateRun(ctx, CreateRunRequest{
		TaskID:    "task-1",
		SessionID: "sess-1",
		Trigger:   models.ReviewTriggerManual,
		AgentID:   "claude-acp",
		Model:     "claude-haiku-4-5",
	})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run.Status != models.ReviewRunPending {
		t.Fatalf("expected pending run, got %q", run.Status)
	}
	if countEvents(eventBus.GetPublishedEvents(), events.TaskReviewRunUpdated) != 1 {
		t.Fatalf("expected one run event, got %v", eventTypes(eventBus.GetPublishedEvents()))
	}
}

func TestReviewService_CreateRunRequiresTaskID(t *testing.T) {
	svc, _, _ := createTestReviewService(t)
	if _, err := svc.CreateRun(context.Background(), CreateRunRequest{}); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestReviewService_RunLifecycleTransitions(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-2")
	run, err := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-2", Trigger: models.ReviewTriggerManual})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}

	if _, err := svc.MarkRunRunning(ctx, run.ID); err != nil {
		t.Fatalf("MarkRunRunning: %v", err)
	}
	active, err := svc.ActiveRun(ctx, "task-2")
	if err != nil || active == nil || active.ID != run.ID {
		t.Fatalf("expected the running run to be active, got %+v err=%v", active, err)
	}

	completed, err := svc.CompleteRun(ctx, CompleteRunRequest{
		RunID: run.ID, Summary: "done", FindingCount: 2, FileCount: 3, RepositoryCount: 1, DurationMs: 900,
	})
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	if completed.Status != models.ReviewRunCompleted || completed.CompletedAt == nil {
		t.Fatalf("expected completed with timestamp, got %+v", completed)
	}
	if completed.FindingCount != 2 || completed.Summary != "done" {
		t.Fatalf("counts not persisted: %+v", completed)
	}

	after, err := svc.ActiveRun(ctx, "task-2")
	if err != nil {
		t.Fatalf("ActiveRun: %v", err)
	}
	if after != nil {
		t.Fatalf("a completed run must not be active, got %+v", after)
	}
	// create + running + completed
	if got := countEvents(eventBus.GetPublishedEvents(), events.TaskReviewRunUpdated); got != 3 {
		t.Fatalf("expected 3 run events, got %d (%v)", got, eventTypes(eventBus.GetPublishedEvents()))
	}
}

func TestReviewService_FailRunRecordsCodeAndBoundsMessage(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-3")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-3"})

	huge := strings.Repeat("x", maxRunErrorMessage*2)
	failed, err := svc.FailRun(ctx, run.ID, "review_unparseable_response", huge, 120)
	if err != nil {
		t.Fatalf("FailRun: %v", err)
	}
	if failed.Status != models.ReviewRunFailed || failed.ErrorCode != "review_unparseable_response" {
		t.Fatalf("unexpected failure state: %+v", failed)
	}
	if len(failed.ErrorMessage) != maxRunErrorMessage {
		t.Fatalf("expected error message bounded to %d, got %d", maxRunErrorMessage, len(failed.ErrorMessage))
	}
	if failed.CompletedAt == nil {
		t.Fatal("expected completed_at on failure")
	}
}

func TestReviewService_CancelRunIsIdempotent(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-4")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-4"})

	cancelled, err := svc.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if cancelled.Status != models.ReviewRunCancelled {
		t.Fatalf("expected cancelled, got %q", cancelled.Status)
	}
	again, err := svc.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("second CancelRun should be a no-op, got %v", err)
	}
	if again.Status != models.ReviewRunCancelled {
		t.Fatalf("expected cancelled to stick, got %q", again.Status)
	}
}

func TestReviewService_CancelCompletedRunLeavesItCompleted(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-5")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-5"})
	if _, err := svc.CompleteRun(ctx, CompleteRunRequest{RunID: run.ID}); err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}

	got, err := svc.CancelRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	if got.Status != models.ReviewRunCompleted {
		t.Fatalf("cancel must not override a terminal run, got %q", got.Status)
	}
}

func TestReviewService_PublishFindingsWithExistingRun(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-6")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-6"})
	eventBus.ClearEvents()

	gotRun, findings, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-6",
		RunID:    run.ID,
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	if gotRun.ID != run.ID {
		t.Fatalf("expected the existing run reused, got %q", gotRun.ID)
	}
	if len(findings) != 1 || findings[0].Status != models.ReviewFindingOpen {
		t.Fatalf("unexpected findings: %+v", findings)
	}
	if findings[0].Side != models.ReviewSideAdditions {
		t.Fatalf("expected additions side default, got %q", findings[0].Side)
	}
	if countEvents(eventBus.GetPublishedEvents(), events.TaskReviewFindingsPublished) != 1 {
		t.Fatalf("expected a findings_published event, got %v", eventTypes(eventBus.GetPublishedEvents()))
	}
}

func TestReviewService_PublishFindingsDeniedByTaskAuthorizer(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-foreign")

	denied := errors.New("task not found")
	var authorized string
	svc.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		authorized = taskID
		return denied
	})

	_, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-foreign",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if !errors.Is(err, denied) {
		t.Fatalf("expected the authorizer's error, got %v", err)
	}
	if authorized != "task-foreign" {
		t.Fatalf("authorizer must be called with the target task_id, got %q", authorized)
	}
	// A denied publish must not store findings or emit any event.
	if len(eventBus.GetPublishedEvents()) != 0 {
		t.Fatalf("denied publish must not emit events, got %v", eventTypes(eventBus.GetPublishedEvents()))
	}
}

func TestReviewService_PublishFindingsAllowedWhenAuthorizerPermits(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-reachable")

	var authorized string
	svc.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		authorized = taskID
		return nil
	})

	_, findings, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-reachable",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	if authorized != "task-reachable" {
		t.Fatalf("authorizer must be called with the target task_id, got %q", authorized)
	}
	if len(findings) != 1 {
		t.Fatalf("expected the findings stored, got %d", len(findings))
	}
}

func TestReviewService_PublishFindingsCreatesAgentRunWhenRunIDEmpty(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-7")

	run, findings, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-7",
		Trigger:  models.ReviewTriggerAgent,
		Summary:  "  agent review  ",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	if run.Trigger != models.ReviewTriggerAgent || run.Status != models.ReviewRunCompleted {
		t.Fatalf("expected a completed agent run, got %+v", run)
	}
	if run.Summary != "agent review" {
		t.Fatalf("expected trimmed summary, got %q", run.Summary)
	}
	if run.FindingCount != 1 || len(findings) != 1 {
		t.Fatalf("expected one finding attributed to the run, got %+v", run)
	}
}

func TestReviewService_PublishFindingsRejectsWholeBatchOnInvalidEntry(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-8")

	invalid := map[string]func(ReviewFindingInput) ReviewFindingInput{
		"missing file":    func(f ReviewFindingInput) ReviewFindingInput { f.FilePath = "  "; return f },
		"zero line":       func(f ReviewFindingInput) ReviewFindingInput { f.StartLine = 0; return f },
		"backwards range": func(f ReviewFindingInput) ReviewFindingInput { f.StartLine = 9; f.EndLine = 2; return f },
		"bad severity":    func(f ReviewFindingInput) ReviewFindingInput { f.Severity = "urgent"; return f },
		"missing title":   func(f ReviewFindingInput) ReviewFindingInput { f.Title = " "; return f },
		"missing body":    func(f ReviewFindingInput) ReviewFindingInput { f.Body = ""; return f },
	}
	for name, mutate := range invalid {
		t.Run(name, func(t *testing.T) {
			_, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
				TaskID:   "task-8",
				Findings: []ReviewFindingInput{validFindingInput(), mutate(validFindingInput())},
			})
			if !errors.Is(err, ErrInvalidReviewFinding) {
				t.Fatalf("expected ErrInvalidReviewFinding, got %v", err)
			}
			if !strings.Contains(err.Error(), "finding 2") {
				t.Fatalf("error should name the offending index, got %v", err)
			}
			stored, listErr := repo.ListTaskReviewFindings(ctx, "task-8")
			if listErr != nil {
				t.Fatalf("ListTaskReviewFindings: %v", listErr)
			}
			if len(stored) != 0 {
				t.Fatalf("a rejected batch must store nothing, found %d", len(stored))
			}
		})
	}
}

func TestReviewService_PublishFindingsRequiresTaskAndFindings(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-9")

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{Findings: []ReviewFindingInput{validFindingInput()}}); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-9"}); !errors.Is(err, ErrInvalidReviewFinding) {
		t.Fatalf("expected ErrInvalidReviewFinding for an empty batch, got %v", err)
	}
}

func TestReviewService_PublishFindingsSupersedesEarlierOpenDuplicate(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-10")

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-10",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-10",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	stored, err := repo.ListTaskReviewFindings(ctx, "task-10")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("expected the duplicate superseded, found %d findings", len(stored))
	}
}

func TestReviewService_PublishFindingsKeepsResolvedDuplicate(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-11")

	_, first, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-11",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	if _, err := svc.UpdateFindingStatus(ctx, first[0].ID, models.ReviewFindingResolved); err != nil {
		t.Fatalf("UpdateFindingStatus: %v", err)
	}

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-11",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	stored, err := repo.ListTaskReviewFindings(ctx, "task-11")
	if err != nil {
		t.Fatalf("ListTaskReviewFindings: %v", err)
	}
	if len(stored) != 2 {
		t.Fatalf("a resolved finding must survive a re-review, found %d", len(stored))
	}
}

func TestReviewService_UpdateFindingStatusStampsAndClearsResolvedAt(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-12")
	_, findings, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-12",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	eventBus.ClearEvents()

	resolved, err := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingResolved)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Status != models.ReviewFindingResolved || resolved.ResolvedAt == nil {
		t.Fatalf("expected resolved with timestamp, got %+v", resolved)
	}
	if countEvents(eventBus.GetPublishedEvents(), events.TaskReviewFindingUpdated) != 1 {
		t.Fatalf("expected a finding_updated event, got %v", eventTypes(eventBus.GetPublishedEvents()))
	}

	dismissed, err := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingDismissed)
	if err != nil {
		t.Fatalf("dismiss: %v", err)
	}
	if dismissed.ResolvedAt == nil {
		t.Fatal("dismiss should also stamp resolved_at")
	}

	reopened, err := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingOpen)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Status != models.ReviewFindingOpen || reopened.ResolvedAt != nil {
		t.Fatalf("reopen must clear resolved_at, got %+v", reopened)
	}
}

func TestReviewService_UpdateFindingStatusRejectsUnknownStatus(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-13")
	_, findings, _ := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-13",
		Findings: []ReviewFindingInput{validFindingInput()},
	})

	if _, err := svc.UpdateFindingStatus(ctx, findings[0].ID, models.ReviewFindingStatus("archived")); !errors.Is(err, ErrInvalidReviewFinding) {
		t.Fatalf("expected ErrInvalidReviewFinding, got %v", err)
	}
	if _, err := svc.UpdateFindingStatus(ctx, "", models.ReviewFindingOpen); !errors.Is(err, ErrReviewFindingNotFound) {
		t.Fatalf("expected ErrReviewFindingNotFound for an empty id, got %v", err)
	}
	if _, err := svc.UpdateFindingStatus(ctx, "missing", models.ReviewFindingOpen); !errors.Is(err, ErrReviewFindingNotFound) {
		t.Fatalf("expected ErrReviewFindingNotFound, got %v", err)
	}
}

func TestReviewService_GetTaskReviewReturnsEmptySlicesNotNil(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-14")

	review, err := svc.GetTaskReview(ctx, "task-14")
	if err != nil {
		t.Fatalf("GetTaskReview: %v", err)
	}
	if review.Runs == nil || review.Findings == nil {
		t.Fatalf("expected empty slices rather than nil, got %+v", review)
	}
	if len(review.Runs) != 0 || len(review.Findings) != 0 {
		t.Fatalf("expected an empty review, got %+v", review)
	}
}

func TestReviewService_GetTaskReviewReturnsRunsAndFindings(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-15")
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-15",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	review, err := svc.GetTaskReview(ctx, "task-15")
	if err != nil {
		t.Fatalf("GetTaskReview: %v", err)
	}
	if len(review.Runs) != 1 || len(review.Findings) != 1 {
		t.Fatalf("expected one run and one finding, got %d/%d", len(review.Runs), len(review.Findings))
	}
	if _, err := svc.GetTaskReview(ctx, ""); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestReviewService_ClearTaskReview(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-16")
	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-16",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	eventBus.ClearEvents()

	if err := svc.ClearTaskReview(ctx, "task-16"); err != nil {
		t.Fatalf("ClearTaskReview: %v", err)
	}
	review, err := svc.GetTaskReview(ctx, "task-16")
	if err != nil {
		t.Fatalf("GetTaskReview: %v", err)
	}
	if len(review.Runs) != 0 || len(review.Findings) != 0 {
		t.Fatalf("expected review cleared, got %+v", review)
	}
	if countEvents(eventBus.GetPublishedEvents(), events.TaskReviewCleared) != 1 {
		t.Fatalf("expected a cleared event, got %v", eventTypes(eventBus.GetPublishedEvents()))
	}
	if err := svc.ClearTaskReview(ctx, ""); !errors.Is(err, ErrTaskIDRequired) {
		t.Fatalf("expected ErrTaskIDRequired, got %v", err)
	}
}

func TestReviewService_NilEventBusIsSafe(t *testing.T) {
	_, _, repo := createTestReviewService(t)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	svc := NewReviewService(repo, nil, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-17")

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-17",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("publishing without an event bus must still persist, got %v", err)
	}
}

// TestReviewService_CompleteDoesNotResurrectCancelledRun covers the direction the
// original suite missed: cancel-over-terminal was tested, but complete-over-
// cancelled was not, so a pass whose inference finished after the user cancelled
// silently flipped back to completed.
func TestReviewService_CompleteDoesNotResurrectCancelledRun(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-cancel-race")
	run, err := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-cancel-race"})
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if _, err := svc.MarkRunRunning(ctx, run.ID); err != nil {
		t.Fatalf("MarkRunRunning: %v", err)
	}
	if _, err := svc.CancelRun(ctx, run.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	got, err := svc.CompleteRun(ctx, CompleteRunRequest{RunID: run.ID, FindingCount: 3})
	if err != nil {
		t.Fatalf("CompleteRun after cancel should be a no-op, got %v", err)
	}
	if got.Status != models.ReviewRunCancelled {
		t.Fatalf("a cancelled run must stay cancelled, got %q", got.Status)
	}
	if got.FindingCount != 0 {
		t.Fatalf("a cancelled run must not take the completion's counts, got %d", got.FindingCount)
	}
}

func TestReviewService_FailDoesNotResurrectCancelledRun(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-fail-race")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-fail-race"})
	if _, err := svc.CancelRun(ctx, run.ID); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}

	got, err := svc.FailRun(ctx, run.ID, "review_execution_failed", "late failure", 10)
	if err != nil {
		t.Fatalf("FailRun after cancel should be a no-op, got %v", err)
	}
	if got.Status != models.ReviewRunCancelled {
		t.Fatalf("a cancelled run must stay cancelled, got %q", got.Status)
	}
}

func TestReviewService_TerminalGuardStillAllowsLiveTransitions(t *testing.T) {
	// The guard must not block the normal path.
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-live")
	run, _ := svc.CreateRun(ctx, CreateRunRequest{TaskID: "task-live"})
	if _, err := svc.MarkRunRunning(ctx, run.ID); err != nil {
		t.Fatalf("MarkRunRunning: %v", err)
	}
	got, err := svc.CompleteRun(ctx, CompleteRunRequest{RunID: run.ID, FindingCount: 2})
	if err != nil {
		t.Fatalf("CompleteRun: %v", err)
	}
	if got.Status != models.ReviewRunCompleted || got.FindingCount != 2 {
		t.Fatalf("expected a normal completion, got %+v", got)
	}
}

func TestReviewService_PublishReportsSupersededIDs(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-superseded")

	_, first, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-superseded",
		Findings: []ReviewFindingInput{validFindingInput()},
	})
	if err != nil {
		t.Fatalf("first publish: %v", err)
	}
	eventBus.ClearEvents()

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-superseded",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("second publish: %v", err)
	}

	// A connected client holds the old finding in memory; the event must name the
	// id it should drop, or the panel shows both at one anchor until a reload.
	var supersededIDs []string
	for _, e := range eventBus.GetPublishedEvents() {
		if e.Type != events.TaskReviewFindingsPublished {
			continue
		}
		payload, ok := e.Data.(map[string]any)
		if !ok {
			t.Fatalf("unexpected payload type %T", e.Data)
		}
		ids, ok := payload["superseded_ids"].([]string)
		if !ok {
			t.Fatalf("expected superseded_ids on the payload, got %#v", payload["superseded_ids"])
		}
		supersededIDs = ids
	}
	if len(supersededIDs) != 1 || supersededIDs[0] != first[0].ID {
		t.Fatalf("expected the first finding's id reported as superseded, got %v", supersededIDs)
	}
}

func TestReviewService_PublishReportsEmptySupersededList(t *testing.T) {
	svc, eventBus, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-nosupersede")

	if _, _, err := svc.PublishFindings(ctx, PublishFindingsRequest{
		TaskID:   "task-nosupersede",
		Findings: []ReviewFindingInput{validFindingInput()},
	}); err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}
	for _, e := range eventBus.GetPublishedEvents() {
		if e.Type != events.TaskReviewFindingsPublished {
			continue
		}
		payload := e.Data.(map[string]any)
		ids, ok := payload["superseded_ids"].([]string)
		if !ok || len(ids) != 0 {
			t.Fatalf("expected an empty (not nil) superseded list, got %#v", payload["superseded_ids"])
		}
	}
}

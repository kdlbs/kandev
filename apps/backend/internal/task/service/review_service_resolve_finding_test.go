package service

// Coverage for ReviewService.ResolveFinding (REQ-TWS-004): own-task
// authorization, unified not-found for unknown vs unreachable findings, and
// the same-status idempotency rule shared with UpdateFindingStatus.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
)

// reviewFindingWriteErrorRepo forces UpdateTaskReviewFindingStatus to fail,
// isolating the write-failure path from the read-failure path.
type reviewFindingWriteErrorRepo struct {
	*sqliterepo.Repository
	err error
}

func (r *reviewFindingWriteErrorRepo) UpdateTaskReviewFindingStatus(_ context.Context, _ string, _ models.ReviewFindingStatus, _ *time.Time) error {
	return r.err
}

// reviewFindingReadBackErrorRepo lets the write succeed but forces the
// read-back (GetTaskReviewFinding) called after the write to fail.
type reviewFindingReadBackErrorRepo struct {
	*sqliterepo.Repository
	failAfter int
	calls     int
	err       error
}

func (r *reviewFindingReadBackErrorRepo) GetTaskReviewFinding(ctx context.Context, findingID string) (*models.TaskReviewFinding, error) {
	r.calls++
	if r.calls > r.failAfter {
		return nil, r.err
	}
	return r.Repository.GetTaskReviewFinding(ctx, findingID)
}

func TestReviewService_ResolveFinding_RequiresFindingID(t *testing.T) {
	svc, _, _ := createTestReviewService(t)
	_, err := svc.ResolveFinding(context.Background(), ResolveFindingRequest{Status: models.ReviewFindingResolved})
	if !errors.Is(err, ErrReviewFindingNotFound) {
		t.Fatalf("expected ErrReviewFindingNotFound, got %v", err)
	}
}

func TestReviewService_ResolveFinding_RejectsInvalidStatus(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-badstatus")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-badstatus", Findings: []ReviewFindingInput{validFindingInput()}})

	_, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: "archived"})
	if !errors.Is(err, ErrInvalidReviewFinding) {
		t.Fatalf("expected ErrInvalidReviewFinding, got %v", err)
	}
}

func TestReviewService_ResolveFinding_UnknownFindingNotFound(t *testing.T) {
	svc, _, _ := createTestReviewService(t)
	_, err := svc.ResolveFinding(context.Background(), ResolveFindingRequest{FindingID: "does-not-exist", Status: models.ReviewFindingResolved})
	if !errors.Is(err, ErrReviewFindingNotFound) {
		t.Fatalf("expected ErrReviewFindingNotFound, got %v", err)
	}
}

func TestReviewService_ResolveFinding_UnreachableTaskGetsSameNotFoundAsUnknown(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-unreachable")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-unreachable", Findings: []ReviewFindingInput{validFindingInput()}})

	svc.SetTaskAuthorizer(func(_ context.Context, taskID string) error {
		if taskID == "task-rf-unreachable" {
			return errors.New("not visible")
		}
		return nil
	})

	_, unknownErr := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: "does-not-exist", Status: models.ReviewFindingResolved})
	_, unreachableErr := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})

	if unknownErr == nil || unreachableErr == nil {
		t.Fatalf("expected errors from both, got %v / %v", unknownErr, unreachableErr)
	}
	if !errors.Is(unknownErr, ErrReviewFindingNotFound) || !errors.Is(unreachableErr, ErrReviewFindingNotFound) {
		t.Fatalf("expected both to be ErrReviewFindingNotFound, got %v / %v", unknownErr, unreachableErr)
	}
	if unknownErr.Error() != unreachableErr.Error() {
		t.Fatalf("expected identical not-found messages, got %q vs %q", unknownErr.Error(), unreachableErr.Error())
	}
}

func TestReviewService_ResolveFinding_ResolvesAndStampsResolvedAt(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-resolve")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-resolve", Findings: []ReviewFindingInput{validFindingInput()}})

	got, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}
	if got.Status != models.ReviewFindingResolved || got.ResolvedAt == nil {
		t.Fatalf("expected resolved with a timestamp, got %+v", got)
	}
}

func TestReviewService_ResolveFinding_DismissStampsResolvedAt(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-dismiss")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-dismiss", Findings: []ReviewFindingInput{validFindingInput()}})

	got, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingDismissed})
	if err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}
	if got.Status != models.ReviewFindingDismissed || got.ResolvedAt == nil {
		t.Fatalf("expected dismissed with a timestamp, got %+v", got)
	}
}

func TestReviewService_ResolveFinding_ReturnToOpenClearsResolvedAt(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-reopen")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-reopen", Findings: []ReviewFindingInput{validFindingInput()}})
	if _, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved}); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	got, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingOpen})
	if err != nil {
		t.Fatalf("ResolveFinding: %v", err)
	}
	if got.Status != models.ReviewFindingOpen || got.ResolvedAt != nil {
		t.Fatalf("expected open with no timestamp, got %+v", got)
	}
}

func TestReviewService_ResolveFinding_SameStatusResubmitLeavesResolvedAtUnchanged(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-idempotent")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-idempotent", Findings: []ReviewFindingInput{validFindingInput()}})

	first, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := svc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}
	if !first.ResolvedAt.Equal(*second.ResolvedAt) {
		t.Fatalf("expected resolved_at unchanged on same-status resubmit, got %v then %v", first.ResolvedAt, second.ResolvedAt)
	}
}

func TestReviewService_ResolveFinding_ReadFailureIsAnError(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	_, _, repo := createTestReviewService(t)
	svc := NewReviewService(repo, nil, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-readfail")
	_, published, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-readfail", Findings: []ReviewFindingInput{validFindingInput()}})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	failing := &reviewFindingReadBackErrorRepo{Repository: repo, failAfter: 0, err: errors.New("disk full")}
	failingSvc := NewReviewService(failing, nil, log)

	_, err = failingSvc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err == nil {
		t.Fatal("expected an error from the authorization/idempotency read, got nil")
	}
	if errors.Is(err, ErrReviewFindingNotFound) {
		t.Fatalf("a persistence read failure must not be reported as not-found, got %v", err)
	}
}

func TestReviewService_ResolveFinding_WriteFailureIsAnError(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	_, _, repo := createTestReviewService(t)
	svc := NewReviewService(repo, nil, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-writefail")
	_, published, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-writefail", Findings: []ReviewFindingInput{validFindingInput()}})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	failing := &reviewFindingWriteErrorRepo{Repository: repo, err: errors.New("disk full")}
	failingSvc := NewReviewService(failing, nil, log)

	_, err = failingSvc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err == nil {
		t.Fatal("expected an error from the write, got nil")
	}
}

func TestReviewService_ResolveFinding_ReadBackFailureIsAnError(t *testing.T) {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json", OutputPath: "stdout"})
	_, _, repo := createTestReviewService(t)
	svc := NewReviewService(repo, nil, log)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-rf-readback")
	_, published, err := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-rf-readback", Findings: []ReviewFindingInput{validFindingInput()}})
	if err != nil {
		t.Fatalf("PublishFindings: %v", err)
	}

	// Allow the first GetTaskReviewFinding (authorization/idempotency read) to
	// succeed, but fail the second (post-write read-back).
	failing := &reviewFindingReadBackErrorRepo{Repository: repo, failAfter: 1, err: errors.New("disk full")}
	failingSvc := NewReviewService(failing, nil, log)

	_, err = failingSvc.ResolveFinding(ctx, ResolveFindingRequest{FindingID: published[0].ID, Status: models.ReviewFindingResolved})
	if err == nil {
		t.Fatal("expected an error from the read-back, got nil")
	}
}

// The shared idempotency rule must also hold for the pre-existing
// UpdateFindingStatus path (used by the UI action), confirming the refactor
// did not change its externally-visible behavior.
func TestReviewService_UpdateFindingStatus_SameStatusResubmitLeavesResolvedAtUnchanged(t *testing.T) {
	svc, _, repo := createTestReviewService(t)
	ctx := context.Background()
	seedTask(t, ctx, repo, "task-ufs-idempotent")
	_, published, _ := svc.PublishFindings(ctx, PublishFindingsRequest{TaskID: "task-ufs-idempotent", Findings: []ReviewFindingInput{validFindingInput()}})

	first, err := svc.UpdateFindingStatus(ctx, published[0].ID, models.ReviewFindingResolved)
	if err != nil {
		t.Fatalf("first update: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := svc.UpdateFindingStatus(ctx, published[0].ID, models.ReviewFindingResolved)
	if err != nil {
		t.Fatalf("second update: %v", err)
	}
	if !first.ResolvedAt.Equal(*second.ResolvedAt) {
		t.Fatalf("expected resolved_at unchanged on same-status resubmit, got %v then %v", first.ResolvedAt, second.ResolvedAt)
	}
}

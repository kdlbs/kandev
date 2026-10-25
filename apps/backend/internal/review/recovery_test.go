package review

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// recoveryFaultStore injects failures at the runner's existing store boundary.
// Successful calls still use the real review service and SQL repository.
type recoveryFaultStore struct {
	Store
	active func(context.Context, string) (*models.TaskReviewRun, error)
	fail   func(context.Context, string, string, string, int) (*models.TaskReviewRun, error)
}

func (s recoveryFaultStore) ActiveRun(ctx context.Context, taskID string) (*models.TaskReviewRun, error) {
	if s.active != nil {
		return s.active(ctx, taskID)
	}
	return s.Store.ActiveRun(ctx, taskID)
}

func (s recoveryFaultStore) FailRun(ctx context.Context, id, code, message string, duration int) (*models.TaskReviewRun, error) {
	if s.fail != nil {
		return s.fail(ctx, id, code, message, duration)
	}
	return s.Store.FailRun(ctx, id, code, message, duration)
}

func TestRunner_RecoveryErrorsStopAdmissionAndReleaseClaim(t *testing.T) {
	for _, fault := range []string{"read", "settlement", "nil settlement", "live settlement", "terminal active", "read after settlement", "stale active"} {
		t.Run(fault, func(t *testing.T) {
			h, repo, service := newRecoverySQLHarness(t)
			orphan := seedRecoveryRun(t, repo, "orphan", models.ReviewTriggerManual, models.ReviewRunRunning, 1)
			store := recoveryFaultStore{Store: service}
			injected := errors.New("storage unavailable")
			switch fault {
			case "read":
				store.active = func(context.Context, string) (*models.TaskReviewRun, error) { return nil, injected }
			case "settlement":
				store.fail = func(context.Context, string, string, string, int) (*models.TaskReviewRun, error) {
					return nil, injected
				}
			case "nil settlement", "live settlement":
				store.fail = func(context.Context, string, string, string, int) (*models.TaskReviewRun, error) {
					if fault == "nil settlement" {
						return nil, nil
					}
					return orphan, nil
				}
			case "terminal active":
				store.active = func(context.Context, string) (*models.TaskReviewRun, error) {
					return &models.TaskReviewRun{ID: orphan.ID, Status: models.ReviewRunFailed}, nil
				}
			case "read after settlement", "stale active":
				reads := 0
				store.active = func(ctx context.Context, taskID string) (*models.TaskReviewRun, error) {
					reads++
					if reads > 1 && fault == "read after settlement" {
						return nil, injected
					}
					return repo.GetTaskReviewRun(ctx, orphan.ID)
				}
			}
			h.runner.store = store
			got, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-1"})
			if err == nil || got != nil {
				t.Fatalf("admission must stop on %s: run=%+v err=%v", fault, got, err)
			}
			if (fault == "read" || fault == "settlement" || fault == "read after settlement") && !errors.Is(err, injected) {
				t.Fatalf("storage cause lost: %v", err)
			}
			assertRecoveryCounts(t, h, repo, 1, 0)
			// The same eligible request succeeds once storage recovers. This also
			// proves that the failed launch released its task claim.
			h.runner.store = service
			fresh, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual})
			if err != nil || fresh == nil || fresh.ID == orphan.ID {
				t.Fatalf("claim was not released after %s: run=%+v err=%v", fault, fresh, err)
			}
			assertRecoveryCounts(t, h, repo, 2, 1)
		})
	}
}

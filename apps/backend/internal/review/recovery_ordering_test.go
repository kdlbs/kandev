package review

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type recoveryCountingInference struct {
	Inference
	calls atomic.Int32
}

func (i *recoveryCountingInference) Run(ctx context.Context, identity ReviewerIdentity, sessionID, prompt string) (*PromptResult, error) {
	i.calls.Add(1)
	return i.Inference.Run(ctx, identity, sessionID, prompt)
}

func TestRunner_RecoveryLiveWorkerRejoinsWithoutDuplicateInference(t *testing.T) {
	h, repo, _ := newRecoverySQLHarness(t)
	blocked := &blockingInference{started: make(chan struct{}), release: make(chan struct{}), response: okResponse("a.go", 2)}
	inference := &recoveryCountingInference{Inference: blocked}
	h.runner.inference = inference
	t.Cleanup(func() { close(blocked.release) })
	run, done, err := h.runner.launch(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual})
	if err != nil {
		t.Fatal(err)
	}
	awaitRecovery(t, blocked.started)
	for range 3 {
		got, err := h.runner.Launch(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual})
		if err != nil || got == nil || got.ID != run.ID || got.Status != models.ReviewRunRunning {
			t.Fatalf("live worker must rejoin: %+v err=%v", got, err)
		}
	}
	if inference.calls.Load() != 1 {
		t.Fatalf("duplicate inference: %d", inference.calls.Load())
	}
	// Inference must leave both cancellation settlement and other task claims
	// available, even while the provider has not returned.
	if !h.runner.settlementMu.TryLock() {
		t.Fatal("inference holds the settlement mutex")
	}
	h.runner.settlementMu.Unlock()
	if !h.runner.claim("independent-task") {
		t.Fatal("inference blocks another task's claim")
	}
	h.runner.release("independent-task")
	ack, err := h.runner.Cancel(context.Background(), run.ID)
	if err != nil || ack == nil || ack.Status != models.ReviewRunCancelled {
		t.Fatalf("cancel live worker: %+v err=%v", ack, err)
	}
	awaitRecovery(t, done)
	stored, err := repo.GetTaskReviewRun(context.Background(), run.ID)
	if err != nil || stored.Status != ack.Status {
		t.Fatalf("cancel acknowledgement changed after worker exit: %+v err=%v", stored, err)
	}
	findings, err := repo.ListTaskReviewFindings(context.Background(), "task-1")
	if err != nil || len(findings) != 0 {
		t.Fatalf("cancelled inference published findings: %+v err=%v", findings, err)
	}
}

// recoveryWriteGate pauses the real service after its reads and before its SQL
// UPDATE. It retains the actual repository and transaction behavior.
type recoveryWriteGate struct {
	*sqliterepo.Repository
	status  models.ReviewRunStatus
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (g *recoveryWriteGate) UpdateTaskReviewRun(ctx context.Context, run *models.TaskReviewRun) error {
	if run.ID == "orphan" && run.Status == g.status {
		g.once.Do(func() {
			close(g.entered)
			<-g.release
		})
	}
	return g.Repository.UpdateTaskReviewRun(ctx, run)
}

type recoveryResult struct {
	run *models.TaskReviewRun
	err error
}

func awaitRecovery(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("review operation did not finish")
	}
}

func TestRunner_RecoveryCancellationAcknowledgementSurvivesSQLSettlement(t *testing.T) {
	for _, first := range []models.ReviewRunStatus{models.ReviewRunFailed, models.ReviewRunCancelled} {
		t.Run(string(first)+"-first", func(t *testing.T) {
			h, repo, _ := newRecoverySQLHarness(t)
			seedRecoveryRun(t, repo, "orphan", models.ReviewTriggerManual, models.ReviewRunRunning, 1)
			gate := &recoveryWriteGate{Repository: repo, status: first, entered: make(chan struct{}), release: make(chan struct{})}
			var release sync.Once
			unblock := func() { release.Do(func() { close(gate.release) }) }
			t.Cleanup(unblock)
			h.runner.store = taskservice.NewReviewService(gate, nil, testLogger(t))
			launch, cancel := make(chan recoveryResult, 1), make(chan recoveryResult, 1)
			startLaunch := func() {
				go func() {
					run, err := h.runner.Run(context.Background(), RunRequest{TaskID: "task-1", Trigger: models.ReviewTriggerManual})
					launch <- recoveryResult{run, err}
				}()
			}
			startCancel := func() {
				go func() {
					run, err := h.runner.Cancel(context.Background(), "orphan")
					cancel <- recoveryResult{run, err}
				}()
			}
			if first == models.ReviewRunFailed {
				startLaunch()
			} else {
				startCancel()
			}
			awaitRecovery(t, gate.entered)
			// The service's terminal guard and SQL write are separate operations.
			// This whole interval must exclude the competing terminal transition.
			if h.runner.settlementMu.TryLock() {
				h.runner.settlementMu.Unlock()
				t.Error("SQL terminal transition is not serialized with its competitor")
			}
			if first == models.ReviewRunFailed {
				startCancel()
			} else {
				startLaunch()
			}
			unblock()
			admitted, ack := <-launch, <-cancel
			if admitted.err != nil || admitted.run == nil || admitted.run.ID == "orphan" || ack.err != nil || ack.run == nil {
				t.Fatalf("launch=%+v cancel=%+v", admitted, ack)
			}
			stored, err := repo.GetTaskReviewRun(context.Background(), "orphan")
			if err != nil || stored.Status != first || stored.Status != ack.run.Status {
				t.Fatalf("acknowledged terminal state changed: stored=%+v ack=%+v err=%v", stored, ack.run, err)
			}
			assertRecoveryCounts(t, h, repo, 2, 1)
		})
	}
}

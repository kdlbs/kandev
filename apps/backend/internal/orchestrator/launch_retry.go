package orchestrator

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	// transientLaunchRetryDelay gives a transiently unavailable runtime
	// (agentctl readiness, a stalled workspace) time to settle before the one
	// automatic retry.
	transientLaunchRetryDelay = 2 * time.Second
	// transientLaunchRetryTimeout bounds the retry launch itself.
	transientLaunchRetryTimeout = 5 * time.Minute
	// transientLaunchRetryTTL is how long a consumed retry marker blocks another
	// automatic attempt for the session. It outlives any launch the retry
	// starts, so a retried launch that fails again is never retried twice, while
	// a transient failure hours later can still self-heal.
	transientLaunchRetryTTL = time.Hour
)

// transientLaunchFailureText lists the launch-stage failures a single
// automatic retry can plausibly clear. The list is deliberately narrow: a real
// configuration or workspace error must keep the task FAILED.
var transientLaunchFailureText = []string{
	"agentctl not ready",
	"context deadline exceeded",
	"failed to launch workspace",
	"workspace execution is not ready",
}

func isTransientLaunchFailure(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	message := strings.ToLower(err.Error())
	for _, text := range transientLaunchFailureText {
		if strings.Contains(message, text) {
			return true
		}
	}
	return false
}

// scheduleTransientLaunchRetry relaunches the failed session once, with the
// session's own (current step) profile, after a transient launch failure left
// the task FAILED. A second failure keeps FAILED and is logged.
func (s *Service) scheduleTransientLaunchRetry(ctx context.Context, taskID, sessionID string, launchErr error) {
	if !isTransientLaunchFailure(launchErr) {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.IsFromOffice || models.IsAutomationTaskOrigin(task.Origin) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.State != models.TaskSessionStateFailed {
		return
	}
	if _, parked := models.LoadWorkflowParking(session.Metadata); parked {
		return
	}
	if !s.claimTransientLaunchRetry(sessionID) {
		s.logger.Warn("automatic launch retry already attempted; keeping the task FAILED",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(launchErr))
		return
	}
	workerCtx, ok := s.beginTransientLaunchRetryWorker()
	if !ok {
		return
	}
	s.logger.Info("scheduling one automatic retry after a transient launch failure",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Error(launchErr))
	go func() {
		defer s.launchRetries.Done()
		s.runTransientLaunchRetry(workerCtx, taskID, sessionID)
	}()
}

// runTransientLaunchRetry waits out the retry delay, re-reads the task and
// session, and relaunches only while both are still FAILED.
func (s *Service) runTransientLaunchRetry(workerCtx context.Context, taskID, sessionID string) {
	delay := transientLaunchRetryDelay
	if s.launchRetryDelay > 0 {
		delay = s.launchRetryDelay
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-workerCtx.Done():
		return
	case <-timer.C:
	}
	retryCtx, cancel := context.WithTimeout(workerCtx, transientLaunchRetryTimeout)
	defer cancel()
	currentTask, taskErr := s.repo.GetTask(retryCtx, taskID)
	if taskErr != nil || currentTask == nil || currentTask.State != v1.TaskStateFailed {
		return
	}
	currentSession, sessionErr := s.repo.GetTaskSession(retryCtx, sessionID)
	if sessionErr != nil || currentSession == nil || currentSession.State != models.TaskSessionStateFailed {
		return
	}
	if retryErr := s.launchRetrySession(retryCtx, taskID, sessionID); retryErr != nil {
		s.logger.Warn("automatic launch retry failed; the task stays FAILED",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(retryErr))
		return
	}
	// The marker stays until its TTL: a retried launch that fails later on
	// must not earn another automatic attempt.
	s.logger.Info("automatic launch retry started the session",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID))
}

// beginTransientLaunchRetryWorker registers one retry worker under the
// service-owned retry context. Stop cancels that context and waits for the
// workers, so a pending retry never outlives the service.
func (s *Service) beginTransientLaunchRetryWorker() (context.Context, bool) {
	s.launchRetryMu.Lock()
	defer s.launchRetryMu.Unlock()
	if s.launchRetryStopped {
		return nil, false
	}
	if s.launchRetryCtx == nil {
		s.launchRetryCtx, s.launchRetryCancel = context.WithCancel(context.Background())
	}
	s.launchRetries.Add(1)
	return s.launchRetryCtx, true
}

// stopTransientLaunchRetries cancels pending automatic retries and waits for
// their workers, bounded so shutdown cannot hang on a slow relaunch.
func (s *Service) stopTransientLaunchRetries() {
	s.launchRetryMu.Lock()
	s.launchRetryStopped = true
	cancel := s.launchRetryCancel
	s.launchRetryMu.Unlock()
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		s.launchRetries.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(sendNowClaimRecoveryTimeout):
		s.logger.Warn("timed out waiting for automatic launch retries during shutdown")
	}
}

// claimTransientLaunchRetry records the one automatic attempt per session and
// launch. A marker older than the TTL is replaced, so a session that fails
// transiently again long after the first attempt still gets its retry. Expired
// markers of other sessions are dropped on the way, which keeps the registry
// bounded.
func (s *Service) claimTransientLaunchRetry(sessionID string) bool {
	now := time.Now()
	s.sweepExpiredTransientLaunchRetries(now)
	prev, loaded := s.transientLaunchRetries.LoadOrStore(sessionID, now)
	if !loaded {
		return true
	}
	attemptedAt, _ := prev.(time.Time)
	if now.Sub(attemptedAt) < transientLaunchRetryTTL {
		return false
	}
	return s.transientLaunchRetries.CompareAndSwap(sessionID, prev, now)
}

func (s *Service) sweepExpiredTransientLaunchRetries(now time.Time) {
	s.transientLaunchRetries.Range(func(key, value any) bool {
		if attemptedAt, ok := value.(time.Time); !ok || now.Sub(attemptedAt) >= transientLaunchRetryTTL {
			s.transientLaunchRetries.CompareAndDelete(key, value)
		}
		return true
	})
}

func (s *Service) launchRetrySession(ctx context.Context, taskID, sessionID string) error {
	if s.launchRetry != nil {
		return s.launchRetry(ctx, taskID, sessionID)
	}
	// runtime_retry resumes sessions that hold a resume token and only falls
	// back to a fresh start for first-launch failures, so the automatic retry
	// never discards conversation history.
	_, err := s.RecoverSession(ctx, taskID, sessionID, recoveryActionRuntimeRetry)
	return err
}

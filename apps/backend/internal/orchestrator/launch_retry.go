package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type transientLaunchFailureIdentity struct {
	attemptID  string
	errorStamp string
	updatedAt  time.Time
}

type transientLaunchRetryContextKey struct{}

func transientLaunchIdentity(session *models.TaskSession) transientLaunchFailureIdentity {
	if session == nil {
		return transientLaunchFailureIdentity{}
	}
	identity := transientLaunchFailureIdentity{
		attemptID: models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]),
		updatedAt: session.UpdatedAt,
	}
	if lastError, ok := models.LoadLastAgentError(session.Metadata); ok {
		identity.errorStamp = lastError.Stamp()
	}
	return identity
}

func (identity transientLaunchFailureIdentity) matches(session *models.TaskSession) bool {
	if session == nil || session.State != models.TaskSessionStateFailed ||
		!session.UpdatedAt.Equal(identity.updatedAt) ||
		models.StringFromAny(session.Metadata[models.SessionMetaKeyAgentStartAttemptID]) != identity.attemptID {
		return false
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok {
		return identity.errorStamp == ""
	}
	return lastError.Stamp() == identity.errorStamp
}

func withTransientLaunchRetryContext(ctx context.Context, identity transientLaunchFailureIdentity) context.Context {
	ctx = context.WithValue(ctx, transientLaunchRetryContextKey{}, true)
	return context.WithValue(ctx, transientLaunchFailureIdentityContextKey{}, identity)
}

type transientLaunchFailureIdentityContextKey struct{}

var errTransientLaunchRetrySuperseded = errors.New("transient launch retry failure was replaced")

func transientLaunchFailureFromContext(ctx context.Context) (transientLaunchFailureIdentity, bool) {
	identity, ok := ctx.Value(transientLaunchFailureIdentityContextKey{}).(transientLaunchFailureIdentity)
	return identity, ok
}

func isTransientLaunchRetryContext(ctx context.Context) bool {
	owned, _ := ctx.Value(transientLaunchRetryContextKey{}).(bool)
	return owned
}

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
	message := strings.ToLower(err.Error())
	// A cancelled launch was stopped on purpose, for example by the user, even
	// when the cancellation surfaced while agentctl was still starting.
	if errors.Is(err, context.Canceled) || strings.Contains(message, context.Canceled.Error()) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
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
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	s.scheduleTransientLaunchRetryForFailure(ctx, taskID, sessionID, launchErr, transientLaunchIdentity(session))
}

func (s *Service) handleRecordedLaunchFailure(
	ctx context.Context,
	taskID, sessionID string,
	launchErr error,
	failureStamp string,
) {
	if failureStamp == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil {
		return
	}
	lastError, ok := models.LoadLastAgentError(session.Metadata)
	if !ok || lastError.Stamp() != failureStamp {
		return
	}
	s.scheduleTransientLaunchRetryForFailure(ctx, taskID, sessionID, launchErr, transientLaunchIdentity(session))
}

func (s *Service) scheduleTransientLaunchRetryForFailure(
	ctx context.Context,
	taskID, sessionID string,
	launchErr error,
	identity transientLaunchFailureIdentity,
) {
	if !isTransientLaunchFailure(launchErr) {
		return
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.State != v1.TaskStateFailed || task.IsFromOffice || models.IsAutomationTaskOrigin(task.Origin) {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID || !identity.matches(session) {
		return
	}
	if _, parked := models.LoadWorkflowParking(session.Metadata); parked {
		return
	}
	workerCtx, workers, ok := s.beginTransientLaunchRetryWorker()
	if !ok {
		return
	}
	if !s.claimTransientLaunchRetry(sessionID) {
		workers.Done()
		s.logger.Warn("automatic launch retry already attempted; keeping the task FAILED",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(launchErr))
		return
	}
	s.logger.Info("scheduling one automatic retry after a transient launch failure",
		zap.String("task_id", taskID),
		zap.String("session_id", sessionID),
		zap.Error(launchErr))
	go func() {
		defer workers.Done()
		s.runTransientLaunchRetry(workerCtx, taskID, sessionID, identity)
	}()
}

// runTransientLaunchRetry waits out the retry delay, re-reads the task and
// session, and relaunches only while both are still FAILED.
func (s *Service) runTransientLaunchRetry(
	workerCtx context.Context,
	taskID, sessionID string,
	identity transientLaunchFailureIdentity,
) {
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
	if sessionErr != nil || !identity.matches(currentSession) {
		return
	}
	if retryErr := s.launchRetrySession(retryCtx, taskID, sessionID, identity); retryErr != nil {
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
func (s *Service) beginTransientLaunchRetryWorker() (context.Context, *sync.WaitGroup, bool) {
	s.launchRetryMu.Lock()
	defer s.launchRetryMu.Unlock()
	if s.launchRetryStopped {
		return nil, nil, false
	}
	if s.launchRetryCtx == nil {
		s.launchRetryCtx, s.launchRetryCancel = context.WithCancel(context.Background())
	}
	if s.launchRetries == nil {
		s.launchRetries = &sync.WaitGroup{}
	}
	s.launchRetries.Add(1)
	return s.launchRetryCtx, s.launchRetries, true
}

// stopTransientLaunchRetries cancels pending automatic retries and waits for
// their workers, bounded so shutdown cannot hang on a slow relaunch.
func (s *Service) stopTransientLaunchRetries() {
	s.awaitTransientLaunchRetries(s.closeTransientLaunchRetries())
}

// closeTransientLaunchRetries refuses new automatic retries and returns the
// current worker generation. Stop calls it under the service mutex, so a racing
// Start resets only after it and its fresh generation stays open.
func (s *Service) closeTransientLaunchRetries() (context.CancelFunc, *sync.WaitGroup) {
	s.launchRetryMu.Lock()
	defer s.launchRetryMu.Unlock()
	s.launchRetryStopped = true
	return s.launchRetryCancel, s.launchRetries
}

// awaitTransientLaunchRetries cancels a closed worker generation and waits for
// its workers, bounded by the shared shutdown timeout.
func (s *Service) awaitTransientLaunchRetries(cancel context.CancelFunc, workers *sync.WaitGroup) {
	if cancel != nil {
		cancel()
	}
	done := make(chan struct{})
	go func() {
		if workers != nil {
			workers.Wait()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(sendNowClaimRecoveryTimeout):
		s.logger.Warn("timed out waiting for automatic launch retries during shutdown")
	}
}

func (s *Service) resetTransientLaunchRetries() {
	s.launchRetryMu.Lock()
	s.launchRetryStopped = false
	s.launchRetryCtx = nil
	s.launchRetryCancel = nil
	s.launchRetries = &sync.WaitGroup{}
	s.launchRetryMu.Unlock()
}

func (s *Service) validateTransientLaunchFailure(
	ctx context.Context,
	taskID, sessionID string,
	identity transientLaunchFailureIdentity,
) error {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.State != v1.TaskStateFailed || task.IsFromOffice || models.IsAutomationTaskOrigin(task.Origin) {
		return errTransientLaunchRetrySuperseded
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID != taskID || !identity.matches(session) {
		return errTransientLaunchRetrySuperseded
	}
	return nil
}

func transientLaunchIdentityFromResumeOptions(options executor.ResumeOptions) (transientLaunchFailureIdentity, bool) {
	if options.TransientRetryUpdatedAt.IsZero() {
		return transientLaunchFailureIdentity{}, false
	}
	return transientLaunchFailureIdentity{
		attemptID:  options.TransientRetryAttemptID,
		errorStamp: options.TransientRetryErrorStamp,
		updatedAt:  options.TransientRetryUpdatedAt,
	}, true
}

func transientLaunchIdentityFromPayload(payload map[string]interface{}) (transientLaunchFailureIdentity, bool, error) {
	rawUpdatedAt, found := payload["transient_retry_updated_at"]
	if !found {
		return transientLaunchFailureIdentity{}, false, nil
	}
	updatedAtText, ok := rawUpdatedAt.(string)
	if !ok {
		return transientLaunchFailureIdentity{}, false, errors.New("invalid transient retry timestamp")
	}
	updatedAt, err := time.Parse(time.RFC3339Nano, updatedAtText)
	if err != nil {
		return transientLaunchFailureIdentity{}, false, err
	}
	return transientLaunchFailureIdentity{
		attemptID:  stringField(payload, "transient_retry_attempt_id"),
		errorStamp: stringField(payload, "transient_retry_error_stamp"),
		updatedAt:  updatedAt,
	}, true, nil
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

func (s *Service) launchRetrySession(
	ctx context.Context,
	taskID, sessionID string,
	identity transientLaunchFailureIdentity,
) error {
	if s.launchRetry != nil {
		return s.launchRetry(ctx, taskID, sessionID)
	}
	// runtime_retry resumes sessions that hold a resume token and only falls
	// back to a fresh start for first-launch failures, so the automatic retry
	// never discards conversation history.
	ctx = withTransientLaunchRetryContext(ctx, identity)
	_, err := s.RecoverSessionWithOptions(ctx, taskID, sessionID, recoveryActionRuntimeRetry, RecoverSessionOptions{
		SettingsPolicy:         executor.ResumeSettingsPolicyStrict,
		Origin:                 string(launchOriginAutomatic),
		transientLaunchFailure: &identity,
	})
	return err
}

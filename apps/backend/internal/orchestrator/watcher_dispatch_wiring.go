package orchestrator

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/github"
)

// serviceTaskStarter adapts Service.StartTask to the coordinator's
// taskStarter interface. Lives in its own file so the wiring stays close
// to the Service definition without polluting watcher_dispatch.go with
// orchestrator-internal types.
type serviceTaskStarter struct{ svc *Service }

func (s serviceTaskStarter) Start(ctx context.Context, taskID, workflowStepID, prompt string, p AutoStartParams) error {
	_, err := s.svc.StartTask(
		ctx, taskID, p.AgentProfileID, "", p.ExecutorProfileID,
		"", prompt, workflowStepID, false, true, nil,
	)
	return err
}

// initWatcherCoordinatorLocked builds the coordinator (once) and (always)
// refreshes the mutable taskCreator dependency via SetTaskCreator. Called
// from SetIssueTaskCreator, which can be invoked multiple times — tests in
// particular may swap creators between scenarios. Re-running the setter MUST
// update the coordinator, otherwise Dispatch silently keeps the original
// creator.
//
// Locked variant: callers MUST hold s.mu (write). Reads s.profileLookup
// directly rather than via getProfileLookup so we don't re-acquire the
// read lock from inside the write-locked critical section.
func (s *Service) initWatcherCoordinatorLocked() {
	if s.watcherCoordinator == nil {
		s.watcherCoordinator = &WatcherDispatchCoordinator{
			startTask: serviceTaskStarter{svc: s},
			shouldAutoStart: func(ctx context.Context, stepID string) bool {
				return s.shouldAutoStartStep(ctx, stepID)
			},
			logger: s.logger,
		}
	}
	s.watcherCoordinator.SetTaskCreator(s.issueTaskCreator)
	if s.profileLookup != nil {
		s.watcherCoordinator.SetProfileLookup(s.profileLookup)
	}
}

// SetProfileLookup wires the soft-deleted-profile pre-flight check into both
// the coordinator-driven Linear/Jira pipeline and the legacy GitHub
// createIssueTask / createReviewTask call sites. Safe to call before or after
// SetIssueTaskCreator — the coordinator picks the value up on its next
// initWatcherCoordinatorLocked pass. Mutex-guarded because bus handlers
// (createIssueTask, createReviewTask) read the field from background
// goroutines and the race detector flags any unsynchronised access.
func (s *Service) SetProfileLookup(p ProfileLookup) {
	s.mu.Lock()
	s.profileLookup = p
	coord := s.watcherCoordinator
	s.mu.Unlock()
	if coord != nil {
		coord.SetProfileLookup(p)
	}
}

// getProfileLookup is the read counterpart to SetProfileLookup. Returns the
// currently-wired ProfileLookup (or nil) under the read lock.
func (s *Service) getProfileLookup() ProfileLookup {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.profileLookup
}

// getWatcherCoordinator / getIssueTaskCreator are the lock-aware reads
// paired with the write paths in SetIssueTaskCreator / SetProfileLookup.
// Used by dispatchWatcherEvent which runs from bus subscriber goroutines.
func (s *Service) getWatcherCoordinator() *WatcherDispatchCoordinator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.watcherCoordinator
}

func (s *Service) getIssueTaskCreator() IssueTaskCreator {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.issueTaskCreator
}

// preflightDeletedProfileForGitHubIssue / ForGitHubReview run the same
// soft-deleted-profile check the coordinator does, for the legacy GitHub
// watcher paths that bypass the coordinator (createIssueTask /
// createReviewTask in event_handlers_github.go). Return true when the
// watcher was self-healed and the caller MUST stop.
func (s *Service) preflightDeletedProfileForGitHubIssue(ctx context.Context, evt *github.NewIssueEvent) bool {
	if evt == nil {
		return false
	}
	return s.preflightDeletedProfileForGitHub(ctx, "issue", evt.AgentProfileID, evt.IssueWatchID, s.disableGitHubIssueWatch)
}

func (s *Service) preflightDeletedProfileForGitHubReview(ctx context.Context, evt *github.NewReviewPREvent) bool {
	if evt == nil {
		return false
	}
	return s.preflightDeletedProfileForGitHub(ctx, "review", evt.AgentProfileID, evt.ReviewWatchID, s.disableGitHubReviewWatch)
}

// preflightDeletedProfileForGitHub is the shared body for the two GitHub
// pre-flights. kind ("issue" / "review") is passed as a zap.String field
// rather than concatenated into the log message so the aggregator can group
// "github watcher: ..." into a single filterable family with kind as an axis.
// disable is the integration-specific store write.
func (s *Service) preflightDeletedProfileForGitHub(
	ctx context.Context, kind, profileID, watchID string,
	disable func(ctx context.Context, watchID, cause string) error,
) bool {
	lookup := s.getProfileLookup()
	if lookup == nil || profileID == "" {
		return false
	}
	deleted, name, err := lookup.LookupProfile(ctx, profileID)
	if err != nil {
		s.logger.Warn("github watcher: profile lookup failed, falling through",
			zap.String("kind", kind),
			zap.String("profile_id", profileID),
			zap.Error(err))
		return false
	}
	if !deleted {
		return false
	}
	cause := formatDeletedProfileCause(profileID, name)
	s.logger.Warn("github watcher: agent profile soft-deleted, self-healing",
		zap.String("kind", kind),
		zap.String("watch_id", watchID),
		zap.String("profile_id", profileID),
		zap.String("profile_name", name))
	if disable != nil {
		if err := disable(ctx, watchID, cause); err != nil {
			s.logger.Error("github watcher: self-heal disable failed",
				zap.String("kind", kind),
				zap.String("watch_id", watchID),
				zap.Error(err))
		}
	}
	return true
}

// disableGitHubIssueWatch / disableGitHubReviewWatch are nil-safe shims
// around the github service's disable methods. nil githubService falls
// through silently — same idiom as reserveIssueWatch / releaseIssueWatch.
func (s *Service) disableGitHubIssueWatch(ctx context.Context, watchID, cause string) error {
	if s.githubService == nil {
		return nil
	}
	return s.githubService.DisableIssueWatchWithError(ctx, watchID, cause)
}

func (s *Service) disableGitHubReviewWatch(ctx context.Context, watchID, cause string) error {
	if s.githubService == nil {
		return nil
	}
	return s.githubService.DisableReviewWatchWithError(ctx, watchID, cause)
}

// dispatchWatcherEvent runs the wiring guards every per-integration bus
// handler shares — issueTaskCreator check and the final goroutine dispatch
// with cancellation detached. integration is used in log message templating
// ("new linear issue ...", "skipping jira task ..."). fields are the
// structured log fields that identify the event in operator logs; pass at
// least the issue_watch_id and an integration-specific identifier field so
// a deferred / dropped event is diagnosable.
//
// Lives in its own helper so per-integration handlers stay below dupl's
// duplicate-block threshold without copy-pasting the same guards.
func (s *Service) dispatchWatcherEvent(ctx context.Context, integration string, src WatcherSource, evt any, fields ...zap.Field) {
	s.logger.Info(fmt.Sprintf("new %s issue detected from watch", integration), fields...)
	if s.getIssueTaskCreator() == nil {
		s.logger.Warn(fmt.Sprintf("issue task creator not configured, skipping %s task creation", integration))
		return
	}
	// Read the coordinator through the RLock accessor (see getWatcherCoordinator):
	// SetIssueTaskCreator writes issueTaskCreator and the coordinator under the
	// same lock, so a concurrent bus event never sees a half-wired Service.
	coord := s.getWatcherCoordinator()
	if coord == nil {
		return
	}

	// Per-watcher throttle. Synchronous: we MUST acquire the slot before
	// spawning the goroutine, otherwise a tight burst of bus events all
	// read the same pre-creation DB count and overshoot the cap. See
	// docs/specs/throttle-watcher-fanout/spec.md.
	//
	// The slot covers the WHOLE dispatch pipeline, including the case where
	// Dispatch returns early because Reserve loses the dedup race (no task
	// created). In that window the pending counter briefly holds a slot that
	// maps to no task, so a poll/`/trigger` collision can momentarily
	// over-throttle a watch by one. The slot is released when the goroutine
	// exits and the DB count is unaffected, so this self-heals on the next
	// tick — acceptable for v1.
	release, ok := s.acquireWatcherSlot(ctx, integration, src.WatchID(evt), src.MaxInflightTasks(evt))
	if !ok {
		return
	}

	// Detach from cancellation but keep request-scoped values (tracing, etc.):
	// the bus delivery context may be cancelled before task creation finishes.
	go func() {
		defer release()
		coord.Dispatch(context.WithoutCancel(ctx), src, evt)
	}()
}

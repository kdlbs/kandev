package github

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/authcircuit"
)

const (
	reviewCleanupCircuitWorkspace = "workspace"
	reviewCleanupCircuitRecord    = "record"
)

type reviewCleanupCycle struct {
	poller                *Poller
	ctx                   context.Context
	watches               map[string]*ReviewWatch
	watchChecked          map[string]bool
	workspaceFingerprints map[string]string
	workspaceFPChecked    map[string]bool
	workspaceSkipRecorded map[string]bool
	coreQuotaSkipRecorded bool
}

func newReviewCleanupCycle(ctx context.Context, poller *Poller) *reviewCleanupCycle {
	return &reviewCleanupCycle{
		poller:                poller,
		ctx:                   ctx,
		watches:               make(map[string]*ReviewWatch),
		watchChecked:          make(map[string]bool),
		workspaceFingerprints: make(map[string]string),
		workspaceFPChecked:    make(map[string]bool),
		workspaceSkipRecorded: make(map[string]bool),
	}
}

func (c *reviewCleanupCycle) registerWatch(watch *ReviewWatch) {
	if watch != nil && watch.ID != "" {
		c.watches[watch.ID] = watch
		c.watchChecked[watch.ID] = true
	}
}

func (c *reviewCleanupCycle) admit(task *ReviewPRTask, tracker *RateTracker) (bool, bool) {
	if task == nil {
		return true, false
	}
	watch := c.reviewWatch(task.ReviewWatchID)
	if watch == nil || watch.WorkspaceID == "" {
		return false, false
	}
	connectionFingerprint := c.workspaceFingerprint(watch.WorkspaceID)
	recordFingerprint := reviewCleanupRecordFingerprint(connectionFingerprint, watch)
	if c.poller.reviewCleanupCircuits.refreshRecordFingerprint(task.ID, recordFingerprint) {
		incReviewCleanupCircuitReset(reviewCleanupCircuitRecord)
	}
	serviceTracker := c.poller.service.rateTracker
	coreExhausted := serviceTracker != nil && serviceTracker.WaitDuration(ResourceCore) > 0
	if !coreExhausted && tracker != nil && tracker != serviceTracker {
		coreExhausted = tracker.WaitDuration(ResourceCore) > 0
	}
	if coreExhausted {
		if !c.coreQuotaSkipRecorded {
			incReviewCleanupCoreQuotaSkip()
			c.coreQuotaSkipRecorded = true
		}
		return false, true
	}
	now := time.Now().UTC()
	if open, class := c.poller.reviewCleanupCircuits.workspaceOpen(watch.WorkspaceID, now); open {
		if !c.workspaceSkipRecorded[watch.WorkspaceID] {
			incReviewCleanupCircuitSkip(reviewCleanupCircuitWorkspace, string(class))
			c.workspaceSkipRecorded[watch.WorkspaceID] = true
		}
		return false, true
	}
	if open, class := c.poller.reviewCleanupCircuits.recordOpen(task.ID, now); open {
		incReviewCleanupCircuitSkip(reviewCleanupCircuitRecord, string(class))
		return false, false
	}
	return true, false
}

func (c *reviewCleanupCycle) apply(
	result scheduledReviewCleanupResult,
	fallbackWorkspace string,
	fallbackWatch *ReviewWatch,
) {
	now := time.Now().UTC()
	for _, success := range result.Successes {
		workspaceID := success.WorkspaceID
		if workspaceID == "" {
			workspaceID = fallbackWorkspace
		}
		fingerprint := c.workspaceFingerprint(workspaceID)
		c.poller.reviewCleanupCircuits.recordWorkspaceOutcome(
			workspaceID, fingerprint, authcircuit.FailureClassNone, now,
		)
		if success.Task != nil {
			watch := c.reviewWatch(success.Task.ReviewWatchID)
			c.poller.reviewCleanupCircuits.recordRecordOutcome(
				success.Task.ID, reviewCleanupRecordFingerprint(fingerprint, watch),
				authcircuit.FailureClassNone, now,
			)
		}
	}
	for _, failure := range result.Failures {
		workspaceID := failure.WorkspaceID
		if workspaceID == "" {
			workspaceID = fallbackWorkspace
		}
		watch := fallbackWatch
		if failure.Task != nil {
			if cached := c.reviewWatch(failure.Task.ReviewWatchID); cached != nil {
				watch = cached
			}
		}
		fingerprint := c.workspaceFingerprint(workspaceID)
		class := classifyPollErr(failure.Err)
		metricClass := reviewCleanupFailureMetricClass(failure.Err, class)
		if failure.RecordScoped && failure.Task != nil {
			incReviewCleanupFailure(reviewCleanupCircuitRecord, metricClass)
			c.poller.reviewCleanupCircuits.recordRecordOutcome(
				failure.Task.ID, reviewCleanupRecordFingerprint(fingerprint, watch), class, now,
			)
		} else {
			incReviewCleanupFailure(reviewCleanupCircuitWorkspace, metricClass)
			c.poller.reviewCleanupCircuits.recordWorkspaceOutcome(workspaceID, fingerprint, class, now)
		}
		c.poller.logger.Debug("scheduled GitHub review cleanup failed",
			zap.String("workspace_id", workspaceID), zap.Error(failure.Err))
	}
}

func (c *reviewCleanupCycle) reviewWatch(watchID string) *ReviewWatch {
	if watchID == "" || c.watchChecked[watchID] {
		return c.watches[watchID]
	}
	c.watchChecked[watchID] = true
	if c.poller.service == nil || c.poller.service.store == nil {
		return nil
	}
	watch, err := c.poller.service.store.GetReviewWatch(c.ctx, watchID)
	if err != nil {
		c.poller.logger.Debug("failed to load review watch for cleanup circuit",
			zap.String("watch_id", watchID), zap.Error(err))
		return nil
	}
	if watch != nil {
		c.watches[watchID] = watch
	}
	return watch
}

func (c *reviewCleanupCycle) workspaceFingerprint(workspaceID string) string {
	if workspaceID == "" || c.workspaceFPChecked[workspaceID] {
		return c.workspaceFingerprints[workspaceID]
	}
	c.workspaceFPChecked[workspaceID] = true
	if c.poller.service == nil {
		return ""
	}
	fingerprint, err := c.poller.service.WorkspaceConnectionFingerprint(c.ctx, workspaceID)
	if err != nil {
		c.poller.logger.Debug("failed to refresh review cleanup connection fingerprint",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return ""
	}
	c.workspaceFingerprints[workspaceID] = fingerprint
	if c.poller.reviewCleanupCircuits.refreshWorkspaceFingerprint(workspaceID, fingerprint) {
		incReviewCleanupCircuitReset(reviewCleanupCircuitWorkspace)
	}
	return fingerprint
}

// reviewCleanupRecordFingerprint follows watch configuration; UpdatedAt and
// LastPolledAt change during routine polling and must not reset a record circuit.
func reviewCleanupRecordFingerprint(connectionFingerprint string, watch *ReviewWatch) string {
	parts := make([]string, 0, 2)
	if connectionFingerprint != "" {
		parts = append(parts, "connection="+connectionFingerprint)
	}
	if watch != nil {
		config, err := json.Marshal(struct {
			WorkspaceID         string
			WorkflowID          string
			WorkflowStepID      string
			Repos               []RepoFilter
			AgentProfileID      string
			ExecutorProfileID   string
			Prompt              string
			ReviewScope         string
			CustomQuery         string
			TargetLogin         string
			Enabled             bool
			PollIntervalSeconds int
			CleanupPolicy       string
		}{
			WorkspaceID:         watch.WorkspaceID,
			WorkflowID:          watch.WorkflowID,
			WorkflowStepID:      watch.WorkflowStepID,
			Repos:               watch.Repos,
			AgentProfileID:      watch.AgentProfileID,
			ExecutorProfileID:   watch.ExecutorProfileID,
			Prompt:              watch.Prompt,
			ReviewScope:         watch.ReviewScope,
			CustomQuery:         watch.CustomQuery,
			TargetLogin:         watch.TargetLogin,
			Enabled:             watch.Enabled,
			PollIntervalSeconds: watch.PollIntervalSeconds,
			CleanupPolicy:       watch.CleanupPolicy,
		})
		if err == nil {
			digest := sha256.Sum256(config)
			parts = append(parts, "watch_config="+hex.EncodeToString(digest[:]))
		}
	}
	return strings.Join(parts, ";")
}

func reviewCleanupFailureMetricClass(err error, class authcircuit.FailureClass) string {
	var apiErr *GitHubAPIError
	if errors.As(err, &apiErr) && isGitHubRateLimitAPIError(apiErr) {
		return "rate_limit"
	}
	return string(class)
}

func (p *Poller) pruneReviewCleanupRecordCircuits(ctx context.Context) {
	if p.service == nil || p.service.store == nil || p.reviewCleanupCircuits == nil {
		return
	}
	rows, err := p.service.store.ListAllReviewPRTasks(ctx)
	if err != nil {
		p.logger.Debug("failed to prune review cleanup record circuits", zap.Error(err))
		return
	}
	active := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		if row != nil && row.ID != "" {
			active[row.ID] = struct{}{}
		}
	}
	p.reviewCleanupCircuits.pruneRecords(active)
}

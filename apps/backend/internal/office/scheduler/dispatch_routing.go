package scheduler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/office/models"
	sqliterepo "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
)

// RouteAttemptOutcomeLaunched is the in-flight outcome string the
// dispatcher writes when an attempt is appended. It is overwritten with
// failed_provider_unavailable / failed_other on classifier verdict.
const (
	RouteAttemptOutcomeLaunched              = "launched"
	RouteAttemptOutcomeFailedProviderUnavail = "failed_provider_unavailable"
	RouteAttemptOutcomeFailedOther           = "failed_other"
	RouteAttemptOutcomeSkippedDegraded       = "skipped_degraded"
	RouteAttemptOutcomeSkippedUserAction     = "skipped_user_action"
	RouteAttemptOutcomeSkippedMissingMapping = "skipped_missing_mapping"
	RouteAttemptOutcomeMaxAttempts           = "skipped_max_attempts"
)

// MaxAttemptsPerRun caps the number of route_attempt rows a single run
// can accumulate. A pathological cycle (provider flapping, post-start
// fallback racing with re-park) could otherwise append rows forever.
// Once the cap is reached the dispatcher parks the run as
// blocked_provider_action_required so a human notices.
const MaxAttemptsPerRun = 20

// DispatchWithRouting attempts the configured providers in order. Three
// outcomes are possible:
//   - launched=true, parked=false → run is in flight; caller skips legacy.
//   - launched=false, parked=false, err=nil → routing disabled / not
//     wired. Caller falls through to the legacy concrete-profile path.
//   - launched=false, parked=true, err=nil → run parked under
//     routing_blocked_status. Caller treats as handled.
//   - launched=false, err!=nil → infrastructure failure; caller invokes
//     existing HandleRunFailure.
//
// launch carries the Office-built prompt/env/profile/workflow context so
// routed launches behave identically to the legacy concrete-profile path
// for everything except the provider/model/mode selection.
func (ss *SchedulerService) DispatchWithRouting(
	ctx context.Context, run *models.Run, agent *models.AgentInstance,
	launch LaunchContext,
) (bool, bool, error) {
	if ss.resolver == nil || ss.taskStarter == nil {
		return false, false, nil
	}
	if agent == nil || run == nil {
		return false, false, fmt.Errorf("dispatch: nil run or agent")
	}
	prior, err := ss.repo.ListRouteAttempts(ctx, run.ID)
	if err != nil {
		return false, false, fmt.Errorf("dispatch: list prior: %w", err)
	}
	if len(prior) >= MaxAttemptsPerRun {
		if perr := ss.parkRunMaxAttempts(ctx, run, agent.WorkspaceID); perr != nil {
			return false, false, perr
		}
		return false, true, nil
	}
	excluded := excludedFromAttempts(prior, run.RouteCycleBaselineSeq)
	res, err := ss.resolver.Resolve(ctx, agent.WorkspaceID, *agent,
		routing.ResolveOptions{
			ExcludeProviders: excluded,
			Reason:           run.Reason,
		})
	if err != nil {
		return false, false, fmt.Errorf("dispatch: resolve: %w", err)
	}
	if !res.Enabled {
		return false, false, nil
	}
	if err := ss.persistRoutingDecision(ctx, run, res); err != nil {
		return false, false, err
	}
	if len(res.Candidates) == 0 {
		_, _, perr := ss.parkRunBlocked(ctx, run, agent.WorkspaceID, res)
		return false, true, perr
	}
	launched, _, terr := ss.tryCandidates(ctx, run, agent, res, launch)
	if terr != nil {
		return false, false, terr
	}
	if launched {
		return true, false, nil
	}
	return false, true, nil
}

// excludedFromAttempts returns the providers that have already failed
// for this run within the current retry cycle. Attempts with seq at or
// below baseline belong to an earlier cycle (a previous park-and-lift
// pass) and are intentionally NOT counted — when the scheduler lifts a
// parked run it bumps the baseline so the run can re-try every provider
// in its order. The dispatcher already loaded the attempts slice once
// (for the MaxAttemptsPerRun cap), so we walk that in-memory slice
// instead of re-querying.
func excludedFromAttempts(prior []models.RouteAttempt, baseline int) []routing.ProviderID {
	excluded := make([]routing.ProviderID, 0, len(prior))
	for _, a := range prior {
		if a.Seq <= baseline {
			continue
		}
		if a.Outcome == RouteAttemptOutcomeFailedProviderUnavail ||
			a.Outcome == RouteAttemptOutcomeFailedOther {
			excluded = append(excluded, routing.ProviderID(a.ProviderID))
		}
	}
	return excluded
}

// parkRunMaxAttempts records a terminal `skipped_max_attempts` row and
// parks the run under blocked_provider_action_required. No earliest-retry
// is set: the cap exists to force human attention, not to schedule yet
// another retry. Operators clear the parked status manually (Retry now
// in the UI) once the underlying cause is fixed.
func (ss *SchedulerService) parkRunMaxAttempts(
	ctx context.Context, run *models.Run, workspaceID string,
) error {
	seq, err := ss.repo.IncrementRouteAttemptSeq(ctx, run.ID)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	attempt := models.RouteAttempt{
		RunID:      run.ID,
		Seq:        seq,
		Outcome:    RouteAttemptOutcomeMaxAttempts,
		ErrorCode:  "max_attempts_exceeded",
		StartedAt:  now,
		FinishedAt: &now,
	}
	if err := ss.repo.AppendRouteAttempt(ctx, &attempt); err != nil {
		return err
	}
	ss.publishRouteAttemptAppended(ctx, run.ID, attempt)
	if err := ss.repo.ParkRunForProviderCapacity(ctx,
		run.ID, routing.StatusBlockedActionRequired, time.Time{}); err != nil {
		return err
	}
	ss.logger.Warn("run parked after route attempts cap",
		zap.String("run_id", run.ID),
		zap.Int("cap", MaxAttemptsPerRun))
	ss.recordRouteParked(workspaceID, run.ID, routing.StatusBlockedActionRequired)
	return nil
}

// persistRoutingDecision snapshots the resolver's effective order + tier
// onto the run row on first dispatch. Subsequent post-start fallbacks
// keep the original snapshot (idempotent overwrite is safe).
func (ss *SchedulerService) persistRoutingDecision(
	ctx context.Context, run *models.Run, res *routing.Resolution,
) error {
	if run.LogicalProviderOrder != nil && *run.LogicalProviderOrder != "" {
		return nil
	}
	orderJSON, err := json.Marshal(res.ProviderOrder)
	if err != nil {
		return fmt.Errorf("dispatch: encode order: %w", err)
	}
	if err := ss.repo.SetRunRoutingDecision(ctx,
		run.ID, string(orderJSON), string(res.RequestedTier)); err != nil {
		return err
	}
	orderStr := string(orderJSON)
	tierStr := string(res.RequestedTier)
	run.LogicalProviderOrder = &orderStr
	run.RequestedTier = &tierStr
	return nil
}

// parkRunBlocked records skip attempts and parks the run with the
// resolver's block reason. Called when len(Candidates)==0.
func (ss *SchedulerService) parkRunBlocked(
	ctx context.Context, run *models.Run, workspaceID string, res *routing.Resolution,
) (bool, *routing.BlockReason, error) {
	br := res.BlockReason
	for _, sk := range res.SkippedDegraded {
		seq, err := ss.repo.IncrementRouteAttemptSeq(ctx, run.ID)
		if err != nil {
			return false, nil, err
		}
		attempt := models.RouteAttempt{
			RunID:      run.ID,
			Seq:        seq,
			ProviderID: string(sk.ProviderID),
			Tier:       string(res.RequestedTier),
			Outcome:    skipOutcome(sk.Reason),
			ErrorCode:  sk.ErrorCode,
			RawExcerpt: sk.RawExcerpt,
			StartedAt:  time.Now().UTC(),
		}
		if err := ss.repo.AppendRouteAttempt(ctx, &attempt); err != nil {
			return false, nil, err
		}
		ss.publishRouteAttemptAppended(ctx, run.ID, attempt)
	}
	if err := ss.repo.ParkRunForProviderCapacity(ctx,
		run.ID, br.Status, br.EarliestRetry); err != nil {
		return false, nil, err
	}
	ss.logger.Info("run parked for provider capacity",
		zap.String("run_id", run.ID),
		zap.String("status", br.Status),
		zap.Time("earliest_retry_at", br.EarliestRetry))
	ss.recordRouteParked(workspaceID, run.ID, br.Status)
	return false, &br, nil
}

func skipOutcome(reason string) models.RouteAttemptOutcome {
	switch reason {
	case routing.SkipReasonDegraded:
		return RouteAttemptOutcomeSkippedDegraded
	case routing.SkipReasonUserAction:
		return RouteAttemptOutcomeSkippedUserAction
	case routing.SkipReasonMissingModelMapping:
		return RouteAttemptOutcomeSkippedMissingMapping
	}
	return RouteAttemptOutcomeSkippedDegraded
}

// tryCandidates walks the resolved candidates in order, launching the
// first that does not classify as a fallback-allowed failure. When all
// candidates exhaust with fallback-allowed errors, the run is parked
// using the same block-reason aggregation as parkRunBlocked.
func (ss *SchedulerService) tryCandidates(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, res *routing.Resolution,
	launch LaunchContext,
) (bool, *routing.BlockReason, error) {
	taskID := extractRunTaskID(run.Payload)
	var prev *routing.Candidate
	for i := range res.Candidates {
		candidate := res.Candidates[i]
		seq, err := ss.recordAttemptStart(ctx, run, candidate, res.RequestedTier)
		if err != nil {
			return false, nil, err
		}
		launchErr := ss.launchCandidate(ctx, taskID, agent.ID, candidate, launch)
		if launchErr == nil {
			if prev != nil {
				// We walked past at least one prior candidate — that's
				// a fallback hop. Recorded once per hop, regardless of
				// whether the prior failure was fatal-skipped or
				// fallback-allowed (both led us here).
				ss.recordRouteFallback(agent.WorkspaceID,
					string(prev.ProviderID), string(candidate.ProviderID),
					"")
			}
			ss.recordRouteAttempt(agent.WorkspaceID,
				string(candidate.ProviderID), metricOutcomeSuccess, "")
			return ss.handleLaunchSuccess(ctx, run, agent, candidate)
		}
		// Emit the failure-side route_attempt before classifying. The
		// outcome label is refined by handleLaunchFailure.
		classified := classifyLaunchError(string(candidate.ProviderID), launchErr)
		outcome := metricOutcomeFallbackErr
		if !classified.FallbackAllowed {
			outcome = metricOutcomeFatalErr
		}
		ss.recordRouteAttempt(agent.WorkspaceID,
			string(candidate.ProviderID), outcome, string(classified.Code))
		fatal, err := ss.handleLaunchFailure(ctx, run, agent, candidate, seq, launchErr)
		if err != nil {
			return false, nil, err
		}
		if fatal {
			return false, nil, launchErr
		}
		// Remember this candidate so the next loop iteration can record
		// the fallback hop on success.
		c := candidate
		prev = &c
	}
	return ss.exhaustedCandidates(ctx, run, agent, res.RequestedTier)
}

// recordAttemptStart increments the attempt sequence and appends the
// in-flight attempt row with outcome=launched.
func (ss *SchedulerService) recordAttemptStart(
	ctx context.Context, run *models.Run,
	candidate routing.Candidate, tier routing.Tier,
) (int, error) {
	seq, err := ss.repo.IncrementRouteAttemptSeq(ctx, run.ID)
	if err != nil {
		return 0, err
	}
	run.CurrentRouteAttemptSeq = seq
	attempt := models.RouteAttempt{
		RunID:      run.ID,
		Seq:        seq,
		ProviderID: string(candidate.ProviderID),
		Model:      candidate.Model,
		Tier:       string(tier),
		Outcome:    RouteAttemptOutcomeLaunched,
		StartedAt:  time.Now().UTC(),
	}
	if err := ss.repo.AppendRouteAttempt(ctx, &attempt); err != nil {
		return 0, err
	}
	ss.publishRouteAttemptAppended(ctx, run.ID, attempt)
	return seq, nil
}

// launchCandidate invokes the task starter with the candidate's route
// override. Returns ErrRoutingNotSupported as-is so the caller can
// treat it as a configuration error rather than a provider failure.
//
// Honors KANDEV_PROVIDER_FAILURES (routingerr.InjectedCode) pre-launch
// so deterministic E2E specs can fail a specific provider without
// requiring the agent binary itself to fail. The injected error is
// returned as a *routingerr.Error wrapped via errors.As so
// classifyLaunchError picks it up unchanged.
//
// launch supplies the Office-built prompt/env/workflow-step/attachments
// so routed launches behave identically to the legacy launch path for
// everything except provider/model selection. Without this, the routed
// path would fall back to task.Description and lose role framing.
func (ss *SchedulerService) launchCandidate(
	ctx context.Context, taskID, agentID string,
	candidate routing.Candidate, launch LaunchContext,
) error {
	if taskID == "" {
		return fmt.Errorf("dispatch: empty task id in run payload")
	}
	if _, ok := routingerr.InjectedCode(string(candidate.ProviderID)); ok {
		// Synthesize a launch failure via Classify so injection is
		// honoured for deterministic E2E specs. Classify short-circuits
		// to the injected code at the head of its decision chain.
		return routingerr.Classify(routingerr.Input{
			Phase:      routingerr.PhaseProcessStart,
			ProviderID: string(candidate.ProviderID),
		})
	}
	return ss.taskStarter.StartTaskWithRoute(ctx, taskID, agentID, launch, RouteOverride{
		ProviderID: string(candidate.ProviderID),
		Model:      candidate.Model,
		Tier:       string(candidate.Tier),
		Mode:       candidate.Mode,
		Flags:      candidate.Flags,
		Env:        candidate.Env,
	})
}

// handleLaunchSuccess records the resolved provider/model on the run
// row and flips every health scope this candidate touched back to
// healthy. Returns (launched=true, nil, nil) on success.
func (ss *SchedulerService) handleLaunchSuccess(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, candidate routing.Candidate,
) (bool, *routing.BlockReason, error) {
	if err := ss.repo.SetRunResolvedProvider(ctx,
		run.ID, string(candidate.ProviderID), candidate.Model); err != nil {
		return false, nil, err
	}
	ss.markHealthScopes(ctx, agent.WorkspaceID, candidate)
	return true, nil, nil
}

// markHealthScopes marks the provider, tier, and model scopes healthy
// for the launched candidate. Best-effort: errors are logged but do
// not fail the launch.
func (ss *SchedulerService) markHealthScopes(
	ctx context.Context, workspaceID string, candidate routing.Candidate,
) {
	scopes := []struct{ scope, value string }{
		{sqliterepo.HealthScopeProvider, ""},
		{sqliterepo.HealthScopeTier, string(candidate.Tier)},
		{sqliterepo.HealthScopeModel, candidate.Model},
	}
	for _, s := range scopes {
		if err := ss.repo.MarkProviderHealthy(ctx,
			workspaceID, string(candidate.ProviderID), s.scope, s.value); err != nil {
			ss.logger.Warn("mark healthy failed",
				zap.String("provider", string(candidate.ProviderID)),
				zap.String("scope", s.scope),
				zap.Error(err))
			continue
		}
		ss.publishProviderHealthy(ctx, workspaceID,
			string(candidate.ProviderID), s.scope, s.value)
	}
	// Emit once per workspace+provider when the provider scope flips
	// back to healthy, not once per scope, so a metrics consumer can
	// observe the high-level recover event.
	recordProviderRecovered(ss.logger, workspaceID, string(candidate.ProviderID))
}

// handleLaunchFailure classifies the launch error and either records
// the candidate as failed-with-fallback (continue loop) or as
// failed-other (fatal, escalate to caller). Returns (fatal, err).
func (ss *SchedulerService) handleLaunchFailure(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, candidate routing.Candidate,
	seq int, launchErr error,
) (bool, error) {
	classified := classifyLaunchError(string(candidate.ProviderID), launchErr)
	now := time.Now().UTC()
	if !classified.FallbackAllowed {
		return true, ss.finishAttempt(ctx, run.ID, seq,
			RouteAttemptOutcomeFailedOther, classified, now)
	}
	if err := ss.degradeProviderForCandidate(ctx, agent.WorkspaceID,
		candidate, classified, now); err != nil {
		return false, err
	}
	return false, ss.finishAttempt(ctx, run.ID, seq,
		RouteAttemptOutcomeFailedProviderUnavail, classified, now)
}

// classifyLaunchError runs routingerr.Classify on a launch error using
// the process-start phase. Adapter-classified errors (when threaded
// through) are honoured via errors.As lookup.
func classifyLaunchError(providerID string, launchErr error) *routingerr.Error {
	var pre *routingerr.Error
	if errors.As(launchErr, &pre) {
		return pre
	}
	return routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhaseProcessStart,
		ProviderID: providerID,
		Stderr:     launchErr.Error(),
	})
}

// degradeProviderForCandidate writes a degraded health row for the
// scope (provider/tier/model) that matches the classifier code.
func (ss *SchedulerService) degradeProviderForCandidate(
	ctx context.Context, workspaceID string,
	candidate routing.Candidate, e *routingerr.Error, now time.Time,
) error {
	scope, scopeVal := sqliterepo.ScopeFromCode(
		string(e.Code), candidate.Model, candidate.Tier)
	current, _ := ss.repo.GetProviderHealth(ctx,
		workspaceID, string(candidate.ProviderID), scope, scopeVal)
	var currentHealth models.ProviderHealth
	if current != nil {
		currentHealth = *current
	}
	retryAt, step := routing.Schedule(currentHealth, e, now)
	row := models.ProviderHealth{
		WorkspaceID: workspaceID,
		ProviderID:  string(candidate.ProviderID),
		Scope:       models.ProviderHealthScope(scope),
		ScopeValue:  scopeVal,
		State:       models.ProviderHealthState(healthStateFor(e)),
		ErrorCode:   string(e.Code),
		RetryAt:     timePtrIfNonZero(retryAt),
		BackoffStep: step,
		LastFailure: &now,
		RawExcerpt:  e.RawExcerpt,
	}
	if err := ss.repo.MarkProviderDegraded(ctx, row); err != nil {
		return err
	}
	ss.publishProviderHealthChanged(ctx, row)
	recordProviderDegraded(ss.logger,
		workspaceID, string(candidate.ProviderID), string(e.Code))
	return nil
}

func healthStateFor(e *routingerr.Error) string {
	if e.UserAction && !e.AutoRetryable {
		return sqliterepo.HealthStateUserActionRequired
	}
	return sqliterepo.HealthStateDegraded
}

func timePtrIfNonZero(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

// finishAttempt updates the in-flight attempt row with the classifier
// outcome. The same path covers both fallback-allowed and fatal errors.
//
// After the DB write, the row is re-read so the WS publish carries a
// fully-hydrated attempt (provider_id / model / tier / started_at,
// which UpdateRouteAttemptOutcome does not touch). Without the
// re-read, the WS reducer on the frontend replaces the existing
// attempt entry with a partial one, clobbering the provider/model
// that was set when the launch attempt began.
func (ss *SchedulerService) finishAttempt(
	ctx context.Context, runID string, seq int,
	outcome string, e *routingerr.Error, finishedAt time.Time,
) error {
	attempt := models.RouteAttempt{
		RunID:           runID,
		Seq:             seq,
		Outcome:         models.RouteAttemptOutcome(outcome),
		ErrorCode:       string(e.Code),
		ErrorConfidence: models.ErrorConfidence(e.Confidence),
		AdapterPhase:    models.AdapterPhase(e.Phase),
		ClassifierRule:  e.ClassifierRule,
		ExitCode:        e.ExitCode,
		RawExcerpt:      e.RawExcerpt,
		ResetHint:       e.ResetHint,
		FinishedAt:      &finishedAt,
	}
	if err := ss.repo.UpdateRouteAttemptOutcome(ctx, &attempt); err != nil {
		return err
	}
	hydrated := ss.hydrateAttempt(ctx, runID, seq, attempt)
	ss.publishRouteAttemptAppended(ctx, runID, hydrated)
	return nil
}

// hydrateAttempt re-reads the (run_id, seq) row so the WS publish has
// the full attempt shape. Falls back to the partial row when the
// re-read fails — better to broadcast something than nothing.
func (ss *SchedulerService) hydrateAttempt(
	ctx context.Context, runID string, seq int, fallback models.RouteAttempt,
) models.RouteAttempt {
	rows, err := ss.repo.ListRouteAttempts(ctx, runID)
	if err != nil {
		return fallback
	}
	for _, row := range rows {
		if row.Seq == seq {
			return row
		}
	}
	return fallback
}

// exhaustedCandidates parks the run when every candidate failed with
// fallback-allowed errors. The block reason is rebuilt from the just-
// recorded health rows so the surfaced earliest-retry is current.
func (ss *SchedulerService) exhaustedCandidates(
	ctx context.Context, run *models.Run,
	agent *models.AgentInstance, tier routing.Tier,
) (bool, *routing.BlockReason, error) {
	rows, err := ss.repo.ListProviderHealth(ctx, agent.WorkspaceID)
	if err != nil {
		return false, nil, err
	}
	br := blockReasonFromHealth(rows, tier)
	if err := ss.repo.ParkRunForProviderCapacity(ctx,
		run.ID, br.Status, br.EarliestRetry); err != nil {
		return false, nil, err
	}
	ss.logger.Info("run parked after candidate exhaustion",
		zap.String("run_id", run.ID),
		zap.String("status", br.Status))
	ss.recordRouteParked(agent.WorkspaceID, run.ID, br.Status)
	return false, &br, nil
}

// blockReasonFromHealth aggregates a workspace's non-healthy rows into
// the same shape the resolver returns. Any auto-retryable code marks
// the run as waiting_for_provider_capacity; otherwise blocked.
func blockReasonFromHealth(
	rows []models.ProviderHealth, _ routing.Tier,
) routing.BlockReason {
	br := routing.BlockReason{Status: routing.StatusBlockedActionRequired}
	for _, row := range rows {
		if !routing.IsAutoRetryableCode(row.ErrorCode) {
			continue
		}
		br.Status = routing.StatusWaitingForCapacity
		if row.RetryAt != nil && !row.RetryAt.IsZero() {
			if br.EarliestRetry.IsZero() || row.RetryAt.Before(br.EarliestRetry) {
				br.EarliestRetry = *row.RetryAt
			}
		}
	}
	return br
}

// extractRunTaskID parses task_id from the run payload JSON. Returns
// "" when the payload is empty or missing the field.
func extractRunTaskID(payload string) string {
	if payload == "" {
		return ""
	}
	var p struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal([]byte(payload), &p); err != nil {
		return ""
	}
	return p.TaskID
}

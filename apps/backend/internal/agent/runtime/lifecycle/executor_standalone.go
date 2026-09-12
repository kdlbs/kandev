package lifecycle

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/subproc"
	"github.com/kandev/kandev/internal/task/models"
)

// defaultRecoveryReadTimeout and defaultRecoveryReadRetries are the
// AC-EXECUTORS-SURVIVAL-002.13 defaults (two seconds, two retries), used
// whenever SetRecoveryRetryConfig has not installed a configured value.
const (
	defaultRecoveryReadTimeout = 2 * time.Second
	defaultRecoveryReadRetries = 2
)

// UnstoppableSessionRecorder is the recovery-guard sink StandaloneExecutor
// drives for a session's not-re-tracked outcome during recovery: retains a
// session's guard for the rest of this backend's lifetime when
// AC-EXECUTORS-SURVIVAL-002.16 applies (at least one live instance could not
// be stopped despite AC-EXECUTORS-SURVIVAL-002.15's bounded retry), marks a
// session whose stop is still resolving when the AC-EXECUTORS-SURVIVAL-003.7
// recovery deadline elapses mid-retry, and releases that guard once the
// resolution is known. Satisfied structurally by *lifecycle.RecoveryGuard
// (see Manager.RecoveryGuard).
type UnstoppableSessionRecorder interface {
	RetainAsUnstoppable(sessionID string)
	// MarkStopInFlight records that sessionID's guard must survive
	// ReleaseAllExceptRetained until its still-resolving stop is known,
	// rather than being released at the deadline bound.
	MarkStopInFlight(sessionID string)
	// Release drops sessionID's guard once its resolution is known. A no-op
	// for a session RetainAsUnstoppable has already claimed.
	Release(sessionID string)
}

// StandaloneExecutor implements Runtime for standalone agentctl execution.
// In this mode, a single agentctl control server manages multiple agent instances.
type StandaloneExecutor struct {
	ctl                 *agentctl.ControlClient
	host                string
	port                int
	authToken           string // per-launch auth token from launcher
	logger              *logger.Logger
	interactiveRunner   *process.InteractiveRunner
	recoveryReadTimeout time.Duration
	recoveryReadRetries int
	unstoppableRecorder UnstoppableSessionRecorder
}

// NewStandaloneExecutor creates a new standalone runtime.
func NewStandaloneExecutor(ctl *agentctl.ControlClient, host string, port int, log *logger.Logger) *StandaloneExecutor {
	return &StandaloneExecutor{
		ctl:    ctl,
		host:   host,
		port:   port,
		logger: log.WithFields(zap.String("runtime", "standalone")),
		// -1 is the "unset" sentinel: zero is itself a valid configured
		// retry count (AC-EXECUTORS-SURVIVAL-002.13 allows zero retries), so
		// the zero value of an unset int field can't be used to mean unset.
		recoveryReadRetries: -1,
	}
}

// SetAuthToken sets the per-launch auth token for authenticating instance clients.
func (r *StandaloneExecutor) SetAuthToken(token string) {
	r.authToken = token
}

// SetRecoveryRetryConfig installs the bounded per-attempt timeout and retry
// count AC-EXECUTORS-SURVIVAL-002.13/002.15 require for recovery reads and
// stops. A zero timeout or negative retry count falls back to the two-second/
// two-retry default the AC itself specifies.
func (r *StandaloneExecutor) SetRecoveryRetryConfig(timeout time.Duration, retries int) {
	r.recoveryReadTimeout = timeout
	r.recoveryReadRetries = retries
}

// SetUnstoppableSessionRecorder installs the AC-EXECUTORS-SURVIVAL-002.16 sink.
// Unset means an unstoppable instance is only logged, never retained --
// acceptable for callers that haven't wired a recovery guard.
func (r *StandaloneExecutor) SetUnstoppableSessionRecorder(recorder UnstoppableSessionRecorder) {
	r.unstoppableRecorder = recorder
}

// stopWithRetry stops instanceID within the bounded per-attempt timeout and
// retry count of AC-EXECUTORS-SURVIVAL-002.13/002.15, with no delay between
// attempts. DeleteInstance itself already treats an already-absent instance
// (404) as success.
func (r *StandaloneExecutor) stopWithRetry(ctx context.Context, instanceID string) error {
	timeout := r.recoveryReadTimeout
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	retries := r.recoveryReadRetries
	if retries < 0 {
		retries = defaultRecoveryReadRetries
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		lastErr = r.ctl.DeleteInstance(attemptCtx, instanceID)
		cancel()
		if lastErr == nil {
			return nil
		}
	}
	return lastErr
}

// listInstancesWithRetry enumerates the adopted control server's live
// instances within the same bounded per-attempt timeout and retry count as
// stopWithRetry (AC-EXECUTORS-SURVIVAL-002.13): this enumeration is the sole
// read source for two AC-EXECUTORS-SURVIVAL-002.3 reconstruction-table
// values -- workspace source roots and provider session identity -- both
// declared to come from the adopted instance, so a transient failure here
// must be retried before an instance's reconstruction is given up on.
func (r *StandaloneExecutor) listInstancesWithRetry(ctx context.Context) ([]*agentctl.InstanceInfo, error) {
	timeout := r.recoveryReadTimeout
	if timeout <= 0 {
		timeout = defaultRecoveryReadTimeout
	}
	retries := r.recoveryReadRetries
	if retries < 0 {
		retries = defaultRecoveryReadRetries
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, timeout)
		instances, err := r.ctl.ListInstances(attemptCtx)
		cancel()
		if err == nil {
			return instances, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func (r *StandaloneExecutor) Name() executor.Name {
	return executor.NameStandalone
}

func (r *StandaloneExecutor) HealthCheck(ctx context.Context) error {
	return r.ctl.Health(ctx)
}

// SubprocessAdmission returns the admission snapshot from the host agentctl
// control server for backend diagnostics.
func (r *StandaloneExecutor) SubprocessAdmission(ctx context.Context) (subproc.Snapshot, error) {
	return r.ctl.SubprocessAdmission(ctx)
}

func (r *StandaloneExecutor) waitForReady(ctx context.Context) error {
	if err := r.ctl.Health(ctx); err == nil {
		return nil
	}

	waitCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		waitCtx, cancel = context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-waitCtx.Done():
			return fmt.Errorf("agentctl not ready: %w", waitCtx.Err())
		case <-ticker.C:
			if err := r.ctl.Health(waitCtx); err == nil {
				return nil
			}
		}
	}
}

func buildStandaloneCreateInstanceRequest(
	req *ExecutorCreateRequest,
	env map[string]string,
	agentType string,
	disableAskQuestion, assumeMcpSse, assumeMcpHttp, requiresProcessKill bool,
	stripEnv []string,
) *agentctl.CreateInstanceRequest {
	return &agentctl.CreateInstanceRequest{
		ID:            req.InstanceID,
		WorkspacePath: req.WorkspacePath,
		AgentCommand:  "", // Agent command set via Configure endpoint
		Protocol:      req.Protocol,
		AgentType:     agentType,
		Env:           env,
		AutoApprovePermissions: autoApprovePermissionsOverride(
			req.AutoApprovePermissions,
			req.AutoApprovePermissionsOverride,
		),
		AutoStart:                  false,
		McpServers:                 req.McpServers,
		SessionID:                  req.SessionID,
		TaskID:                     req.TaskID,
		DisableAskQuestion:         disableAskQuestion,
		AssumeMcpSse:               assumeMcpSse,
		AssumeMcpHttp:              assumeMcpHttp,
		McpMode:                    req.McpMode,
		McpProviders:               req.McpProviders,
		McpProfile:                 req.McpProfile,
		NamespacesMCPToolsByServer: namespacesMCPToolsByServerFromReq(req),
		RequiresProcessKill:        requiresProcessKill,
		StripEnv:                   stripEnv,
		BaseBranches:               getMetadataStringMap(req.Metadata, MetadataKeyBaseBranches),
		RemoteContributions:        req.RemoteContributions,
		ContributionDestinations:   req.ContributionDestinations,
		ComparisonTargets:          req.ComparisonTargets,
		WorkspaceSourceRoots:       req.WorkspaceSourceRoots,
	}
}

func (r *StandaloneExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	if err := r.waitForReady(ctx); err != nil {
		return nil, err
	}

	// Build environment variables
	env := req.Env
	if env == nil {
		env = make(map[string]string)
	}
	env["KANDEV_TASK_ID"] = req.TaskID
	env["KANDEV_SESSION_ID"] = req.SessionID

	// Create instance via control API
	// Agent command is NOT set - workspace access only. Agent is started explicitly via agentctl client.
	agentType := ""
	if req.AgentConfig != nil {
		agentType = req.AgentConfig.ID()
	}
	disableAskQuestion := !agents.SupportsInteractiveMCPTools(req.AgentConfig)
	assumeMcpSse := false
	assumeMcpHttp := false
	requiresProcessKill := false
	var stripEnv []string
	if req.AgentConfig != nil {
		if rt := req.AgentConfig.Runtime(); rt != nil {
			assumeMcpSse = rt.AssumeMcpSse
			assumeMcpHttp = rt.AssumeMcpHttp
			requiresProcessKill = rt.RequiresProcessKill
			stripEnv = rt.StripEnv
		}
	}

	createReq := buildStandaloneCreateInstanceRequest(
		req, env, agentType, disableAskQuestion, assumeMcpSse, assumeMcpHttp, requiresProcessKill, stripEnv,
	)

	r.logger.Info("CreateInstance: sending request to agentctl",
		zap.String("instance_id", req.InstanceID),
		zap.String("req_protocol", req.Protocol),
		zap.String("createReq_protocol", createReq.Protocol))

	resp, err := r.ctl.CreateInstance(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create standalone instance: %w", err)
	}

	// Create agentctl client pointing to the instance port
	client := agentctl.NewClient(r.host, resp.Port, r.logger,
		agentctl.WithExecutionID(req.InstanceID),
		agentctl.WithSessionID(req.SessionID),
		agentctl.WithAuthToken(r.authToken))

	// Extract runtime-specific values from metadata
	worktreeID := getMetadataString(req.Metadata, MetadataKeyWorktreeID)
	worktreeBranch := getMetadataString(req.Metadata, MetadataKeyWorktreeBranch)

	// Build metadata
	metadata := make(map[string]interface{})
	metadata["standalone_port"] = resp.Port
	if worktreeID != "" {
		metadata["worktree_id"] = worktreeID
		metadata["worktree_path"] = req.WorkspacePath
		metadata["worktree_branch"] = worktreeBranch
	}

	r.logger.Debug("standalone instance created",
		zap.String("instance_id", req.InstanceID),
		zap.Int("port", resp.Port),
		zap.String("workspace", req.WorkspacePath))

	return &ExecutorInstance{
		InstanceID:           req.InstanceID,
		TaskID:               req.TaskID,
		SessionID:            req.SessionID,
		RuntimeName:          r.Name(),
		Client:               client,
		StandaloneInstanceID: resp.ID,
		StandalonePort:       resp.Port,
		WorkspacePath:        req.WorkspacePath,
		Metadata:             metadata,
	}, nil
}

func (r *StandaloneExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance.StandaloneInstanceID == "" {
		return nil // No standalone instance to stop
	}

	if err := r.ctl.DeleteInstance(ctx, instance.StandaloneInstanceID); err != nil {
		return fmt.Errorf("failed to stop standalone instance: %w", err)
	}

	return nil
}

// RecoverInstances enumerates the adopted control server's live instances and
// correlates them to the live standalone recovery-inventory records read at
// startup step 3 (AC-EXECUTORS-SURVIVAL-002.1/002.8), applying the
// AC-EXECUTORS-SURVIVAL-002.10 duplicate tiebreak via CorrelateRecoveryInstances.
// Every losing duplicate, ambiguous-session instance, and record-less orphan
// (AC-EXECUTORS-SURVIVAL-002.6) is stopped within the bounded retry budget of
// AC-EXECUTORS-SURVIVAL-002.13/002.15 (SetRecoveryRetryConfig); every winner
// is returned for the caller to re-track. When a losing duplicate's stop
// exhausts its retries, that session's winner is also stopped and dropped
// from the result rather than re-tracked (AC-EXECUTORS-SURVIVAL-002.15); if
// the winner's own stop then also fails, the session is reported to the
// installed UnstoppableSessionRecorder (AC-EXECUTORS-SURVIVAL-002.16).
//
// Every stop runs in its own goroutine against context.Background(), never
// ctx: if ctx carries a deadline (AC-EXECUTORS-SURVIVAL-003.7) and it elapses
// while stops are still outstanding, this stops WAITING for them -- treating
// every session whose losing duplicate hasn't yet resolved as not re-tracked
// right now -- but never cancels work already dispatched. Outstanding stops,
// and any winner-stop or unstoppable report they trigger, keep running and
// recording their outcome in the background after this call returns.
//
// Enumeration itself is retried within the AC-EXECUTORS-SURVIVAL-002.13
// bounded budget (listInstancesWithRetry) before being treated as failed --
// workspace source roots and provider session identity are read back from
// this same response, so a transient enumeration failure must not skip the
// retry that AC applies to every other adopted-instance read. When the
// adopted server still cannot be enumerated after retries are exhausted,
// this reports nothing recovered, stops nothing, and leaves every record to
// the existing stale-execution repair path (AC-EXECUTORS-SURVIVAL-002.12)
// rather than treating an enumeration failure as though every instance were
// an orphan.
func (r *StandaloneExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	instances, err := r.listInstancesWithRetry(ctx)
	if err != nil {
		r.logger.Warn("failed to enumerate standalone instances for recovery; leaving every record to the existing repair path",
			zap.Error(err))
		return nil, nil
	}

	correlation := CorrelateRecoveryInstances(records, instances)

	// winnersBySession is a stable snapshot of every session's recovered
	// winner, independent of correlation.Winners (which this call mutates
	// and returns): AC-EXECUTORS-SURVIVAL-003.7 requires an in-flight loser
	// stop, once dispatched, to still be able to find and stop its session's
	// winner even after that session has already been dropped from the
	// returned map at the deadline.
	winnersBySession := make(map[string]*agentctl.InstanceInfo, len(correlation.Winners))
	for sessionID, winner := range correlation.Winners {
		winnersBySession[sessionID] = winner
	}

	pending := pendingLoserCounts(correlation.ToStop, correlation.Winners)
	results := r.dispatchRecoveryStops(correlation.ToStop)
	tracker := &jointFailureTracker{exec: r, winners: winnersBySession}
	r.collectRecoveryStops(ctx, results, correlation.Winners, pending, tracker)

	return r.buildRecoveredInstances(correlation.Winners, indexRecordsBySession(records)), nil
}

// recoveryStopOutcome is one instance's bounded stop attempt result.
type recoveryStopOutcome struct {
	inst *agentctl.InstanceInfo
	err  error
}

// dispatchRecoveryStops starts one goroutine per not-re-tracked instance,
// each stopping it within the bounded retry budget against
// context.Background() -- never the caller's ctx, so an elapsed recovery
// deadline can never abort a stop already in flight
// (AC-EXECUTORS-SURVIVAL-003.7). The returned channel is closed once every
// goroutine has reported.
func (r *StandaloneExecutor) dispatchRecoveryStops(toStop []*agentctl.InstanceInfo) <-chan recoveryStopOutcome {
	results := make(chan recoveryStopOutcome, len(toStop))
	var wg sync.WaitGroup
	for _, inst := range toStop {
		wg.Add(1)
		go func(inst *agentctl.InstanceInfo) {
			defer wg.Done()
			results <- recoveryStopOutcome{inst: inst, err: r.stopWithRetry(context.Background(), inst.ID)}
		}(inst)
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	return results
}

// pendingLoserCounts counts, per session with a winner, how many of its
// losing duplicates have not yet reported a stop outcome.
func pendingLoserCounts(toStop []*agentctl.InstanceInfo, winners map[string]*agentctl.InstanceInfo) map[string]int {
	pending := make(map[string]int, len(winners))
	for _, inst := range toStop {
		if _, hasWinner := winners[inst.SessionID]; hasWinner {
			pending[inst.SessionID]++
		}
	}
	return pending
}

// collectRecoveryStops waits for dispatchRecoveryStops's results, applying
// each as it arrives, until either every result has landed or ctx's deadline
// (if any) elapses first. On deadline elapse it synchronously drops every
// session with an outstanding loser stop from winners -- AC-EXECUTORS-SURVIVAL-003.7's
// "treated as not-re-tracked" -- then keeps draining the remaining results in
// the background so their outcomes (including a later AC-EXECUTORS-SURVIVAL-002.16
// report) still get recorded.
//
// winners is only ever mutated from this call's own goroutine, and only
// before this function returns: the caller iterates it immediately after
// (buildRecoveredInstances), and the background drain goroutine started at
// the deadline branch never touches it, to avoid a concurrent map
// read/write with that iteration.
func (r *StandaloneExecutor) collectRecoveryStops(
	ctx context.Context,
	results <-chan recoveryStopOutcome,
	winners map[string]*agentctl.InstanceInfo,
	pending map[string]int,
	tracker *jointFailureTracker,
) {
	deadlineCh, stopTimer := recoveryDeadlineChannel(ctx)
	defer stopTimer()

	for {
		select {
		case res, ok := <-results:
			if !ok {
				return
			}
			r.handleRecoveryStopResult(res, pending, tracker)
			if res.err != nil {
				delete(winners, res.inst.SessionID)
			}
		case <-deadlineCh:
			for sessionID, n := range pending {
				if n > 0 {
					delete(winners, sessionID)
					// AC-EXECUTORS-SURVIVAL-003.7: this session's stop is still
					// retrying past the deadline -- its guard must survive
					// ReleaseAllExceptRetained until drainRecoveryStopsAfterDeadline
					// below learns the resolution and releases (or retains) it.
					if r.unstoppableRecorder != nil {
						r.unstoppableRecorder.MarkStopInFlight(sessionID)
					}
				}
			}
			go r.drainRecoveryStopsAfterDeadline(results, pending, tracker)
			return
		}
	}
}

// drainRecoveryStopsAfterDeadline finishes applying every recovery stop
// outcome still outstanding once collectRecoveryStops has already returned
// past the AC-EXECUTORS-SURVIVAL-003.7 deadline. Each affected session's
// MarkStopInFlight guard is released once every one of its pending stops has
// resolved: either directly here (every loser stopped, nothing left to
// jointly stop) or by stopWinnerAfterLoserFailure's own terminal Release/
// RetainAsUnstoppable call -- so the "retain until that stop resolves"
// exception never outlives the actual resolution. Release is safe to call a
// second time here even after stopWinnerAfterLoserFailure already resolved
// the guard: it no-ops for an already-released or already-retained session.
func (r *StandaloneExecutor) drainRecoveryStopsAfterDeadline(
	results <-chan recoveryStopOutcome,
	pending map[string]int,
	tracker *jointFailureTracker,
) {
	for res := range results {
		r.handleRecoveryStopResult(res, pending, tracker)
		if r.unstoppableRecorder != nil && pending[res.inst.SessionID] == 0 {
			r.unstoppableRecorder.Release(res.inst.SessionID)
		}
	}
}

// recoveryDeadlineChannel returns a channel that fires at ctx's deadline, or
// a nil channel (which never fires) when ctx carries none.
func recoveryDeadlineChannel(ctx context.Context) (<-chan time.Time, func()) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return nil, func() {}
	}
	timer := time.NewTimer(time.Until(deadline))
	return timer.C, func() { timer.Stop() }
}

// handleRecoveryStopResult applies one stop outcome: on success it only
// clears the session's pending-loser count; on failure it also logs and, for
// a losing duplicate whose session has a winner, triggers the joint failure
// handling (AC-EXECUTORS-SURVIVAL-002.15).
func (r *StandaloneExecutor) handleRecoveryStopResult(res recoveryStopOutcome, pending map[string]int, tracker *jointFailureTracker) {
	sessionID := res.inst.SessionID
	if _, tracked := pending[sessionID]; tracked {
		pending[sessionID]--
	}
	if res.err == nil {
		return
	}
	r.logger.Warn("failed to stop a not-re-tracked standalone instance during recovery after exhausting retries",
		zap.String("instance_id", res.inst.ID), zap.String("session_id", sessionID), zap.Error(res.err))
	tracker.onLoserStopFailed(sessionID)
}

// jointFailureTracker ensures AC-EXECUTORS-SURVIVAL-002.15/002.16's joint
// winner-stop-and-possible-unstoppable-report handling runs at most once per
// session, whether triggered synchronously (before the recovery deadline) or
// from the background drain after it (AC-EXECUTORS-SURVIVAL-003.7).
type jointFailureTracker struct {
	exec    *StandaloneExecutor
	winners map[string]*agentctl.InstanceInfo
	handled sync.Map // sessionID -> *sync.Once
}

func (t *jointFailureTracker) onLoserStopFailed(sessionID string) {
	winner, ok := t.winners[sessionID]
	if !ok {
		return // orphan, or a session with no winner: nothing to jointly affect
	}
	once, _ := t.handled.LoadOrStore(sessionID, &sync.Once{})
	once.(*sync.Once).Do(func() {
		t.exec.stopWinnerAfterLoserFailure(sessionID, winner)
	})
}

// stopWinnerAfterLoserFailure stops a session's winning instance on the same
// bounded retry terms after one of its losing duplicates could not be
// stopped; on failure it reports the session to the installed
// UnstoppableSessionRecorder (AC-EXECUTORS-SURVIVAL-002.16). Always runs to
// completion once started, even past the recovery deadline
// (AC-EXECUTORS-SURVIVAL-003.7).
func (r *StandaloneExecutor) stopWinnerAfterLoserFailure(sessionID string, winner *agentctl.InstanceInfo) {
	if err := r.stopWithRetry(context.Background(), winner.ID); err != nil {
		r.logger.Warn("failed to stop the winning instance after its losing duplicate could not be stopped; retaining session as unstoppable",
			zap.String("instance_id", winner.ID), zap.String("session_id", sessionID), zap.Error(err))
		if r.unstoppableRecorder != nil {
			r.unstoppableRecorder.RetainAsUnstoppable(sessionID)
		}
		return
	}
	r.logger.Warn("stopped the winning instance because its losing duplicate could not be stopped; session left not re-tracked",
		zap.String("instance_id", winner.ID), zap.String("session_id", sessionID))
	if r.unstoppableRecorder != nil {
		r.unstoppableRecorder.Release(sessionID)
	}
}

// indexRecordsBySession maps every named recovery-inventory record by
// session ID for use while building recovered executions.
func indexRecordsBySession(records []*models.ExecutorRunning) map[string]*models.ExecutorRunning {
	recordBySession := make(map[string]*models.ExecutorRunning, len(records))
	for _, rec := range records {
		if rec != nil && rec.SessionID != "" {
			recordBySession[rec.SessionID] = rec
		}
	}
	return recordBySession
}

// buildRecoveredInstances converts every still-winning correlated instance
// into an ExecutorInstance for the caller to re-track.
func (r *StandaloneExecutor) buildRecoveredInstances(
	winners map[string]*agentctl.InstanceInfo,
	recordBySession map[string]*models.ExecutorRunning,
) []*ExecutorInstance {
	recovered := make([]*ExecutorInstance, 0, len(winners))
	for sessionID, inst := range winners {
		record := recordBySession[sessionID]

		client := agentctl.NewClient(r.host, inst.Port, r.logger,
			agentctl.WithExecutionID(inst.ID),
			agentctl.WithSessionID(sessionID),
			agentctl.WithAuthToken(r.authToken))

		// AC-EXECUTORS-SURVIVAL-002.14: task identity and workspace path's
		// declared source is the recovery-inventory record, never the
		// adopted instance -- an instance must not be able to influence what
		// the backend believes either belongs to. A record with no TaskID
		// (or, defensively, no record at all -- correlation should never
		// produce a winner without one) is left empty here rather than
		// falling back to the instance's self-reported value; the caller
		// routes an empty TaskID to the AC-EXECUTORS-SURVIVAL-002.4 refusal
		// path instead of tracking a session it cannot safely operate.
		var taskID, workspacePath string
		var metadata map[string]interface{}
		var agentProfileID string
		if record != nil {
			taskID = record.TaskID
			workspacePath = record.WorktreePath
			metadata = record.Metadata
			// AC-EXECUTORS-SURVIVAL-002.14: agent profile identity's declared
			// source is the recovery-inventory record's execution-profile
			// column -- never the adopted instance.
			agentProfileID = record.ExecutionProfileID
		}

		recovered = append(recovered, &ExecutorInstance{
			InstanceID:           inst.ID,
			TaskID:               taskID,
			SessionID:            sessionID,
			AgentProfileID:       agentProfileID,
			RuntimeName:          r.Name(),
			Client:               client,
			StandaloneInstanceID: inst.ID,
			StandalonePort:       inst.Port,
			WorkspacePath:        workspacePath,
			Metadata:             metadata,
			Env:                  inst.Env,
			WorkspaceSourceRoots: inst.WorkspaceSourceRoots,
			ProviderSessionID:    inst.ProviderSessionID,
		})
	}
	return recovered
}

// SetInteractiveRunner sets the interactive runner for passthrough mode.
func (r *StandaloneExecutor) SetInteractiveRunner(runner *process.InteractiveRunner) {
	r.interactiveRunner = runner
}

// GetInteractiveRunner returns the interactive runner for passthrough mode.
func (r *StandaloneExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return r.interactiveRunner
}

func (r *StandaloneExecutor) RequiresCloneURL() bool          { return false }
func (r *StandaloneExecutor) ShouldApplyPreferredShell() bool { return true }
func (r *StandaloneExecutor) IsAlwaysResumable() bool         { return false }

package lifecycle

import (
	"context"
	"fmt"
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

// UnstoppableSessionRecorder retains a session's recovery guard for the rest
// of this backend's lifetime when AC-EXECUTORS-SURVIVAL-002.16 applies: at
// least one live instance for that session could not be stopped despite
// AC-EXECUTORS-SURVIVAL-002.15's bounded retry. Satisfied structurally by
// *lifecycle.RecoveryGuard (see Manager.RecoveryGuard).
type UnstoppableSessionRecorder interface {
	RetainAsUnstoppable(sessionID string)
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
// When the adopted server cannot be enumerated at all, this reports nothing
// recovered, stops nothing, and leaves every record to the existing
// stale-execution repair path (AC-EXECUTORS-SURVIVAL-002.12) rather than
// treating an enumeration failure as though every instance were an orphan.
//
// Scope note: this does not yet implement the full four-source reconstruction
// table of design part 3 (task-environment identity, agent identity/command
// re-derivation, workspace source roots and provider session identity read
// back from the instance) -- those require new agentctl wire-contract fields
// not yet added. A recovered execution here carries the same field set the
// pre-existing (never-before-reachable) recovery consumer in Manager.Start
// already builds.
func (r *StandaloneExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	instances, err := r.ctl.ListInstances(ctx)
	if err != nil {
		r.logger.Warn("failed to enumerate standalone instances for recovery; leaving every record to the existing repair path",
			zap.Error(err))
		return nil, nil
	}

	recordBySession := make(map[string]*models.ExecutorRunning, len(records))
	for _, rec := range records {
		if rec != nil && rec.SessionID != "" {
			recordBySession[rec.SessionID] = rec
		}
	}

	correlation := CorrelateRecoveryInstances(records, instances)

	// AC-EXECUTORS-SURVIVAL-002.15: a stop that exhausts its bounded retries
	// is recorded; when the failed instance was a losing duplicate for a
	// session that DOES have a winner, that winner must not be re-tracked
	// either -- collected here and resolved below, after every ToStop
	// instance has had its own retry budget, so one session's outcome never
	// depends on iteration order (AC-EXECUTORS-SURVIVAL-002.11).
	sessionsWithFailedLoserStop := make(map[string]bool)
	for _, inst := range correlation.ToStop {
		if err := r.stopWithRetry(ctx, inst.ID); err != nil {
			r.logger.Warn("failed to stop a not-re-tracked standalone instance during recovery after exhausting retries",
				zap.String("instance_id", inst.ID),
				zap.String("session_id", inst.SessionID),
				zap.Error(err))
			if _, hasWinner := correlation.Winners[inst.SessionID]; hasWinner {
				sessionsWithFailedLoserStop[inst.SessionID] = true
			}
		}
	}

	for sessionID := range sessionsWithFailedLoserStop {
		winner := correlation.Winners[sessionID]
		delete(correlation.Winners, sessionID)
		if err := r.stopWithRetry(ctx, winner.ID); err != nil {
			r.logger.Warn("failed to stop the winning instance after its losing duplicate could not be stopped; retaining session as unstoppable",
				zap.String("instance_id", winner.ID),
				zap.String("session_id", sessionID),
				zap.Error(err))
			if r.unstoppableRecorder != nil {
				r.unstoppableRecorder.RetainAsUnstoppable(sessionID)
			}
			continue
		}
		r.logger.Warn("stopped the winning instance because its losing duplicate could not be stopped; session left not re-tracked",
			zap.String("instance_id", winner.ID),
			zap.String("session_id", sessionID))
	}

	recovered := make([]*ExecutorInstance, 0, len(correlation.Winners))
	for sessionID, inst := range correlation.Winners {
		record := recordBySession[sessionID]

		client := agentctl.NewClient(r.host, inst.Port, r.logger,
			agentctl.WithExecutionID(inst.ID),
			agentctl.WithSessionID(sessionID),
			agentctl.WithAuthToken(r.authToken))

		taskID := inst.TaskID
		var metadata map[string]interface{}
		if record != nil {
			if record.TaskID != "" {
				taskID = record.TaskID
			}
			metadata = record.Metadata
		}

		recovered = append(recovered, &ExecutorInstance{
			InstanceID:           inst.ID,
			TaskID:               taskID,
			SessionID:            sessionID,
			RuntimeName:          r.Name(),
			Client:               client,
			StandaloneInstanceID: inst.ID,
			StandalonePort:       inst.Port,
			WorkspacePath:        inst.WorkspacePath,
			Metadata:             metadata,
		})
	}

	return recovered, nil
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

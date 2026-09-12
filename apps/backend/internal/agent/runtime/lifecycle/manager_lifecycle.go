package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/tracing"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	containerStateCreated = "created"
	containerStateExited  = "exited"
	containerStateRunning = "running"
)

// Start starts the lifecycle manager background tasks
func (m *Manager) Start(ctx context.Context) error {
	if m.executorRegistry == nil {
		m.logger.Warn("no runtime registry configured")
		return nil
	}

	runtimeNames := m.executorRegistry.List()
	m.logger.Info("starting lifecycle manager", zap.Int("runtimes", len(runtimeNames)))

	// Check health of all registered runtimes
	healthResults := m.executorRegistry.HealthCheckAll(ctx)
	for name, err := range healthResults {
		if err != nil {
			m.logger.Warn("runtime health check failed",
				zap.String("runtime", string(name)),
				zap.Error(err))
		} else {
			m.logger.Info("runtime is healthy", zap.String("runtime", string(name)))
		}
	}

	// AC-EXECUTORS-SURVIVAL-003.1: start this pass's re-tracked-session set
	// empty so a stale entry from an earlier Start() call (tests, or a
	// hypothetical re-Start) never leaks into this pass's outcome.
	m.resetRetrackedSessions()

	// Read the live standalone recovery-inventory records (startup step 3,
	// AC-EXECUTORS-SURVIVAL-002.8) before recovery contacts any control
	// server, and hand them to every runtime's RecoverInstances unchanged.
	records, listErr := m.ListLiveStandaloneExecutorsRunning(ctx)
	if listErr != nil {
		// A failed read leaves the record set unknown, which is not the same
		// thing as empty. AC-EXECUTORS-SURVIVAL-002.6 stops a live instance
		// because it is known to have no record; passing an unknown set on as
		// an empty one would make every live instance look record-less and
		// send all of them down that orphan-stop path, so a transient database
		// error during startup would kill every agent that just survived the
		// restart. Recover nothing instead, on the same terms
		// AC-EXECUTORS-SURVIVAL-002.12 sets for the mirror failure -- the
		// adopted server that cannot be enumerated: report no recovered
		// instances, stop no instance, and leave every record to the existing
		// stale-execution repair path.
		m.logger.Error("skipping recovery: live standalone recovery-inventory records could not be read, so no live instance can be correlated to a session",
			zap.Error(listErr))
		records = nil
	}

	// Take a recovery guard for every named session except a confirmed
	// passthrough one, BEFORE any control server is contacted, so a
	// concurrent launch request cannot race re-tracking
	// (AC-EXECUTORS-SURVIVAL-002.8, AC-EXECUTORS-SURVIVAL-005.3). No lookup
	// configured is treated the same as a failed read: every session is
	// guarded rather than excluded.
	passthroughLookup := m.passthroughLookup
	if passthroughLookup == nil {
		passthroughLookup = func(context.Context, string) (bool, bool) { return false, false }
	}
	guardedSessions := SessionsToGuard(ctx, sessionIDsFromExecutorRunning(records), passthroughLookup)
	for _, sessionID := range guardedSessions {
		m.recoveryGuard.AcquireOrObserve(sessionID)
	}

	// AC-EXECUTORS-SURVIVAL-005.3: the same classification also decides what
	// re-tracking may reach. A confirmed-passthrough session owns a real
	// agentctl instance that does outlive this backend, so without this the
	// enumeration would correlate it to its record and re-track a session
	// whose PTY agent died with the backend -- reporting it as having
	// survived, which the AC forbids. Only standalone records reach here
	// (ListLiveStandaloneExecutorsRunning), and StandaloneExecutor is the
	// only backend that reads them, so this narrows nothing else.
	records = recoverableRecords(records, guardedSessions)

	// AC-EXECUTORS-SURVIVAL-003.7: bound this pass's
	// adoption+enumeration+reconstruction work with a single deadline, clocked
	// from this launch's first control-server contact (or, absent one, from
	// right now). recoveryCtx now covers not just RecoverAll (the producer)
	// but every per-instance reconstruction step in the consumer loop below,
	// per design 02's "Startup" steps 4-7 sharing one deadline. A backend
	// implementation that honors ctx.Deadline() (see
	// StandaloneExecutor.RecoverInstances) may still let work already in
	// flight at the deadline finish in the background rather than aborting
	// it -- cancelling recoveryCtx once this pass is over only stops the
	// deadline timer itself from leaking, it never reaches that background
	// work, which deliberately runs against context.Background() instead.
	recoveryCtx, cancelRecovery := context.WithDeadline(ctx, m.recoveryDeadlineDeadline())
	var recovered []*ExecutorInstance
	if listErr == nil {
		var err error
		recovered, err = m.executorRegistry.RecoverAll(recoveryCtx, records)
		if err != nil {
			m.logger.Warn("failed to recover executions from some runtimes", zap.Error(err))
		}
	}

	var stopWG sync.WaitGroup
	if len(recovered) > 0 {
		for _, ri := range recovered {
			if recoveryCtx.Err() != nil {
				// AC-EXECUTORS-SURVIVAL-003.7: the deadline elapsed before this
				// record could be reconstructed -- treat it as not re-tracked
				// and stop it (against context.Background(), matching every
				// other recovery-time stop: an elapsed deadline must not abort
				// a stop already dispatched).
				m.logger.Warn("recovery deadline elapsed before this record could be reconstructed; treating as not re-tracked",
					zap.String("instance_id", ri.InstanceID),
					zap.String("session_id", ri.SessionID))
				m.dispatchUnreconstructableStop(&stopWG, ri)
				continue
			}
			execution := &AgentExecution{
				ID:        ri.InstanceID,
				TaskID:    ri.TaskID,
				SessionID: ri.SessionID,
				// AC-EXECUTORS-SURVIVAL-002.14: agent profile identity's
				// declared source is the recovery-inventory record's
				// execution-profile column, carried onto ri by
				// buildRecoveredInstances -- never the adopted instance.
				AgentProfileID:       ri.AgentProfileID,
				ContainerID:          ri.ContainerID,
				ContainerIP:          ri.ContainerIP,
				WorkspacePath:        ri.WorkspacePath,
				RuntimeName:          ri.RuntimeName,
				Status:               v1.AgentStatusRunning,
				StartedAt:            time.Now(),
				metadata:             ri.Metadata,
				agentctl:             ri.Client,
				standaloneInstanceID: ri.StandaloneInstanceID,
				standalonePort:       ri.StandalonePort,
				promptDoneCh:         make(chan PromptCompletionSignal, 1),
				// AC-EXECUTORS-SURVIVAL-002.14: run identity is re-derived from
				// the runtime environment, which is itself read back from the
				// adopted instance rather than the database (both deliberately
				// memory-only, per design 03's "runtime environment" /
				// "run identity" rows).
				RunID: ri.Env["KANDEV_RUN_ID"],
				// AC-EXECUTORS-SURVIVAL-002.14: workspace source roots are read
				// back from the adopted instance, never pushed -- agentctl owns
				// this allowlist and a rebind may have changed it since launch.
				WorkspaceSourceRoots: ri.WorkspaceSourceRoots,
				// AC-EXECUTORS-SURVIVAL-002.14: provider session identity comes
				// from the adopted instance, which holds the live provider
				// session -- never from a durable/database value, which could be
				// stale relative to what the instance actually resumed.
				ACPSessionID: ri.ProviderSessionID,
			}
			// AC-EXECUTORS-SURVIVAL-002.14's declared source for task
			// identity is the recovery-inventory record alone, never the
			// adopted instance (buildRecoveredInstances no longer falls
			// back to the instance's self-reported task ID). An empty
			// value here means the record itself is authoritatively
			// missing it: refuse to re-track, per AC-EXECUTORS-SURVIVAL-002.4,
			// rather than trusting the instance's own claim about which
			// task it belongs to.
			if execution.TaskID == "" {
				m.logger.Error("refusing to re-track recovered execution: task identity was not present in the recovery-inventory record",
					zap.String("instance_id", execution.ID),
					zap.String("session_id", execution.SessionID))
				m.dispatchUnreconstructableStop(&stopWG, ri)
				continue
			}
			// A recovered instance is resuming a live provider session (its
			// ACPSessionID above comes straight from the adopted instance), so
			// ACP session setup is already complete -- unlike a fresh launch,
			// there is no InitializeAndPrompt call on this path to mark it.
			// Leaving this false blocks SetSessionMode/SetSessionModel/
			// SetSessionConfigOption ("ACP session is not ready") on every
			// re-tracked execution until its next full relaunch.
			execution.setSessionInitialized(true)
			execution.setRuntimeEnvironment(ri.Env)
			if err := m.hydrateRecoveredTaskEnvironmentID(recoveryCtx, execution); err != nil {
				m.logger.Error("refusing to re-track recovered execution: task environment identity could not be reconstructed",
					zap.String("instance_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.Error(err))
				m.dispatchUnreconstructableStop(&stopWG, ri)
				continue
			}
			// AC-EXECUTORS-SURVIVAL-002.14: Office profile identity is a new key
			// in the same persisted metadata this record already carries, not a
			// new source -- an empty value here is a legitimate non-Office
			// launch, not a missing reconstruction.
			execution.OfficeAgentProfileID = getMetadataString(ri.Metadata, MetadataKeyOfficeAgentProfileID)
			// AC-EXECUTORS-SURVIVAL-002.14: agent identity, agent and
			// continuation commands and their arguments, and the history
			// setting are re-derived from the restored agent profile and the
			// agent-type registry, never read from the instance. When the
			// re-derivation cannot answer (AC-EXECUTORS-SURVIVAL-002.4: the
			// declared source answered and the value is not present -- an
			// unresolvable profile or an agent type no longer in the
			// registry), this instance is authoritatively missing a required
			// value: refuse to re-track it and leave it to the stop path
			// AC-EXECUTORS-SURVIVAL-002.6 defines, rather than tracking a
			// session the backend cannot safely operate.
			if err := m.reDeriveRecoveredAgentIdentity(recoveryCtx, execution); err != nil {
				m.logger.Error("refusing to re-track recovered execution: agent identity could not be reconstructed",
					zap.String("instance_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.String("agent_profile_id", execution.AgentProfileID),
					zap.Error(err))
				m.dispatchUnreconstructableStop(&stopWG, ri)
				continue
			}
			// AC-EXECUTORS-SURVIVAL-004.2/004.5: retrieve this instance's
			// retained turn status before this session's state is published.
			// A read failure (retries exhausted) must not publish the session
			// as running -- it is authoritatively unknown, not running -- so
			// it takes the same not-re-tracked stop path as an
			// unreconstructable agent identity above.
			turnOutcome, turnOutcomeResult := m.retrieveRecoveredTurnOutcome(recoveryCtx, ri)
			if turnOutcomeResult == recoveredTurnOutcomeReadFailed {
				m.logger.Error("refusing to re-track recovered execution: turn status could not be retrieved",
					zap.String("instance_id", execution.ID),
					zap.String("session_id", execution.SessionID))
				m.dispatchUnreconstructableStop(&stopWG, ri)
				continue
			}
			// Create trace span for the recovered session
			_, recoverySpan := tracing.TraceSessionRecovered(
				context.Background(), execution.TaskID, execution.SessionID, execution.ID,
			)
			execution.SetSessionSpan(recoverySpan)
			if ri.Client != nil {
				ri.Client.SetTraceContext(execution.SessionTraceContext())
			}

			// Create short-lived init span so recovery-phase operations are visible
			_, initSpan := tracing.TraceSessionInit(
				execution.SessionTraceContext(), execution.TaskID, execution.SessionID, execution.ID,
			)

			if err := m.executionStore.Add(execution); err != nil {
				// Should not happen at startup — duplicate sessions in the recovery
				// list signal a DB consistency issue, not a normal race. Log loudly
				// and skip; the first one to land wins.
				m.logger.Error("skipping duplicate execution during recovery",
					zap.String("execution_id", execution.ID),
					zap.String("session_id", execution.SessionID),
					zap.Error(err))
				if ri.Client != nil {
					ri.Client.Close()
				}
				execution.EndSessionSpan()
				initSpan.End()
				continue
			}
			m.setRuntimeInterest(execution.SessionID, true)
			// AC-EXECUTORS-SURVIVAL-003.1: this execution is durably in the
			// store as of the Add above, so its session is re-tracked from
			// this point on regardless of which branch below applies the
			// retained turn outcome or publishes running.
			m.markSessionRetracked(execution.SessionID)

			// Reconcile the persistence row to match the recovered in-memory ID.
			// If executors_running.agent_execution_id had drifted (e.g. from a
			// prior bug or manual edit), the recovered runtime instance is the
			// truth — overwrite the row to match. No-op if already in sync.
			m.persistExecutorRunning(recoveryCtx, execution)

			// Re-seed the base-branch map before reconnecting. A surviving
			// agentctl instance never passes through waitForAgentctlReady, so
			// without this its trackers keep whatever map they had — or none,
			// if it was created by a path that could not supply one — and
			// their diff stats stay pinned to an integration-branch fallback.
			if client, releaseClient := execution.AcquireAgentCtlClient(); client != nil {
				m.pushTaskBaseBranches(recoveryCtx, execution.TaskID, execution.ID, client)
				m.pushTaskComparisonTargets(recoveryCtx, execution.TaskID, execution.ID, client)
				releaseClient()
			}

			// AC-EXECUTORS-SURVIVAL-004.2: apply the retained outcome, or
			// publish running when nothing was retained, before streams
			// reconnect below -- reconnecting is what would eventually
			// deliver the same terminal event live (AC-EXECUTORS-SURVIVAL-
			// 004.3/004.4), so this ordering makes the recovery-time
			// application the one that wins in the common case.
			if turnOutcomeResult == recoveredTurnOutcomeApplied {
				m.applyRecoveredTurnOutcome(recoveryCtx, execution, ri, turnOutcome)
			} else {
				m.publishRecoveredExecutionRunning(recoveryCtx, execution)
			}

			// AC-EXECUTORS-SURVIVAL-003.7/002.8: release this session's guard
			// now that its outcome has actually been published, rather than
			// waiting for every session in this pass to finish (design 02
			// step 7's "releasing each session's guard as its outcome is
			// published"). ReleaseAllExceptRetained below still covers any
			// session a later layer marked retained-unstoppable or
			// stop-in-flight, and any session this loop never reached.
			m.recoveryGuard.Release(execution.SessionID)

			// Reconnect to workspace streams (shell, git, file changes) in background
			// This is needed so shell.input, git status, etc. work after backend restart
			go m.streamManager.ReconnectAll(execution)

			initSpan.End()
		}
		m.logger.Info("recovered executions", zap.Int("count", len(recovered)))
	}
	cancelRecovery()

	// AC-EXECUTORS-SURVIVAL-002.11: every dispatchUnreconstructableStop call
	// above ran on its own goroutine so one record's bounded-retry stop
	// latency never starves another record's reconstruction of this pass's
	// shared deadline budget. Wait for all of them to resolve (each one
	// settles its own session's guard via RetainAsUnstoppable or Release
	// before returning) so Start remains synchronous overall and
	// ReleaseAllExceptRetained below never races a still-resolving stop.
	stopWG.Wait()

	// Recovery is synchronous above: by this point every guarded session's
	// outcome (re-tracked or not) is already decided, so every guard still
	// held for this pass is released now. Most were already released
	// individually inside the loop as each session's outcome was published;
	// this covers a session whose record was skipped by the deadline check,
	// a duplicate-execution anomaly, or anything a later layer marks
	// retained-unstoppable or stop-in-flight (AC-EXECUTORS-SURVIVAL-002.16,
	// AC-EXECUTORS-SURVIVAL-003.7), which ReleaseAllExceptRetained leaves held.
	m.recoveryGuard.ReleaseAllExceptRetained()

	// AC-EXECUTORS-SURVIVAL-003.6: this pass's recovery work (adoption,
	// enumeration, and every recovered instance's reconstruction above) is
	// synchronous and now complete, so a caller outside a reconciliation
	// pass may safely enumerate live standalone instances from this point on.
	m.recoveryComplete.Store(true)

	// Start remote status polling loop for runtimes exposing remote status.
	m.wg.Add(1)
	go m.remoteStatusLoop(ctx)
	m.logger.Info("remote status loop started")
	// Set up callbacks for passthrough mode (using standalone runtime)
	if standaloneRT, err := m.executorRegistry.GetBackend(executor.NameStandalone); err == nil {
		if interactiveRunner := standaloneRT.GetInteractiveRunner(); interactiveRunner != nil {
			// Turn complete callback
			interactiveRunner.SetTurnCompleteCallback(func(sessionID, processID string) {
				m.handlePassthroughTurnComplete(sessionID, processID)
			})

			// Output callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetOutputCallback(func(output *agentctltypes.ProcessOutput) {
				m.handlePassthroughOutput(output)
			})

			// Status callback for standalone passthrough (no WorkspaceTracker)
			interactiveRunner.SetStatusCallback(func(status *agentctltypes.ProcessStatusUpdate) {
				m.handlePassthroughStatus(status)
			})

			m.logger.Info("passthrough callbacks configured")
		}
	}

	return nil
}

// hydrateRecoveredTaskEnvironmentID fills in a recovered execution's
// task-environment identity (AC-EXECUTORS-SURVIVAL-002.14): the declared
// source is the durable store, via the session's own task-environment
// reference, not the adopted instance -- ExecutorInstance carries no such
// field at all. Task-environment identity is one of AC-EXECUTORS-SURVIVAL-002.3's
// required reconstructed values, so unlike an earlier version of this method
// (which logged and ignored both failure shapes below), neither is silently
// swallowed: a lookup failure is reported to the caller as an
// AC-EXECUTORS-SURVIVAL-002.13 failed read, and a durable store that answers
// successfully with no value at all is the AC-EXECUTORS-SURVIVAL-002.4
// authoritatively-absent case. A nil provider is a capability this
// deployment never wired at all -- every production backend wires one via
// SetWorkspaceInfoProvider, only bare-bones test doubles don't -- so it is
// treated like any other absent optional recovery capability: skip, not
// refuse.
func (m *Manager) hydrateRecoveredTaskEnvironmentID(ctx context.Context, execution *AgentExecution) error {
	if m.workspaceInfoProvider == nil {
		return nil
	}
	info, err := m.workspaceInfoProvider.GetWorkspaceInfoForSession(ctx, execution.TaskID, execution.SessionID)
	if err != nil {
		return fmt.Errorf("task environment identity read failed: %w", err)
	}
	if info == nil || info.TaskEnvironmentID == "" {
		return errors.New("task environment identity: durable store answered with no value")
	}
	execution.TaskEnvironmentID = info.TaskEnvironmentID
	return nil
}

// reDeriveRecoveredAgentIdentity implements AC-EXECUTORS-SURVIVAL-002.14's
// last reconstruction-table row: agent identity, agent and continuation
// commands and their arguments, and the history setting are re-derived from
// the restored agent profile and the agent-type registry -- the same
// computation an ordinary launch performs, never read from the adopted
// instance, so a compromised or stale instance cannot tell the backend which
// command to run for the session's next turn.
//
// Re-derivation reflects the profile and registry as they stand at recovery,
// not as they stood before the restart (AC-EXECUTORS-SURVIVAL-002.3): a
// profile edited or an agent type removed during the outage takes effect
// exactly as it would on the session's next ordinary turn.
//
// getAgentConfigForExecution and buildFreshAgentCommand are the same
// primitives an in-session context reset uses (manager_interaction.go's
// prepareAgentRestart), reused as-is: both already consume nothing but
// execution fields already restored by this point (AgentProfileID,
// RuntimeName, metadata), with no launch-request-only dependency.
func (m *Manager) reDeriveRecoveredAgentIdentity(ctx context.Context, execution *AgentExecution) error {
	agentConfig, err := m.getAgentConfigForExecution(execution)
	if err != nil {
		return fmt.Errorf("agent identity: %w", err)
	}
	execution.AgentID = agentConfig.ID()
	if rt := agentConfig.Runtime(); rt != nil {
		execution.historyEnabled = rt.SessionConfig.HistoryContextInjection ||
			rt.SessionConfig.NewSessionOnWorkspaceRebind
	}
	commands, err := m.buildFreshAgentCommand(ctx, execution, agentConfig)
	if err != nil {
		return fmt.Errorf("agent and continuation commands: %w", err)
	}
	execution.AgentCommand = commands.initial
	execution.ContinueCommand = commands.continue_
	execution.AgentArgs = commands.args
	execution.ContinueArgs = commands.continueArgs
	return nil
}

// recoveryStopper is the optional capability satisfied by *StandaloneExecutor
// that stops a recovered instance within the bounded per-attempt timeout and
// retry count AC-EXECUTORS-SURVIVAL-002.15 requires for every recovery-time
// stop -- the same budget stopWithRetry already applies to the
// AC-EXECUTORS-SURVIVAL-002.6/002.10 loser/orphan stop paths in
// StandaloneExecutor.RecoverInstances. A backend that doesn't implement it
// falls back to a single StopInstance call: AC-EXECUTORS-SURVIVAL-002.15's
// retry contract covers the WORKTREE and LOCAL executor types only, both
// standalone-runtime.
type recoveryStopper interface {
	stopWithRetry(ctx context.Context, instanceID string) error
}

// dispatchUnreconstructableStop starts stopUnreconstructableRecoveredInstance
// on its own goroutine and registers it on wg, so one record's bounded-retry
// stop latency cannot block the caller's loop from reconstructing every
// other record within the same AC-EXECUTORS-SURVIVAL-003.7 shared deadline
// (AC-EXECUTORS-SURVIVAL-002.11: "no such instance's outcome shall depend on
// another's"). MarkStopInFlight runs synchronously first, before the loop
// can reach ReleaseAllExceptRetained, so this session's guard survives the
// bulk release until stopUnreconstructableRecoveredInstance resolves it
// (Release on success, RetainAsUnstoppable on failure).
func (m *Manager) dispatchUnreconstructableStop(wg *sync.WaitGroup, ri *ExecutorInstance) {
	m.recoveryGuard.MarkStopInFlight(ri.SessionID)
	wg.Add(1)
	go func() {
		defer wg.Done()
		m.stopUnreconstructableRecoveredInstance(context.Background(), ri)
	}()
}

// stopUnreconstructableRecoveredInstance implements the
// AC-EXECUTORS-SURVIVAL-002.4 refusal path's "leave the instance to the stop
// path defined by AC-EXECUTORS-SURVIVAL-002.6" for an instance whose agent
// identity could not be re-derived: it was never added to the execution
// store, so RemoveExecution's teardown does not apply -- stop the live
// instance directly through its owning runtime backend. Run against
// context.Background(), matching every sibling AC-002.6/002.10 recovery stop
// (executor_standalone.go's dispatchRecoveryStops): an elapsed recovery
// deadline (AC-EXECUTORS-SURVIVAL-003.7) must not abort a stop already in
// flight. Every return path resolves this session's recovery guard
// (AC-EXECUTORS-SURVIVAL-002.16): a live instance this call could not stop
// (no backend for its runtime, or the stop itself failed) retains the guard
// for the rest of this backend's lifetime rather than letting it fall
// through to a bulk release that would let a fresh launch race the still-live
// instance; the instance is left to the existing stale-execution repair path
// like any other recovery-time stop failure that isn't part of the joint
// winner/loser accounting.
func (m *Manager) stopUnreconstructableRecoveredInstance(ctx context.Context, ri *ExecutorInstance) {
	backend, err := m.executorRegistry.GetBackend(ri.RuntimeName)
	if err != nil {
		m.logger.Warn("cannot stop unreconstructable recovered instance: no backend for its runtime",
			zap.String("instance_id", ri.InstanceID),
			zap.String("session_id", ri.SessionID),
			zap.String("runtime", string(ri.RuntimeName)),
			zap.Error(err))
		m.recoveryGuard.RetainAsUnstoppable(ri.SessionID)
		return
	}
	if stopper, ok := backend.(recoveryStopper); ok {
		if err := stopper.stopWithRetry(context.Background(), ri.StandaloneInstanceID); err != nil {
			m.logger.Warn("failed to stop unreconstructable recovered instance after exhausting retries",
				zap.String("instance_id", ri.InstanceID),
				zap.String("session_id", ri.SessionID),
				zap.Error(err))
			m.recoveryGuard.RetainAsUnstoppable(ri.SessionID)
			return
		}
		m.recoveryGuard.Release(ri.SessionID)
		return
	}
	if err := backend.StopInstance(ctx, ri, true); err != nil {
		m.logger.Warn("failed to stop unreconstructable recovered instance",
			zap.String("instance_id", ri.InstanceID),
			zap.String("session_id", ri.SessionID),
			zap.Error(err))
		m.recoveryGuard.RetainAsUnstoppable(ri.SessionID)
		return
	}
	m.recoveryGuard.Release(ri.SessionID)
}

// defaultRecoveryDeadline is the AC-EXECUTORS-SURVIVAL-003.7 fallback used
// when no recovery deadline was configured (recoveryDeadline left zero).
const defaultRecoveryDeadline = 30 * time.Second

// recoveryDeadlineDeadline resolves the wall-clock instant this startup
// pass's recovery work must stop admitting new work by, per
// AC-EXECUTORS-SURVIVAL-003.7.
func (m *Manager) recoveryDeadlineDeadline() time.Time {
	start := m.recoveryDeadlineStart
	if start.IsZero() {
		start = time.Now()
	}
	period := m.recoveryDeadline
	if period <= 0 {
		period = defaultRecoveryDeadline
	}
	return start.Add(period)
}

// sessionIDsFromExecutorRunning extracts the distinct, non-empty session
// identities named by a set of recovery-inventory records.
func sessionIDsFromExecutorRunning(records []*models.ExecutorRunning) []string {
	sessionIDs := make([]string, 0, len(records))
	for _, rec := range records {
		if rec != nil && rec.SessionID != "" {
			sessionIDs = append(sessionIDs, rec.SessionID)
		}
	}
	return sessionIDs
}

// GetRecoveredExecutions returns a snapshot of all currently tracked executions
// This can be used by the orchestrator to sync with the database
func (m *Manager) GetRecoveredExecutions() []RecoveredExecution {
	executions := m.executionStore.List()
	result := make([]RecoveredExecution, 0, len(executions))
	for _, exec := range executions {
		result = append(result, RecoveredExecution{
			ExecutionID:        exec.ID,
			TaskID:             exec.TaskID,
			SessionID:          exec.SessionID,
			ContainerID:        exec.ContainerID,
			AgentProfileID:     exec.officeProfileID(),
			ExecutionProfileID: exec.AgentProfileID,
		})
	}
	return result
}

// IsShuttingDown reports whether graceful shutdown has begun. Set by
// StopAllAgents before it starts tearing down executions so concurrent
// handlers (e.g. passthrough exit auto-restart, agentctl HTTP calls) can
// skip or downgrade work that would otherwise race the teardown.
func (m *Manager) IsShuttingDown() bool {
	return m.shuttingDown.Load()
}

// closeStopCh closes the manager shutdown channel at most once.
func (m *Manager) closeStopCh() {
	m.stopOnce.Do(func() { close(m.stopCh) })
}

// Stop stops the lifecycle manager and releases resources held by executors.
func (m *Manager) Stop() error {
	m.logger.Info("stopping lifecycle manager")

	m.closeStopCh()
	if m.streamManager != nil {
		m.streamManager.Wait()
	}
	m.wg.Wait()

	// Close executor backends that hold resources (e.g., Docker SDK client).
	if m.executorRegistry != nil {
		m.executorRegistry.CloseAll()
	}

	return nil
}

// StopReasonBackendShutdown identifies the graceful backend-shutdown path.
// Executors may preserve resumable remote state for this reason only; user
// stops and rollback cleanup deliberately retain their destructive semantics.
const StopReasonBackendShutdown = "backend shutdown"

// StopAllAgents attempts a graceful shutdown of all active agents concurrently.
func (m *Manager) StopAllAgents(ctx context.Context) error {
	m.shuttingDown.Store(true)

	executions := m.executionStore.List()
	if len(executions) == 0 {
		return nil
	}

	var wg sync.WaitGroup
	errCh := make(chan error, len(executions))

	for _, exec := range executions {
		wg.Add(1)
		go func(e *AgentExecution) {
			defer wg.Done()
			if err := m.StopAgentWithReason(ctx, e.ID, StopReasonBackendShutdown, false); err != nil {
				errCh <- err
				m.logger.Warn("failed to stop agent during shutdown",
					zap.String("execution_id", e.ID),
					zap.Error(err))
			}
		}(exec)
	}

	wg.Wait()
	close(errCh)

	var errs []error
	for err := range errCh {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

const stopReasonStaleExecutionCleanup = "stale execution cleanup"

// CleanupStaleExecutionBySessionID cleans up a stale execution: stops the runtime
// instance, closes the client connection, and removes it from tracking.
//
// A "stale" execution is one where the agent process has stopped externally (crashed, killed,
// or terminated outside of our control) but the execution is still tracked in memory.
//
// When to use this:
//   - After detecting the agentctl HTTP server is unreachable
//   - When the agent container no longer exists (Docker runtime)
//   - After server restart when recovering persisted state
//   - When IsAgentRunningForSession returns false but execution exists
//
// This method performs cleanup:
//  1. Stops the runtime instance (workspace tracker, shell, etc.) via the executor backend
//  2. Closes the agentctl HTTP client connection
//  3. Removes the execution from the in-memory tracking store
//
// What this does NOT do:
//   - Clean up worktrees or containers (caller's responsibility)
//   - Update database session state (caller's responsibility)
//
// Safe to call even if the process is already stopped — StopInstance is idempotent.
//
// Returns nil if no execution exists for the session (idempotent).
func (m *Manager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists {
		return nil // No execution to clean up
	}
	return m.cleanupStaleExecution(ctx, execution)
}

// CleanupStaleExecutionBySessionIDIfCurrent cleans up only the execution that
// the caller observed. The operation shares the session-keyed singleflight
// used by launch paths, so a successor launch either wins before cleanup or
// runs after the stale execution has been removed. The persisted timestamp
// check closes the remaining same-ID workspace-promotion window.
func (m *Manager) CleanupStaleExecutionBySessionIDIfCurrent(
	ctx context.Context,
	sessionID, expectedExecutionID string,
	expectedUpdatedAt time.Time,
) error {
	if sessionID == "" || expectedExecutionID == "" {
		return nil
	}
	_, err := m.doCoalescedExecution(ctx, sessionID, func(sharedCtx context.Context) (interface{}, error) {
		execution, exists := m.executionStore.GetBySessionID(sessionID)
		if !exists || execution == nil || execution.ID != expectedExecutionID {
			return nil, nil
		}
		if reader, ok := m.runningWriter.(executorRunningReader); ok && !expectedUpdatedAt.IsZero() {
			current, readErr := reader.GetExecutorRunningBySessionID(sharedCtx, sessionID)
			if readErr != nil || current == nil || current.AgentExecutionID != expectedExecutionID ||
				!current.UpdatedAt.Equal(expectedUpdatedAt) {
				return nil, nil
			}
		}
		return nil, m.cleanupStaleExecution(sharedCtx, execution)
	})
	return err
}

func (m *Manager) cleanupStaleExecution(ctx context.Context, execution *AgentExecution) error {
	if execution == nil {
		return nil
	}
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	execution.agentctlLifecycleMu.Lock()
	sessionID := execution.SessionID

	m.logger.Info("cleaning up stale agent execution",
		zap.String("session_id", sessionID),
		zap.String("execution_id", execution.ID))

	// Stop the runtime instance (workspace tracker, shell, etc.) to prevent leaked
	// goroutines. Without this, the old agentctl instance keeps running when a new
	// execution is created for the same session, causing git polling on deleted worktrees.
	// This is idempotent — returns success if the instance is already gone.
	if err := m.stopAgentViaBackend(ctx, execution.ID, execution, stopReasonStaleExecutionCleanup, false, false); err != nil {
		execution.agentctlLifecycleMu.Unlock()
		return fmt.Errorf("stop stale runtime %s: %w", execution.ID, err)
	}

	// End session trace span only after the runtime has stopped. A failed stop
	// remains retryable and should keep the execution's lifecycle intact.
	execution.EndSessionSpan()

	// Close agentctl connection if it exists
	if client := execution.currentAgentCtlClient(); client != nil { // protected by agentctlLifecycleMu
		client.Close()
	}
	execution.detachAgentctlClient()
	execution.agentctlLifecycleMu.Unlock()

	// Remove from execution store
	m.RemoveExecution(execution.ID)

	// Delete the persistence row in lockstep with store removal so we never
	// leave a phantom executors_running row pointing at a non-existent
	// execution. Best-effort; "not found" is expected and silently swallowed.
	m.deleteExecutorRunning(ctx, sessionID, execution.ID)

	return nil
}

// RemoveExecution removes an execution from tracking.
//
// ⚠️  WARNING: This is a potentially dangerous operation that should only be called when:
//  1. The agent process has been fully stopped (via StopAgent)
//  2. All cleanup operations have completed (worktree cleanup, container removal)
//  3. The execution is in a terminal state (Completed, Failed, or Cancelled)
//
// This method:
//   - Removes the execution from the in-memory store
//   - Makes the sessionID available for new executions
//   - Does NOT stop the agent process (call StopAgent first)
//   - Does NOT close the agentctl client (use the execution lifecycle lock first)
//   - Does NOT clean up resources (worktrees, containers, etc.)
//
// After calling this, the executionID and sessionID can no longer be used to query
// or control the execution. Any references to this execution will become invalid.
//
// Typical usage: Called by cleanup loops or after successful StopAgent completion.
// For stale/dead executions, use CleanupStaleExecutionBySessionID instead.
func (m *Manager) RemoveExecution(executionID string) {
	m.releaseActivity(executionActivityKey(executionID))
	if execution, ok := m.executionStore.Get(executionID); ok {
		m.closeStreamCoalescer(execution)
		m.cleanupPassthroughMCPConfig(execution)
		m.setRuntimeInterest(execution.SessionID, false)
	}
	m.executionStore.Remove(executionID)
	m.logger.Debug("removed execution from tracking",
		zap.String("execution_id", executionID))
}

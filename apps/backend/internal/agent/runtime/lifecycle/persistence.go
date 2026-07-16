package lifecycle

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ExecutorRunningWriter is the narrow persistence interface the lifecycle manager
// uses to keep the executors_running table in lockstep with the in-memory
// ExecutionStore. It is the *only* component allowed to write
// executors_running.agent_execution_id / container_id / runtime / status —
// orchestrator-side flows that need to update the row use the narrow setters
// (UpdateResumeToken with CAS) which can't clobber lifecycle-owned columns.
//
// This split is the structural fix for the agent-execution-id divergence bug:
// previously the orchestrator persisted execution_id in three tables via full-row
// UPDATEs, racing with the in-memory store and producing phantom IDs. After this
// refactor, executors_running is the single source of truth and the lifecycle
// manager owns its lifecycle.
type ExecutorRunningWriter interface {
	// UpsertExecutorRunning inserts or updates the row. Caller passes a fully
	// populated *models.ExecutorRunning; the underlying SQL preserves nothing
	// from the prior row (idempotent re-creation on every successful Add).
	UpsertExecutorRunning(ctx context.Context, running *models.ExecutorRunning) error

	// DeleteExecutorRunningBySessionID removes the row when an execution is
	// torn down. Idempotent: no-op if no row exists.
	DeleteExecutorRunningBySessionID(ctx context.Context, sessionID string) error

	// RepairExecutorRunningDead repairs a row in place (status=stopped, local_pid
	// cleared, last_seen re-stamped) while preserving resume_token/worktree. Used
	// instead of deletion when a stale-cleanup would otherwise destroy a resumable
	// row (#1597 resume-safety invariant). Idempotent-friendly: returns
	// ErrExecutorRunningNotFound if no row exists.
	RepairExecutorRunningDead(ctx context.Context, sessionID string) error
}

// SetExecutorRunningWriter wires the writer used to persist row state in
// lockstep with executionStore.Add / Remove. Must be called during DI before
// any Launch / createExecution can run, otherwise the in-memory store will
// drift from the DB and the divergence bug returns.
//
// Optional only for tests that don't exercise the persistence path.
func (m *Manager) SetExecutorRunningWriter(w ExecutorRunningWriter) {
	m.runningWriter = w
}

// buildRunningFromExecution maps an in-memory execution into the persistence
// shape. Used at every executionStore.Add success site so the DB row is always
// derived from the same source of truth as the store.
//
// Carries forward resume_token / last_message_uuid / metadata from any
// pre-existing row (a previous run's resume state) so the lifecycle write
// doesn't clobber data the orchestrator's narrow CAS update wrote earlier.
func buildRunningFromExecution(execution *AgentExecution, prior *models.ExecutorRunning) *models.ExecutorRunning {
	agentctlURL := execution.AgentctlURL()
	agentctlPort := agentctlPortFromExecution(execution, agentctlURL)
	pid := agentctlPIDFromExecution(execution)
	var lastSeenAt *time.Time
	if agentctlURL != "" || agentctlPort > 0 || pid > 0 {
		now := time.Now().UTC()
		lastSeenAt = &now
	}

	running := &models.ExecutorRunning{
		ID:                 execution.SessionID,
		SessionID:          execution.SessionID,
		TaskID:             execution.TaskID,
		ExecutionProfileID: execution.AgentProfileID,
		Runtime:            execution.RuntimeName,
		Status:             executorRunningStatusFromExecution(execution),
		Resumable:          true,
		AgentExecutionID:   execution.ID,
		ContainerID:        execution.ContainerID,
		AgentctlURL:        agentctlURL,
		AgentctlPort:       agentctlPort,
		PID:                pid,
		WorktreeID:         getMetadataString(execution.Metadata, MetadataKeyWorktreeID),
		WorktreePath:       getMetadataString(execution.Metadata, "worktree_path"),
		WorktreeBranch:     getMetadataString(execution.Metadata, MetadataKeyWorktreeBranch),
		Metadata:           FilterPersistentMetadata(execution.Metadata),
		LastSeenAt:         lastSeenAt,
	}
	if prior != nil {
		running.ExecutorID = prior.ExecutorID
		if prior.ExecutionProfileID == execution.AgentProfileID {
			running.ResumeToken = prior.ResumeToken
			running.LastMessageUUID = prior.LastMessageUUID
		}
		// Preserve metadata keys the orchestrator owns (context_window, prepare_result, etc.)
		// by merging prior metadata under our own keys. FilterPersistentMetadata above stripped
		// transient lifecycle-only keys; the prior row's metadata has the orchestrator-owned
		// keys we want to carry forward.
		if running.Metadata == nil {
			running.Metadata = make(map[string]interface{})
		}
		for k, v := range prior.Metadata {
			if _, ok := running.Metadata[k]; !ok {
				running.Metadata[k] = v
			}
		}
	}
	return running
}

func agentctlPortFromExecution(execution *AgentExecution, agentctlURL string) int {
	if execution == nil {
		return 0
	}
	if execution.standalonePort > 0 {
		return execution.standalonePort
	}
	if port := metadataInt(execution.Metadata, MetadataKeySSHLocalForwardPort); port > 0 {
		return port
	}
	if port := metadataInt(execution.Metadata, MetadataKeyLocalPort); port > 0 {
		return port
	}
	if agentctlURL == "" {
		return 0
	}
	parsed, err := url.Parse(agentctlURL)
	if err != nil {
		return 0
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil {
		return 0
	}
	return port
}

// resolveLocalPID returns the host-local liveness handle for an execution's
// executors_running row. For local/standalone runtimes this is the standalone
// agentctl control-server PID Kandev spawned on this host; for every other
// runtime it is 0. SSH/remote processes live on another host (tracked via the
// remote-host PID column) and docker processes live in a container, so this
// never returns a pid for a non-local runtime — a local-process liveness check
// can therefore never run against a remote row (#1597 runtime-aware liveness).
func (m *Manager) resolveLocalPID(execution *AgentExecution) int {
	if execution == nil {
		return 0
	}
	if execution.RuntimeName == agentruntime.RuntimeStandalone {
		return int(m.standaloneHostPID.Load())
	}
	return 0
}

func agentctlPIDFromExecution(execution *AgentExecution) int {
	if execution == nil {
		return 0
	}
	return metadataInt(execution.Metadata, MetadataKeySSHRemoteAgentctlPID)
}

func metadataInt(metadata map[string]interface{}, key string) int {
	if metadata == nil {
		return 0
	}
	switch v := metadata[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// isTerminalExecutorRunningStatus reports whether a row status (as produced by
// executorRunningStatusFromExecution) marks the execution as finished — the
// states in which the row must not carry a live local liveness handle.
func isTerminalExecutorRunningStatus(status string) bool {
	switch status {
	case models.ExecutorRunningStatusFailed,
		models.ExecutorRunningStatusStopped,
		models.ExecutorRunningStatusComplete:
		return true
	default:
		return false
	}
}

func executorRunningStatusFromExecution(execution *AgentExecution) string {
	if execution == nil {
		return models.ExecutorRunningStatusStarting
	}
	switch execution.Status {
	case v1.AgentStatusRunning:
		return models.ExecutorRunningStatusRunning
	case v1.AgentStatusReady:
		return models.ExecutorRunningStatusReady
	case v1.AgentStatusFailed:
		return models.ExecutorRunningStatusFailed
	case v1.AgentStatusStopped:
		return models.ExecutorRunningStatusStopped
	case v1.AgentStatusCompleted:
		return models.ExecutorRunningStatusComplete
	default:
		return models.ExecutorRunningStatusStarting
	}
}

// persistExecutorRunning writes the executors_running row for an execution that
// was just successfully Add'd to the in-memory store. Called from createExecution
// and Launch immediately after a successful Add so the DB row is created in
// lockstep with the in-memory entry.
//
// Carries forward resume_token / metadata from any prior row so an in-flight
// resume token written by a previous execution's storeResumeToken handler is
// not lost. The lifecycle manager owns agent_execution_id / container_id /
// runtime / status; the orchestrator owns resume_token / last_message_uuid /
// metadata.context_window via narrow CAS updates.
//
// Logs and continues on persistence failure rather than failing the launch —
// the in-memory store already has the truth, and the row will be re-upserted
// on the next launch through this same path. (storeResumeToken does NOT
// re-create a missing row; it uses a narrow CAS UPDATE keyed on
// agent_execution_id, so a failure here leaves resume_token persistence broken
// until the next full launch.) The store is the runtime authority; the row is
// its durable mirror.
func (m *Manager) persistExecutorRunning(ctx context.Context, execution *AgentExecution) {
	if m.runningWriter == nil {
		// Permitted in tests that don't exercise persistence; logged so a
		// missed wire-up in production stands out.
		m.logger.Debug("no executor-running writer configured; skipping row persistence",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID))
		return
	}

	// Best-effort read of any pre-existing row so we carry forward the orchestrator-
	// owned columns. A prior row exists when: (a) backend is restarting and a
	// recovered execution is being persisted, or (b) a session is being re-launched
	// after a fresh-fallback cleanup that did NOT delete the row.
	//
	// The upsert overwrites resume_token / last_message_uuid with excluded values,
	// so those are safe to write only when we know their current values — i.e. when
	// the prior read succeeded or positively confirmed no row exists. If the read
	// itself FAILS (transient DB error, not "not found"), we must NOT proceed: a
	// blind upsert would blank a live resume_token, costing the session its resume
	// ability (#1597 resume-safety invariant). Skipping is fail-safe — the row keeps
	// its current columns and the next transition (or reconciliation) re-persists.
	var prior *models.ExecutorRunning
	if reader, ok := m.runningWriter.(executorRunningReader); ok {
		existing, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID)
		switch {
		case err == nil:
			prior = existing
		case errors.Is(err, models.ErrExecutorRunningNotFound):
			// No prior row — first insert; nothing to carry forward.
		default:
			m.logger.Warn("skipping executors_running upsert: prior-row read failed, refusing to risk clobbering resume_token",
				zap.String("execution_id", execution.ID),
				zap.String("session_id", execution.SessionID),
				zap.Error(err))
			return
		}
	}

	running := buildRunningFromExecution(execution, prior)
	// Attach the host-local liveness handle for local/standalone rows. Kept out
	// of buildRunningFromExecution (a pure mapper) because the PID lives on the
	// manager, wired from the agentctl launcher at DI. resolveLocalPID returns 0
	// for non-local runtimes, so a local-process check can never target a remote
	// (SSH/docker) row. A terminal row carries no handle: for standalone the
	// resolved PID is the shared agentctl control server, which outlives the
	// session, and a completed/failed/stopped row must not claim a live process
	// (#1597 truthful executor rows) — matching RepairExecutorRunningDead.
	if !isTerminalExecutorRunningStatus(running.Status) {
		running.LocalPID = m.resolveLocalPID(execution)
	}
	// last_seen_at reflects an actual liveness observation: every hooked
	// transition re-stamps it. buildRunningFromExecution already stamps when a
	// live endpoint is known; also stamp when we only have a local handle so a
	// local row is never left with a NULL last_seen_at once populated.
	if running.LastSeenAt == nil && running.LocalPID > 0 {
		now := time.Now().UTC()
		running.LastSeenAt = &now
	}
	if err := m.runningWriter.UpsertExecutorRunning(ctx, running); err != nil {
		m.logger.Error("failed to persist executors_running row in lockstep with store",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID),
			zap.Error(err))
	}
}

// deleteExecutorRunning tears down the persistence row when an execution is
// cleaned up (CleanupStaleExecutionBySessionID). Called after executionStore.Remove
// so the in-memory and persistent state are gone in the same operation.
//
// Resume-safety invariant (#1597 resume-safety invariant): a row that still holds
// a resume_token is REPAIRED in place (status=stopped, local_pid cleared) rather
// than deleted, so a session stays resumable even if a subsequent relaunch fails.
// On the happy path the relaunch UPSERTs a fresh row over the repaired one, so
// repairing costs nothing; on the failure path it preserves the only handle to a
// resumable conversation. Rows with no resume_token are deleted as before.
//
// Deliberate deviation from models.RowMustBePreserved, which also preserves
// tokenless rows backing a non-terminal session: this path gates on the token
// alone because the token IS the resumable agent state. A row without one means
// the agent never established (or never reported) an ACP session — there is no
// agent-side context a preserved row could resume, and Kandev's own chat
// history lives in the task tables, untouched by this delete. Preserving a
// tokenless row here would keep only incidental metadata that the relaunch
// upsert rebuilds anyway, at the cost of wiring session-state reads into the
// lifecycle tier. Orchestrator-side reconciliation, which already knows session
// state, applies the full invariant via pruneOrRepairExecutorRow.
//
// Best-effort: a failure here is logged but doesn't propagate.
func (m *Manager) deleteExecutorRunning(ctx context.Context, sessionID string) {
	if m.runningWriter == nil {
		return
	}

	// Inspect the row first so we never delete one we couldn't read (fail-safe:
	// an unreadable row might hold a resume_token).
	if reader, ok := m.runningWriter.(executorRunningReader); ok {
		existing, err := reader.GetExecutorRunningBySessionID(ctx, sessionID)
		switch {
		case err == nil && existing != nil && existing.ResumeToken != "":
			if repairErr := m.runningWriter.RepairExecutorRunningDead(ctx, sessionID); repairErr != nil &&
				!errors.Is(repairErr, models.ErrExecutorRunningNotFound) {
				m.logger.Warn("failed to repair resumable executors_running row on cleanup; leaving row intact",
					zap.String("session_id", sessionID),
					zap.Error(repairErr))
			} else {
				m.logger.Info("repaired resumable executors_running row instead of deleting (resume-safety invariant)",
					zap.String("session_id", sessionID))
			}
			return
		case errors.Is(err, models.ErrExecutorRunningNotFound):
			m.logger.Debug("delete executors_running on cleanup: row not found",
				zap.String("session_id", sessionID))
			return
		case err != nil:
			m.logger.Warn("skipping executors_running delete: prior-row read failed, refusing to risk deleting a resumable row",
				zap.String("session_id", sessionID),
				zap.Error(err))
			return
		}
	} else {
		m.logger.Warn("delete executors_running on cleanup: writer does not support reading; resume-safety check skipped",
			zap.String("session_id", sessionID))
	}

	if err := m.runningWriter.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
		// "not found" is expected for sessions that were never launched; everything
		// else is a real I/O failure (write timeout, locked DB) and should surface.
		if errors.Is(err, models.ErrExecutorRunningNotFound) {
			m.logger.Debug("delete executors_running on cleanup: row not found",
				zap.String("session_id", sessionID))
		} else {
			m.logger.Warn("delete executors_running on cleanup",
				zap.String("session_id", sessionID),
				zap.Error(err))
		}
	}
}

// executorRunningReader is the optional read-side of the writer used to fetch
// a prior row before re-upserting. Implementing it lets the writer carry forward
// orchestrator-owned columns; not implementing it just means we always write
// fresh state (acceptable for first-time inserts).
type executorRunningReader interface {
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
}

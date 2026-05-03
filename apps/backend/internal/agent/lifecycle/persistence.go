package lifecycle

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
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
	running := &models.ExecutorRunning{
		ID:               execution.SessionID,
		SessionID:        execution.SessionID,
		TaskID:           execution.TaskID,
		Runtime:          execution.RuntimeName,
		Status:           "starting",
		Resumable:        true,
		AgentExecutionID: execution.ID,
		ContainerID:      execution.ContainerID,
		WorktreeID:       getMetadataString(execution.Metadata, MetadataKeyWorktreeID),
		WorktreePath:     getMetadataString(execution.Metadata, "worktree_path"),
		WorktreeBranch:   getMetadataString(execution.Metadata, MetadataKeyWorktreeBranch),
		Metadata:         FilterPersistentMetadata(execution.Metadata),
	}
	if prior != nil {
		running.ExecutorID = prior.ExecutorID
		running.ResumeToken = prior.ResumeToken
		running.LastMessageUUID = prior.LastMessageUUID
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
// the in-memory store already has the truth, and the row will be written
// correctly on the next Upsert call (e.g., from storeResumeToken). The store
// is the runtime authority; the row is its durable mirror.
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
	var prior *models.ExecutorRunning
	if reader, ok := m.runningWriter.(executorRunningReader); ok {
		if existing, err := reader.GetExecutorRunningBySessionID(ctx, execution.SessionID); err == nil {
			prior = existing
		}
	}

	running := buildRunningFromExecution(execution, prior)
	if err := m.runningWriter.UpsertExecutorRunning(ctx, running); err != nil {
		m.logger.Error("failed to persist executors_running row in lockstep with store",
			zap.String("execution_id", execution.ID),
			zap.String("session_id", execution.SessionID),
			zap.Error(err))
	}
}

// deleteExecutorRunning removes the persistence row when an execution is torn
// down (CleanupStaleExecutionBySessionID). Called after executionStore.Remove so
// the in-memory and persistent state are gone in the same operation.
//
// Best-effort: a failure here is logged but doesn't propagate. A subsequent
// Launch for the same session will UPSERT and overwrite anyway.
func (m *Manager) deleteExecutorRunning(ctx context.Context, sessionID string) {
	if m.runningWriter == nil {
		return
	}
	if err := m.runningWriter.DeleteExecutorRunningBySessionID(ctx, sessionID); err != nil {
		// "not found" is expected for sessions that were never launched; log
		// at debug. Other errors at warn so they show up in operations.
		m.logger.Debug("delete executors_running on cleanup",
			zap.String("session_id", sessionID),
			zap.Error(err))
	}
}

// executorRunningReader is the optional read-side of the writer used to fetch
// a prior row before re-upserting. Implementing it lets the writer carry forward
// orchestrator-owned columns; not implementing it just means we always write
// fresh state (acceptable for first-time inserts).
type executorRunningReader interface {
	GetExecutorRunningBySessionID(ctx context.Context, sessionID string) (*models.ExecutorRunning, error)
}

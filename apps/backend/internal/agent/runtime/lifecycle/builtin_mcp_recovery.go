package lifecycle

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	p "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

type builtinRecoveryStore interface {
	SessionRecoveryStore
	harnessGenerationReader
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

func (m *Manager) restoreBuiltinExecution(ctx context.Context, execution *AgentExecution) error {
	if _, bound := execution.metadataValue(p.BuiltinBindingMetadataKey); !bound {
		return nil
	}
	err := m.restoreBuiltinOwner(ctx, execution)
	if err == nil {
		err = m.verifyBuiltinExecution(ctx, execution)
	}
	if err == nil {
		return nil
	}
	if blockErr := m.blockBuiltinRecovery(ctx, execution); blockErr != nil {
		return fmt.Errorf("built-in MCP recovery block could not be persisted: %w", errors.Join(err, blockErr))
	}
	return &BuiltinMCPRecoveryError{Cause: err}
}

func (m *Manager) restoreBuiltinOwner(ctx context.Context, execution *AgentExecution) error {
	_, _, scope := m.streamManager.mcpDispatchState()
	if scope == nil {
		return errors.New("built-in MCP principal scope is unavailable")
	}
	scoped, err := scope(ctx, execution.TaskID, execution.SessionID)
	if err != nil {
		return err
	}
	principal, ok := mcpscope.PrincipalFromContext(scoped)
	if !ok || principal.CallerTaskID != execution.TaskID || principal.CallerSessionID != execution.SessionID || principal.WorkspaceID == "" {
		return errors.New("built-in MCP recovery owner is unavailable")
	}
	execution.WorkspaceID = principal.WorkspaceID
	return nil
}

func (m *Manager) blockBuiltinRecovery(ctx context.Context, execution *AgentExecution) error {
	store, ok := m.runningWriter.(builtinRecoveryStore)
	if !ok {
		return errors.New("built-in MCP recovery store is unavailable")
	}
	session, err := store.GetTaskSession(ctx, execution.SessionID)
	if err != nil {
		return err
	}
	if session == nil || session.TaskID != execution.TaskID {
		return errors.New("built-in MCP recovery session owner changed")
	}
	incarnation := session.QueueIncarnationID
	if incarnation == "" {
		incarnation = session.ID
	}
	identity := RestoreIdentity{SessionID: session.ID, IncarnationID: incarnation}
	current, err := store.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnation)
	if err != nil && !errors.Is(err, sql.ErrNoRows) && !errors.Is(err, models.ErrTaskSessionNotFound) {
		return err
	}
	if current != nil {
		identity.HarnessGeneration = uint64(current.Generation)
	}
	_, err = (&RecoveryAdmission{Store: store}).Block(ctx, identity, "builtin_mcp_binding", "builtin_mcp")
	return err
}

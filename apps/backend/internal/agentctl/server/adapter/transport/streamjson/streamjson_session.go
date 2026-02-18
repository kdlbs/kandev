package streamjson

import (
	"context"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"go.uber.org/zap"
)

// emitPendingCommands emits any pending available commands for the given session.
// Must be called after the mutex is unlocked.
func (a *Adapter) emitPendingCommands(sessionID string, commands []streams.AvailableCommand) {
	if len(commands) == 0 {
		return
	}
	a.sendUpdate(AgentEvent{
		Type:              streams.EventTypeAvailableCommands,
		SessionID:         sessionID,
		AvailableCommands: commands,
	})
	a.logger.Debug("emitted pending slash commands",
		zap.String("session_id", sessionID),
		zap.Int("count", len(commands)))
}

// takePendingCommands atomically takes and clears pending commands.
// Must be called with the mutex held.
func (a *Adapter) takePendingCommands() []streams.AvailableCommand {
	commands := a.pendingAvailableCommands
	a.pendingAvailableCommands = nil
	return commands
}

// NewSession creates a new stream-json session.
// Note: Sessions are created implicitly with the first prompt.
// The mcpServers parameter is ignored as this protocol handles MCP separately.
func (a *Adapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
	a.mu.Lock()
	sessionID := uuid.New().String()
	a.sessionID = sessionID
	pendingCommands := a.takePendingCommands()
	a.mu.Unlock()

	a.logger.Info("created new session placeholder", zap.String("session_id", sessionID))
	a.emitPendingCommands(sessionID, pendingCommands)

	return sessionID, nil
}

// LoadSession resumes an existing stream-json session.
// The session ID will be passed to the agent via --resume flag (handled by process manager).
func (a *Adapter) LoadSession(ctx context.Context, sessionID string) error {
	a.mu.Lock()
	a.sessionID = sessionID
	pendingCommands := a.takePendingCommands()
	a.mu.Unlock()

	a.logger.Info("loaded session", zap.String("session_id", sessionID))
	a.emitPendingCommands(sessionID, pendingCommands)

	return nil
}

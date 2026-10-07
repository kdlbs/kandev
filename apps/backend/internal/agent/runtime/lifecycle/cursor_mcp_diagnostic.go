package lifecycle

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

func (m *Manager) logCursorMCPDiagnostic(execution *AgentExecution, recorder *prepareProgressRecorder, serverID string, diagnostic *mcpconfig.NativeMCPDiagnostic) {
	diagnostic = normalizeCursorMCPDiagnostic(diagnostic)
	if m == nil || m.logger == nil || execution == nil || diagnostic == nil {
		return
	}
	preparationID := ""
	if recorder != nil {
		preparationID = recorder.preparationID
	}
	fields := []zap.Field{
		zap.String("task_id", execution.TaskID),
		zap.String("session_id", execution.SessionID),
		zap.String("preparation_id", preparationID),
		zap.String("server_id", serverID),
		zap.String("operation", string(diagnostic.Operation)),
		zap.String("stage", string(diagnostic.Stage)),
		zap.String("kind", string(diagnostic.Kind)),
		zap.String("message", diagnostic.Message),
	}
	if diagnostic.ExitCode != nil {
		fields = append(fields, zap.Int("exit_code", *diagnostic.ExitCode))
	}
	if diagnostic.CleanupMessage != "" {
		fields = append(fields, zap.String("cleanup_message", diagnostic.CleanupMessage))
	}
	m.logger.Zap().Warn("native MCP preparation failed", fields...)
}

func normalizeCursorMCPDiagnostic(diagnostic *mcpconfig.NativeMCPDiagnostic) *mcpconfig.NativeMCPDiagnostic {
	if diagnostic == nil {
		return nil
	}
	return mcpconfig.NormalizeNativeMCPDiagnostic(diagnostic, diagnostic.Operation)
}

func cursorMCPFenceDiagnostic(ctx context.Context, diagnostic *mcpconfig.NativeMCPDiagnostic) *mcpconfig.NativeMCPDiagnostic {
	if ctx == nil || ctx.Err() == nil {
		return nil
	}
	return normalizeCursorMCPDiagnostic(diagnostic)
}

func cursorMCPFenceReadiness(ctx context.Context, diagnostic *mcpconfig.NativeMCPDiagnostic) mcpconfig.NativeMCPReadiness {
	readiness := cursorMCPUnavailableReadiness()
	readiness.Diagnostic = cursorMCPFenceDiagnostic(ctx, diagnostic)
	return readiness
}

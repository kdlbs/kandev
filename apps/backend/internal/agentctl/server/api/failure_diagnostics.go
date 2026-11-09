package api

import (
	"context"
	"errors"
	"strings"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/fsdiagnostics"
)

const (
	diagnosticOperationAgentConfigure = "agent_configure"
	diagnosticOperationGitInspection  = "git_inspection"

	diagnosticClassAgentRunning     = "agent_running"
	diagnosticClassContextCanceled  = "context_canceled"
	diagnosticClassDeadlineExceeded = "deadline_exceeded"
	diagnosticClassEvidenceChanged  = "evidence_changed"
	diagnosticClassUnknown          = "unknown"
)

// agentConfigureFailureFields returns bounded context for a failed configure
// request. It never carries the raw error, a path, or a command argument.
func (s *Server) agentConfigureFailureFields(err error) []zap.Field {
	fields := []zap.Field{
		zap.String("operation", diagnosticOperationAgentConfigure),
		zap.String("error_class", classifyAgentConfigureError(err)),
	}
	return append(fields, s.diagnosticIdentityFields()...)
}

// gitInspectionFailureFields returns bounded context for a failed Git status
// read, reusing the shared filesystem classifier.
func (s *Server) gitInspectionFailureFields(err error) []zap.Field {
	fields := []zap.Field{
		zap.String("operation", diagnosticOperationGitInspection),
		zap.String("error_class", fsdiagnostics.ErrorClass(err)),
	}
	return append(fields, s.diagnosticIdentityFields()...)
}

func classifyAgentConfigureError(err error) string {
	switch {
	case errors.Is(err, process.ErrConfigureWhileRunning):
		return diagnosticClassAgentRunning
	case errors.Is(err, context.Canceled):
		return diagnosticClassContextCanceled
	case errors.Is(err, context.DeadlineExceeded):
		return diagnosticClassDeadlineExceeded
	default:
		return diagnosticClassUnknown
	}
}

// diagnosticIdentityFields returns the instance's task, session, and instance
// identifiers that are canonical lowercase UUIDs. A missing or malformed
// identity is omitted instead of guessed.
func (s *Server) diagnosticIdentityFields() []zap.Field {
	if s == nil || s.cfg == nil {
		return nil
	}
	candidates := []struct{ key, value string }{
		{"task_id", s.cfg.TaskID},
		{"session_id", s.cfg.SessionID},
		{"instance_id", s.cfg.InstanceID},
	}
	fields := make([]zap.Field, 0, len(candidates))
	for _, candidate := range candidates {
		if isCanonicalUUID(candidate.value) {
			fields = append(fields, zap.String(candidate.key, candidate.value))
		}
	}
	return fields
}

func isCanonicalUUID(value string) bool {
	if len(value) != 36 || strings.ToLower(value) != value {
		return false
	}
	parsed, err := uuid.Parse(value)
	return err == nil && parsed.String() == value
}

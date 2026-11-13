package handlers

import (
	"context"
	"github.com/kandev/kandev/internal/agent/usage"
)

func (h *Handlers) withCandidateUsage(ctx context.Context, sessionID string) context.Context {
	if sessionID == "" {
		if reader, ok := h.hostExecutor.(interface {
			CandidateExecutionUsageEnabled() bool
			PreviewProfileUsage(context.Context, string) *usage.ExecutionUsageReport
		}); ok && reader.CandidateExecutionUsageEnabled() {
			return usage.WithCandidateUsageReader(ctx, reader.PreviewProfileUsage)
		}
	} else if reader, ok := h.executor.(interface {
		CandidateExecutionUsageEnabled() bool
		PreviewInferenceProfileUsage(context.Context, string, string) *usage.ExecutionUsageReport
	}); ok && reader.CandidateExecutionUsageEnabled() {
		return usage.WithCandidateUsageReader(ctx, func(readCtx context.Context, profileID string) *usage.ExecutionUsageReport {
			return reader.PreviewInferenceProfileUsage(readCtx, sessionID, profileID)
		})
	}
	return ctx
}

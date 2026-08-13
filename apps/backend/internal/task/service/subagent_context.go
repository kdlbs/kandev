package service

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
)

// RecordSubagentContextRequest carries the orchestrator's already-resolved
// identity and the parsed subagent_task payload for one frame. The
// orchestrator owns turn-id resolution and the ToolStatus/normalized-kind
// guard; this request is the seam between that decision and persistence.
type RecordSubagentContextRequest struct {
	TaskSessionID    string
	TaskID           string
	TurnID           string
	ToolCallID       string
	ParentToolCallID string
	// ToolStatus is the ACP tool-call status of the launching Task call.
	// Terminality (AC-11) keys off this field only; Payload.Status
	// ("agent_status" — the child's own report) is stored verbatim and never
	// consulted for terminality.
	ToolStatus string
	Payload    *streams.SubagentTaskPayload
	// ObservedAt is Kandev's observation time for this frame (AC-1a) — not a
	// subagent start time.
	ObservedAt time.Time
}

// subagentTerminalToolStatuses mirrors internal/orchestrator's unexported
// isTerminalToolStatus (event_handlers_streaming.go:762). The two lists are
// pinned together by TestSubagentContextTerminalStatusesMatchOrchestrator so
// a future edit to one fails the other.
var subagentTerminalToolStatuses = map[string]bool{
	"complete": true, "completed": true, "success": true,
	"error": true, "failed": true, "cancelled": true,
}

func isTerminalToolStatus(status string) bool {
	return subagentTerminalToolStatuses[status]
}

// subagentContextSourceLive is the models.SubagentContext.Source value for
// a row observed on the live orchestrator frame path, as opposed to
// "backfill" (written only by the one-time migration).
const subagentContextSourceLive = "live"

// RecordSubagentContext persists one observation of a subagent (Task tool)
// invocation. It returns nothing: that is the mechanism by which AC-27
// holds — a writer with no error return cannot fail the enclosing message
// write, turn, or agent stream, and no future caller can accidentally
// propagate one. See docs/specs/subagent-context-persistence/spec.md.
func (s *Service) RecordSubagentContext(ctx context.Context, req RecordSubagentContextRequest) {
	if s.subagentContexts == nil {
		return
	}
	if req.TaskSessionID == "" || req.TaskID == "" || req.ToolCallID == "" {
		subagentContextTotal.Add("skipped_no_identity", 1)
		return
	}
	if req.Payload == nil {
		return
	}
	subagentContextTotal.Add("attempted", 1)

	totalTokens, totalTokensAnomalous := normalizeOmitEmptyMetric(req.Payload.TotalTokens)
	durationMs, durationAnomalous := normalizeOmitEmptyMetric(req.Payload.DurationMs)
	toolUseCount, toolUseCountAnomalous := normalizeToolUseCount(req.Payload.ToolUseCount)
	for _, anomalous := range []bool{totalTokensAnomalous, durationAnomalous, toolUseCountAnomalous} {
		if anomalous {
			subagentContextTotal.Add("anomalous_value", 1)
		}
	}

	sc := &models.SubagentContext{
		TaskSessionID:    req.TaskSessionID,
		TaskID:           req.TaskID,
		ToolCallID:       req.ToolCallID,
		TurnID:           nilIfEmpty(req.TurnID),
		ParentToolCallID: nilIfEmpty(req.ParentToolCallID),
		SubagentType:     nilIfEmpty(req.Payload.SubagentType),
		Description:      nilIfEmpty(req.Payload.Description),
		AgentID:          nilIfEmpty(req.Payload.AgentID),
		ChildSessionID:   nilIfEmpty(req.Payload.ChildSessionID),
		Model:            nilIfEmpty(req.Payload.Model),
		AgentStatus:      nilIfEmpty(req.Payload.Status),
		ToolStatus:       nilIfEmpty(req.ToolStatus),
		IsAsync:          req.Payload.IsAsync,
		TotalTokens:      totalTokens,
		ToolUseCount:     toolUseCount,
		DurationMs:       durationMs,
		Source:           subagentContextSourceLive,
		ObservedAt:       req.ObservedAt,
		UpdatedAt:        req.ObservedAt,
	}
	if isTerminalToolStatus(req.ToolStatus) {
		settledAt := req.ObservedAt
		sc.SettledAt = &settledAt
	}

	if err := s.subagentContexts.UpsertSubagentContext(ctx, sc); err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to record subagent context",
				zap.String("session_id", req.TaskSessionID),
				zap.String("tool_call_id", req.ToolCallID),
				zap.Error(err))
		}
		subagentContextTotal.Add("failed", 1)
		return
	}
	subagentContextTotal.Add("persisted", 1)
}

// nilIfEmpty turns the payload's "" (absent) into NULL, so COUNT(model)
// counts models rather than blanks.
func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// normalizeOmitEmptyMetric handles TotalTokens/DurationMs, which are plain
// int64 with `omitempty` on the wire payload: a genuine 0 and "not reported"
// are indistinguishable at this layer, so both normalize to NULL, matching
// AC-7. A negative value normalizes to NULL and reports anomalous (AC-9).
func normalizeOmitEmptyMetric(value int64) (*int64, bool) {
	if value == 0 {
		return nil, false
	}
	if value < 0 {
		return nil, true
	}
	v := value
	return &v, false
}

// normalizeToolUseCount handles ToolUseCount, which is a *int specifically so
// a reported 0 survives (AC-8) while "not reported" stays nil (AC-7). A
// negative reported value normalizes to NULL and reports anomalous (AC-9).
func normalizeToolUseCount(reported *int) (*int64, bool) {
	if reported == nil {
		return nil, false
	}
	v := int64(*reported)
	if v < 0 {
		return nil, true
	}
	return &v, false
}

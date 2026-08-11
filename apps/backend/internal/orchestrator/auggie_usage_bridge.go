package orchestrator

import (
	"context"
	"os"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/auggieusage"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

const auggieAgentName = "auggie"

// publishPromptUsageFromWireOrDisk publishes session_prompt_usage when wire
// usage is present, or (for auggie only) when disk has new completed exchanges.
// Wire wins: a non-nil payload.Data.Usage never double-counts disk.
func (s *Service) publishPromptUsageFromWireOrDisk(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) {
	if payload == nil || payload.SessionID == "" || s.eventBus == nil {
		return
	}
	if payload.Data != nil && payload.Data.Usage != nil {
		if !s.emitPromptUsage(ctx, payload, session, payload.Data.Usage) {
			return
		}
		// Wire wins for publish; still advance the disk watermark when possible
		// so a later nil-wire complete does not re-emit the same exchanges.
		if isAuggieSession(session) {
			s.advanceAuggieUsageWatermark(ctx, payload, session)
		}
		return
	}
	if !isAuggieSession(session) {
		return
	}
	s.publishAuggieDiskPromptUsage(ctx, payload, session)
}

func isAuggieSession(session *models.TaskSession) bool {
	if session == nil || session.AgentProfileSnapshot == nil {
		return false
	}
	name, _ := session.AgentProfileSnapshot["agent_name"].(string)
	return name == auggieAgentName
}

func (s *Service) publishAuggieDiskPromptUsage(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) {
	acpID := resolveAuggieACPSessionID(payload, session)
	if acpID == "" {
		return
	}
	path, err := auggieusage.SessionPath("", acpID)
	if err != nil {
		return
	}
	after := auggieUsageSeqFromMetadata(session)
	exchanges, err := auggieusage.ReadNewExchangeUsages(path, after)
	if err != nil {
		if !os.IsNotExist(err) && s.logger != nil {
			s.logger.Debug("auggie usage disk read failed",
				zap.String("session_id", payload.SessionID),
				zap.Error(err))
		}
		// Optional short deferred re-read if file not flushed yet.
		if os.IsNotExist(err) {
			s.scheduleAuggieUsageReread(payload.SessionID, payload.TaskID, payload.AgentID, acpID, after)
		}
		return
	}
	if len(exchanges) == 0 {
		return
	}
	usage, maxSeq, model := promptUsageFromAuggieExchanges(exchanges)
	if usage == nil {
		// Zero-token completed exchanges: advance watermark without publishing
		// empty piles so we do not re-walk them forever.
		if maxSeq > after {
			s.persistAuggieUsageSeq(ctx, payload.SessionID, maxSeq)
		}
		return
	}
	// Prefer disk model when present so piles key on the exchange model.
	emitPayload := *payload
	if model != "" {
		dataCopy := lifecycle.AgentStreamEventData{}
		if emitPayload.Data != nil {
			dataCopy = *emitPayload.Data
		}
		dataCopy.CurrentModelID = model
		emitPayload.Data = &dataCopy
	}
	// Advance only after a successful publish so a failed bus write can retry.
	if !s.emitPromptUsage(ctx, &emitPayload, session, usage) {
		return
	}
	s.persistAuggieUsageSeq(ctx, payload.SessionID, maxSeq)
}

func promptUsageFromAuggieExchanges(exchanges []auggieusage.ExchangeUsage) (*streams.PromptUsage, int, string) {
	in, out, cr, cw, model, maxSeq := auggieusage.SumExchanges(exchanges)
	// total_tokens = input+output for the delta batch (costs.md).
	total := in + out
	if total == 0 && cr == 0 && cw == 0 {
		return nil, maxSeq, model
	}
	return &streams.PromptUsage{
		InputTokens:       in,
		OutputTokens:      out,
		CachedReadTokens:  cr,
		CachedWriteTokens: cw,
		TotalTokens:       total,
		Estimated:         false,
	}, maxSeq, model
}

func (s *Service) advanceAuggieUsageWatermark(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
) {
	acpID := resolveAuggieACPSessionID(payload, session)
	if acpID == "" {
		return
	}
	path, err := auggieusage.SessionPath("", acpID)
	if err != nil {
		return
	}
	after := auggieUsageSeqFromMetadata(session)
	exchanges, err := auggieusage.ReadNewExchangeUsages(path, after)
	if err != nil || len(exchanges) == 0 {
		return
	}
	_, _, _, _, _, maxSeq := auggieusage.SumExchanges(exchanges)
	if maxSeq > after {
		s.persistAuggieUsageSeq(ctx, payload.SessionID, maxSeq)
	}
}

func resolveAuggieACPSessionID(payload *lifecycle.AgentStreamEventPayload, session *models.TaskSession) string {
	if session != nil {
		if id := acpSessionIDFromTaskMetadata(session.Metadata); id != "" {
			return id
		}
	}
	if payload != nil && payload.Data != nil && payload.Data.ACPSessionID != "" {
		return payload.Data.ACPSessionID
	}
	return ""
}

func acpSessionIDFromTaskMetadata(metadata map[string]interface{}) string {
	if metadata == nil {
		return ""
	}
	// map[string]interface{} and map[string]any are the same type in Go.
	acp, ok := metadata["acp"].(map[string]any)
	if !ok {
		return ""
	}
	id, _ := acp["session_id"].(string)
	return id
}

func auggieUsageSeqFromMetadata(session *models.TaskSession) int {
	if session == nil || session.Metadata == nil {
		return 0
	}
	raw, ok := session.Metadata[models.SessionMetaKeyAuggieUsageSeq]
	if !ok || raw == nil {
		return 0
	}
	switch v := raw.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	case jsonNumber:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

// jsonNumber matches encoding/json.Number without importing encoding/json here
// solely for the type assert (repo may return int from custom decode).
type jsonNumber interface {
	Int64() (int64, error)
}

func (s *Service) persistAuggieUsageSeq(ctx context.Context, sessionID string, maxSeq int) {
	if s.repo == nil || sessionID == "" || maxSeq <= 0 {
		return
	}
	if err := s.repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyAuggieUsageSeq, maxSeq); err != nil && s.logger != nil {
		s.logger.Warn("failed to persist auggie usage watermark",
			zap.String("session_id", sessionID),
			zap.Int("auggie_usage_seq", maxSeq),
			zap.Error(err))
	}
}

// emitPromptUsage publishes one session_prompt_usage event. Returns true when
// the bus accepted the event so callers can advance the disk watermark safely.
func (s *Service) emitPromptUsage(
	ctx context.Context,
	payload *lifecycle.AgentStreamEventPayload,
	session *models.TaskSession,
	usage *streams.PromptUsage,
) bool {
	if usage == nil || payload == nil || payload.SessionID == "" || s.eventBus == nil {
		return false
	}
	model, agentType := resolvePromptUsageLabels(payload, session)
	eventPayload := lifecycle.SessionPromptUsageEventPayload{
		TaskID:    payload.TaskID,
		SessionID: payload.SessionID,
		AgentID:   payload.AgentID,
		AgentType: agentType,
		Model:     model,
		Usage:     usage,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}
	subject := events.BuildSessionPromptUsageSubject(payload.SessionID)
	if err := s.eventBus.Publish(ctx, subject, bus.NewEvent(events.SessionPromptUsageUpdated, "orchestrator", eventPayload)); err != nil {
		if s.logger != nil {
			s.logger.Warn("failed to publish session prompt usage",
				zap.String("session_id", payload.SessionID),
				zap.Error(err))
		}
		return false
	}
	return true
}

// scheduleAuggieUsageReread attempts one deferred read if the session file was
// not present at complete time. Does not block teardown.
func (s *Service) scheduleAuggieUsageReread(sessionID, taskID, agentID, acpID string, after int) {
	if s == nil || sessionID == "" || acpID == "" {
		return
	}
	// Capture service pointer; fire-and-forget short delay.
	time.AfterFunc(400*time.Millisecond, func() {
		s.runAuggieUsageReread(sessionID, taskID, agentID, acpID, after)
	})
}

func (s *Service) runAuggieUsageReread(sessionID, taskID, agentID, acpID string, after int) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	path, err := auggieusage.SessionPath("", acpID)
	if err != nil {
		return
	}
	exchanges, err := auggieusage.ReadNewExchangeUsages(path, after)
	if err != nil || len(exchanges) == 0 {
		return
	}
	usage, maxSeq, model := promptUsageFromAuggieExchanges(exchanges)
	if usage == nil {
		// Match the sync path: advance past zero-token completed exchanges.
		if maxSeq > after {
			s.persistAuggieUsageSeq(ctx, sessionID, maxSeq)
		}
		return
	}
	session, usage, maxSeq, model, ok := s.reconcileAuggieReread(ctx, sessionID, path, after, usage, maxSeq, model)
	if !ok {
		return
	}
	payload := &lifecycle.AgentStreamEventPayload{
		TaskID:    taskID,
		SessionID: sessionID,
		AgentID:   agentID,
		Data: &lifecycle.AgentStreamEventData{
			CurrentModelID: model,
			ACPSessionID:   acpID,
		},
	}
	if !s.emitPromptUsage(ctx, payload, session, usage) {
		return
	}
	s.persistAuggieUsageSeq(ctx, sessionID, maxSeq)
}

func (s *Service) reconcileAuggieReread(
	ctx context.Context,
	sessionID, path string,
	after int,
	usage *streams.PromptUsage,
	maxSeq int,
	model string,
) (*models.TaskSession, *streams.PromptUsage, int, string, bool) {
	if s.repo == nil {
		return nil, usage, maxSeq, model, true
	}
	session, _ := s.repo.GetTaskSession(ctx, sessionID)
	if session == nil {
		return nil, usage, maxSeq, model, true
	}
	cur := auggieUsageSeqFromMetadata(session)
	if cur >= maxSeq {
		return session, nil, maxSeq, model, false
	}
	if cur <= after {
		return session, usage, maxSeq, model, true
	}
	// Watermark moved since schedule; re-filter so we do not double-count.
	exchanges, err := auggieusage.ReadNewExchangeUsages(path, cur)
	if err != nil || len(exchanges) == 0 {
		return session, nil, maxSeq, model, false
	}
	usage, maxSeq, model = promptUsageFromAuggieExchanges(exchanges)
	if usage == nil {
		return session, nil, maxSeq, model, false
	}
	return session, usage, maxSeq, model, true
}

package logger

import (
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// ExecutionFlowMessage identifies the bounded, body-free execution envelope.
const ExecutionFlowMessage = "execution flow"

var executionBootID atomic.Value

// SetExecutionBootID binds backend events to the boot reported by system info.
func SetExecutionBootID(id string) { executionBootID.Store(flowIdentifier(id)) }

// ExecutionFlow contains identities and transition facts only. It deliberately
// has no free-form error, prompt, response, command, path or metadata fields.
// Each event describes one boundary, never a token, chunk or polling sample.
type ExecutionFlow struct {
	Stage             string `json:"stage"`
	Outcome           string `json:"outcome"`
	Reason            string `json:"reason,omitempty"`
	BootID            string `json:"boot_id,omitempty"`
	RequestID         string `json:"request_id,omitempty"`
	CorrelationID     string `json:"correlation_id,omitempty"`
	TaskID            string `json:"task_id,omitempty"`
	SessionID         string `json:"session_id,omitempty"`
	AttemptID         string `json:"attempt_id,omitempty"`
	TurnID            string `json:"turn_id,omitempty"`
	ExecutionID       string `json:"execution_id,omitempty"`
	ParentExecutionID string `json:"parent_execution_id,omitempty"`
	AuthorType        string `json:"author_type,omitempty"`
	ExecutorID        string `json:"executor_id,omitempty"`
	ProfileID         string `json:"profile_id,omitempty"`
	MessageID         string `json:"message_id,omitempty"`
	QueueID           string `json:"queue_id,omitempty"`
	SubmissionID      string `json:"submission_id,omitempty"`
	ReservationID     string `json:"reservation_id,omitempty"`
	WorkflowID        string `json:"workflow_id,omitempty"`
	StepID            string `json:"step_id,omitempty"`
	EntryID           string `json:"entry_id,omitempty"`
	OwnerID           string `json:"owner_id,omitempty"`
	Model             string `json:"model,omitempty"`
	Grade             string `json:"grade,omitempty"`
	HarnessGeneration uint64 `json:"harness_generation,omitempty"`
	RouteGeneration   uint64 `json:"route_generation,omitempty"`
	PromptGeneration  uint64 `json:"prompt_generation,omitempty"`
	Sequence          int64  `json:"sequence,omitempty"`
	StreamID          string `json:"stream_id,omitempty"`
	DurationMS        int64  `json:"duration_ms"`
	Population        *int   `json:"population,omitempty"`
	Limit             *int   `json:"limit,omitempty"`
	GradePopulation   *int   `json:"grade_population,omitempty"`
	GradeLimit        *int   `json:"grade_limit,omitempty"`
	PopulationKnown   *bool  `json:"population_known,omitempty"`
}

// Sanitize drops unbounded or non-identifier strings. The same allowlist is
// applied before child logs cross the launcher boundary.
func (e ExecutionFlow) Sanitize() ExecutionFlow {
	for _, field := range []*string{&e.Stage, &e.Outcome, &e.Reason, &e.BootID,
		&e.RequestID, &e.CorrelationID, &e.TaskID, &e.SessionID, &e.AttemptID, &e.TurnID,
		&e.ExecutionID, &e.ParentExecutionID, &e.AuthorType, &e.ExecutorID, &e.ProfileID, &e.MessageID, &e.QueueID, &e.SubmissionID,
		&e.ReservationID, &e.StreamID, &e.WorkflowID, &e.StepID, &e.EntryID, &e.OwnerID, &e.Model, &e.Grade} {
		*field = flowIdentifier(*field)
	}
	return e
}

func flowIdentifier(value string) string {
	if len(value) > 192 {
		return ""
	}
	for _, c := range value {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' {
			continue
		}
		if !strings.ContainsRune("-_.:/", c) {
			return ""
		}
	}
	return value
}

// ParseExecutionFlow never forwards unknown JSON properties or arbitrary text.
func ParseExecutionFlow(raw []byte) *ExecutionFlow {
	if len(raw) == 0 || len(raw) > 8192 {
		return nil
	}
	var e ExecutionFlow
	if json.Unmarshal(raw, &e) != nil {
		return nil
	}
	e = e.Sanitize()
	if e.Stage == "" || e.Outcome == "" {
		return nil
	}
	return &e
}

// Flow emits an unsampled transition through the existing bounded logger.
func (l *Logger) Flow(ctx context.Context, event ExecutionFlow) {
	if l == nil {
		return
	}
	EmitExecutionFlow(l.zap, ctx, event)
}

// EmitExecutionFlow supports services that already own a zap logger. Callers
// must pass their ordinary logger, never a population-diagnostic sampler.
func EmitExecutionFlow(log *zap.Logger, ctx context.Context, event ExecutionFlow) {
	if log == nil {
		return
	}
	if event.BootID == "" {
		event.BootID, _ = executionBootID.Load().(string)
	}
	if event.RequestID == "" && ctx != nil {
		event.RequestID, _ = ctx.Value(RequestIDKey).(string)
	}
	if event.CorrelationID == "" && ctx != nil {
		event.CorrelationID, _ = ctx.Value(CorrelationIDKey).(string)
	}
	log.Info(ExecutionFlowMessage, zap.Any("flow", event.Sanitize()))
}

// BeginFlow records the entry and returns an exit recorder. Callers name the
// actual phase failure; raw errors are not copied into this envelope.
func (l *Logger) BeginFlow(ctx context.Context, event ExecutionFlow) func(string, string) {
	started := time.Now()
	event.Outcome = "started"
	l.Flow(ctx, event)
	return func(outcome, reason string) {
		event.Outcome, event.Reason = outcome, reason
		event.DurationMS = time.Since(started).Milliseconds()
		l.Flow(ctx, event)
	}
}

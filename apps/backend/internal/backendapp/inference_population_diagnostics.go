package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

func newInferencePopulationLogger(log *zap.Logger) *zap.Logger {
	if log == nil {
		log = zap.NewNop()
	}
	return log.WithOptions(zap.WrapCore(func(core zapcore.Core) zapcore.Core {
		return zapcore.NewSamplerWithOptions(core, time.Minute, 1, 0)
	}))
}

func inferencePopulationReadFailure(err error) string {
	switch {
	case errors.Is(err, agentruntime.ErrInferenceOwnershipUnsupported):
		return "ownership_unsupported"
	case errors.Is(err, context.Canceled):
		return "read_cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "read_deadline"
	default:
		return "read_failed"
	}
}

func inferencePopulationLocator(ref agentruntime.InferenceClientRef, status *utility.AttemptStatus) sessioncapacity.InferenceBinding {
	id := sessioncapacity.InferenceBinding{SessionID: ref.Identity.SessionID, ParentExecutionID: ref.Identity.ParentExecutionID}
	if ref.Client != nil {
		id.InstanceID = ref.Client.InferenceInstanceID()
	}
	if status != nil {
		id.OwnerID, id.AttemptID = status.OwnerID, status.AttemptID
	}
	return id
}

// Raw errors and arbitrary transport strings never enter diagnostic fields.
// Each closed source/reason pair emits at most once per minute, across all IDs.
func (b *sharedInferenceConsumer) logPopulationUnknown(source, reason string, id sessioncapacity.InferenceBinding, detail ...zap.Field) {
	if b.populationLogger == nil {
		return
	}
	fields := []zap.Field{zap.String("source", source), zap.String("reason", reason)}
	for _, field := range []struct{ key, value string }{
		{"instance_id", id.InstanceID}, {"owner_id", id.OwnerID}, {"attempt_id", id.AttemptID},
		{"session_id", id.SessionID}, {"execution_id", id.ParentExecutionID},
	} {
		if parsed, err := uuid.Parse(field.value); err == nil {
			fields = append(fields, zap.String(field.key, parsed.String()))
		}
	}
	fields = append(fields, detail...)
	b.populationLogger.Warn("capacity population unknown: "+source+"."+reason, fields...)
}

const inferenceDiagnosticUnknown = "unrecognized"
const inferenceDiagnosticUnavailable = "unavailable"
const inferenceDiagnosticChildLimit = 4

type inferenceChildDiagnostic struct {
	AttemptID      string `json:"attempt_id,omitempty"`
	OwnerID        string `json:"owner_id,omitempty"`
	PreparationID  string `json:"preparation_id,omitempty"`
	State          string `json:"state"`
	PromptDispatch string `json:"prompt_dispatch"`
	Known          bool   `json:"known"`
	Settled        bool   `json:"settled"`
	Retained       bool   `json:"retained"`
}

type inferenceSnapshotDiagnostic struct {
	OwnerID                string                     `json:"owner_id,omitempty"`
	Known                  bool                       `json:"known"`
	AttemptCount           int                        `json:"attempt_count"`
	ChildKnownFalseCount   int                        `json:"child_known_false_count"`
	ChildStateUnknownCount int                        `json:"child_state_unknown_count"`
	SettledCount           int                        `json:"settled_count"`
	OwnerDiagnostics       any                        `json:"owner_diagnostics"`
	Children               []inferenceChildDiagnostic `json:"children"`
	Truncated              bool                       `json:"truncated"`
}

func inferenceDiagnosticUUID(value string) string {
	id, err := uuid.Parse(value)
	if err != nil {
		return ""
	}
	return id.String()
}

func inferenceDiagnosticEnum(value string, allowed ...string) string {
	if slices.Contains(allowed, value) {
		return value
	}
	return inferenceDiagnosticUnknown
}

func inferenceDiagnosticState(value string) string {
	return inferenceDiagnosticEnum(value, utility.AttemptPrepared, utility.AttemptStarting, utility.AttemptRunning,
		utility.AttemptUnknown, utility.AttemptNotStarted, utility.AttemptTerminated)
}

func inferenceDiagnosticError(value string) string {
	return inferenceDiagnosticEnum(value, "none", "closed", "corrupt", "full", "cancelled", "deadline", "other", "decode", "invalid_state")
}

func sanitizedOwnerDiagnostic(raw *utility.AttemptDiagnostics) any {
	if raw == nil || raw.Version != 1 {
		return inferenceDiagnosticUnavailable
	}
	d := *raw
	d.InitialReason = inferenceDiagnosticEnum(raw.InitialReason, "volatile_unproven", "volatile_fresh", "journal_unavailable",
		"journal_read_failed", "existing_journal_without_owner", "journal_fresh_empty", "retained_decode_failed", "retained_state_invalid", "retained_owner_loaded")
	d.JournalMode = inferenceDiagnosticEnum(raw.JournalMode, "not_configured", "configured_unavailable", "fresh", "existing")
	d.JournalErrorClass = inferenceDiagnosticError(raw.JournalErrorClass)
	d.FirstPersistFailure = inferenceDiagnosticError(raw.FirstPersistFailure)
	d.LastPersistResult = inferenceDiagnosticEnum(raw.LastPersistResult, "not_attempted", "skipped_unavailable", "not_configured", "ok", "write_failed")
	d.DeliveryIncarnationID = inferenceDiagnosticUUID(raw.DeliveryIncarnationID)
	return d
}

func inferenceSnapshotDiagnosticFields(snapshot *utility.AttemptSnapshot) []zap.Field {
	d := inferenceSnapshotDiagnostic{OwnerID: inferenceDiagnosticUUID(snapshot.OwnerID), Known: snapshot.Known,
		AttemptCount: len(snapshot.Attempts), OwnerDiagnostics: sanitizedOwnerDiagnostic(snapshot.Diagnostics), Children: []inferenceChildDiagnostic{}}
	for _, a := range snapshot.Attempts {
		if !a.Known {
			d.ChildKnownFalseCount++
		}
		if a.State == utility.AttemptUnknown {
			d.ChildStateUnknownCount++
		}
		if a.Settled {
			d.SettledCount++
		}
		child := inferenceChildDiagnostic{AttemptID: inferenceDiagnosticUUID(a.AttemptID), OwnerID: inferenceDiagnosticUUID(a.OwnerID),
			PreparationID: inferenceDiagnosticUUID(a.PreparationID), State: inferenceDiagnosticState(a.State),
			PromptDispatch: inferenceDiagnosticEnum(a.PromptDispatch, utility.PromptNotSent, utility.PromptPossiblySent), Known: a.Known, Settled: a.Settled, Retained: a.Retained}
		d.Children = append(d.Children, child)
		sort.Slice(d.Children, func(i, j int) bool {
			if d.Children[i].Known != d.Children[j].Known {
				return !d.Children[i].Known
			}
			return d.Children[i].AttemptID < d.Children[j].AttemptID
		})
		if len(d.Children) > inferenceDiagnosticChildLimit {
			d.Children = d.Children[:inferenceDiagnosticChildLimit]
		}
	}
	d.Truncated = len(d.Children) < d.AttemptCount
	return boundedInferenceDiagnostic("snapshot", d)
}

func boundedInferenceDiagnostic(key string, detail any) []zap.Field {
	encoded, err := json.Marshal(detail)
	if err != nil || len(encoded) > 3072 {
		return []zap.Field{zap.Any(key, map[string]any{"detail": "size_limited", "truncated": true})}
	}
	return []zap.Field{zap.Any(key, detail)}
}

type inferenceBindingDiagnostic struct {
	InstanceID    string `json:"instance_id,omitempty"`
	OwnerID       string `json:"owner_id,omitempty"`
	AttemptID     string `json:"attempt_id,omitempty"`
	TerminalState string `json:"terminal_state"`
	ProviderKnown bool   `json:"provider_known"`
}

func inferenceBindingDiagnosticFields(bindings map[string]sessioncapacity.InferenceBinding) []zap.Field {
	children := make([]inferenceBindingDiagnostic, 0, inferenceDiagnosticChildLimit)
	for _, b := range bindings {
		children = append(children, inferenceBindingDiagnostic{InstanceID: inferenceDiagnosticUUID(b.InstanceID),
			OwnerID: inferenceDiagnosticUUID(b.OwnerID), AttemptID: inferenceDiagnosticUUID(b.AttemptID),
			TerminalState: inferenceDiagnosticTerminalState(b.TerminalState), ProviderKnown: b.ProviderKnown})
		sort.Slice(children, func(i, j int) bool { return children[i].AttemptID < children[j].AttemptID })
		if len(children) > inferenceDiagnosticChildLimit {
			children = children[:inferenceDiagnosticChildLimit]
		}
	}
	return boundedInferenceDiagnostic("bindings", map[string]any{"count": len(bindings), "children": children,
		"truncated": len(children) < len(bindings), "provider_finish": "not_observed"})
}

func inferenceDiagnosticTerminalState(value string) string {
	if value == "" {
		return "not_observed"
	}
	return inferenceDiagnosticState(value)
}

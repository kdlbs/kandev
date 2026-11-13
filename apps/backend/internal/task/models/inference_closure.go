package models

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
)

type inferenceClosurePresenceContextKey struct{}

// WithInferenceClosurePresenceCheck asks a session reader to preserve any
// independent closure block even if an adapter hides optional interfaces.
func WithInferenceClosurePresenceCheck(ctx context.Context) context.Context {
	return context.WithValue(ctx, inferenceClosurePresenceContextKey{}, true)
}

func InferenceClosurePresenceCheckRequested(ctx context.Context) bool {
	requested, _ := ctx.Value(inferenceClosurePresenceContextKey{}).(bool)
	return requested
}

const (
	InferenceClosureFenced    = "fenced"
	InferenceClosureObserved  = "observed"
	InferenceClosureSettled   = "settled"
	InferenceClosureCommitted = "committed"
	InferenceClosureConsumed  = "consumed"
)

const InferenceClosureMetadataKey = "inference_scope_closure"
const InferenceClosureConsumer = "inference_scope_closure"

var ErrInferenceClosureConflict = errors.New("inference closure identity or state changed")
var ErrInferenceClosureBlocked = errors.New("inference closure must finish before session admission")

// InferenceClosureCheckpoint records a single same-session recovery operation.
// The independent recovery block remains authoritative if metadata is lost.
type InferenceClosureCheckpoint struct {
	Version              int             `json:"version"`
	OperationID          string          `json:"operation_id"`
	SessionID            string          `json:"session_id"`
	IncarnationID        string          `json:"incarnation_id"`
	SourceExecutionID    string          `json:"source_execution_id"`
	CandidateExecutionID string          `json:"candidate_execution_id"`
	Stage                string          `json:"stage"`
	ClaimState           json.RawMessage `json:"claim_state,omitempty"`
	OwnerState           []byte          `json:"owner_state,omitempty"`
	Observation          json.RawMessage `json:"observation,omitempty"`
	ClaimsSettled        bool            `json:"claims_settled"`
	Receipt              json.RawMessage `json:"receipt,omitempty"`
	SuccessorOwnerID     string          `json:"successor_owner_id,omitempty"`
}

// InferenceClosureStatus contains only the identity and progress needed to
// retry the same operation. Claim and owner state remain in the durable store.
type InferenceClosureStatus struct {
	Version              int    `json:"version"`
	OperationID          string `json:"operation_id"`
	SessionID            string `json:"session_id"`
	IncarnationID        string `json:"incarnation_id"`
	SourceExecutionID    string `json:"source_execution_id"`
	CandidateExecutionID string `json:"candidate_execution_id"`
	Stage                string `json:"stage"`
	ClaimsSettled        bool   `json:"claims_settled"`
}

func (c *InferenceClosureCheckpoint) PublicStatus() *InferenceClosureStatus {
	if c == nil {
		return nil
	}
	return &InferenceClosureStatus{Version: c.Version, OperationID: c.OperationID, SessionID: c.SessionID,
		IncarnationID: c.IncarnationID, SourceExecutionID: c.SourceExecutionID,
		CandidateExecutionID: c.CandidateExecutionID, Stage: c.Stage, ClaimsSettled: c.ClaimsSettled}
}

// PublicSessionMetadata preserves ordinary metadata while limiting the
// inference-closure checkpoint to its public status projection.
func PublicSessionMetadata(metadata map[string]interface{}) map[string]interface{} {
	value, present := metadata[InferenceClosureMetadataKey]
	if !present {
		return metadata
	}
	result := maps.Clone(metadata)
	delete(result, InferenceClosureMetadataKey)
	raw, err := json.Marshal(value)
	if err != nil {
		return result
	}
	var checkpoint InferenceClosureCheckpoint
	if json.Unmarshal(raw, &checkpoint) == nil && checkpoint.Valid() {
		result[InferenceClosureMetadataKey] = checkpoint.PublicStatus()
	}
	return result
}

func (c InferenceClosureCheckpoint) Valid() bool {
	return c.Version == 1 && c.OperationID != "" && c.SessionID != "" &&
		c.IncarnationID != "" && c.SourceExecutionID != "" && c.CandidateExecutionID != "" &&
		c.SourceExecutionID != c.CandidateExecutionID && inferenceClosureStage(c.Stage) >= 0
}

// ReadyForSuccessor requires the durable settlement evidence that precedes
// committed or consumed ownership. A stage string alone is not that evidence.
func (c InferenceClosureCheckpoint) ReadyForSuccessor() bool {
	return c.Valid() && (c.Stage == InferenceClosureCommitted || c.Stage == InferenceClosureConsumed) &&
		c.ClaimsSettled && json.Valid(c.ClaimState) && json.Valid(c.OwnerState) &&
		json.Valid(c.Observation) && json.Valid(c.Receipt)
}

// CanAdvance requires the exact immutable recovery identity and one next stage.
func (c InferenceClosureCheckpoint) CanAdvance(next InferenceClosureCheckpoint) bool {
	if !c.sameClosureIdentity(next) || inferenceClosureStage(next.Stage) != inferenceClosureStage(c.Stage)+1 {
		return false
	}
	if len(next.OwnerState) == 0 || !json.Valid(next.OwnerState) || !json.Valid(next.Observation) {
		return false
	}
	if inferenceClosureStage(next.Stage) >= 2 && !next.ClaimsSettled {
		return false
	}
	if inferenceClosureStage(next.Stage) >= 3 && !json.Valid(next.Receipt) {
		return false
	}
	return next.Stage != InferenceClosureConsumed || next.SuccessorOwnerID != ""
}

func inferenceClosureStage(stage string) int {
	switch stage {
	case InferenceClosureFenced:
		return 0
	case InferenceClosureObserved:
		return 1
	case InferenceClosureSettled:
		return 2
	case InferenceClosureCommitted:
		return 3
	case InferenceClosureConsumed:
		return 4
	}
	return -1
}

func (c InferenceClosureCheckpoint) sameClosureIdentity(next InferenceClosureCheckpoint) bool {
	return c.Valid() && next.Valid() && c.OperationID == next.OperationID &&
		c.SessionID == next.SessionID && c.IncarnationID == next.IncarnationID &&
		c.SourceExecutionID == next.SourceExecutionID && c.CandidateExecutionID == next.CandidateExecutionID
}

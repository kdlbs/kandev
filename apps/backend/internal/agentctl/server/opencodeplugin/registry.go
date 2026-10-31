package opencodeplugin

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// Admission is the behavior-owned result of one validated native observation.
// The shared host uses it for durable delivery without knowing the behavior's
// transport payload or configuration shape.
type Admission struct {
	EventID          string
	BehaviorID       string
	NativeSessionID  string
	NativeGeneration uint64
	Observation      streams.WorkloadRunObservation
	SubmissionID     string
	PromptGeneration uint64
	ClaimKey         string
	// Check is an optional pre-journal fence. The committer must call it
	// immediately before durable admission and abort when it returns false.
	Check func() bool
}

// Behavior is one compiled typed receiver. It owns its configuration shape,
// native admission rules, and terminal-claim retirement. The shared host owns
// the lease, transport, revisioning, receipts, and durable commit boundary.
type Behavior interface {
	ID() string
	Version() int
	DecodeConfig(raw json.RawMessage) (any, error)
	EncodeConfig(state any) (json.RawMessage, error)
	Admit(state any, envelope ObservationEnvelope) (Admission, error)
	Retire(state any, admission Admission) (any, bool)
}

// Registry is the static set of behaviors this agentctl build understands.
type Registry struct {
	byID map[string]Behavior
}

// NewRegistry builds a registry from explicit entries.
func NewRegistry(behaviors ...Behavior) *Registry {
	registry := &Registry{byID: make(map[string]Behavior, len(behaviors))}
	for _, behavior := range behaviors {
		if behavior == nil {
			continue
		}
		registry.byID[behavior.ID()] = behavior
	}
	return registry
}

// ProductionRegistry returns the reviewed production behavior set.
func ProductionRegistry() *Registry {
	return NewRegistry(ShellOutcomesBehavior{})
}

// Get resolves an exact compiled version.
func (r *Registry) Get(id string, version int) (Behavior, bool) {
	if r == nil {
		return nil, false
	}
	behavior, ok := r.byID[id]
	if !ok || behavior.Version() != version {
		return nil, false
	}
	return behavior, true
}

// Refs returns the registry's stable behavior identities.
func (r *Registry) Refs() []BehaviorRef {
	if r == nil {
		return nil
	}
	refs := make([]BehaviorRef, 0, len(r.byID))
	for _, behavior := range r.byID {
		refs = append(refs, BehaviorRef{ID: behavior.ID(), Version: behavior.Version()})
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].ID < refs[j].ID })
	return refs
}

// ShellOutcomesBehavior normalizes native shell terminal facts.
type ShellOutcomesBehavior struct{}

func (ShellOutcomesBehavior) ID() string   { return BehaviorShellOutcomes }
func (ShellOutcomesBehavior) Version() int { return ShellOutcomesVersion }

type shellOutcomesConfig struct {
	Claims []Claim `json:"claims"`
}

// DecodeConfig validates a complete shell-outcomes configuration.
func (ShellOutcomesBehavior) DecodeConfig(raw json.RawMessage) (any, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return []Claim{}, nil
	}
	var config shellOutcomesConfig
	if err := json.Unmarshal(raw, &config); err != nil {
		return nil, fmt.Errorf("decode shell-outcomes config: %w", err)
	}
	return normalizeClaims(config.Claims), nil
}

// EncodeConfig encodes the complete shell claim set. It is the only
// shell-specific configuration helper the process manager needs.
func (ShellOutcomesBehavior) EncodeConfig(state any) (json.RawMessage, error) {
	claims, ok := state.([]Claim)
	if !ok {
		return nil, errors.New("shell-outcomes config state is invalid")
	}
	raw, err := json.Marshal(shellOutcomesConfig{Claims: claims})
	if err != nil {
		return nil, fmt.Errorf("encode shell-outcomes config: %w", err)
	}
	return raw, nil
}

// MarshalShellConfig encodes a complete shell claim set.
func MarshalShellConfig(claims []Claim) (json.RawMessage, error) {
	return ShellOutcomesBehavior{}.EncodeConfig(normalizeClaims(claims))
}

// Admit maps a native terminal fact onto the shared workload contract only for
// an exact owned claim in the current native session generation.
func (ShellOutcomesBehavior) Admit(state any, envelope ObservationEnvelope) (Admission, error) {
	claims, ok := state.([]Claim)
	if !ok {
		return Admission{}, errors.New("shell-outcomes config is unavailable")
	}
	var terminal ShellTerminalPayload
	if err := json.Unmarshal(envelope.Payload, &terminal); err != nil {
		return Admission{}, fmt.Errorf("decode shell terminal payload: %w", err)
	}
	if terminal.Kind != "shell.terminal" {
		return Admission{}, errors.New("unsupported shell payload kind")
	}
	claim, ok := findClaim(claims, terminal.ShellID)
	if !ok {
		return Admission{}, errors.New("shell payload has no owned claim")
	}
	if claim.NativeSessionID != envelope.NativeSessionID || claim.NativeGeneration != envelope.NativeGeneration {
		return Admission{}, errors.New("shell payload does not match the native session generation")
	}
	stateValue, err := shellTerminalState(terminal.Status, terminal.ExitCode)
	if err != nil {
		return Admission{}, err
	}
	observation := streams.WorkloadRunObservation{
		WorkID:       claim.WorkID,
		RunID:        claim.RunID,
		Kind:         streams.WorkloadKindShell,
		Title:        claim.Title,
		State:        stateValue,
		SourceCallID: claim.SourceCallID,
		ExitCode:     terminal.ExitCode,
		Capabilities: streams.InspectionOnlyWorkloadCapabilities(),
	}
	if terminal.StartedAt != nil {
		started := time.UnixMilli(*terminal.StartedAt).UTC()
		observation.StartedAt = &started
	}
	if terminal.CompletedAt != nil {
		finished := time.UnixMilli(*terminal.CompletedAt).UTC()
		observation.FinishedAt = &finished
	}
	return Admission{
		EventID:          envelope.EventID,
		BehaviorID:       BehaviorShellOutcomes,
		NativeSessionID:  envelope.NativeSessionID,
		NativeGeneration: envelope.NativeGeneration,
		Observation:      observation,
		SubmissionID:     claim.SubmissionID,
		PromptGeneration: claim.PromptGeneration,
		ClaimKey:         claim.ShellID,
	}, nil
}

// Retire removes the accepted claim so long conversations do not exhaust the
// bounded live-claim capacity. A stale retirement never removes a replacement.
func (ShellOutcomesBehavior) Retire(state any, admission Admission) (any, bool) {
	claims, ok := state.([]Claim)
	if !ok {
		return state, false
	}
	next := make([]Claim, 0, len(claims))
	for _, claim := range claims {
		if claim.ShellID == admission.ClaimKey &&
			claim.NativeSessionID == admission.NativeSessionID &&
			claim.NativeGeneration == admission.NativeGeneration {
			continue
		}
		next = append(next, claim)
	}
	if len(next) == len(claims) {
		return state, false
	}
	return next, true
}

func normalizeClaims(claims []Claim) []Claim {
	result := make([]Claim, 0, len(claims))
	for _, claim := range claims {
		if claim.Validate() != nil {
			continue
		}
		if len(result) >= MaxClaims {
			break
		}
		claim.Title = truncateClaimValue(claim.Title, 512)
		claim.SourceCallID = truncateClaimValue(claim.SourceCallID, 256)
		claim.SubmissionID = truncateClaimValue(claim.SubmissionID, 256)
		result = append(result, claim)
	}
	return result
}

func findClaim(claims []Claim, shellID string) (Claim, bool) {
	for _, claim := range claims {
		if claim.ShellID == shellID {
			return claim, true
		}
	}
	return Claim{}, false
}

func truncateClaimValue(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}

func shellTerminalState(status string, exit *int) (streams.RunState, error) {
	switch status {
	case "exited":
		if exit == nil {
			return streams.RunStateEnded, nil
		}
		if *exit == 0 {
			return streams.RunStateCompleted, nil
		}
		return streams.RunStateFailed, nil
	case "timeout":
		return streams.RunStateFailed, nil
	case "killed":
		return streams.RunStateInterrupted, nil
	default:
		return "", fmt.Errorf("unsupported shell terminal status %q", status)
	}
}

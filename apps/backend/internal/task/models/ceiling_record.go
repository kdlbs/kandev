package models

import (
	"encoding/json"
	"fmt"
	"time"
)

// CeilingLaunchKind names which launch shape a deferred record replays. The set is
// closed: a record naming anything outside it is unreplayable, and guessing would
// answer one kind of launch with another.
type CeilingLaunchKind string

const (
	CeilingLaunchStart              CeilingLaunchKind = "start"
	CeilingLaunchStartCreated       CeilingLaunchKind = "start_created"
	CeilingLaunchPromptEnsure       CeilingLaunchKind = "prompt_ensure"
	CeilingLaunchWorkflowStepEnsure CeilingLaunchKind = "workflow_step_ensure"
	CeilingLaunchQueueDrainEnsure   CeilingLaunchKind = "queue_drain_ensure"
	CeilingLaunchResume             CeilingLaunchKind = "resume"
	CeilingLaunchDynamicRelaunch    CeilingLaunchKind = "dynamic_relaunch"
)

// ceilingLaunchKinds is an explicit membership map rather than a switch with a
// default arm, so a kind added to the type without being classified here is absent
// from the set instead of silently admitted to it.
var ceilingLaunchKinds = map[CeilingLaunchKind]bool{
	CeilingLaunchStart:              true,
	CeilingLaunchStartCreated:       true,
	CeilingLaunchPromptEnsure:       true,
	CeilingLaunchWorkflowStepEnsure: true,
	CeilingLaunchQueueDrainEnsure:   true,
	CeilingLaunchResume:             true,
	CeilingLaunchDynamicRelaunch:    true,
}

// AllCeilingLaunchKinds is the canonical list. Code that must cover every kind
// should range over this rather than hand-writing its own literal.
var AllCeilingLaunchKinds = []CeilingLaunchKind{
	CeilingLaunchStart,
	CeilingLaunchStartCreated,
	CeilingLaunchPromptEnsure,
	CeilingLaunchWorkflowStepEnsure,
	CeilingLaunchQueueDrainEnsure,
	CeilingLaunchResume,
	CeilingLaunchDynamicRelaunch,
}

// IsCeilingLaunchKind reports membership of the closed set.
func IsCeilingLaunchKind(kind CeilingLaunchKind) bool {
	return ceilingLaunchKinds[kind]
}

// CeilingDeferral is the ceiling's half of the shared deferred_launch record.
type CeilingDeferral struct {
	Kind       CeilingLaunchKind
	Payload    map[string]interface{}
	Origin     string
	ReasonCode string
	QueuedAt   time.Time

	// Population, PopulationKnown and Ceiling are the admission controller's
	// reading at the moment of this refusal (or of the most recent reason-code
	// change). They are not part of the launch being replayed, so they are not
	// part of CeilingDeferralsEquivalent's comparison — only the AC-49 card
	// carrier reads them.
	Population      int
	PopulationKnown bool
	Ceiling         int
}

// CeilingRecordKeys builds the ceiling keys for a deferral. It returns only the
// keys the ceiling owns, so a caller merges them into whatever the record already
// holds rather than replacing it.
func CeilingRecordKeys(deferral CeilingDeferral) map[string]interface{} {
	keys := map[string]interface{}{
		CeilingDeferredKey:       true,
		CeilingLaunchKindKey:     string(deferral.Kind),
		CeilingLaunchOriginKey:   deferral.Origin,
		CeilingReasonCodeKey:     deferral.ReasonCode,
		CeilingQueuedAtKey:       deferral.QueuedAt.UTC().Format(time.RFC3339),
		CeilingLaunchPayloadKey:  deferral.Payload,
		CeilingValueAtRefusalKey: deferral.Ceiling,
	}
	if deferral.Payload == nil {
		keys[CeilingLaunchPayloadKey] = map[string]interface{}{}
	}
	if deferral.PopulationKnown {
		keys[CeilingPopulationAtRefusalKey] = deferral.Population
	}
	return keys
}

// MergeCeilingRecord folds a deferral's keys into a task's existing
// deferred_launch value and reports the record to store.
//
// Where the existing value is an object the ceiling keys are added and every key
// it does not own is left alone, so a dependency-chain intent and a ceiling
// deferral coexist on one record. Where the existing value is present but is not
// an object — a scalar, an array, or null — no reader can interpret it as an
// intent, so this replaces that one key's value and reports the discarded value so
// the caller can record it; every other key in the task's metadata is untouched
// either way.
func MergeCeilingRecord(
	existing interface{}, deferral CeilingDeferral,
) (record map[string]interface{}, discarded interface{}, replaced bool) {
	ceilingKeys := CeilingRecordKeys(deferral)

	current, isObject := existing.(map[string]interface{})
	if existing == nil {
		return ceilingKeys, nil, false
	}
	if !isObject {
		return ceilingKeys, existing, true
	}

	merged := make(map[string]interface{}, len(current)+len(ceilingKeys))
	for key, value := range current {
		merged[key] = value
	}
	for key, value := range ceilingKeys {
		merged[key] = value
	}
	return merged, nil, false
}

// ReadCeilingDeferral extracts the ceiling half of a record.
//
// A record whose kind is absent or outside the closed set is reported as
// unreplayable rather than defaulted: the one kind an older reader understands is
// "start", and answering a resume with a fresh task launch is the specific outcome
// that must not happen.
func ReadCeilingDeferral(record map[string]interface{}) (CeilingDeferral, error) {
	if record == nil {
		return CeilingDeferral{}, fmt.Errorf("deferred launch record is absent")
	}
	if flag, _ := record[CeilingDeferredKey].(bool); !flag {
		return CeilingDeferral{}, fmt.Errorf("record carries no ceiling deferral")
	}

	rawKind, _ := record[CeilingLaunchKindKey].(string)
	kind := CeilingLaunchKind(rawKind)
	if !IsCeilingLaunchKind(kind) {
		return CeilingDeferral{}, &UnreplayableCeilingRecordError{Kind: rawKind}
	}

	deferral := CeilingDeferral{Kind: kind}
	deferral.Payload, _ = record[CeilingLaunchPayloadKey].(map[string]interface{})
	if deferral.Payload == nil {
		deferral.Payload = map[string]interface{}{}
	}
	deferral.Origin, _ = record[CeilingLaunchOriginKey].(string)
	deferral.ReasonCode, _ = record[CeilingReasonCodeKey].(string)
	if stamp, ok := record[CeilingQueuedAtKey].(string); ok {
		if parsed, err := time.Parse(time.RFC3339, stamp); err == nil {
			deferral.QueuedAt = parsed.UTC()
		}
	}
	deferral.Ceiling = CeilingRecordInt(record[CeilingValueAtRefusalKey])
	if population, ok := record[CeilingPopulationAtRefusalKey]; ok {
		deferral.Population = CeilingRecordInt(population)
		deferral.PopulationKnown = true
	}
	return deferral, nil
}

// CeilingRecordInt reads a number stored on a deferred_launch record
// regardless of which numeric representation decoded it: json.Number for a
// caller using UseNumber (the repository's own read path), float64 for
// encoding/json's ordinary interface{} decoding (test fixtures and any other
// caller).
func CeilingRecordInt(value interface{}) int {
	switch v := value.(type) {
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

// UnreplayableCeilingRecordError reports a record whose launch kind is absent or
// unrecognised. It is distinguishable from an ordinary read failure because the
// disposition differs: the record is dropped rather than retried.
type UnreplayableCeilingRecordError struct {
	Kind string
}

func (e *UnreplayableCeilingRecordError) Error() string {
	if e.Kind == "" {
		return "ceiling launch kind is absent"
	}
	return fmt.Sprintf("ceiling launch kind %q is outside the known set", e.Kind)
}

package models

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
)

// CeilingDeferralsEquivalent reports whether two deferrals describe the same
// launch, which is what decides between discarding a duplicate refusal and
// surfacing a collision.
//
// The kind is compared first: two refusals of different kinds for one task are
// never the same launch, whatever their payloads hold. Only the nested replay
// payload is then compared — the record's bookkeeping (queue timestamp, surface
// stamps, reason code) describes the deferral rather than the launch.
//
// Comparison is over canonical JSON rather than Go values, because the payload
// carries slices, maps and pointers for which Go's == is either a compile error or
// a pointer comparison. Both sides are normalized at comparison time rather than at
// write time, so a record written by an earlier version is judged on the same terms
// as a fresh one.
func CeilingDeferralsEquivalent(a, b CeilingDeferral) (bool, error) {
	if a.Kind != b.Kind {
		return false, nil
	}
	left, err := canonicalComparablePayload(a.Payload)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize the stored launch payload: %w", err)
	}
	right, err := canonicalComparablePayload(b.Payload)
	if err != nil {
		return false, fmt.Errorf("failed to canonicalize the incoming launch payload: %w", err)
	}
	return bytes.Equal(left, right), nil
}

// CeilingDeferralsEquivalentForAdmission compares two observations of one
// deferred launch at an admission boundary. A legacy record may gain its
// workflow-entry binding between reads, so an absent binding and a present
// binding are equivalent only after the optional field is removed from both
// payloads. When both observations carry a binding, the binding remains part
// of the identity and must compare exactly.
func CeilingDeferralsEquivalentForAdmission(a, b CeilingDeferral) (bool, error) {
	left := a
	right := b
	_, leftBound := a.Payload[CeilingLaunchEntryBindingKey]
	_, rightBound := b.Payload[CeilingLaunchEntryBindingKey]
	if leftBound != rightBound {
		left.Payload = comparablePayloadWithoutOptionalBinding(a.Payload)
		right.Payload = comparablePayloadWithoutOptionalBinding(b.Payload)
	}
	return CeilingDeferralsEquivalent(left, right)
}

func comparablePayloadWithoutOptionalBinding(payload map[string]interface{}) map[string]interface{} {
	copyOfPayload := maps.Clone(payload)
	delete(copyOfPayload, CeilingLaunchEntryBindingKey)
	return copyOfPayload
}

// canonicalComparablePayload renders a payload to its comparison form: marshalled
// through JSON so Go types collapse to the wire shape, decoded with number literals
// preserved, then stripped of values that carry no information.
func canonicalComparablePayload(payload map[string]interface{}) ([]byte, error) {
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var decoded interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	normalized, present := normalizeComparableValue(decoded)
	if !present {
		return nil, nil
	}
	return json.Marshal(normalized)
}

// normalizeComparableValue collapses absent, nil and empty to one value, reporting
// present=false for anything that carries no information. A nil slice, an empty
// slice, a nil map, an empty map, a nil pointer and an absent key all reduce to the
// same thing, because they all mean the caller supplied nothing.
//
// Slice elements are normalized but never dropped: order and arity are significant,
// since attachment order reaches the composed prompt, so removing an element would
// make two different launches compare equal.
func normalizeComparableValue(value interface{}) (interface{}, bool) {
	switch typed := value.(type) {
	case nil:
		return nil, false
	case map[string]interface{}:
		normalized := make(map[string]interface{}, len(typed))
		for key, nested := range typed {
			if cleaned, present := normalizeComparableValue(nested); present {
				normalized[key] = cleaned
			}
		}
		if len(normalized) == 0 {
			return nil, false
		}
		return normalized, true
	case []interface{}:
		if len(typed) == 0 {
			return nil, false
		}
		normalized := make([]interface{}, len(typed))
		for i, nested := range typed {
			cleaned, present := normalizeComparableValue(nested)
			if !present {
				cleaned = nil
			}
			normalized[i] = cleaned
		}
		return normalized, true
	default:
		return typed, true
	}
}

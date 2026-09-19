package models

import "testing"

func deferralWith(kind CeilingLaunchKind, payload map[string]interface{}) CeilingDeferral {
	return CeilingDeferral{Kind: kind, Payload: payload}
}

func mustEquivalent(t *testing.T, a, b CeilingDeferral, want bool, why string) {
	t.Helper()
	got, err := CeilingDeferralsEquivalent(a, b)
	if err != nil {
		t.Fatalf("CeilingDeferralsEquivalent: %v", err)
	}
	if got != want {
		t.Fatalf("equivalent = %v, want %v: %s", got, want, why)
	}
}

// TestDifferentKindsAreNeverDuplicates: two refusals of different kinds for one
// task are different launches, whatever their payloads look like.
func TestDifferentKindsAreNeverDuplicates(t *testing.T) {
	payload := map[string]interface{}{"session_id": "s1"}
	mustEquivalent(t,
		deferralWith(CeilingLaunchResume, payload),
		deferralWith(CeilingLaunchPromptEnsure, payload),
		false, "kind is compared first")
}

// TestAbsentNilAndEmptyAreOneValue is rule (a). Both refusals came from a caller
// supplying nothing, so treating null and [] as different would raise a collision
// on the card for two launches that are the same launch.
func TestAbsentNilAndEmptyAreOneValue(t *testing.T) {
	var nilSlice []interface{}
	var nilMap map[string]interface{}

	base := map[string]interface{}{"session_id": "s1"}
	variants := map[string]map[string]interface{}{
		"nil slice":    {"session_id": "s1", "attachments": nilSlice},
		"empty slice":  {"session_id": "s1", "attachments": []interface{}{}},
		"nil map":      {"session_id": "s1", "env": nilMap},
		"empty map":    {"session_id": "s1", "env": map[string]interface{}{}},
		"explicit nil": {"session_id": "s1", "route": nil},
		"empty nested": {"session_id": "s1", "route": map[string]interface{}{"headers": map[string]interface{}{}}},
	}
	for name, payload := range variants {
		t.Run(name, func(t *testing.T) {
			mustEquivalent(t,
				deferralWith(CeilingLaunchStart, base),
				deferralWith(CeilingLaunchStart, payload),
				true, name+" must equal an absent key")
		})
	}
}

// TestMapKeyOrderIsNotADifference is rule (b).
func TestMapKeyOrderIsNotADifference(t *testing.T) {
	mustEquivalent(t,
		deferralWith(CeilingLaunchStart, map[string]interface{}{
			"env": map[string]interface{}{"A": "1", "B": "2"}, "prompt": "p",
		}),
		deferralWith(CeilingLaunchStart, map[string]interface{}{
			"prompt": "p", "env": map[string]interface{}{"B": "2", "A": "1"},
		}),
		true, "map key order is not a difference")
}

// TestSliceOrderIsADifference is rule (c). Attachment order reaches the composed
// prompt, so two launches whose attachments differ only in order are not the same
// launch and the collision must surface rather than the second being discarded.
func TestSliceOrderIsADifference(t *testing.T) {
	mustEquivalent(t,
		deferralWith(CeilingLaunchStart, map[string]interface{}{"attachments": []interface{}{"a", "b"}}),
		deferralWith(CeilingLaunchStart, map[string]interface{}{"attachments": []interface{}{"b", "a"}}),
		false, "slice order is a difference")
}

// TestOnlyPayloadFieldsAreCompared is rule (d): the record's bookkeeping does not
// describe the launch, so two refusals queued at different times with different
// surface-attempt counts are still the same launch.
func TestOnlyPayloadFieldsAreCompared(t *testing.T) {
	payload := map[string]interface{}{"session_id": "s1", "prompt": "p"}
	first := CeilingDeferral{
		Kind: CeilingLaunchStartCreated, Payload: payload,
		ReasonCode: "ceiling", Origin: "automatic",
	}
	second := CeilingDeferral{
		Kind: CeilingLaunchStartCreated, Payload: payload,
		ReasonCode: "ceiling_superseded", Origin: "manual",
	}
	mustEquivalent(t, first, second, true, "only the nested payload is compared")
}

func TestDifferentPayloadValuesAreNotDuplicates(t *testing.T) {
	mustEquivalent(t,
		deferralWith(CeilingLaunchStartCreated, map[string]interface{}{"prompt": "first"}),
		deferralWith(CeilingLaunchStartCreated, map[string]interface{}{"prompt": "second"}),
		false, "a differing prompt is a differing launch")
}

// TestNumberLiteralsSurviveComparison guards the same float64 round-trip that the
// compare-and-set token avoids: a large integer must not compare unequal to itself.
func TestNumberLiteralsSurviveComparison(t *testing.T) {
	mustEquivalent(t,
		deferralWith(CeilingLaunchStart, map[string]interface{}{"priority": int64(1757606400)}),
		deferralWith(CeilingLaunchStart, map[string]interface{}{"priority": int64(1757606400)}),
		true, "a large integer equals itself")
	mustEquivalent(t,
		deferralWith(CeilingLaunchStart, map[string]interface{}{"priority": int64(1757606400)}),
		deferralWith(CeilingLaunchStart, map[string]interface{}{"priority": int64(1757606401)}),
		false, "adjacent large integers are distinguishable")
}

// TestNormalizationAppliesToBothSides: a record written before the payload gained a
// field compares on the same terms as a fresh one, because normalization happens at
// comparison time rather than at write time.
func TestNormalizationAppliesToBothSides(t *testing.T) {
	stored := map[string]interface{}{"prompt": "p", "attachments": []interface{}{}}
	fresh := map[string]interface{}{"prompt": "p"}
	mustEquivalent(t,
		deferralWith(CeilingLaunchStart, stored),
		deferralWith(CeilingLaunchStart, fresh),
		true, "an older record normalizes the same way")
}

func TestAdmissionEquivalenceAllowsLegacyBindingEnrichmentButProtectsSuccessors(t *testing.T) {
	legacy := deferralWith(CeilingLaunchStartCreated, map[string]interface{}{
		"session_id": "session-1", "prompt": "workflow",
	})
	bound := deferralWith(CeilingLaunchStartCreated, map[string]interface{}{
		"session_id": "session-1", "prompt": "workflow",
		CeilingLaunchEntryBindingKey: map[string]interface{}{
			"workflow_id": "workflow-1", "destination_step_id": "step-1",
			"route_operation_id": "route-1", "entry_identity": "entry-1",
		},
	})
	got, err := CeilingDeferralsEquivalentForAdmission(legacy, bound)
	if err != nil {
		t.Fatalf("CeilingDeferralsEquivalentForAdmission: %v", err)
	}
	if !got {
		t.Fatal("binding enrichment must not make the same launch appear to be a successor")
	}

	successor := bound
	successor.Payload = map[string]interface{}{
		"session_id": "session-1", "prompt": "workflow",
		CeilingLaunchEntryBindingKey: map[string]interface{}{
			"workflow_id": "workflow-1", "destination_step_id": "step-2",
			"route_operation_id": "route-2", "entry_identity": "entry-2",
		},
	}
	got, err = CeilingDeferralsEquivalentForAdmission(bound, successor)
	if err != nil {
		t.Fatalf("CeilingDeferralsEquivalentForAdmission successor: %v", err)
	}
	if got {
		t.Fatal("two bound workflow entries must not compare equal")
	}
}

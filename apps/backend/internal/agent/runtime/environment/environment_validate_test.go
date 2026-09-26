package environment

import (
	"context"
	"testing"
)

// TestValidate_NoOverrideSideEffectOnSetThatWouldOverrideUnderResolve pins
// AC-19b: Validate keeps the func([]Definition) error signature and emits no
// observable override side effect (Resolve's third return value has no
// counterpart here at all), even for a definition set that DOES resolve by
// tier precedence when passed to Resolve instead.
func TestValidate_NoOverrideSideEffectOnSetThatWouldOverrideUnderResolve(t *testing.T) {
	defs := []Definition{
		{Key: "SHARED", Literal: "managed-value", Origin: OriginManagedRuntime},
		{Key: "SHARED", Literal: "profile-value", Origin: OriginAgentProfile},
	}

	// Resolve on the same set produces an override: proves this is genuinely
	// the "would override" case Validate must stay silent about.
	_, records, err := Resolve(context.Background(), defs, noReveal)
	if err != nil {
		t.Fatalf("Resolve() error = %v, want nil", err)
	}
	if len(records) != 1 {
		t.Fatalf("Resolve() records = %#v, want exactly one override", records)
	}

	if err := Validate(defs); err != nil {
		t.Fatalf("Validate() error = %v, want nil (Tier 1 accepts the set precedence would also accept)", err)
	}
}

// TestValidate_MatchesResolveErrorVerdict pins the shared-error-path half of
// AC-19b/AC-18: Validate's accept/reject verdict is derived from the exact
// same selectDefinitions call Resolve uses, so a conflicting set is rejected
// identically by both.
func TestValidate_MatchesResolveErrorVerdict(t *testing.T) {
	defs := []Definition{
		{Key: "SHARED", Literal: "one", Origin: OriginManagedRuntime},
		{Key: "SHARED", Literal: "two", Origin: OriginExecutorProfile},
	}

	if err := Validate(defs); err == nil {
		t.Fatal("Validate() error = nil, want conflict")
	}

	_, _, err := Resolve(context.Background(), defs, noReveal)
	if err == nil {
		t.Fatal("Resolve() error = nil, want conflict")
	}
}

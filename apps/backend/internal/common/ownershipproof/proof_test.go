package ownershipproof

import "testing"

func TestDeriveDependsOnEveryInput(t *testing.T) {
	base := Derive("cred", "chal", "bind")
	for _, tc := range []struct{ name, cred, chal, bind string }{
		{"credential", "other", "chal", "bind"},
		{"challenge", "cred", "other", "bind"},
		{"binding", "cred", "chal", "other"},
	} {
		if Derive(tc.cred, tc.chal, tc.bind) == base {
			t.Errorf("proof did not vary with the %s", tc.name)
		}
	}
	if base == "cred" {
		t.Fatal("proof is the credential verbatim")
	}
	// The separator must keep adjacent fields from running together, so
	// these two distinct (challenge, binding) pairs cannot collide.
	if Derive("cred", "ab", "c") == Derive("cred", "a", "bc") {
		t.Fatal("challenge and binding are not unambiguously separated")
	}
}

func TestMatchesAcceptsAnyOfferedProofAndRefusesNothing(t *testing.T) {
	chal, bind := "chal", "bind"
	mine := Derive("mine", chal, bind)
	theirs := Derive("theirs", chal, bind)

	if !Matches("mine", chal, bind, []string{theirs, mine}) {
		t.Error("did not accept a matching proof offered alongside another")
	}
	if Matches("mine", chal, bind, []string{theirs}) {
		t.Error("accepted a proof derived from a different credential")
	}
	if Matches("mine", chal, bind, nil) {
		t.Error("accepted an empty set of proofs")
	}
	if Matches("mine", chal, bind, []string{""}) {
		t.Error("accepted an empty proof")
	}
	if Matches("", chal, bind, []string{Derive("", chal, bind)}) {
		t.Error("accepted a proof when no credential was held")
	}
	if Matches("mine", "", bind, []string{Derive("mine", "", bind)}) {
		t.Error("accepted a proof over an empty challenge")
	}
}

package agents

import "testing"

func TestEffectiveSubmitSequence(t *testing.T) {
	if got := EffectiveSubmitSequence(""); got != "\r" {
		t.Errorf("empty = %q, want \\r", got)
	}
	if got := EffectiveSubmitSequence("\n"); got != "\n" {
		t.Errorf("explicit \\n = %q", got)
	}
}

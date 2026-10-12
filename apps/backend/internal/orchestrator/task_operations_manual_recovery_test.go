package orchestrator

import (
	"errors"
	"testing"
)

// Codex ACP reports an exhausted usage limit as a plain agent message using a
// typographic apostrophe. Manual recovery must recognize both the curly and
// the straight form so the queued prompt is retained instead of being retried.
func TestIsManualRecoveryPromptError_UsageLimitApostrophes(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"curly apostrophe", "You\u2019ve hit your usage limit. try again later.", true},
		{"straight apostrophe", "You've hit your usage limit. try again later.", true},
		{"uppercase curly", "YOU\u2019VE HIT YOUR USAGE LIMIT", true},
		{"machine token", "usageLimitExceeded", true},
		{"unrelated failure", "internal error", false},
		{"nil error", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var err error
			if tc.name != "nil error" {
				err = errors.New(tc.msg)
			}
			if got := isManualRecoveryPromptError(err); got != tc.want {
				t.Fatalf("isManualRecoveryPromptError(%q) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

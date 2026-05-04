package acp

import (
	"context"
	"strings"
	"testing"
)

// TestSetConfigOption_WithoutConnectionReturnsError pins the precondition
// that SetConfigOption must surface an error rather than panic when invoked
// before Initialize() has wired up the ACP connection. The same precondition
// is enforced by SetMode/SetModel; this test keeps the new method aligned.
func TestSetConfigOption_WithoutConnectionReturnsError(t *testing.T) {
	a := newTestAdapter()

	err := a.SetConfigOption(context.Background(), "model", "claude-3-7-sonnet")
	if err == nil {
		t.Fatalf("expected error when adapter not initialized")
	}
	if !strings.Contains(err.Error(), "not initialized") {
		t.Errorf("error = %q, want one containing %q", err.Error(), "not initialized")
	}
}

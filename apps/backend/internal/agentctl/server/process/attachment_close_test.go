package process

import (
	"context"
	"testing"
	"time"
)

// TestCloseCurrentClosesTheCurrentStream pins the "Capability and close
// reason" bullet of system design part 2: agentctl can close the current
// backend stream with an explicit code and reason (used for the
// agent_exited and agentctl_shutdown close reasons), by invoking the same
// close function StreamStart/FinalizeStreamStart already registered for
// supersede and offline-budget expiry.
func TestCloseCurrentClosesTheCurrentStream(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)

	type closeCall struct {
		code   int
		reason string
	}
	calls := make(chan closeCall, 1)
	closeFn := func(code int, reason string) { calls <- closeCall{code, reason} }

	if _, err := as.StreamStart(context.Background(), "s1"); err != nil {
		t.Fatalf("StreamStart() error = %v", err)
	}
	as.FinalizeStreamStart("s1", "", closeFn, doneCh())

	as.CloseCurrent(CloseCodeNormal, CloseReasonAgentExited)

	select {
	case got := <-calls:
		if got.code != CloseCodeNormal || got.reason != CloseReasonAgentExited {
			t.Fatalf("close call = %+v, want {%d %q}", got, CloseCodeNormal, CloseReasonAgentExited)
		}
	default:
		t.Fatal("CloseCurrent() did not invoke the current stream's close function")
	}
}

// TestCloseCurrentIsNoOpWithoutACurrentStream pins that CloseCurrent is safe
// and does nothing when no stream is current (for example, an instance that
// has never had a backend attach, or one already detached).
func TestCloseCurrentIsNoOpWithoutACurrentStream(t *testing.T) {
	as := testAttachment(t, time.Hour, nil)

	// Must not panic even though as.current is nil.
	as.CloseCurrent(CloseCodeNormal, CloseReasonAgentctlShutdown)
}

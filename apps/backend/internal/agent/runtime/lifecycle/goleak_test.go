package lifecycle

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the lifecycle package. The
// Manager owns a remote-status polling loop registered on m.wg plus
// detached helpers (stream connectors, agentctl-ready watcher, passthrough
// stderr poller) spawned across manager_*.go and session.go. Regressions
// where a Stop path returns before background work drains surface here.
//
// OTel note: `agentctl/tracing.initTracing` lazily inits the SDK provider
// only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set — in tests it isn't, so
// the noop provider is used and no batchSpanProcessor / OTLP retry
// goroutines are spawned. No IgnoreTopFunction is needed here.
//
// sshKeepaliveInterval/sshKeepaliveDeadline are zeroed for this test binary
// so the fifteen CreateInstance/ResumeRemoteInstance call sites across the
// package don't each acquire a real 15s SSH transport watchdog under this
// leak check. A non-positive value disables the watchdog
// (sshKeepaliveTuningValid); tests that exercise it set millisecond values
// of their own and must not run with t.Parallel().
func TestMain(m *testing.M) {
	sshKeepaliveInterval = 0
	sshKeepaliveDeadline = 0
	goleak.VerifyTestMain(m)
}

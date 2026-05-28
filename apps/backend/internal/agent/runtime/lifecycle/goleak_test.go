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
// IgnoreTopFunction notes:
//   - The OTLP HTTP trace exporter and its retry helper run inside
//     tracing.InitTracing (called by NewManager via the agent execution
//     trace path); they only exit when the process does, which is fine
//     in production but trips goleak.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("go.opentelemetry.io/otel/sdk/trace.(*batchSpanProcessor).processQueue"),
		goleak.IgnoreTopFunction("go.opentelemetry.io/otel/exporters/otlp/otlptrace/internal/retry.WaitFunc.func1"),
	)
}

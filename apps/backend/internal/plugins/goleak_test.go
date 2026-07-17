package plugins

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. HealthMonitor owns a single ticker loop guarded by Start/Stop —
// goleak catches regressions where a Start path forgets to register on the
// WaitGroup, or a Stop path returns before the loop drains.
//
// host_data_wire_test.go's real-transport tests (hcplugin.TestPluginGRPCConn,
// mirroring pkg/pluginsdk/serve_test.go) dial a second gRPC connection back
// to kandev's Host server over the go-plugin broker — the same connection a
// real plugin subprocess uses to call host.Sessions()/host.Tasks()/etc.
// pkg/pluginsdk's serve.go (dialBrokerWithRetry / newHostClient) has no
// Close() hook for that connection: in production it is only ever torn down
// by killing the plugin subprocess, which drops the connection and its
// goroutines for free. Our in-process test harness has no subprocess to
// kill, so grpc's per-ClientConn balancer/resolver CallbackSerializer
// goroutines for that one connection outlive the test. This is a test-harness
// artifact of go-plugin's process-death cleanup model, not a leak in
// production or in code this package owns — see docs/decisions/0042 and
// pkg/pluginsdk/serve.go's "Host injection" file header.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("google.golang.org/grpc/internal/grpcsync.(*CallbackSerializer).run"),
	)
}

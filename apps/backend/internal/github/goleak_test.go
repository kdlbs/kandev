package github

import (
	"net/http"
	"os"
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the github package — the Poller
// owns three loops (prMonitor, reviewQueue, issueWatch) gated on a single
// context via Stop. Regressions where Stop forgets to cancel the context
// or drain the WaitGroup surface here.
//
// HTTP/2 pool drain: anonymousHTTPClient and PATClient's rewriteTransport both
// route through http.DefaultTransport. On runners with outbound network (CI),
// one or more tests end up keeping an HTTP/2 client connection (readLoop
// goroutine) idle in the default pool — invisible locally where the network
// is sandboxed. CloseIdleConnections is the stdlib's documented way to drain
// these readers before checking for leaks; doing it here keeps the assertion
// strict (no broad IgnoreTopFunction suppression) without polluting prod code.
func TestMain(m *testing.M) {
	exitCode := m.Run()
	if exitCode == 0 {
		if tr, ok := http.DefaultTransport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
		if err := goleak.Find(); err != nil {
			println("goleak:", err.Error())
			exitCode = 1
		}
	}
	os.Exit(exitCode)
}

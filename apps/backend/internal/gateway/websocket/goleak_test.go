package websocket

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain enforces no goroutine leaks across the gateway/websocket package.
// Per-connection read/write pumps in client.go and the terminal bridge in
// terminal_pumps.go are the primary leak vectors — both must exit cleanly
// when the underlying connection closes.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

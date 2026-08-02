package api

import (
	"testing"

	"go.uber.org/goleak"
)

// The fan-out and cancellation tests in this package start goroutines per
// repository, so a bounded group that failed to drain, or a collect call left
// blocked on a gate, would otherwise pass unnoticed.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

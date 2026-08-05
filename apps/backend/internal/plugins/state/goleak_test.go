package state

import (
	"testing"

	"go.uber.org/goleak"
)

// TestMain asserts that no goroutines from this package outlive the test
// process. TestUserStorePostgresConditionalWriteIsRaceFreeUnderConcurrency
// races writers across goroutines it must join before returning — goleak
// catches a regression where one is left running past the test.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

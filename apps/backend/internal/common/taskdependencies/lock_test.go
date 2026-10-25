package taskdependencies

import (
	"testing"
	"time"
)

func TestAcquireMutationLockSerializesCallers(t *testing.T) {
	unlock := AcquireMutationLock()
	acquired := make(chan struct{})
	go func() {
		secondUnlock := AcquireMutationLock()
		close(acquired)
		secondUnlock()
	}()

	select {
	case <-acquired:
		t.Fatal("second caller acquired the mutation lock before the first released it")
	case <-time.After(100 * time.Millisecond):
	}

	unlock()
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("second caller did not acquire the mutation lock after release")
	}
}

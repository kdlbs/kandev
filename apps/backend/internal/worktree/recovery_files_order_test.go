package worktree

import (
	"errors"
	"reflect"
	"testing"
)

func TestSyncRecoveryContentBeforeAttributesOrdersOperations(t *testing.T) {
	var calls []string
	err := syncRecoveryContentBeforeAttributes(
		func() error {
			calls = append(calls, "sync")
			return nil
		},
		func() error {
			calls = append(calls, "attributes")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("sync content before attributes: %v", err)
	}
	if want := []string{"sync", "attributes"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("operation order = %v, want %v", calls, want)
	}
}

func TestSyncRecoveryContentBeforeAttributesStopsOnSyncFailure(t *testing.T) {
	syncErr := errors.New("sync failed")
	attributesCalled := false
	err := syncRecoveryContentBeforeAttributes(
		func() error { return syncErr },
		func() error {
			attributesCalled = true
			return nil
		},
	)
	if !errors.Is(err, syncErr) || attributesCalled {
		t.Fatalf("sync failure = %v, attributes called = %v", err, attributesCalled)
	}
}

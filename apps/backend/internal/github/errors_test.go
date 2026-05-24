package github

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// TestErrorsAreClassifiable verifies that the sentinels declared in errors.go
// remain reachable through errors.Is even when callers wrap them, which is
// the contract HTTP handlers and cleanup paths rely on.
func TestErrorsAreClassifiable(t *testing.T) {
	t.Run("isTaskNotFound recognizes sentinel-wrapped errors", func(t *testing.T) {
		wrapped := fmt.Errorf("%w: task task-1", ErrTaskNotFound)
		if !isTaskNotFound(wrapped) {
			t.Errorf("expected sentinel-wrapped error to be classified as task-not-found")
		}
		if isTaskNotFound(errors.New("something else not found")) {
			t.Errorf("expected unrelated 'not found' string to no longer match")
		}
		if isTaskNotFound(nil) {
			t.Errorf("nil error must not classify as not-found")
		}
	})

	t.Run("AssociateExistingPRByURL wraps ErrInvalidPRURL on malformed input", func(t *testing.T) {
		// Service with a stub client so the client-nil guard does not fire
		// before parsePRURL runs.
		svc := &Service{client: NewMockClient()}
		_, err := svc.AssociateExistingPRByURL(context.Background(), "t1", "", "not a pr url")
		if err == nil {
			t.Fatal("expected error for malformed PR URL")
		}
		if !errors.Is(err, ErrInvalidPRURL) {
			t.Errorf("error not classifiable as ErrInvalidPRURL: %v", err)
		}
	})

	t.Run("ErrInvalidToken survives the ConfigureToken wrap pattern", func(t *testing.T) {
		// ConfigureToken wraps the underlying PAT-client error with
		// `fmt.Errorf("%w: %w", ErrInvalidToken, err)`. Verify the wrap
		// pattern preserves errors.Is reachability for both the sentinel
		// and the inner cause. Exercising the full ConfigureToken would
		// require a real HTTP roundtrip — covered by integration tests.
		inner := errors.New("401 Unauthorized")
		wrapped := fmt.Errorf("%w: %w", ErrInvalidToken, inner)
		if !errors.Is(wrapped, ErrInvalidToken) {
			t.Errorf("wrapped error not classifiable as ErrInvalidToken: %v", wrapped)
		}
		if !errors.Is(wrapped, inner) {
			t.Errorf("wrapped error did not preserve inner cause in the chain: %v", wrapped)
		}
	})
}

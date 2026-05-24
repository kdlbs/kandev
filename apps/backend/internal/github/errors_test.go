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

	t.Run("ConfigureToken wraps ErrInvalidToken when validation fails", func(t *testing.T) {
		// The validation call is the first thing ConfigureToken does after
		// the secret-manager nil guard. Wire a no-op secret manager and
		// rely on the (real) PAT client failing for an obviously bad token.
		svc := &Service{
			authMethod:    AuthMethodPAT,
			secretManager: noopSecretManager{},
		}
		err := svc.ConfigureToken(context.Background(), "obviously-not-a-real-pat")
		if err == nil {
			t.Fatal("expected ConfigureToken to fail for an invalid token")
		}
		if !errors.Is(err, ErrInvalidToken) {
			t.Errorf("error not classifiable as ErrInvalidToken: %v", err)
		}
	})
}

type noopSecretManager struct{}

func (noopSecretManager) Create(context.Context, string, string) (string, error) {
	return "", nil
}
func (noopSecretManager) Update(context.Context, string, string) error { return nil }
func (noopSecretManager) Delete(context.Context, string) error         { return nil }

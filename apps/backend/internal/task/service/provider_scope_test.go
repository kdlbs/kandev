package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestApplyRepositoryUpdatesRejectsInvalidProviderScopeWithoutClearingIdentity(t *testing.T) {
	repository := &models.Repository{ProviderScope: "existing-scope"}
	invalid := strings.Repeat("x", maxProviderScopeBytes+1)

	err := applyRepositoryUpdates(repository, &UpdateRepositoryRequest{ProviderScope: &invalid})

	if !errors.Is(err, ErrInvalidRepositorySettings) {
		t.Fatalf("applyRepositoryUpdates error = %v, want ErrInvalidRepositorySettings", err)
	}
	if repository.ProviderScope != "existing-scope" {
		t.Fatalf("invalid scope changed repository identity to %q", repository.ProviderScope)
	}
}

package backendapp

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

type cursorCloudProfileReader interface {
	ListExecutors(context.Context) ([]*models.Executor, error)
	ListExecutorProfiles(context.Context, string) ([]*models.ExecutorProfile, error)
}

// cursorCloudAgentAvailable reports whether the current user can use at least
// one saved, locally valid cloud profile. Provider connectivity is not part of
// discovery, so an outage does not hide a configured agent family.
func cursorCloudAgentAvailable(
	ctx context.Context,
	tasks cursorCloudProfileReader,
	secretStore secrets.SecretStore,
) bool {
	if tasks == nil || secretStore == nil {
		return false
	}
	executors, err := tasks.ListExecutors(ctx)
	if err != nil {
		return false
	}
	for _, executor := range executors {
		if executor == nil || executor.Type != models.ExecutorTypeCursorCloud || executor.Status != models.ExecutorStatusActive {
			continue
		}
		profiles, err := tasks.ListExecutorProfiles(ctx, executor.ID)
		if err != nil {
			continue
		}
		for _, profile := range profiles {
			if cursorCloudProfileIsConfigured(ctx, profile, secretStore) {
				return true
			}
		}
	}
	return false
}

func cursorCloudProfileIsConfigured(ctx context.Context, profile *models.ExecutorProfile, store secrets.SecretStore) bool {
	if profile == nil {
		return false
	}
	secretID := strings.TrimSpace(profile.Config[cursorcloud.ExecutorConfigSecretID])
	callbackURL := strings.TrimSpace(profile.Config[cursorcloud.ExecutorConfigCallbackURL])
	return secretID != "" && secrets.ValidateGlobalReference(ctx, store, secretID) == nil &&
		cursorcloud.ValidateCallbackURL(callbackURL) == nil
}

package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"

	runtimeenv "github.com/kandev/kandev/internal/agent/runtime/environment"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.10
func TestSecretRecoveryNamesAgentProfile(t *testing.T) {
	mgr := newTestManager(t)
	mgr.secretStore = newInMemorySecretStore()
	// A same-named replacement must not repair a reference to the deleted ID.
	if err := mgr.secretStore.Create(context.Background(), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: "replacement-id", Name: "MY_TOKEN"}, Value: "private-value",
	}); err != nil {
		t.Fatal(err)
	}
	profile := &AgentProfileInfo{ProfileName: "Claude review", EnvVars: []settingsmodels.ProfileEnvVar{{Key: "MY_TOKEN", SecretID: "deleted-id"}}}
	env, err := mgr.buildEnvForExecution(context.Background(), "exec-1", &LaunchRequest{EnvironmentResolutionRequired: true}, nil, profile)
	var secretErr *runtimeenv.SecretError
	if env != nil || !errors.As(err, &secretErr) {
		t.Fatalf("resolution = %v, %v", env, err)
	}
	for _, want := range []string{"Claude review", "MY_TOKEN", "Re-select", "agent profile"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q lacks %q", err, want)
		}
	}
	for _, private := range []string{"deleted-id", "replacement-id", "private-value"} {
		if strings.Contains(err.Error(), private) {
			t.Errorf("error leaked %q: %v", private, err)
		}
	}
}

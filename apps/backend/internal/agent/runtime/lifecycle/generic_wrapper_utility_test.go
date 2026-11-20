package lifecycle

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/secrets"
)

func TestGenericWrapperUtilitySecretScope(t *testing.T) {
	for _, scope := range []secrets.SecretScope{secrets.ScopeGlobal, secrets.ScopeWorkspace} {
		t.Run(string(scope), func(t *testing.T) {
			profile := &AgentProfileInfo{ProfileID: "profile", AgentID: "inference-ok", EnvVars: []settingsmodels.ProfileEnvVar{{Key: "WRAPPER_SECRET", SecretID: "secret-1"}}}
			harness := newInferenceHarness(t, &stubProfileResolver{profile: profile}, newInferenceAgent("inference-ok", true, true))
			store := newInMemorySecretStore()
			store.store["secret-1"] = &secrets.SecretWithValue{Secret: secrets.Secret{ID: "secret-1", Scope: scope, WorkspaceID: "workspace"}, Value: "private-value"}
			harness.manager.secretStore = store
			_, err := harness.manager.ExecuteInferenceProfilePrompt(context.Background(), "session-1", "profile", "fixture")
			if scope == secrets.ScopeGlobal {
				require.NoError(t, err)
				require.Equal(t, "private-value", harness.recorded()[0].InferenceConfig.Env["WRAPPER_SECRET"])
			} else {
				require.Error(t, err)
				require.Empty(t, harness.recorded())
				require.NotContains(t, err.Error(), "private-value")
			}
		})
	}
}

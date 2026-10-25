package instance

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/githubauth"
	"github.com/stretchr/testify/require"
)

func TestInstanceAdmissionUsesOnlyRequestManagedAuthorization(t *testing.T) {
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if strings.HasPrefix(key, "GIT_CONFIG_") {
			t.Setenv(key, "")
			require.NoError(t, os.Unsetenv(key))
		}
	}
	for key, value := range map[string]string{
		githubauth.CredentialBrokerURLEnv: "https://startup.example", githubauth.CredentialLeaseEnv: "startup-lease",
		githubauth.CredentialReissueCapabilityEnv: "startup-capability", githubauth.CredentialRepositoryEnv: "startup-repository",
		githubauth.CredentialHelperPathEnv: "/installed/agentctl", githubauth.CredentialCLIShimDirEnv: "/installed/shims",
		"GIT_CONFIG_COUNT": "4", "GIT_CONFIG_KEY_0": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_0": "",
		"GIT_CONFIG_KEY_1": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_1": githubauth.ManagedGitCredentialHelper,
		"GIT_CONFIG_KEY_2": "credential.useHttpPath", "GIT_CONFIG_VALUE_2": "true",
		"GIT_CONFIG_KEY_3": "core.hooksPath", "GIT_CONFIG_VALUE_3": "/user/hooks",
	} {
		t.Setenv(key, value)
	}
	for _, managed := range []bool{false, true} {
		name := "executor"
		if managed {
			name = "managed"
		}
		t.Run(name, func(t *testing.T) {
			mgr := NewManager(&config.Config{}, newTestLogger(t))
			t.Cleanup(func() { require.NoError(t, mgr.Shutdown(context.Background())) })
			request := &CreateRequest{}
			if managed {
				request.Env = map[string]string{
					githubauth.CredentialBrokerURLEnv: "https://current.example", githubauth.CredentialLeaseEnv: "current-lease",
					"GIT_CONFIG_COUNT": "3", "GIT_CONFIG_KEY_0": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_0": "",
					"GIT_CONFIG_KEY_1": "credential.https://github.com.helper", "GIT_CONFIG_VALUE_1": githubauth.ManagedGitCredentialHelper,
					"GIT_CONFIG_KEY_2": "credential.useHttpPath", "GIT_CONFIG_VALUE_2": "true",
				}
			}
			admission, err := mgr.admitCreate(context.Background(), request)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, mgr.abandonPartialInstance(admission.bundle)); mgr.activeCreations.Done() })
			values := map[string]string{}
			for _, entry := range admission.agentEnv {
				key, value, _ := strings.Cut(entry, "=")
				values[key] = value
			}
			if managed {
				require.Equal(t, "current-lease", values[githubauth.CredentialLeaseEnv])
				require.Equal(t, "https://current.example", values[githubauth.CredentialBrokerURLEnv])
			} else {
				_, present := values[githubauth.CredentialLeaseEnv]
				require.False(t, present, "unexpected inherited managed authorization key")
				_, present = values[githubauth.CredentialBrokerURLEnv]
				require.False(t, present, "unexpected inherited managed authorization key")
			}
			_, present := values[githubauth.CredentialReissueCapabilityEnv]
			require.False(t, present, "unexpected inherited managed authorization key")
			_, present = values[githubauth.CredentialRepositoryEnv]
			require.False(t, present, "unexpected inherited managed authorization key")
			require.Equal(t, "/installed/agentctl", values[githubauth.CredentialHelperPathEnv])
			require.Equal(t, "/installed/shims", values[githubauth.CredentialCLIShimDirEnv])
			if managed {
				require.Equal(t, "4", values["GIT_CONFIG_COUNT"])
				require.Equal(t, githubauth.ManagedGitCredentialHelper, values["GIT_CONFIG_VALUE_2"])
			} else {
				require.Equal(t, "1", values["GIT_CONFIG_COUNT"])
			}
			require.Equal(t, "core.hooksPath", values["GIT_CONFIG_KEY_0"])
			require.Equal(t, "/user/hooks", values["GIT_CONFIG_VALUE_0"])
		})
	}
}

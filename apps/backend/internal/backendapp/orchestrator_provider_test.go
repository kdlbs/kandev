package backendapp

import (
	"path/filepath"
	"testing"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
)

func TestDefaultProviderHostnameRejectsPluginOwnedProviders(t *testing.T) {
	if _, err := defaultProviderHostname("bitbucket"); err == nil {
		t.Fatal("defaultProviderHostname(bitbucket) expected an error; plugin providers must supply their persisted clone URL")
	}
}

func TestKeywordBindingRequiresActualLocalCredentialContext(t *testing.T) {
	home := t.TempDir()
	other := filepath.Join(t.TempDir(), "different-account")
	for _, tc := range []struct {
		name, executor string
		env            map[string]string
		variables      []settingsmodels.ProfileEnvVar
		known          bool
	}{
		{"known host", "local", map[string]string{}, nil, true},
		{"known literal HOME", "worktree", map[string]string{"HOME": home}, []settingsmodels.ProfileEnvVar{{Key: "HOME", Value: home}}, true},
		{"different actual HOME", "local", map[string]string{"HOME": other}, nil, false},
		{"different candidate HOME", "local", map[string]string{"HOME": home}, []settingsmodels.ProfileEnvVar{{Key: "HOME", Value: other}}, false},
		{"remote", "ssh", map[string]string{"HOME": home}, nil, false},
		{"unknown executor", "", map[string]string{"HOME": home}, nil, false},
		{"missing snapshot", "local", nil, nil, false},
		{"credential home override", "local", map[string]string{"CLAUDE_CONFIG_DIR": other}, nil, false},
		{"provider auth override", "local", map[string]string{"ANTHROPIC_API_KEY": "configured"}, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			profile := &settingsmodels.AgentProfile{ID: "candidate", AgentID: "family", EnvVars: tc.variables}
			got := keywordBindingFromLaunch(tc.executor, tc.env, profile, home, func() string { calls++; return "subscription" })
			if (got.CredentialLocator != "") != tc.known {
				t.Fatalf("binding knowledge=%+v, want known=%v", got, tc.known)
			}
			if !tc.known && calls != 0 {
				t.Fatal("unproven launch queried backend billing")
			}
		})
	}
}

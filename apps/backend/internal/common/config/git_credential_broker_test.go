package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGitHubCredentialBrokerDisableReissueConfig(t *testing.T) {
	disableEntry, ok := CatalogEntryForKey("githubCredentialBroker.disableReissue")
	if !ok {
		t.Fatal("disableReissue is missing from the startup catalog")
	}
	if disableEntry.Default != "false" || len(disableEntry.EnvVars) != 1 ||
		disableEntry.EnvVars[0] != "KANDEV_GITHUB_CREDENTIAL_BROKER_DISABLE_REISSUE" {
		t.Fatalf("disableReissue catalog entry = %#v", disableEntry)
	}
	keyEntry, ok := CatalogEntryForKey("githubCredentialBroker.reissueSigningKey")
	if !ok || !keyEntry.Sensitive || len(keyEntry.EnvVars) != 1 ||
		keyEntry.EnvVars[0] != "KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY" {
		t.Fatalf("reissue signing key catalog entry = %#v, found=%v", keyEntry, ok)
	}

	defaultConfig, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("load default config: %v", err)
	}
	if got := defaultConfig.GitHubCredentialBroker.DisableReissue; got {
		t.Fatal("disableReissue default = true, want false")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("githubCredentialBroker:\n  disableReissue: true\n"), 0o600); err != nil {
		t.Fatalf("write YAML config: %v", err)
	}
	yamlConfig, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("load YAML config: %v", err)
	}
	if got := yamlConfig.GitHubCredentialBroker.DisableReissue; !got {
		t.Fatal("disableReissue YAML value = false, want true")
	}

	t.Setenv("KANDEV_GITHUB_CREDENTIAL_BROKER_DISABLE_REISSUE", "true")
	envConfig, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("load environment config: %v", err)
	}
	if got := envConfig.GitHubCredentialBroker.DisableReissue; !got {
		t.Fatal("disableReissue environment value = false, want true")
	}

	keyConfigDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(keyConfigDir, "config.yaml"), []byte("githubCredentialBroker:\n  reissueSigningKey: yaml-key\n"), 0o600); err != nil {
		t.Fatalf("write signing key YAML: %v", err)
	}
	t.Setenv("KANDEV_GITHUB_CREDENTIAL_BROKER_REISSUE_SIGNING_KEY", "environment-key")
	keyConfig, err := LoadWithPath(keyConfigDir)
	if err != nil {
		t.Fatalf("load signing key configuration: %v", err)
	}
	if got := keyConfig.GitHubCredentialBroker.ReissueSigningKey; got != "environment-key" {
		t.Fatalf("environment signing key = %q, want environment override", got)
	}
}

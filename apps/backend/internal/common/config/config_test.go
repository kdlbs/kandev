package config

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testGitHubAppPrivateKey(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	}))
}

func completeGitHubAppConfig(t *testing.T) GitHubAppConfig {
	t.Helper()
	return GitHubAppConfig{
		AppID:         123,
		ClientID:      "Iv1.client",
		ClientSecret:  "client-secret",
		PrivateKey:    testGitHubAppPrivateKey(t),
		WebhookSecret: "webhook-secret",
		Slug:          "kandev-test",
		PublicBaseURL: "https://kandev.example.com",
	}
}

func TestGitHubAppConfig_AllOrNone(t *testing.T) {
	t.Run("omitted is unavailable but valid", func(t *testing.T) {
		cfg := minimalValidConfig()
		if err := validate(cfg); err != nil {
			t.Fatalf("validate omitted app: %v", err)
		}
		if got := cfg.GitHubApp.Availability(); got.Available || got.Configured {
			t.Fatalf("Availability() = %+v, want unconfigured", got)
		}
	})

	t.Run("complete is available", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.GitHubApp = completeGitHubAppConfig(t)
		if err := validate(cfg); err != nil {
			t.Fatalf("validate complete app: %v", err)
		}
		got := cfg.GitHubApp.Availability()
		if !got.Available || !got.Configured || got.Reason != "" {
			t.Fatalf("Availability() = %+v, want available", got)
		}
	})

	t.Run("partial is rejected without exposing values", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.GitHubApp.ClientSecret = "do-not-leak"
		err := validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "githubApp") {
			t.Fatalf("validate partial app error = %v", err)
		}
		if strings.Contains(err.Error(), "do-not-leak") {
			t.Fatalf("validation error exposed secret: %v", err)
		}
	})
}

func TestGitHubAppConfig_SourcePresenceAndValidation(t *testing.T) {
	if (GitHubAppConfig{}).Configured() {
		t.Fatal("empty GitHub App config reported configured")
	}
	partial := GitHubAppConfig{ClientSecret: "do-not-leak"}
	if !partial.Configured() {
		t.Fatal("partial GitHub App config must be authoritative")
	}
	if err := partial.Validate(); err == nil {
		t.Fatal("partial GitHub App config validated")
	} else if strings.Contains(err.Error(), partial.ClientSecret) {
		t.Fatalf("validation error exposed secret: %v", err)
	}
}

func TestGitHubAppConfig_PrivateKeyFileAndMultiline(t *testing.T) {
	inline := completeGitHubAppConfig(t)
	inlineKey, err := inline.PrivateKeyPEM()
	if err != nil {
		t.Fatalf("inline PrivateKeyPEM: %v", err)
	}
	if !strings.Contains(string(inlineKey), "BEGIN RSA PRIVATE KEY") {
		t.Fatalf("inline key was not returned as PEM")
	}

	keyPath := filepath.Join(t.TempDir(), "app.pem")
	if err := os.WriteFile(keyPath, inlineKey, 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	fromFile := inline
	fromFile.PrivateKey = ""
	fromFile.PrivateKeyFile = keyPath
	fileKey, err := fromFile.PrivateKeyPEM()
	if err != nil {
		t.Fatalf("file PrivateKeyPEM: %v", err)
	}
	if string(fileKey) != string(inlineKey) {
		t.Fatalf("file key differs from inline key")
	}

	both := fromFile
	both.PrivateKey = string(inlineKey)
	if _, err := both.PrivateKeyPEM(); err == nil {
		t.Fatal("expected inline and file key to be mutually exclusive")
	}
}

func TestGitHubAppConfig_PublicBaseURLSafety(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		wantErr bool
	}{
		{name: "https public", baseURL: "https://kandev.example.com"},
		{name: "http localhost", baseURL: "http://localhost:38429"},
		{name: "http loopback", baseURL: "http://127.0.0.1:38429"},
		{name: "http public", baseURL: "http://kandev.example.com", wantErr: true},
		{name: "relative", baseURL: "/callback", wantErr: true},
		{name: "credentials", baseURL: "https://user:pass@kandev.example.com", wantErr: true},
		{name: "query", baseURL: "https://kandev.example.com?secret=x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := minimalValidConfig()
			cfg.GitHubApp = completeGitHubAppConfig(t)
			cfg.GitHubApp.PublicBaseURL = tt.baseURL
			err := validate(cfg)
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate base URL %q error = %v, wantErr %v", tt.baseURL, err, tt.wantErr)
			}
		})
	}
}

func TestGitHubAppConfig_EnvironmentBindings(t *testing.T) {
	app := completeGitHubAppConfig(t)
	t.Setenv("KANDEV_GITHUB_APP_APP_ID", "123")
	t.Setenv("KANDEV_GITHUB_APP_CLIENT_ID", app.ClientID)
	t.Setenv("KANDEV_GITHUB_APP_CLIENT_SECRET", app.ClientSecret)
	t.Setenv("KANDEV_GITHUB_APP_PRIVATE_KEY", strings.ReplaceAll(app.PrivateKey, "\n", "\\n"))
	t.Setenv("KANDEV_GITHUB_APP_WEBHOOK_SECRET", app.WebhookSecret)
	t.Setenv("KANDEV_GITHUB_APP_SLUG", app.Slug)
	t.Setenv("KANDEV_GITHUB_APP_PUBLIC_BASE_URL", app.PublicBaseURL)

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if !cfg.GitHubApp.Availability().Available {
		t.Fatalf("GitHubApp availability = %+v", cfg.GitHubApp.Availability())
	}
	key, err := cfg.GitHubApp.PrivateKeyPEM()
	if err != nil {
		t.Fatalf("PrivateKeyPEM: %v", err)
	}
	if !strings.Contains(string(key), "\n") {
		t.Fatal("escaped environment private key was not normalized")
	}
}

func TestGitHubCredentialBrokerConfigIsIndependentFromGitHubApp(t *testing.T) {
	cfg := minimalValidConfig()
	cfg.GitHubCredentialBroker.PublicBaseURL = "https://kandev.example.com"
	if err := validate(cfg); err != nil {
		t.Fatalf("validate broker-only config: %v", err)
	}
	if availability := cfg.GitHubApp.Availability(); availability.Configured || availability.Available {
		t.Fatalf("broker URL must not configure GitHub App: %+v", availability)
	}

	cfg.GitHubCredentialBroker.PublicBaseURL = "http://kandev.example.com"
	if err := validate(cfg); err == nil || !strings.Contains(err.Error(), "githubCredentialBroker.publicBaseUrl") {
		t.Fatalf("validate insecure broker URL error = %v", err)
	}
}

func TestGitHubCredentialBrokerConfigEnvironmentBinding(t *testing.T) {
	t.Setenv("KANDEV_GITHUB_CREDENTIAL_BROKER_PUBLIC_BASE_URL", "https://kandev.example.com")
	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if got := cfg.GitHubCredentialBroker.PublicBaseURL; got != "https://kandev.example.com" {
		t.Fatalf("broker public base URL = %q", got)
	}
}

// minimalValidConfig returns a Config that passes validate() out of the box.
// Tests modify a copy to exercise individual validation branches.
func minimalValidConfig() *Config {
	return &Config{
		Server:   ServerConfig{Port: 38429},
		Database: DatabaseConfig{Driver: "sqlite"},
		Auth:     AuthConfig{TokenDuration: 3600},
		Logging:  LoggingConfig{Level: "info", Format: "text"},
		RepositoryDiscovery: RepositoryDiscoveryConfig{
			MaxDepth: 5,
		},
	}
}

func TestResolvedHomeDir_Default(t *testing.T) {
	cfg := &Config{}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".kandev")
	if got := cfg.ResolvedHomeDir(); got != want {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, want)
	}
}

func TestResolvedHomeDir_WithHomeDir(t *testing.T) {
	cfg := &Config{HomeDir: "/custom/kandev"}
	if got := cfg.ResolvedHomeDir(); got != "/custom/kandev" {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, "/custom/kandev")
	}
}

func TestResolvedHomeDir_TildeExpansion(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	cfg := &Config{HomeDir: "~/.kandev-dev"}
	want := filepath.Join(home, ".kandev-dev")
	if got := cfg.ResolvedHomeDir(); got != want {
		t.Errorf("ResolvedHomeDir() = %q, want %q", got, want)
	}
}

func TestResolvedDataDir_Default(t *testing.T) {
	cfg := &Config{}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("cannot determine home directory")
	}
	want := filepath.Join(home, ".kandev", "data")
	if got := cfg.ResolvedDataDir(); got != want {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, want)
	}
}

func TestResolvedDataDir_DerivedFromHomeDir(t *testing.T) {
	// Data always lives under <HomeDir>/data. No independent override.
	cfg := &Config{HomeDir: "/custom/kandev"}
	want := filepath.Join("/custom/kandev", "data")
	if got := cfg.ResolvedDataDir(); got != want {
		t.Errorf("ResolvedDataDir() = %q, want %q", got, want)
	}
}

func TestValidate_DatabaseDriver(t *testing.T) {
	t.Run("sqlite_ok", func(t *testing.T) {
		cfg := minimalValidConfig()
		if err := validate(cfg); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("mixed_case_postgres_normalized", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.Database.Driver = "Postgres"
		cfg.Database.Port = 5432
		cfg.Database.User = "u"
		cfg.Database.DBName = "db"
		cfg.Database.SSLMode = "disable"
		if err := validate(cfg); err != nil {
			t.Fatalf("expected mixed-case 'Postgres' to normalize, got %v", err)
		}
		if cfg.Database.Driver != "postgres" {
			t.Errorf("driver not normalized: got %q, want %q", cfg.Database.Driver, "postgres")
		}
	})

	t.Run("unknown_driver_rejected", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.Database.Driver = "mysql"
		err := validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "database.driver") {
			t.Fatalf("expected database.driver error, got %v", err)
		}
	})
}

func TestValidate_PostgresSSLMode(t *testing.T) {
	for _, mode := range []string{"disable", "require", "verify-ca", "verify-full"} {
		t.Run(mode, func(t *testing.T) {
			cfg := minimalValidConfig()
			cfg.Database.Driver = "postgres"
			cfg.Database.Port = 5432
			cfg.Database.User = "u"
			cfg.Database.DBName = "db"
			cfg.Database.SSLMode = mode
			if err := validate(cfg); err != nil && strings.Contains(err.Error(), "sslMode") {
				t.Errorf("sslMode %q rejected unexpectedly: %v", mode, err)
			}
		})
	}

	t.Run("invalid_rejected", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.Database.Driver = "postgres"
		cfg.Database.Port = 5432
		cfg.Database.User = "u"
		cfg.Database.DBName = "db"
		cfg.Database.SSLMode = "bogus"
		err := validate(cfg)
		if err == nil || !strings.Contains(err.Error(), "sslMode") {
			t.Fatalf("expected sslMode error, got %v", err)
		}
	})

	t.Run("sqlite_ignores_sslmode", func(t *testing.T) {
		cfg := minimalValidConfig()
		cfg.Database.SSLMode = "bogus"
		if err := validate(cfg); err != nil {
			t.Errorf("sqlite should ignore sslMode, got %v", err)
		}
	})
}

// TestFeatures_DefaultOff pins the production-safety invariant: every
// feature flag in FeaturesConfig is false unless the deployment explicitly
// sets the matching env var. A regression that flips a default to true
// would ship an in-progress feature to users on the next release.
// See docs/decisions/0007-runtime-feature-flags.md.
func TestFeatures_DefaultOff(t *testing.T) {
	// Force a clean env so KANDEV_FEATURES_* and profile-selector vars
	// from the host shell can't bleed in and turn a default-off check
	// into a default-on accident. Clearing the profile selectors ensures
	// DetectEnvironment returns prod, so FeatureFlagDefaults uses the
	// prod value ("false") rather than the dev value ("true").
	t.Setenv("KANDEV_FEATURES_OFFICE", "")
	t.Setenv("KANDEV_FEATURES_PLUGINS", "")
	t.Setenv("KANDEV_DEBUG_DEV_MODE", "")
	t.Setenv("KANDEV_DEBUG_PPROF_ENABLED", "")
	t.Setenv("KANDEV_E2E_MOCK", "")

	dir := t.TempDir()
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Features.Office {
		t.Errorf("Features.Office = true, want false (production default must be off)")
	}
	if cfg.Features.Plugins {
		t.Errorf("Features.Plugins = true, want false (production default must be off)")
	}
}

// TestFeatures_OfficeEnabledByEnv proves the documented opt-in path:
// setting KANDEV_FEATURES_OFFICE=true flips Features.Office to true. This
// is what `apps/cli/src/dev.ts` relies on for dev mode and what release
// deployments would set if they ever wanted Office on.
func TestFeatures_OfficeEnabledByEnv(t *testing.T) {
	t.Setenv("KANDEV_FEATURES_OFFICE", "true")

	dir := t.TempDir()
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if !cfg.Features.Office {
		t.Errorf("Features.Office = false, want true (KANDEV_FEATURES_OFFICE=true must flip the flag)")
	}
}

// TestFeatures_PluginsEnabledByEnv proves the documented opt-in path:
// setting KANDEV_FEATURES_PLUGINS=true flips Features.Plugins to true.
func TestFeatures_PluginsEnabledByEnv(t *testing.T) {
	t.Setenv("KANDEV_FEATURES_PLUGINS", "true")

	dir := t.TempDir()
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if !cfg.Features.Plugins {
		t.Errorf("Features.Plugins = false, want true (KANDEV_FEATURES_PLUGINS=true must flip the flag)")
	}
}

func TestServerHostFromEnv(t *testing.T) {
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Fatalf("Server.Host = %q, want 127.0.0.1", cfg.Server.Host)
	}
}

func TestResolvedBinds(t *testing.T) {
	tests := []struct {
		name    string
		host    string
		hosts   []string
		want    []string
		wantErr bool
	}{
		{name: "single host", host: "127.0.0.1", want: []string{"127.0.0.1"}},
		{name: "empty falls back to wildcard default", host: "", want: []string{"0.0.0.0"}},
		{name: "comma separated list", host: "127.0.0.1,100.64.0.1", want: []string{"127.0.0.1", "100.64.0.1"}},
		{name: "whitespace trimmed", host: " 127.0.0.1 , 100.64.0.1 ", want: []string{"127.0.0.1", "100.64.0.1"}},
		{name: "duplicates dropped", host: "127.0.0.1,127.0.0.1,::1", want: []string{"127.0.0.1", "::1"}},
		{name: "wildcard collapses set", host: "127.0.0.1,0.0.0.0", want: []string{"0.0.0.0"}},
		{name: "ipv6 wildcard collapses set", host: "::,127.0.0.1", want: []string{"::"}},
		{name: "hostname allowed", host: "my-tailnet-host", want: []string{"my-tailnet-host"}},
		{name: "hosts array used when host unset", host: "", hosts: []string{"127.0.0.1", "100.64.0.1"}, want: []string{"127.0.0.1", "100.64.0.1"}},
		{name: "explicit host wins over hosts array", host: "127.0.0.1", hosts: []string{"0.0.0.0"}, want: []string{"127.0.0.1"}},
		{name: "hosts array as comma string", hosts: []string{"127.0.0.1,100.64.0.1"}, want: []string{"127.0.0.1", "100.64.0.1"}},
		{name: "equivalent ipv6 forms dedupe", host: "::1,0:0:0:0:0:0:0:1", want: []string{"::1"}},
		{name: "longhand unspecified ipv6 is wildcard", host: "0:0:0:0:0:0:0:0,127.0.0.1", want: []string{"::"}},
		{name: "invalid entry errors", host: "127.0.0.1,not a host!!", wantErr: true},
		{name: "invalid entry after wildcard still errors", host: "0.0.0.0,not a host!!", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sc := ServerConfig{Host: tt.host, Hosts: tt.hosts}
			got, err := sc.ResolvedBinds()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolvedBinds() expected error, got %v", got)
				}
				if !strings.Contains(err.Error(), "not a host!!") {
					t.Fatalf("ResolvedBinds() error = %q, want it to name the bad entry", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolvedBinds() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ResolvedBinds() = %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("ResolvedBinds() = %v, want %v", got, tt.want)
				}
			}
		})
	}
}

func TestIsLoopbackHost(t *testing.T) {
	tests := []struct {
		host string
		want bool
	}{
		{"127.0.0.1", true},
		{"127.0.0.53", true},
		{"::1", true},
		{"localhost", true},
		{"LocalHost", true},
		{"0.0.0.0", false},
		{"::", false},
		{"", false},
		{"100.64.0.1", false},
		{"my-tailnet-host", false},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			if got := IsLoopbackHost(tt.host); got != tt.want {
				t.Fatalf("IsLoopbackHost(%q) = %v, want %v", tt.host, got, tt.want)
			}
		})
	}
}

func TestNonLoopbackBinds(t *testing.T) {
	sc := ServerConfig{Host: "127.0.0.1,100.64.0.1,::1"}
	got, err := sc.NonLoopbackBinds()
	if err != nil {
		t.Fatalf("NonLoopbackBinds() error: %v", err)
	}
	if len(got) != 1 || got[0] != "100.64.0.1" {
		t.Fatalf("NonLoopbackBinds() = %v, want [100.64.0.1]", got)
	}

	loopOnly := ServerConfig{Host: "127.0.0.1"}
	if got, err := loopOnly.NonLoopbackBinds(); err != nil || len(got) != 0 {
		t.Fatalf("NonLoopbackBinds() = %v (err %v), want empty", got, err)
	}

	wildcard := ServerConfig{Host: "0.0.0.0"}
	if got, err := wildcard.NonLoopbackBinds(); err != nil || len(got) != 1 || got[0] != "0.0.0.0" {
		t.Fatalf("NonLoopbackBinds() = %v (err %v), want [0.0.0.0]", got, err)
	}
}

// TestServerHostEnvOverridesConfigHosts verifies env-over-file precedence: a
// KANDEV_SERVER_HOST env override must win over a config-file server.hosts
// array, so launching with a loopback host binds only loopback even if the
// config file left non-loopback addresses in server.hosts. Regression for the
// desktop/headless loopback contract.
func TestServerHostEnvOverridesConfigHosts(t *testing.T) {
	dir := t.TempDir()
	cfgYAML := "server:\n  hosts:\n    - 0.0.0.0\n    - 100.64.0.1\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgYAML), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	t.Setenv("KANDEV_SERVER_HOST", "127.0.0.1")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	binds, err := cfg.Server.ResolvedBinds()
	if err != nil {
		t.Fatalf("ResolvedBinds: %v", err)
	}
	if len(binds) != 1 || binds[0] != "127.0.0.1" {
		t.Fatalf("ResolvedBinds() = %v, want [127.0.0.1] (env host must override config server.hosts)", binds)
	}
}

// TestServerHostsFromConfigWhenHostUnset confirms server.hosts is honored when
// no host/env override is present.
func TestServerHostsFromConfigWhenHostUnset(t *testing.T) {
	dir := t.TempDir()
	cfgYAML := "server:\n  hosts:\n    - 127.0.0.1\n    - 100.64.0.1\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(cfgYAML), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	// Ensure no ambient KANDEV_SERVER_HOST override leaks into the test.
	t.Setenv("KANDEV_SERVER_HOST", "")

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	binds, err := cfg.Server.ResolvedBinds()
	if err != nil {
		t.Fatalf("ResolvedBinds: %v", err)
	}
	want := []string{"127.0.0.1", "100.64.0.1"}
	if len(binds) != len(want) || binds[0] != want[0] || binds[1] != want[1] {
		t.Fatalf("ResolvedBinds() = %v, want %v", binds, want)
	}
}

// TestFeaturesConfig_JSONShape pins the wire format of GET /api/v1/features.
// The handler in helpers.go serializes FeaturesConfig directly so new
// fields flow through without an extra edit; this test guarantees the
// `json` tag is present on every field. A regression (struct field added
// without a tag) would surface as a capitalized JSON key and break the
// frontend's case-sensitive read in apps/web/app/actions/features.ts.
func TestFeaturesConfig_JSONShape(t *testing.T) {
	cfg := FeaturesConfig{Office: true, Plugins: true}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(raw)
	want := `{"office":true,"plugins":true}`
	if got != want {
		t.Errorf("FeaturesConfig JSON = %s; want %s — missing or wrong `json:` struct tag", got, want)
	}
}

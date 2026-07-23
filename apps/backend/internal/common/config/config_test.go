package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// TestFeatures_ProductionDefaults pins the production policy: in-progress
// features remain off, while the shipped app status bar remains on unless a
// deployment or saved runtime override explicitly disables it.
func TestFeatures_ProductionDefaults(t *testing.T) {
	// Force a clean env so KANDEV_FEATURES_* and profile-selector vars from the
	// host shell cannot change the production-profile defaults under test.
	t.Setenv("KANDEV_FEATURES_OFFICE", "")
	t.Setenv("KANDEV_FEATURES_PLUGINS", "")
	unsetEnv(t, "KANDEV_FEATURES_APP_STATUS_BAR")
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
	if cfg.Features.AppStatusBar {
		t.Error("Features.AppStatusBar = true, want false (status surface must remain opt-in by default)")
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

func TestFeatures_AppStatusBarDisabledByEnv(t *testing.T) {
	t.Setenv("KANDEV_FEATURES_APP_STATUS_BAR", "false")

	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Features.AppStatusBar {
		t.Error("Features.AppStatusBar = true, want false (KANDEV_FEATURES_APP_STATUS_BAR=false must hide it)")
	}
}

func unsetEnv(t *testing.T, name string) {
	t.Helper()
	value, set := os.LookupEnv(name)
	_ = os.Unsetenv(name)
	t.Cleanup(func() {
		if set {
			_ = os.Setenv(name, value)
			return
		}
		_ = os.Unsetenv(name)
	})
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
	cfg := FeaturesConfig{Office: true, Plugins: true, AppStatusBar: true}
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	got := string(raw)
	want := `{"office":true,"plugins":true,"appStatusBar":true}`
	if got != want {
		t.Errorf("FeaturesConfig JSON = %s; want %s — missing or wrong `json:` struct tag", got, want)
	}
}

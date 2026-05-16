package config

import (
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

// TestFeatures_DefaultOff pins the production-safety invariant: every
// feature flag in FeaturesConfig is false unless the deployment explicitly
// sets the matching env var. A regression that flips a default to true
// would ship an in-progress feature to users on the next release.
// See docs/decisions/0007-runtime-feature-flags.md.
func TestFeatures_DefaultOff(t *testing.T) {
	// Force a clean env so KANDEV_FEATURES_* from the host shell can't
	// bleed in and turn a default-off check into a default-on accident.
	t.Setenv("KANDEV_FEATURES_OFFICE", "")

	dir := t.TempDir()
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Features.Office {
		t.Errorf("Features.Office = true, want false (production default must be off)")
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

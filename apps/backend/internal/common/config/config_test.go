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

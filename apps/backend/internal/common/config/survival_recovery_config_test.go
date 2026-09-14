package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSurvivalRecoveryConfigDefaults(t *testing.T) {
	cfg, err := LoadWithPath(t.TempDir())
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Agentctl.RecoveryDeadline != 30*time.Second {
		t.Errorf("RecoveryDeadline = %v, want 30s", cfg.Agentctl.RecoveryDeadline)
	}
	if cfg.Agentctl.RecoveryReadTimeout != 2*time.Second {
		t.Errorf("RecoveryReadTimeout = %v, want 2s", cfg.Agentctl.RecoveryReadTimeout)
	}
	if cfg.Agentctl.RecoveryReadRetries != 2 {
		t.Errorf("RecoveryReadRetries = %d, want 2", cfg.Agentctl.RecoveryReadRetries)
	}
	if cfg.Agentctl.UnownedPeriod != 10*time.Minute {
		t.Errorf("UnownedPeriod = %v, want 10m", cfg.Agentctl.UnownedPeriod)
	}
	if cfg.Agentctl.DetachedEventLimit != 100 {
		t.Errorf("DetachedEventLimit = %d, want 100", cfg.Agentctl.DetachedEventLimit)
	}
}

func TestSurvivalRecoveryConfigYAMLRoundTrips(t *testing.T) {
	dir := t.TempDir()
	contents := `agentctl:
  recoveryDeadline: 45s
  recoveryReadTimeout: 500ms
  recoveryReadRetries: 3
  unownedPeriod: 15m
  detachedEventLimit: 250
`
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Agentctl.RecoveryDeadline != 45*time.Second {
		t.Errorf("RecoveryDeadline = %v, want 45s", cfg.Agentctl.RecoveryDeadline)
	}
	if cfg.Agentctl.RecoveryReadTimeout != 500*time.Millisecond {
		t.Errorf("RecoveryReadTimeout = %v, want 500ms", cfg.Agentctl.RecoveryReadTimeout)
	}
	if cfg.Agentctl.RecoveryReadRetries != 3 {
		t.Errorf("RecoveryReadRetries = %d, want 3", cfg.Agentctl.RecoveryReadRetries)
	}
	if cfg.Agentctl.UnownedPeriod != 15*time.Minute {
		t.Errorf("UnownedPeriod = %v, want 15m", cfg.Agentctl.UnownedPeriod)
	}
	if cfg.Agentctl.DetachedEventLimit != 250 {
		t.Errorf("DetachedEventLimit = %d, want 250", cfg.Agentctl.DetachedEventLimit)
	}
}

func TestSurvivalRecoveryConfigEnvOverridesYAML(t *testing.T) {
	dir := t.TempDir()
	contents := "agentctl:\n  recoveryDeadline: 45s\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("KANDEV_ACP_RECOVERY_DEADLINE", "1m")
	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Agentctl.RecoveryDeadline != time.Minute {
		t.Errorf("RecoveryDeadline = %v, want 1m (env should win over YAML)", cfg.Agentctl.RecoveryDeadline)
	}
}

func TestSurvivalRecoveryConfigRejectsOutOfRangeEnv(t *testing.T) {
	cases := []struct {
		name   string
		envVar string
		value  string
	}{
		{"recoveryDeadline too low", "KANDEV_ACP_RECOVERY_DEADLINE", "500ms"},
		{"recoveryDeadline too high", "KANDEV_ACP_RECOVERY_DEADLINE", "10m"},
		{"recoveryReadTimeout too low", "KANDEV_ACP_RECOVERY_READ_TIMEOUT", "50ms"},
		{"recoveryReadRetries negative", "KANDEV_ACP_RECOVERY_READ_RETRIES", "-1"},
		{"recoveryReadRetries too high", "KANDEV_ACP_RECOVERY_READ_RETRIES", "11"},
		{"detachedEventLimit zero", "KANDEV_ACP_DETACHED_EVENT_LIMIT", "0"},
		{"detachedEventLimit too high", "KANDEV_ACP_DETACHED_EVENT_LIMIT", "10001"},
		{"recoveryDeadline malformed", "KANDEV_ACP_RECOVERY_DEADLINE", "not-a-duration"},
		{"recoveryReadRetries malformed", "KANDEV_ACP_RECOVERY_READ_RETRIES", "not-an-int"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.envVar, tc.value)
			if _, err := LoadWithPath(t.TempDir()); err == nil {
				t.Fatalf("LoadWithPath: expected rejection for %s=%q, got no error (value must be rejected, not clamped to default)", tc.envVar, tc.value)
			}
		})
	}
}

func TestSurvivalRecoveryConfigRejectsOutOfRangeYAML(t *testing.T) {
	dir := t.TempDir()
	contents := "agentctl:\n  recoveryDeadline: 10m\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadWithPath(dir); err == nil {
		t.Fatal("LoadWithPath: expected rejection for out-of-range YAML recoveryDeadline, got no error")
	}
}

func TestSurvivalRecoveryConfigRejectsReadBudgetOverCeiling(t *testing.T) {
	dir := t.TempDir()
	// timeout(4s) * (retries(2)+1) = 12s exceeds min(6s, deadline(30s)/5=6s) = 6s.
	contents := "agentctl:\n  recoveryReadTimeout: 4s\n  recoveryReadRetries: 2\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadWithPath(dir); err == nil {
		t.Fatal("LoadWithPath: expected rejection when read-timeout*(retries+1) exceeds the ceiling, got no error")
	}
}

func TestSurvivalRecoveryConfigReadBudgetValidatedAgainstConfiguredDeadline(t *testing.T) {
	dir := t.TempDir()
	// timeout(2s) * (retries(2)+1) = 6s. Against the default 30s deadline this
	// is within the 6s ceiling, but against a configured 10s deadline the
	// ceiling drops to min(6s, 2s) = 2s, so it must be rejected.
	contents := "agentctl:\n  recoveryDeadline: 10s\n  recoveryReadTimeout: 2s\n  recoveryReadRetries: 2\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := LoadWithPath(dir); err == nil {
		t.Fatal("LoadWithPath: expected rejection validated against the actually-configured recoveryDeadline, got no error")
	}
}

func TestSurvivalRecoveryConfigCatalogEntriesPresent(t *testing.T) {
	for _, key := range []string{
		"agentctl.recoveryDeadline",
		"agentctl.recoveryReadTimeout",
		"agentctl.recoveryReadRetries",
		"agentctl.unownedPeriod",
		"agentctl.detachedEventLimit",
	} {
		if _, ok := CatalogEntryForKey(key); !ok {
			t.Errorf("catalog is missing %q", key)
		}
	}
}

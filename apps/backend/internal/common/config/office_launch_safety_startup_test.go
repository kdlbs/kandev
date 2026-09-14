package config

import (
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOfficeLaunchSafetyYAMLBelowMinimumClampsAndStartsNormally pins
// AC-OFFICE-LAUNCH-SAFETY-001.5/003.1/004.3/005.1 and
// AC-OFFICE-BACKPRESSURE-002.1/003.5: a configured office.* launch-safety
// value below 1 must resolve to the documented default and let boot
// proceed, not fail config.Load.
func TestOfficeLaunchSafetyYAMLBelowMinimumClampsAndStartsNormally(t *testing.T) {
	cases := []struct {
		yamlKey string
		want    int
		field   func(*Config) int
	}{
		{"maxConcurrentInstance", 8, func(c *Config) int { return c.Office.MaxConcurrentInstance }},
		{"maxConcurrentWorkspace", 4, func(c *Config) int { return c.Office.MaxConcurrentWorkspace }},
		{"workspaceBudgetPerHour", 120, func(c *Config) int { return c.Office.WorkspaceBudgetPerHour }},
		{"routineBudgetPerHour", 20, func(c *Config) int { return c.Office.RoutineBudgetPerHour }},
		{"promotionAgeMinutes", 15, func(c *Config) int { return c.Office.PromotionAgeMinutes }},
		{"maxCausationDepth", 8, func(c *Config) int { return c.Office.MaxCausationDepth }},
		{"selfTriggerAllowance", 3, func(c *Config) int { return c.Office.SelfTriggerAllowance }},
		{"gateFailureThreshold", 3, func(c *Config) int { return c.Office.GateFailureThreshold }},
	}

	for _, tc := range cases {
		t.Run(tc.yamlKey, func(t *testing.T) {
			dir := t.TempDir()
			configFile := filepath.Join(dir, "config.yaml")
			contents := "office:\n  " + tc.yamlKey + ": 0\n"
			if err := os.WriteFile(configFile, []byte(contents), 0o600); err != nil {
				t.Fatalf("write config: %v", err)
			}
			t.Setenv("KANDEV_SERVER_PORT", "")

			cfg, err := LoadWithPath(dir)
			if err != nil {
				t.Fatalf("LoadWithPath returned an error instead of clamping and starting normally: %v", err)
			}
			if got := tc.field(cfg); got != tc.want {
				t.Fatalf("office.%s = %d, want documented default %d", tc.yamlKey, got, tc.want)
			}
			found := false
			for _, w := range cfg.Source.Warnings {
				if strings.Contains(w, "office."+tc.yamlKey) {
					found = true
				}
			}
			if !found {
				t.Fatalf("warnings = %#v, want one naming office.%s", cfg.Source.Warnings, tc.yamlKey)
			}
		})
	}
}

// TestOfficeLaunchSafetyEnvOutOfRangeLogsWarning pins the same ACs' "log
// the rejected value at warn level" clause for the environment-variable
// path, which already clamped silently before this fix.
func TestOfficeLaunchSafetyEnvOutOfRangeLogsWarning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("KANDEV_SERVER_PORT", "")
	t.Setenv("KANDEV_OFFICE_MAX_CONCURRENT_INSTANCE", "0")

	var buf strings.Builder
	orig := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(orig) })

	cfg, err := LoadWithPath(dir)
	if err != nil {
		t.Fatalf("LoadWithPath: %v", err)
	}
	if cfg.Office.MaxConcurrentInstance != 8 {
		t.Fatalf("office.maxConcurrentInstance = %d, want default 8", cfg.Office.MaxConcurrentInstance)
	}
	if !strings.Contains(buf.String(), "office.maxConcurrentInstance") {
		t.Fatalf("log output = %q, want a warning naming office.maxConcurrentInstance", buf.String())
	}
}

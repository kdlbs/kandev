package config

import (
	"strings"
	"testing"
	"time"
)

func TestConfigurationCatalogIsComplete(t *testing.T) {
	if err := ValidateCatalog(); err != nil {
		t.Fatalf("ValidateCatalog: %v", err)
	}
	entries := ConfigurationCatalog()
	if len(entries) < 50 {
		t.Fatalf("catalog has %d entries, want the stable startup inventory", len(entries))
	}
	for _, key := range []string{
		"server.trustedProxies",
		"tasks.preparationTimeout",
		"credentials.file",
		"limits.ghMaxConcurrent",
		"messageQueue.maxPerSession",
		"agentctl.notificationQueueCapacity",
		"launcher.noBrowser",
	} {
		if _, ok := CatalogEntryForKey(key); !ok {
			t.Errorf("catalog is missing %q", key)
		}
	}
}

func TestConfigurationCatalogMarksSecretsAndExclusions(t *testing.T) {
	for _, key := range []string{"database.password", "auth.jwtSecret", "office.jwtSigningKey"} {
		entry, ok := CatalogEntryForKey(key)
		if !ok || !entry.Sensitive {
			t.Errorf("catalog entry %q is not marked sensitive: %#v", key, entry)
		}
	}
	exclusions := ConfigurationExclusions()
	foundGenerated := false
	for _, exclusion := range exclusions {
		if exclusion.EnvVar == "KANDEV_DESKTOP_HEALTH_TOKEN" && exclusion.Class == "generated" && strings.TrimSpace(exclusion.Reason) != "" {
			foundGenerated = true
		}
	}
	if !foundGenerated {
		t.Fatal("generated health token exclusion is missing")
	}
}

func TestAgentctlStartupConfigRoundTripsAndRejectsInvalidValues(t *testing.T) {
	want := AgentctlStartupConfig{
		Configured:                true,
		IdleTimeout:               2 * time.Hour,
		IdleReaperInterval:        3 * time.Minute,
		NotificationQueueCapacity: 4096,
		OTLPEndpoint:              "http://collector:4318",
	}
	raw, err := EncodeAgentctlStartupConfig(want)
	if err != nil {
		t.Fatalf("EncodeAgentctlStartupConfig: %v", err)
	}
	got, err := DecodeAgentctlStartupConfig(raw)
	if err != nil {
		t.Fatalf("DecodeAgentctlStartupConfig: %v", err)
	}
	if got != want {
		t.Fatalf("decoded contract = %#v, want %#v", got, want)
	}

	invalid := want
	invalid.NotificationQueueCapacity = 1
	if _, err := EncodeAgentctlStartupConfig(invalid); err == nil {
		t.Fatal("EncodeAgentctlStartupConfig accepted an invalid queue capacity")
	}
}

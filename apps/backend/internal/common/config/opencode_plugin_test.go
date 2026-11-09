package config

import (
	"encoding/json"
	"testing"
)

func TestManagedAgentctlStartupConfigCarriesOpenCodePluginFlag(t *testing.T) {
	cfg := &Config{}
	cfg.Features.OpenCodePlugin = false
	if got := cfg.ManagedAgentctlStartupConfig(); got.OpenCodePluginEnabled {
		t.Fatal("OpenCodePluginEnabled = true, want false when the runtime flag is off")
	}
	cfg.Features.OpenCodePlugin = true
	if got := cfg.ManagedAgentctlStartupConfig(); !got.OpenCodePluginEnabled {
		t.Fatal("OpenCodePluginEnabled = false, want true when the runtime flag is on")
	}
}

func TestAgentctlStartupConfigOpenCodePluginRoundTrip(t *testing.T) {
	startup := AgentctlStartupConfig{
		Configured:                true,
		IdleTimeout:               1,
		IdleReaperInterval:        1,
		NotificationQueueCapacity: 1024,
		OpenCodePluginEnabled:     true,
	}
	encoded, err := EncodeAgentctlStartupConfig(startup)
	if err != nil {
		t.Fatalf("EncodeAgentctlStartupConfig: %v", err)
	}
	decoded, err := DecodeAgentctlStartupConfig(encoded)
	if err != nil {
		t.Fatalf("DecodeAgentctlStartupConfig: %v", err)
	}
	if !decoded.OpenCodePluginEnabled {
		t.Fatal("OpenCodePluginEnabled did not survive the child-process encoding round trip")
	}
	var legacy AgentctlStartupConfig
	if err := json.Unmarshal([]byte(`{"configured":true}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.OpenCodePluginEnabled {
		t.Fatal("a peer that omits OpenCodePluginEnabled must decode as disabled")
	}
}

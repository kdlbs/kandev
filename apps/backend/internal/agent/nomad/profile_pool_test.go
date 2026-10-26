package nomad

import (
	"strings"
	"testing"
	"time"
)

func validPoolConfig() map[string]string {
	return map[string]string{
		"address": "https://nomad.example.test:4646", "namespace": "default", "region": "global",
		"datacenter": "dc1", "platform": "darwin/arm64", "nomad_token": "$kandev-secret:secret-nomad-token",
	}
}

func TestParseProfile_LegacyPinnedUnchanged(t *testing.T) {
	p, err := ParseProfileConfig(validTestProfile())
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeLegacyPinned || p.NodeID != "node-b" || p.SSHHost != "worker-b.example" || p.NodePool != "" || p.MemoryMaxMB != 0 {
		t.Fatalf("legacy profile changed: %+v", p)
	}
	cfg := validTestProfile()
	cfg["node_pool"] = "kandev"
	if _, err := ParseProfileConfig(cfg); err == nil {
		t.Fatal("legacy profile accepted a pool-only key")
	}
	cfg = validTestProfile()
	delete(cfg, "workspace_root")
	if _, err := ParseProfileConfig(cfg); err == nil {
		t.Fatal("legacy profile lost a required key")
	}
}

func TestParseProfile_PoolDefaults(t *testing.T) {
	p, err := ParseProfileConfig(validPoolConfig())
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModePool || p.NodePool != "kandev" || p.Placement != PlacementSpread || p.CPUMHz != 100 ||
		p.MemoryMB != 256 || p.MemoryMaxMB != 2048 || p.ExcludeLocal || p.PreferLocal || p.NodeID != "" ||
		p.DisconnectLostAfter != DefaultDisconnectLostAfter {
		t.Fatalf("unexpected defaults: %+v", p)
	}
	cfg := validPoolConfig()
	cfg["placement"], cfg["pin_node_id"], cfg["prefer_local"], cfg["disconnect_lost_after"] = "binpack", "node-1", "true", "2h"
	p, err = ParseProfileConfig(cfg)
	if err != nil || p.Placement != PlacementBinpack || p.PinNodeID != "node-1" || !p.PreferLocal || p.DisconnectLostAfter != 2*time.Hour {
		t.Fatalf("explicit pool config: %+v %v", p, err)
	}
	cfg["pin_node_id"] = "bad/id"
	if _, err = ParseProfileConfig(cfg); err == nil {
		t.Fatal("unsafe pin_node_id accepted")
	}
	cfg = validPoolConfig()
	cfg["placement"] = "random"
	if _, err = ParseProfileConfig(cfg); err == nil {
		t.Fatal("invalid placement accepted")
	}
}

func TestParseProfile_PoolRejectsLegacyTransportKeys(t *testing.T) {
	for _, key := range []string{"ssh_port", "ssh_host_fingerprint", "workspace_root", "allocation_root"} {
		cfg := validPoolConfig()
		cfg[key] = "x"
		_, err := ParseProfileConfig(cfg)
		if err == nil || !strings.Contains(err.Error(), "host, port and host key now come from node metadata") {
			t.Fatalf("%s: err = %v", key, err)
		}
	}
}

func TestParseProfile_ExcludeAndPreferLocalConflict(t *testing.T) {
	cfg := validPoolConfig()
	cfg["exclude_local"], cfg["prefer_local"] = "true", "true"
	if _, err := ParseProfileConfig(cfg); err == nil {
		t.Fatal("exclude_local and prefer_local accepted together")
	}
}

func TestParseProfile_MemoryMaxBelowMemory(t *testing.T) {
	cfg := validPoolConfig()
	cfg["memory_mb"], cfg["memory_max_mb"] = "512", "256"
	if _, err := ParseProfileConfig(cfg); err == nil {
		t.Fatal("memory_max_mb below memory_mb accepted")
	}
}

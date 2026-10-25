package hostutility

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestProfileContextRevisionUsesOpaqueCanonicalLaunchIdentity(t *testing.T) {
	key := []byte("profile-cache-test-key")
	base := ProfileProbeContext{
		Scope:    "user-1:profile-1",
		Env:      map[string]string{"ZED": "secret-value", "CODEX_PATH": "/opt/codex"},
		CLIFlags: []string{"--one", "value"}, CommandPrefix: []string{"npx", "--"},
	}
	revision := profileContextRevision(key, base, "codex-acp", "acp", []string{"npx", "agent"}, false, []string{"HOME"}, 3, 7)
	canonical := ProfileProbeContext{
		Scope:    base.Scope,
		Env:      map[string]string{"CODEX_PATH": "/opt/codex", "ZED": "secret-value"},
		CLIFlags: []string{"--one", "value"}, CommandPrefix: []string{"npx", "--"},
	}
	if got := profileContextRevision(key, canonical, "codex-acp", "acp", []string{"npx", "agent"}, false, []string{"HOME"}, 3, 7); got != revision {
		t.Fatalf("map order changed revision: %q != %q", got, revision)
	}
	if strings.Contains(revision, "secret-value") || strings.Contains(revision, "/opt/codex") {
		t.Fatalf("revision contains launch input: %q", revision)
	}
	changedSecret := canonical
	changedSecret.Env = map[string]string{"CODEX_PATH": "/opt/codex", "ZED": "rotated-secret"}
	if got := profileContextRevision(key, changedSecret, "codex-acp", "acp", []string{"npx", "agent"}, false, []string{"HOME"}, 3, 7); got == revision {
		t.Fatal("secret rotation reused the old context revision")
	}
	changedOrder := canonical
	changedOrder.CLIFlags = []string{"value", "--one"}
	if got := profileContextRevision(key, changedOrder, "codex-acp", "acp", []string{"npx", "agent"}, false, []string{"HOME"}, 3, 7); got == revision {
		t.Fatal("ordered CLI argument change reused the old context revision")
	}
}

func TestProfileCapabilityCacheIsBoundedLRUAndExpires(t *testing.T) {
	cache := newProfileCapabilityCache()
	now := time.Unix(100, 0)
	for i := 0; i < profileCapabilityCacheMaxEntries; i++ {
		cache.setCapability(fmt.Sprintf("key-%03d", i), "codex-acp", ProfileCapabilityResult{
			Capabilities:    AgentCapabilities{AgentType: "codex-acp", Status: StatusOK},
			ContextRevision: fmt.Sprintf("revision-%03d", i),
		}, now)
	}
	if _, ok := cache.getCapability("key-000", now); !ok {
		t.Fatal("first entry missing before LRU eviction")
	}
	cache.setCapability("key-new", "codex-acp", ProfileCapabilityResult{
		Capabilities: AgentCapabilities{AgentType: "codex-acp", Status: StatusUnsupported},
	}, now)
	if _, ok := cache.getCapability("key-000", now); !ok {
		t.Fatal("recently used entry was evicted")
	}
	if _, ok := cache.getCapability("key-001", now); ok {
		t.Fatal("least-recently-used entry survived bounded eviction")
	}
	if got := len(cache.items); got != profileCapabilityCacheMaxEntries {
		t.Fatalf("cache size = %d, want %d", got, profileCapabilityCacheMaxEntries)
	}
	if _, ok := cache.getCapability("key-new", now.Add(profileCapabilityCacheTTL)); ok {
		t.Fatal("expired entry was returned")
	}
}

func TestProfileCapabilityCacheDoesNotStoreTransientFailures(t *testing.T) {
	cache := newProfileCapabilityCache()
	now := time.Now()
	cache.setCapability("failed", "codex-acp", ProfileCapabilityResult{
		Capabilities: AgentCapabilities{AgentType: "codex-acp", Status: StatusFailed},
	}, now)
	if _, ok := cache.getCapability("failed", now); ok {
		t.Fatal("transient failure was cached")
	}
}

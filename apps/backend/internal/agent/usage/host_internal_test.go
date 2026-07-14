package usage

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeHostClient struct {
	hasCreds bool
	usage    *ProviderUsage
	err      error
	calls    int
}

func (f *fakeHostClient) FetchUsage(_ context.Context) (*ProviderUsage, error) {
	f.calls++
	return f.usage, f.err
}

func (f *fakeHostClient) HasSubscriptionCredentials() bool { return f.hasCreds }

func TestHostServiceList(t *testing.T) {
	now := time.Now()
	okUsage := &ProviderUsage{
		Provider:  "anthropic",
		Plan:      "max",
		Windows:   []UtilizationWindow{{Label: "5-hour", UtilizationPct: 42, ResetAt: now}},
		FetchedAt: now,
	}
	noCreds := &fakeHostClient{hasCreds: false}
	ok := &fakeHostClient{hasCreds: true, usage: okUsage}
	failing := &fakeHostClient{hasCreds: true, err: errors.New("boom")}

	svc := &HostService{
		cache: NewUsageCache(),
		entries: []hostEntry{
			{agentID: "claude-acp", cacheKey: "k1", client: noCreds},
			{agentID: "codex-acp", cacheKey: "k2", client: ok},
			{agentID: "other-acp", cacheKey: "k3", client: failing},
		},
	}

	got := svc.List(context.Background(), false)
	if len(got) != 2 {
		t.Fatalf("List = %+v, want 2 entries", got)
	}
	if got[0].AgentID != "codex-acp" || got[0].Usage != okUsage || got[0].Error != "" {
		t.Errorf("entry[0] = %+v", got[0])
	}
	if got[1].AgentID != "other-acp" || got[1].Usage != nil || got[1].Error != hostUsageFetchError {
		t.Errorf("entry[1] = %+v", got[1])
	}
	if noCreds.calls != 0 {
		t.Errorf("client without creds was fetched %d times", noCreds.calls)
	}

	// Second List hits the cache for the successful entry.
	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Errorf("expected cached fetch, got %d calls", ok.calls)
	}
}

func TestHostServiceList_FreshBypassesStaleCache(t *testing.T) {
	now := time.Now()
	okUsage := &ProviderUsage{Provider: "anthropic", FetchedAt: now}
	ok := &fakeHostClient{hasCreds: true, usage: okUsage}
	svc := &HostService{
		cache:   NewUsageCache(),
		entries: []hostEntry{{agentID: "claude-acp", cacheKey: "k1", client: ok}},
	}

	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Fatalf("initial fetch calls = %d", ok.calls)
	}

	// Entry is younger than freshMaxAge: fresh serves the cached value.
	_ = svc.List(context.Background(), true)
	if ok.calls != 1 {
		t.Errorf("fresh within clamp should hit cache, calls = %d", ok.calls)
	}

	// Age the entry past freshMaxAge but below the 5-minute TTL: a normal List
	// still serves the cache, a fresh List re-queries the provider.
	svc.cache.mu.Lock()
	svc.cache.entries["k1"].fetchedAt = now.Add(-freshMaxAge - time.Second)
	svc.cache.mu.Unlock()

	_ = svc.List(context.Background(), false)
	if ok.calls != 1 {
		t.Errorf("stale-but-within-TTL cached List should not refetch, calls = %d", ok.calls)
	}
	_ = svc.List(context.Background(), true)
	if ok.calls != 2 {
		t.Errorf("fresh List should refetch, calls = %d", ok.calls)
	}
}

func TestNewHostServiceRegistersHostAgents(t *testing.T) {
	svc := NewHostService(nil)
	if len(svc.entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(svc.entries))
	}
	if svc.entries[0].agentID != "claude-acp" || svc.entries[1].agentID != "codex-acp" {
		t.Errorf("agent IDs = %q, %q", svc.entries[0].agentID, svc.entries[1].agentID)
	}
}

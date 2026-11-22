package routing_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

func TestResolveAuthorProviderSkippedWithRecord(t *testing.T) {
	r := newResolver(t, &fakeRepo{cfg: twoProviderCfg()})
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}),
		routing.ResolveOptions{AuthorProvider: "claude-acp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ProviderID != "codex-acp" {
		t.Fatalf("candidates = %+v, want codex-acp only", res.Candidates)
	}
	if len(res.SkippedDegraded) != 1 {
		t.Fatalf("skipped = %+v, want one author skip", res.SkippedDegraded)
	}
	sk := res.SkippedDegraded[0]
	if sk.ProviderID != "claude-acp" || sk.Reason != routing.SkipReasonAuthorProvider || sk.AutoRetry {
		t.Fatalf("author skip = %+v", sk)
	}
}

func TestResolveAuthorProviderOnlyProviderBlocksForAction(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.ProviderOrder = []routing.ProviderID{"claude-acp"}
	r := newResolver(t, &fakeRepo{cfg: cfg})
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}),
		routing.ResolveOptions{AuthorProvider: "claude-acp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates = %+v, want none", res.Candidates)
	}
	if res.BlockReason.Status != routing.StatusBlockedActionRequired {
		t.Fatalf("status = %q, want %q", res.BlockReason.Status, routing.StatusBlockedActionRequired)
	}
}

func TestResolveAuthorProviderWithDegradedAlternativeWaitsForCapacity(t *testing.T) {
	retryAt := fixedNow.Add(5 * time.Minute)
	repo := &fakeRepo{
		cfg: twoProviderCfg(),
		health: []models.ProviderHealth{
			degradedRow("codex-acp", "provider", "", "rate_limited", retryAt),
		},
	}
	r := newResolver(t, repo)
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}),
		routing.ResolveOptions{AuthorProvider: "claude-acp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 0 {
		t.Fatalf("candidates = %+v, want none", res.Candidates)
	}
	if res.BlockReason.Status != routing.StatusWaitingForCapacity {
		t.Fatalf("status = %q, want %q", res.BlockReason.Status, routing.StatusWaitingForCapacity)
	}
	if !res.BlockReason.EarliestRetry.Equal(retryAt) {
		t.Fatalf("earliest retry = %v, want %v", res.BlockReason.EarliestRetry, retryAt)
	}
}

func TestResolveAuthorProviderIgnoredWhenRoutingDisabled(t *testing.T) {
	cfg := twoProviderCfg()
	cfg.Enabled = false
	r := newResolver(t, &fakeRepo{cfg: cfg})
	res, err := r.Resolve(context.Background(), wsID, agentWithOverrides(t, routing.AgentOverrides{}),
		routing.ResolveOptions{AuthorProvider: "claude-acp"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Candidates) != 1 || res.Candidates[0].ProviderID != "claude-acp" {
		t.Fatalf("candidates = %+v, want the first provider unchanged", res.Candidates)
	}
}

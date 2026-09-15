package webapp

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTokenManagerStoresOnlyDigestAndBindsReleaseScope(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager(func() time.Time { return now })
	binding := CapabilityBinding{
		UserID:          "user-1",
		InstanceID:      "instance-1",
		ReleaseID:       "release-1",
		WebAppKey:       "main",
		Placement:       "task-canvas",
		ScopeKind:       "task",
		WorkspaceID:     "workspace-1",
		TaskID:          "task-1",
		GrantGeneration: 3,
		Artifact:        Artifact{Digest: strings.Repeat("a", 64), RelativePath: "releases/" + strings.Repeat("a", 64)},
		Entry:           "ui/index.html",
	}
	token, err := manager.Issue(binding, time.Minute)
	if err != nil {
		t.Fatalf("Issue() unexpected error: %v", err)
	}
	if token == "" || manager.StoredTokenCount() != 1 {
		t.Fatalf("token/count = %q/%d", token, manager.StoredTokenCount())
	}
	got, err := manager.Validate(token)
	if err != nil {
		t.Fatalf("Validate() unexpected error: %v", err)
	}
	if got.UserID != binding.UserID || got.ReleaseID != binding.ReleaseID || got.GrantGeneration != 3 {
		t.Fatalf("binding = %+v", got)
	}
	if manager.HasRawToken(token) {
		t.Fatal("token manager retained the raw capability")
	}

	now = now.Add(2 * time.Minute)
	if _, err := manager.Validate(token); !errors.Is(err, ErrRuntimeTokenExpired) {
		t.Fatalf("expired token error = %v, want ErrRuntimeTokenExpired", err)
	}
}

func TestTokenManagerIssueURLDoesNotUseQueryToken(t *testing.T) {
	manager := NewTokenManager(nil)
	binding := CapabilityBinding{UserID: "u", InstanceID: "i", ReleaseID: "r", WebAppKey: "main", Placement: "task-canvas", Artifact: Artifact{Digest: strings.Repeat("b", 64), RelativePath: "releases/" + strings.Repeat("b", 64)}, Entry: "ui/index.html"}
	url, err := manager.IssueURL("http://127.0.0.1:38429", binding, time.Minute)
	if err != nil {
		t.Fatalf("IssueURL() unexpected error: %v", err)
	}
	if strings.Contains(url, "?") || strings.Contains(url, "token=") {
		t.Fatalf("runtime URL contains a query token: %q", url)
	}
}

func TestTokenManagerBoundsExpiredTombstones(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	manager := NewTokenManager(func() time.Time { return now })
	for i := 0; i < maxExpiredCapabilityTombstones+25; i++ {
		manager.expired[digestToken(strings.Repeat("x", i+1))] = now
	}
	if _, err := manager.Issue(CapabilityBinding{UserID: "u", InstanceID: "i", ReleaseID: "r", WebAppKey: "main", Placement: "task-canvas", Entry: "ui/index.html"}, time.Minute); err != nil {
		t.Fatalf("issue token: %v", err)
	}
	if len(manager.expired) > maxExpiredCapabilityTombstones {
		t.Fatalf("expired tombstones = %d, want at most %d", len(manager.expired), maxExpiredCapabilityTombstones)
	}
	now = now.Add(RuntimeTokenTTL + time.Second)
	manager.StoredTokenCount()
	if len(manager.expired) != 0 {
		t.Fatalf("expired tombstones after retention = %d, want 0", len(manager.expired))
	}
}

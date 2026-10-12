package canvas

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestPreparationStoreExpiresAndReleasesStagedFiles(t *testing.T) {
	store, err := NewPreparationStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPreparationStore() error = %v", err)
	}
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	store.SetClock(func() time.Time { return now })
	created, err := store.Create(context.Background(), Preparation{
		ID: "prep-1", UserID: "user-1", WorkspaceID: "workspace-1", CanvasID: "canvas-1",
		ReleaseID: "release-1", PackageDigest: "digest-1", Bundle: []byte("bundle"), Source: []byte("source"),
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.ExpiresAt.Sub(now) != PreparationTTL {
		t.Fatalf("expires in %s, want %s", created.ExpiresAt.Sub(now), PreparationTTL)
	}
	if got, err := store.Open(context.Background(), "user-1", "prep-1", ExportBundle); err != nil || string(got) != "bundle" {
		t.Fatalf("Open(bundle) = %q, %v", got, err)
	}
	now = now.Add(PreparationTTL + time.Second)
	if _, err := store.Get(context.Background(), "user-1", "prep-1"); !errors.Is(err, ErrPreparationExpired) {
		t.Fatalf("Get(expired) error = %v, want ErrPreparationExpired", err)
	}
	if err := store.Cleanup(context.Background()); err != nil {
		t.Fatalf("Cleanup() error = %v", err)
	}
	if _, err := store.Get(context.Background(), "user-1", "prep-1"); !errors.Is(err, ErrPreparationNotFound) {
		t.Fatalf("Get(cleaned) error = %v, want ErrPreparationNotFound", err)
	}
}

func TestPreparationStoreLimitsPerUserAndStagedBytes(t *testing.T) {
	store, err := NewPreparationStore(t.TempDir())
	if err != nil {
		t.Fatalf("NewPreparationStore() error = %v", err)
	}
	for i := 0; i < MaxPreparationsPerUser; i++ {
		_, err := store.Create(context.Background(), Preparation{
			ID: string(rune('a' + i)), UserID: "user-1", WorkspaceID: "workspace-1",
			Bundle: []byte("bundle"), Source: []byte("source"),
		})
		if err != nil {
			t.Fatalf("Create(%d) error = %v", i, err)
		}
	}
	if _, err := store.Create(context.Background(), Preparation{ID: "overflow", UserID: "user-1", WorkspaceID: "workspace-1", Bundle: []byte("bundle")}); !errors.Is(err, ErrPreparationLimit) {
		t.Fatalf("Create(per-user overflow) error = %v, want ErrPreparationLimit", err)
	}
}

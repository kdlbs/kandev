package tempartifacts

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
)

func TestRegistryCreatesAndClosesOwnedArtifact(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, time.August, 8, 12, 0, 0, 0, time.UTC)
	store := newFakeArtifactStore()
	registry := NewRegistry(Config{
		Store: store, TempRoot: root, OwnerPID: 4242,
		Now:      func() time.Time { return now },
		NewID:    func() string { return "artifact-1" },
		NewToken: func() string { return "token-1" },
	})

	lease, err := registry.Create(
		context.Background(), storage.TemporaryArtifactKindImproveBundle, nil,
	)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if filepath.Dir(lease.Path()) != root || filepath.Base(lease.Path()) == "" {
		t.Fatalf("artifact path = %q, want direct child of %q", lease.Path(), root)
	}
	marker := filepath.Join(lease.Path(), MarkerName)
	info, err := os.Stat(marker)
	if err != nil {
		t.Fatalf("marker: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("marker mode = %o, want 600", info.Mode().Perm())
	}

	if err := lease.Heartbeat(context.Background()); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	if err := lease.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
	artifact, err := store.GetTemporaryArtifact(context.Background(), "artifact-1")
	if err != nil {
		t.Fatalf("GetTemporaryArtifact: %v", err)
	}
	if artifact.State != storage.TemporaryArtifactStateClosed || artifact.ClosedAt == nil {
		t.Fatalf("artifact after close = %#v", artifact)
	}
}

func TestRegistryRejectsUnsafeOwnedArtifactPaths(t *testing.T) {
	root := t.TempDir()
	store := newFakeArtifactStore()
	registry := NewRegistry(Config{Store: store, TempRoot: root, OwnerPID: 1})
	lease, err := registry.Create(context.Background(), storage.TemporaryArtifactKindHostUtility, nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	artifact := store.artifacts[lease.Path()]

	marker := filepath.Join(lease.Path(), MarkerName)
	if err := os.Remove(marker); err != nil {
		t.Fatalf("remove marker: %v", err)
	}
	if err := registry.Validate(artifact); err == nil {
		t.Fatal("missing marker was accepted")
	}

	if err := writeMarker(marker, markerData{ID: artifact.ID, Kind: artifact.Kind, Token: "wrong"}); err != nil {
		t.Fatalf("write mismatched marker: %v", err)
	}
	if err := registry.Validate(artifact); err == nil {
		t.Fatal("mismatched marker was accepted")
	}

	if err := os.RemoveAll(lease.Path()); err != nil {
		t.Fatalf("remove artifact: %v", err)
	}
	outside := filepath.Join(root, "outside")
	if err := os.Mkdir(outside, 0o700); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}
	if err := os.Symlink(outside, lease.Path()); err != nil {
		t.Fatalf("replace artifact with symlink: %v", err)
	}
	if err := registry.Validate(artifact); err == nil {
		t.Fatal("symlink replacement was accepted")
	}
}

type fakeArtifactStore struct {
	artifacts map[string]storage.TemporaryArtifact
}

func newFakeArtifactStore() *fakeArtifactStore {
	return &fakeArtifactStore{artifacts: make(map[string]storage.TemporaryArtifact)}
}

func (s *fakeArtifactStore) CreateTemporaryArtifact(_ context.Context, artifact *storage.TemporaryArtifact) error {
	if _, exists := s.artifacts[artifact.Path]; exists {
		return errors.New("duplicate artifact path")
	}
	s.artifacts[artifact.Path] = *artifact
	return nil
}

func (s *fakeArtifactStore) GetTemporaryArtifact(_ context.Context, id string) (storage.TemporaryArtifact, error) {
	for _, artifact := range s.artifacts {
		if artifact.ID == id {
			return artifact, nil
		}
	}
	return storage.TemporaryArtifact{}, storage.ErrNotFound
}

func (s *fakeArtifactStore) ListTemporaryArtifacts(context.Context) ([]storage.TemporaryArtifact, error) {
	artifacts := make([]storage.TemporaryArtifact, 0, len(s.artifacts))
	for _, artifact := range s.artifacts {
		artifacts = append(artifacts, artifact)
	}
	return artifacts, nil
}

func (s *fakeArtifactStore) HeartbeatTemporaryArtifact(_ context.Context, id string, at time.Time) error {
	for path, artifact := range s.artifacts {
		if artifact.ID != id || artifact.State != storage.TemporaryArtifactStateActive {
			continue
		}
		artifact.LastHeartbeatAt = &at
		s.artifacts[path] = artifact
		return nil
	}
	return storage.ErrNotFound
}

func (s *fakeArtifactStore) TransitionTemporaryArtifact(
	_ context.Context,
	id string,
	next storage.TemporaryArtifactState,
	lastError string,
	at time.Time,
) error {
	for path, artifact := range s.artifacts {
		if artifact.ID != id {
			continue
		}
		artifact.State = next
		artifact.LastError = lastError
		if next == storage.TemporaryArtifactStateClosed {
			artifact.ClosedAt = &at
		}
		s.artifacts[path] = artifact
		return nil
	}
	return storage.ErrNotFound
}

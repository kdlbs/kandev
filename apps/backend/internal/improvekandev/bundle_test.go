package improvekandev

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/system/storage"
	"github.com/kandev/kandev/internal/system/storage/tempartifacts"
)

func TestCreateBundleDirWritesOwnerMarker(t *testing.T) {
	dir, err := createBundleDir("user-1")
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if !strings.HasPrefix(filepath.Base(dir), bundlePrefix) {
		t.Errorf("bundle dir base %q must start with %q", filepath.Base(dir), bundlePrefix)
	}

	info, err := os.Stat(filepath.Join(dir, ownerMarkerName))
	if err != nil {
		t.Fatalf("owner marker: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("owner marker mode = %o, want 600", info.Mode().Perm())
	}
	if _, err := validateBundleDir(dir, "user-1"); err != nil {
		t.Fatalf("validate owner: %v", err)
	}
	if _, err := validateBundleDir(dir, "user-2"); err == nil {
		t.Fatal("different owner unexpectedly accepted")
	}
}

func TestValidateBundleDir_AcceptsValid(t *testing.T) {
	dir, err := createBundleDir("user-1")
	if err != nil {
		t.Fatalf("createBundleDir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	resolved, err := validateBundleDir(dir, "user-1")
	if err != nil {
		t.Errorf("validateBundleDir(%q): %v", dir, err)
	}
	if !strings.HasPrefix(filepath.Base(resolved), bundlePrefix) {
		t.Errorf("resolved base %q must start with %q", filepath.Base(resolved), bundlePrefix)
	}
}

func TestValidateBundleDir_RejectsBad(t *testing.T) {
	cases := []struct {
		name string
		dir  string
	}{
		{"empty", ""},
		{"home", "/etc"},
		{"wrong_prefix", filepath.Join(os.TempDir(), "not-kandev")},
		{"missing", filepath.Join(os.TempDir(), "kandev-improve-doesnotexist")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := validateBundleDir(tc.dir, "user-1"); err == nil {
				t.Errorf("expected error for %q", tc.dir)
			}
		})
	}
}

func TestCreateBundleDirWithRegistryDoesNotPersistAuthenticatedOwner(t *testing.T) {
	store := &recordingArtifactStore{}
	registry := tempartifacts.NewRegistry(tempartifacts.Config{
		Store: store, TempRoot: os.TempDir(), OwnerPID: 1234, RunID: "test-run",
		NewID: func() string { return "artifact-1" }, NewToken: func() string { return "token-1" },
	})
	dir, err := createBundleDirWithRegistry(context.Background(), "user-1", registry)
	if err != nil {
		t.Fatalf("createBundleDirWithRegistry: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if strings.Contains(string(store.artifact.Metadata), "user-1") {
		t.Fatalf("temporary artifact metadata contains authenticated owner: %s", store.artifact.Metadata)
	}
	var metadata map[string]any
	if err := json.Unmarshal(store.artifact.Metadata, &metadata); err != nil {
		t.Fatalf("decode temporary artifact metadata: %v", err)
	}
	if _, ok := metadata["owner"]; ok {
		t.Fatalf("temporary artifact metadata contains owner field: %#v", metadata)
	}
}

type recordingArtifactStore struct {
	artifact storage.TemporaryArtifact
}

func (s *recordingArtifactStore) CreateTemporaryArtifact(_ context.Context, artifact *storage.TemporaryArtifact) error {
	s.artifact = *artifact
	return nil
}

func (s *recordingArtifactStore) GetTemporaryArtifact(_ context.Context, id string) (storage.TemporaryArtifact, error) {
	if s.artifact.ID != id {
		return storage.TemporaryArtifact{}, storage.ErrNotFound
	}
	return s.artifact, nil
}

func (s *recordingArtifactStore) ListTemporaryArtifacts(context.Context) ([]storage.TemporaryArtifact, error) {
	if s.artifact.ID == "" {
		return nil, nil
	}
	return []storage.TemporaryArtifact{s.artifact}, nil
}

func (s *recordingArtifactStore) HeartbeatTemporaryArtifact(_ context.Context, id string, at time.Time) error {
	if s.artifact.ID != id || s.artifact.State != storage.TemporaryArtifactStateActive {
		return storage.ErrNotFound
	}
	s.artifact.LastHeartbeatAt = &at
	return nil
}

func (s *recordingArtifactStore) TransitionTemporaryArtifact(
	_ context.Context,
	id string,
	next storage.TemporaryArtifactState,
	lastError string,
	at time.Time,
) error {
	if s.artifact.ID != id {
		return storage.ErrNotFound
	}
	s.artifact.State = next
	s.artifact.LastError = lastError
	if next == storage.TemporaryArtifactStateClosed {
		s.artifact.ClosedAt = &at
	}
	return nil
}

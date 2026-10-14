package recoveryartifact

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddRequestedArtifactIdentitiesCapturesPreviouslyMissingPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery.json")
	if err := os.WriteFile(path, []byte("published recovery journal"), 0o600); err != nil {
		t.Fatal(err)
	}

	identities := make(map[string]string)
	err := addRequestedArtifactIdentities([]string{path}, identities, Registration{
		ArtifactPaths: []string{path},
	})
	if err != nil {
		t.Fatalf("addRequestedArtifactIdentities: %v", err)
	}
	want, ok := FilesystemIdentity(path)
	if !ok {
		t.Fatal("filesystem identity unavailable for a regular file")
	}
	if got := identities[path]; got != want {
		t.Fatalf("identity for previously registered path = %q, want %q", got, want)
	}
}

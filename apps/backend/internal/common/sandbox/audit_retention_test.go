//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStoredAuditArchiveSurvivesRuntimeCleanupAndRejectsSubstitution(t *testing.T) {
	state, scratch, declaration := auditArchiveFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(state, "audit", "events.ndjson"), []byte("recorded evidence\n"), 0600))
	archive, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{declaration}, nil)
	require.NoError(t, err)
	base := t.TempDir()
	require.NoError(t, os.Chmod(base, 0700))
	binding := strings.Repeat("a", 64)
	key, err := StoreAuditArchive(t.Context(), base, binding, archive)
	require.NoError(t, err)
	require.Len(t, key, 64)
	digest := archive.Digest
	require.NoError(t, archive.Close())
	require.NoError(t, os.RemoveAll(state))
	loaded, err := LoadAuditArchive(t.Context(), base, scratch, key, binding, digest)
	require.NoError(t, err)
	content, final, err := loaded.ReadChunk(t.Context(), 0, 0, 256<<10)
	require.NoError(t, err)
	require.True(t, final)
	require.Equal(t, "recorded evidence\n", string(content))
	require.NoError(t, loaded.Close())
	_, err = LoadAuditArchive(t.Context(), base, scratch, key, strings.Repeat("b", 64), digest)
	require.Error(t, err, "another attempt binding must not reuse the archive")
	_, err = LoadAuditArchive(t.Context(), base, scratch, key, binding, strings.Repeat("b", 64))
	require.Error(t, err, "another artifact cannot reuse the archive")
	require.DirExists(t, filepath.Join(base, key), "closing a read must preserve historical evidence")
	require.NoError(t, RemoveAuditArchive(t.Context(), base, binding, digest))
	require.NoDirExists(t, filepath.Join(base, key))
	require.NoError(t, RemoveAuditArchive(t.Context(), base, binding, digest), "repeated session cleanup is safe")
}

func TestStoredAuditArchiveRejectsChangedBytesAndKeepsReadsFrozen(t *testing.T) {
	for _, mutation := range []string{"rewrite", "symlink", "extra", "manifest", "duplicate-fields"} {
		t.Run(mutation, func(t *testing.T) {
			state, scratch, declaration := auditArchiveFixture(t)
			require.NoError(t, os.WriteFile(filepath.Join(state, "audit", "events.ndjson"), []byte("original\n"), 0600))
			archive, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{declaration}, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, archive.Close()) }()
			base, binding := t.TempDir(), strings.Repeat("a", 64)
			require.NoError(t, os.Chmod(base, 0700))
			key, err := StoreAuditArchive(t.Context(), base, binding, archive)
			require.NoError(t, err)
			same, err := StoreAuditArchive(t.Context(), base, binding, archive)
			require.NoError(t, err)
			require.Equal(t, key, same, "a retry must preserve the same immutable archive")
			loaded, err := LoadAuditArchive(t.Context(), base, scratch, key, binding, archive.Digest)
			require.NoError(t, err)
			defer func() { require.NoError(t, loaded.Close()) }()
			file := filepath.Join(base, key, "source-00")
			switch mutation {
			case "rewrite":
				require.NoError(t, os.Chmod(file, 0600))
				require.NoError(t, os.WriteFile(file, []byte("modified\n"), 0600))
			case "symlink":
				require.NoError(t, os.Remove(file))
				require.NoError(t, os.Symlink(filepath.Join(state, "audit", "events.ndjson"), file))
			case "extra":
				require.NoError(t, os.WriteFile(filepath.Join(base, key, "unexpected"), nil, 0600))
			case "manifest":
				require.NoError(t, os.Chmod(filepath.Join(base, key, "manifest.json"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(base, key, "manifest.json"), []byte("{}"), 0600))
			case "duplicate-fields":
				manifest := filepath.Join(base, key, "manifest.json")
				raw, readErr := os.ReadFile(manifest)
				require.NoError(t, readErr)
				require.NoError(t, os.Chmod(manifest, 0600))
				raw = []byte(strings.Replace(string(raw), `"Version":1`, `"Version":1,"Version":1`, 1))
				require.NoError(t, os.WriteFile(manifest, raw, 0600))
			}
			_, err = LoadAuditArchive(t.Context(), base, scratch, key, binding, archive.Digest)
			require.Error(t, err)
			content, _, err := loaded.ReadChunk(t.Context(), 0, 0, 256<<10)
			require.NoError(t, err)
			require.Equal(t, "original\n", string(content), "each verification reads its own frozen bytes")
		})
	}
}

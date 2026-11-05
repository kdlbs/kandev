package sandbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func fixtureRuntimeDigest(content []byte) string {
	value := sha256.Sum256(content)
	return hex.EncodeToString(value[:])
}

func TestSandboxRuntimeFilesArePrivateImmutableCopies(t *testing.T) {
	base := t.TempDir()
	content := []byte("fixture runtime")
	artifacts := []RuntimeArtifact{{Name: "profiles/policy.json", Content: `{"endpoint":""}`, Bindings: []ArtifactBinding{{Path: []string{"endpoint"}, Reference: "callback"}}}}
	files, err := MaterializeRuntime(context.Background(), base, content, fixtureRuntimeDigest(content), artifacts, map[string]string{"callback": "private-fixture"})
	require.NoError(t, err)
	defer func() { require.NoError(t, files.Close()) }()
	content[0] = 'X'
	stored, err := os.ReadFile(files.Executable)
	require.NoError(t, err)
	require.Equal(t, "fixture runtime", string(stored))
	info, err := os.Stat(files.Executable)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0500), info.Mode().Perm())
	profile, err := os.ReadFile(files.Artifacts["profiles/policy.json"])
	require.NoError(t, err)
	require.JSONEq(t, `{"endpoint":"private-fixture"}`, string(profile))
	info, err = os.Stat(files.Artifacts["profiles/policy.json"])
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0400), info.Mode().Perm())
	require.Equal(t, fixtureRuntimeDigest(profile), files.Digests["profiles/policy.json"])
	for _, dir := range []string{files.Root, files.Directories.Home, files.Directories.Config, files.Directories.State, files.Directories.Temp} {
		require.True(t, validRuntimeDirectory(dir), dir)
	}
	require.NoError(t, files.Close())
	require.NoDirExists(t, files.Root)
}

func TestSandboxRuntimeFilesRejectCorruptionAndLeaveNoPartialAttempt(t *testing.T) {
	content := []byte("fixture runtime")
	for _, tc := range []struct {
		name, digest string
		artifacts    []RuntimeArtifact
	}{
		{"wrong runtime", fixtureRuntimeDigest([]byte("different")), nil},
		{"traversal", fixtureRuntimeDigest(content), []RuntimeArtifact{{Name: "../escape", Content: "data"}}},
		{"reserved path", fixtureRuntimeDigest(content), []RuntimeArtifact{{Name: "runtime", Content: "data"}}},
		{"duplicate", fixtureRuntimeDigest(content), []RuntimeArtifact{{Name: "policy", Content: "one"}, {Name: "policy", Content: "two"}}},
		{"unbound secret", fixtureRuntimeDigest(content), []RuntimeArtifact{{Name: "profile.json", Content: `{"token":""}`, Bindings: []ArtifactBinding{{Path: []string{"token"}, Reference: "missing"}}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			files, err := MaterializeRuntime(context.Background(), base, content, tc.digest, tc.artifacts, nil)
			require.Error(t, err)
			require.Nil(t, files)
			entries, err := os.ReadDir(base)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestSandboxRuntimeCleanupRefusesReplacedDirectory(t *testing.T) {
	base := t.TempDir()
	content := []byte("fixture runtime")
	files, err := MaterializeRuntime(context.Background(), base, content, fixtureRuntimeDigest(content), nil, nil)
	require.NoError(t, err)
	require.NoError(t, os.Rename(files.Root, files.Root+"-retained"))
	require.NoError(t, os.Mkdir(files.Root, 0700))
	sentinel := filepath.Join(files.Root, "foreign")
	require.NoError(t, os.WriteFile(sentinel, []byte("preserve"), 0600))
	require.ErrorIs(t, files.Close(), ErrInvalidPolicy)
	require.FileExists(t, sentinel)
}

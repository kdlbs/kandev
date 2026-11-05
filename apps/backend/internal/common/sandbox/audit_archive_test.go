//go:build linux

package sandbox

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditArchiveFreezesLargeDeclaredSources(t *testing.T) {
	state, scratch, source := auditArchiveFixture(t)
	raw := bytes.Repeat([]byte("audit-record\n"), 150000)
	name := filepath.Join(state, "audit", "events.ndjson")
	require.NoError(t, os.WriteFile(name, raw, 0600))
	archive, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, archive.Close()) }()
	require.Len(t, archive.Sources, 1)
	require.Equal(t, "audit/events.ndjson", archive.Sources[0].Name)
	require.Equal(t, int64(len(raw)), archive.Sources[0].Size)
	digest := sha256.Sum256(raw)
	require.Equal(t, hex.EncodeToString(digest[:]), archive.Sources[0].Digest)
	require.NoError(t, os.WriteFile(name, []byte("changed"), 0600))
	var collected []byte
	for offset := int64(0); offset < int64(len(raw)); {
		chunk, final, err := archive.ReadChunk(t.Context(), 0, offset, 256<<10)
		require.NoError(t, err)
		require.LessOrEqual(t, len(chunk), 256<<10)
		collected = append(collected, chunk...)
		offset += int64(len(chunk))
		require.Equal(t, offset == int64(len(raw)), final)
	}
	require.Equal(t, raw, collected, "the provider reads frozen bytes, never a changing file")
}

func TestAuditArchiveAllowsAppendButRejectsRewriteOrReplacement(t *testing.T) {
	for _, change := range []string{"append", "rewrite", "replace"} {
		t.Run(change, func(t *testing.T) {
			state, scratch, source := auditArchiveFixture(t)
			name := filepath.Join(state, "audit", "events.ndjson")
			require.NoError(t, os.WriteFile(name, []byte("first\n"), 0600))
			first, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, first.Close()) }()
			if change == "replace" {
				require.NoError(t, os.Remove(name))
			}
			contents := []byte("first\nsecond\n")
			if change == "rewrite" {
				contents = []byte("other\nsecond\n")
			}
			require.NoError(t, os.WriteFile(name, contents, 0600))
			next, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, first.Sources)
			if change != "append" {
				require.Error(t, err)
				require.Nil(t, next)
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, first.Digest, next.Digest)
			require.NoError(t, next.Close())
		})
	}
}

func TestAuditArchiveRejectsUnsafeSourcesAndCancellation(t *testing.T) {
	for _, change := range []string{"symlink", "oversized", "cancelled", "non-audit"} {
		t.Run(change, func(t *testing.T) {
			state, scratch, source := auditArchiveFixture(t)
			name := filepath.Join(state, "audit", "events.ndjson")
			require.NoError(t, os.WriteFile(name, []byte("events\n"), 0600))
			ctx := t.Context()
			switch change {
			case "symlink":
				require.NoError(t, os.Symlink(name, filepath.Join(state, "audit", "alias.ndjson")))
			case "oversized":
				source.MaxBytes = 1
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "non-audit":
				source.Kind = "identity"
			}
			archive, err := FreezeAuditSources(ctx, state, scratch, []RuntimeSource{source}, nil)
			require.Error(t, err)
			require.Nil(t, archive)
			files, err := os.ReadDir(scratch)
			require.NoError(t, err)
			require.Empty(t, files, "failed freezing removes only its owned scratch files")
		})
	}
}

func auditArchiveFixture(t *testing.T) (string, string, RuntimeSource) {
	t.Helper()
	state, scratch := t.TempDir(), t.TempDir()
	require.NoError(t, os.Chmod(state, 0700))
	require.NoError(t, os.Chmod(scratch, 0700))
	require.NoError(t, os.Mkdir(filepath.Join(state, "audit"), 0700))
	return state, scratch, RuntimeSource{Key: "fixture.audit.v2", Kind: "audit", Directory: "audit", Suffix: ".ndjson", MaxDepth: 1, MaxFiles: 4, MaxBytes: 8 << 20}
}

func TestAuditArchiveRejectsMissingOrAliasedPriorSource(t *testing.T) {
	for _, change := range []string{"missing", "hardlink"} {
		t.Run(change, func(t *testing.T) {
			state, scratch, source := auditArchiveFixture(t)
			name := filepath.Join(state, "audit", "events.ndjson")
			require.NoError(t, os.WriteFile(name, []byte("first\n"), 0600))
			first, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, nil)
			require.NoError(t, err)
			defer func() { require.NoError(t, first.Close()) }()
			if change == "missing" {
				require.NoError(t, os.Remove(name))
			} else {
				require.NoError(t, os.Link(name, filepath.Join(state, "audit", "alias.ndjson")))
			}
			next, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, first.Sources)
			require.Error(t, err)
			require.Nil(t, next)
		})
	}
}

func TestAuditArchiveKeepsEmptyFilesAndStableSourceOrder(t *testing.T) {
	state, scratch, source := auditArchiveFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(state, "audit", "z.ndjson"), []byte("last\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(state, "audit", "a.ndjson"), nil, 0600))
	first, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, nil)
	require.NoError(t, err)
	defer func() { require.NoError(t, first.Close()) }()
	require.Len(t, first.Sources, 2)
	require.Equal(t, "audit/a.ndjson", first.Sources[0].Name)
	content, final, err := first.ReadChunk(t.Context(), 0, 0, 256<<10)
	require.NoError(t, err)
	require.Empty(t, content)
	require.True(t, final)
	next, err := FreezeAuditSources(t.Context(), state, scratch, []RuntimeSource{source}, first.Sources)
	require.NoError(t, err)
	require.Equal(t, first.Digest, next.Digest)
	require.NoError(t, next.Close())
	_, _, err = first.ReadChunk(t.Context(), 2, 0, 256<<10)
	require.Error(t, err)
	_, _, err = first.ReadChunk(t.Context(), 0, 1, 256<<10)
	require.Error(t, err)
	_, _, err = first.ReadChunk(t.Context(), 0, 0, 1<<20)
	require.Error(t, err)
}

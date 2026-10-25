package sandbox

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSandboxRuntimeSourcesReadOnlyBoundedPrivateFiles(t *testing.T) {
	state := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(state, "records", "session"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(state, "records", "session", "audit.ndjson"), []byte("record\n"), 0600))
	source := RuntimeSource{Key: "fixture.audit.v1", Kind: "audit", Directory: "records", Suffix: ".ndjson", MaxDepth: 2, MaxFiles: 4, MaxBytes: 1024}
	records, err := ReadRuntimeSource(t.Context(), state, source)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "record\n", string(records[0].Content))
	source.Kind = "status"
	records, err = ReadRuntimeSource(t.Context(), state, source)
	require.NoError(t, err, "declared status evidence uses the same bounded private reader")
	require.Len(t, records, 1)
	source.MaxDepth = 1
	records, err = ReadRuntimeSource(t.Context(), state, source)
	require.NoError(t, err)
	require.Empty(t, records)
	source.Directory = "missing"
	records, err = ReadRuntimeSource(t.Context(), state, source)
	require.NoError(t, err)
	require.Empty(t, records)
}

func TestSandboxRuntimeSourceSelectsEventFileWithoutLedger(t *testing.T) {
	state := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(state, "records", "session"), 0700))
	require.NoError(t, os.WriteFile(filepath.Join(state, "records", "ledger.ndjson"), []byte("ledger\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(state, "records", "session", "audit-events.ndjson"), []byte("event\n"), 0600))
	source := RuntimeSource{Key: "fixture.audit.v1", Kind: "audit", Directory: "records", Suffix: "audit-events.ndjson", MaxDepth: 2, MaxFiles: 4, MaxBytes: 1024}
	records, err := ReadRuntimeSource(t.Context(), state, source)
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, "event\n", string(records[0].Content))
}

func TestSandboxRuntimeSourcesRejectEscapesAndLimits(t *testing.T) {
	for _, kind := range []string{"parent", "absolute", "symlink", "directory-alias", "oversized", "too-many", "cancelled", "suffix-path", "empty-suffix"} {
		t.Run(kind, func(t *testing.T) {
			state := t.TempDir()
			require.NoError(t, os.Mkdir(filepath.Join(state, "records"), 0700))
			source := RuntimeSource{Key: "fixture.identity.v1", Kind: "identity", Directory: "records", Suffix: ".json", MaxDepth: 1, MaxFiles: 1, MaxBytes: 16}
			require.NoError(t, os.WriteFile(filepath.Join(state, "records", "one.json"), []byte("{}"), 0600))
			ctx := t.Context()
			switch kind {
			case "suffix-path":
				source.Suffix = "../one.json"
			case "empty-suffix":
				source.Suffix = ""
			case "parent":
				source.Directory = "../records"
			case "absolute":
				source.Directory = state
			case "symlink":
				require.NoError(t, os.Symlink(filepath.Join(state, "records", "one.json"), filepath.Join(state, "records", "alias.json")))
			case "directory-alias":
				require.NoError(t, os.Symlink(filepath.Join(state, "records"), filepath.Join(state, "alias")))
				source.Directory = "alias"
			case "oversized":
				source.MaxBytes = 1
			case "too-many":
				require.NoError(t, os.WriteFile(filepath.Join(state, "records", "two.json"), []byte("{}"), 0600))
			case "cancelled":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			records, err := ReadRuntimeSource(ctx, state, source)
			require.Error(t, err)
			require.Empty(t, records)
		})
	}
}

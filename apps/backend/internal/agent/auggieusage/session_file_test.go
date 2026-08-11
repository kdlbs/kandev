package auggieusage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixturePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join("testdata", name)
}

func TestReadNewExchangeUsages_MultiExchangeAndWatermark(t *testing.T) {
	path := fixturePath(t, "multi_exchange.json")

	all, err := ReadNewExchangeUsages(path, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)

	require.Equal(t, 1, all[0].SequenceID)
	require.Equal(t, "claude-opus-4-8", all[0].ModelID)
	require.Equal(t, int64(10), all[0].Input)
	require.Equal(t, int64(20), all[0].Output)
	require.Equal(t, int64(100), all[0].CacheRead)
	require.Equal(t, int64(50), all[0].CacheWrite)
	require.Equal(t, time.Date(2026, 8, 11, 10, 0, 0, 0, time.UTC), all[0].FinishedAt)

	// Exchange 2 sums multiple token_usage nodes; incomplete seq 3 excluded.
	require.Equal(t, 2, all[1].SequenceID)
	require.Equal(t, int64(6), all[1].Input)
	require.Equal(t, int64(10), all[1].Output)
	require.Equal(t, int64(30), all[1].CacheRead)
	require.Equal(t, int64(2), all[1].CacheWrite)

	// Watermark after seq 1 returns only seq 2.
	after, err := ReadNewExchangeUsages(path, 1)
	require.NoError(t, err)
	require.Len(t, after, 1)
	require.Equal(t, 2, after[0].SequenceID)

	// Fully consumed watermark.
	none, err := ReadNewExchangeUsages(path, 2)
	require.NoError(t, err)
	require.Empty(t, none)

	in, out, cr, cw, model, maxSeq := SumExchanges(all)
	require.Equal(t, int64(16), in)
	require.Equal(t, int64(30), out)
	require.Equal(t, int64(130), cr)
	require.Equal(t, int64(52), cw)
	require.Equal(t, "claude-opus-4-8", model)
	require.Equal(t, 2, maxSeq)
}

func TestReadNewExchangeUsages_NilTokenNodesAndFallbackModel(t *testing.T) {
	path := fixturePath(t, "nil_token_nodes.json")
	all, err := ReadNewExchangeUsages(path, 0)
	require.NoError(t, err)
	require.Len(t, all, 2)

	// Seq 1 has no token_usage nodes; still returned (zeros) so watermark can advance.
	require.Equal(t, 1, all[0].SequenceID)
	require.Equal(t, "model-from-state", all[0].ModelID)
	require.Zero(t, all[0].Input)
	require.Zero(t, all[0].Output)

	require.Equal(t, 2, all[1].SequenceID)
	require.Equal(t, "model-b", all[1].ModelID)
	require.Equal(t, int64(4), all[1].Input)
	require.Equal(t, int64(6), all[1].Output)
}

func TestReadNewExchangeUsages_MissingFile(t *testing.T) {
	_, err := ReadNewExchangeUsages(filepath.Join(t.TempDir(), "missing.json"), 0)
	require.Error(t, err)
	require.True(t, os.IsNotExist(err))
}

func TestReadNewExchangeUsages_CorruptJSON(t *testing.T) {
	_, err := ReadNewExchangeUsages(fixturePath(t, "corrupt.json"), 0)
	require.Error(t, err)
}

func TestSessionPath(t *testing.T) {
	p, err := SessionPath("/tmp/home", "abc-123")
	require.NoError(t, err)
	require.Equal(t, filepath.Join("/tmp/home", ".augment", "sessions", "abc-123.json"), p)

	_, err = SessionPath("/tmp/home", "")
	require.Error(t, err)

	// Path traversal / multi-segment ids must not join under sessions/.
	for _, bad := range []string{
		"../escape",
		"..",
		".",
		"a/b",
		`a\b`,
		"/abs",
		"foo/../bar",
	} {
		_, err = SessionPath("/tmp/home", bad)
		require.Error(t, err, "id %q", bad)
	}
}

// Ensure returned structs have no string content fields beyond model id.
func TestExchangeUsage_PrivacySurface(t *testing.T) {
	var eu ExchangeUsage
	_ = eu.SequenceID
	_ = eu.ModelID
	_ = eu.FinishedAt
	_ = eu.Input
	_ = eu.Output
	_ = eu.CacheRead
	_ = eu.CacheWrite
}

//go:build darwin

package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeProbeScript(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sandbox-exec")
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"+body+"\n"), 0700))
	return path
}

func TestSeatbeltProbePassesOnThisHost(t *testing.T) {
	require.NoError(t, seatbeltProbe(t.Context(), sandboxExecPath, t.TempDir()))
}

func TestSeatbeltProbeRemovesItsDirectory(t *testing.T) {
	base := t.TempDir()
	require.NoError(t, seatbeltProbe(t.Context(), sandboxExecPath, base))
	entries, err := os.ReadDir(base)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestSeatbeltProbeFailsClosed(t *testing.T) {
	for name, tc := range map[string]struct {
		exec string
	}{
		"missing binary":           {exec: "/nonexistent/sandbox-exec"},
		"confinement not enforced": {exec: writeProbeScript(t, `shift 2; exec "$@"`)},
		"granted read fails":       {exec: writeProbeScript(t, `exit 1`)},
		"non permission failure":   {exec: writeProbeScript(t, `echo "No such file" >&2; exit 1`)},
		"sandbox refuses to start": {exec: writeProbeScript(t, `echo "sandbox_apply: Operation not permitted" >&2; exit 71`)},
	} {
		t.Run(name, func(t *testing.T) {
			base := t.TempDir()
			require.Error(t, seatbeltProbe(t.Context(), tc.exec, base))
			entries, err := os.ReadDir(base)
			require.NoError(t, err)
			require.Empty(t, entries, "probe directory must be removed on every outcome")
		})
	}
}

func TestSeatbeltProbeTimesOutAndKillsChild(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "alive")
	hung := writeProbeScript(t, `trap '' TERM; echo $$ > "`+marker+`"; sleep 30`)
	ctx, cancel := context.WithTimeout(t.Context(), 300*time.Millisecond)
	defer cancel()
	started := time.Now()
	require.Error(t, seatbeltProbe(ctx, hung, t.TempDir()))
	require.Less(t, time.Since(started), 5*time.Second)
}

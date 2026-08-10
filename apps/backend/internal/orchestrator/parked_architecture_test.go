package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
)

// parkedFeatureSourceFiles lists the source files (excluding tests) this
// slice introduces end to end: the parked projection, its accessors, the
// probe port, and the settle hook. AC-35 narrows the parent monolith's list
// to these two — V1 ships no sampling loop or recogniser registry (D-5, D-7).
var parkedFeatureSourceFiles = []string{
	"parked_state.go",
	"parked_projection.go",
}

// parkedFeatureForbiddenIdentifiers are the flag key, config field, and
// accessor names AC-35 forbids the parked-projection symbols from
// referencing, so this slice stays independent of
// features.claudeBackgroundPromptHandoff, which remains a wholly separate
// gate (referenced legitimately elsewhere in package orchestrator for the
// unrelated ForegroundActivity handoff — service.go:60 and
// turn_activity.go:1193/:1200/:1214/:1329).
var parkedFeatureForbiddenIdentifiers = []string{
	"claudeBackgroundPromptHandoff",
	"ClaudeBackgroundPromptHandoff",
	"claudeBackgroundPromptHandoffEnabled",
	"claudeBackgroundPromptHandoffEnabledForSession",
}

// TestParkedFeatureNeverReferencesTheHandoffFlag is AC-35's architecture
// clause: none of the symbols this slice introduces may reference the
// claudeBackgroundPromptHandoff flag key, the
// Features.ClaudeBackgroundPromptHandoff config field, or the
// claudeBackgroundPromptHandoffEnabled(ForSession) accessors. Scoped to
// file/symbol granularity, not package granularity — package orchestrator
// legitimately references that flag elsewhere for an unrelated gate, so a
// package-wide check would be false by construction.
func TestParkedFeatureNeverReferencesTheHandoffFlag(t *testing.T) {
	_, sourceFile, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")
	dir := filepath.Dir(sourceFile)

	var violations []string
	for _, name := range parkedFeatureSourceFiles {
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		require.NoError(t, err, "read %s", name)
		src := string(data)
		for _, forbidden := range parkedFeatureForbiddenIdentifiers {
			if strings.Contains(src, forbidden) {
				violations = append(violations, name+" references forbidden identifier "+forbidden)
			}
		}
	}
	sort.Strings(violations)
	require.Empty(t, violations, "parked-projection source reads the claudeBackgroundPromptHandoff flag (AC-35):\n%s", strings.Join(violations, "\n"))
}

// TestComputeParked_IdenticalWithFlagForcedOnOrOff is AC-35's behavioural
// clause: a parked session's projection is computed identically for the same
// inputs whether features.claudeBackgroundPromptHandoff is forced on or off.
func TestComputeParked_IdenticalWithFlagForcedOnOrOff(t *testing.T) {
	ctx := context.Background()
	probe := &fakeBackgroundProbe{result: probeResultLive}
	svc := parkedTestService(t, "t-ac35", "s-ac35", probe)

	for _, flag := range []bool{true, false} {
		svc.config.ClaudeBackgroundPromptHandoff = flag
		attestShellLaunch(ctx, svc, "t-ac35", "s-ac35")

		svc.updateTaskSessionState(ctx, "t-ac35", "s-ac35", models.TaskSessionStateWaitingForInput, "", false)

		require.True(t, svc.ParkedProjectionSnapshot("s-ac35"), "flag=%v: expected parked=true", flag)

		// Reset back to RUNNING so the next iteration's WAITING_FOR_INPUT
		// transition re-exercises the settle hook from a clean slate.
		svc.updateTaskSessionState(ctx, "t-ac35", "s-ac35", models.TaskSessionStateRunning, "", true)
	}
}

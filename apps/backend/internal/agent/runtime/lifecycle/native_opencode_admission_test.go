package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestNativeOpenCodeCommandRejectsRevokedContext(t *testing.T) {
	writeNativeOpenCodeVersionFixture(t, "1.18.32")
	for _, deadline := range []bool{false, true} {
		t.Run(fmt.Sprint(deadline), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			want := context.Canceled
			if deadline {
				cancel()
				ctx, cancel = context.WithDeadline(context.Background(), time.Unix(0, 0))
				want = context.DeadlineExceeded
			} else {
				cancel()
			}
			defer cancel()
			m := nativeOpenCodeAdmissionManager("1.18.32")
			_, err := m.resolveOpenCodeCommandOptions(ctx, agentruntime.RuntimeStandalone,
				agents.NewOpenCodeACP(), m.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader))
			if !errors.Is(err, want) {
				t.Fatalf("managed command error = %v, want %v", err, want)
			}
			cmd, err := m.openCodePassthroughCommand(ctx, agents.NewOpenCodeACP(), agentruntime.RuntimeStandalone)
			if !errors.Is(err, want) || !cmd.IsEmpty() {
				t.Fatalf("interactive command = %v, error = %v, want empty and %v", cmd, err, want)
			}
		})
	}
}

func TestNativeOpenCodeTransientProbeRequiresSupportedSelectedVersion(t *testing.T) {
	writeNativeOpenCodeProbeFailureFixture(t)
	for _, version := range []string{"1.18.32", "2.0.18", "", "invalid", "3.0.0"} {
		t.Run(version, func(t *testing.T) {
			m := nativeOpenCodeAdmissionManager(version)
			ctx := context.Background()
			options, managedErr := m.resolveOpenCodeCommandOptions(ctx, agentruntime.RuntimeStandalone,
				agents.NewOpenCodeACP(), m.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader))
			cmd, interactiveErr := m.openCodePassthroughCommand(ctx, agents.NewOpenCodeACP(), agentruntime.RuntimeStandalone)
			_, versionErr := agents.OpenCodeACPArgsForVersion(version)
			if versionErr != nil {
				if managedErr == nil || interactiveErr == nil || !cmd.IsEmpty() {
					t.Fatalf("unsupported selection: options=%+v managed=%v interactive=%v command=%v", options, managedErr, interactiveErr, cmd)
				}
				return
			}
			if managedErr != nil || interactiveErr != nil || cmd.IsEmpty() || options.NativeRuntimeVersion != version {
				t.Fatalf("supported selection: options=%+v managed=%v interactive=%v command=%v", options, managedErr, interactiveErr, cmd)
			}
		})
	}
}

func TestNativeOpenCodeUnsupportedProbeNeverFallsBack(t *testing.T) {
	writeNativeOpenCodeVersionFixture(t, "3.0.0")
	m := nativeOpenCodeAdmissionManager("1.18.32")
	ctx := context.Background()
	_, err := m.resolveOpenCodeCommandOptions(ctx, agentruntime.RuntimeStandalone,
		agents.NewOpenCodeACP(), m.managedRuntimeSelections.(managedruntime.OpenCodeSelectionReader))
	if !agents.IsUnsupportedOpenCodeMajorError(err) {
		t.Fatalf("managed error = %v, want definite unsupported major", err)
	}
	cmd, err := m.openCodePassthroughCommand(ctx, agents.NewOpenCodeACP(), agentruntime.RuntimeStandalone)
	if !agents.IsUnsupportedOpenCodeMajorError(err) || !cmd.IsEmpty() {
		t.Fatalf("interactive command=%v error=%v, want empty and unsupported major", cmd, err)
	}
}

func TestNativeOpenCodeFallbackPreservesProbeIdentity(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	probeErr := fmt.Errorf("probe attempt: %w", context.DeadlineExceeded)
	options, err := m.fallbackOpenCodeCommandOptions(probeErr,
		agents.OpenCodeRuntimeResolution{Version: "2.0.18"}, agents.CommandOptions{})
	if err != nil || options.NativeRuntimeVersion != "2.0.18" {
		t.Fatalf("live caller's expired probe attempt: options=%+v error=%v", options, err)
	}
	for _, version := range []string{"", "invalid", "1.2.3-beta.1", "3.0.0"} {
		_, err := m.fallbackOpenCodeCommandOptions(probeErr,
			agents.OpenCodeRuntimeResolution{Version: version}, agents.CommandOptions{})
		if !errors.Is(err, probeErr) {
			t.Fatalf("selected version %q: error=%v, want original probe identity", version, err)
		}
	}
	cancelledProbe := fmt.Errorf("probe cancelled: %w", context.Canceled)
	_, err = m.fallbackOpenCodeCommandOptions(cancelledProbe,
		agents.OpenCodeRuntimeResolution{Version: "1.18.32"}, agents.CommandOptions{})
	if !errors.Is(err, cancelledProbe) {
		t.Fatalf("cancelled probe error=%v, want original identity", err)
	}
}

func nativeOpenCodeAdmissionManager(version string) *Manager {
	family, packageName := managedruntime.OpenCodeFamilyV1, "opencode-ai"
	if len(version) > 0 && version[0] == '2' {
		family, packageName = managedruntime.OpenCodeFamilyV2, "@opencode/cli"
	}
	m := &Manager{logger: newTestLogger()}
	m.SetManagedRuntimeSelectionStore(managedRuntimeOpenCodeStore{openCodeSelection: managedruntime.OpenCodeSelection{
		SchemaVersion: 1, Family: family, Source: managedruntime.OpenCodeSourceNative,
		Package: packageName, AppliedDefaultVersion: version, Revision: 1,
	}})
	return m
}

func writeNativeOpenCodeProbeFailureFixture(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	name, body := "opencode", "#!/bin/sh\nexit 1\n"
	if runtime.GOOS == "windows" {
		name, body = "opencode.cmd", "@exit /b 1\r\n"
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}

package lifecycle

import (
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
)

func TestFallbackOpenCodeCommandOptionsUsesSelectedVersionOnTransientProbeFailure(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	selected := agents.OpenCodeRuntimeResolution{
		Family:  managedruntime.OpenCodeFamilyV1,
		Source:  managedruntime.OpenCodeSourceNative,
		Version: "1.18.33",
	}
	options := agents.CommandOptions{ManagedRuntimeVersion: "1.18.33"}

	got, err := m.fallbackOpenCodeCommandOptions(
		errors.New("read native OpenCode version: exit status 1"), selected, options)
	if err != nil {
		t.Fatalf("fallback returned error: %v", err)
	}
	if got.NativeRuntimeVersion != "1.18.33" {
		t.Fatalf("NativeRuntimeVersion = %q, want the selected 1.18.33", got.NativeRuntimeVersion)
	}
	if got.ManagedRuntimeVersion != "1.18.33" {
		t.Fatalf("fallback dropped the managed version: %+v", got)
	}
}

func TestFallbackOpenCodeCommandOptionsFailsClosedWithoutAUsableVersion(t *testing.T) {
	m := &Manager{logger: newTestLogger()}
	probeErr := errors.New("read native OpenCode version: exit status 1")
	selected := agents.OpenCodeRuntimeResolution{Version: "not-a-version"}

	if _, err := m.fallbackOpenCodeCommandOptions(probeErr, selected, agents.CommandOptions{}); err == nil {
		t.Fatal("fallback without a usable selected version must preserve the probe error")
	}
}

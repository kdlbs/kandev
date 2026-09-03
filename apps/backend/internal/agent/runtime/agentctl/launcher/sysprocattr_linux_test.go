//go:build linux

package launcher

import (
	"syscall"
	"testing"
)

// TestBuildSysProcAttrOmitsPdeathsigWhenSurvivalEnabled pins kill-path #2
// from design 01's kill-paths list ("Pdeathsig"): AC-EXECUTORS-SURVIVAL-001.2
// requires agentctl to keep running when the backend exits without a
// graceful shutdown, including SIGKILL, "without depending on any backend
// shutdown step" -- Pdeathsig fires from the kernel on ANY parent exit, so it
// must not be armed at all when the capability is engaged for this launch.
func TestBuildSysProcAttrOmitsPdeathsigWhenSurvivalEnabled(t *testing.T) {
	attr := buildSysProcAttr(true)
	if attr.Pdeathsig != 0 {
		t.Fatalf("Pdeathsig = %v, want unset (0) when the capability is engaged", attr.Pdeathsig)
	}
	if !attr.Setpgid {
		t.Error("Setpgid must stay true regardless of the capability: it isolates agentctl from terminal Ctrl+C, unrelated to survival")
	}
}

// TestBuildSysProcAttrSetsPdeathsigWhenSurvivalDisabled is the regression
// companion: today's behavior is unchanged when the capability is off.
func TestBuildSysProcAttrSetsPdeathsigWhenSurvivalDisabled(t *testing.T) {
	attr := buildSysProcAttr(false)
	if attr.Pdeathsig != syscall.SIGTERM {
		t.Fatalf("Pdeathsig = %v, want SIGTERM when the capability is disabled", attr.Pdeathsig)
	}
}

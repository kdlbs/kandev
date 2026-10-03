package runtime_test

import (
	"testing"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

func TestRuntimeExportsAgentRuntimeTypes(t *testing.T) {
	t.Parallel()

	ref := agentruntime.ExecutionReference{SessionID: "session", ExecutionID: "execution"}
	if ref != (lifecycle.ExecutionReference{SessionID: "session", ExecutionID: "execution"}) {
		t.Fatalf("runtime execution reference changed: %#v", ref)
	}

	probe := agentctlclient.ProbeResultUnknown
	if probe != agentruntime.ProbeResultUnknown {
		t.Fatalf("runtime probe result changed: %q", probe)
	}

	assertGitLogResult := func(*agentruntime.GitLogResult) {}
	assertCumulativeDiffResult := func(*agentruntime.CumulativeDiffResult) {}
	assertGitStatusResult := func(*agentruntime.GitStatusResult) {}
	assertGitLogResult((*agentctlclient.GitLogResult)(nil))
	assertCumulativeDiffResult((*agentctlclient.CumulativeDiffResult)(nil))
	assertGitStatusResult((*agentctlclient.GitStatusResult)(nil))
}

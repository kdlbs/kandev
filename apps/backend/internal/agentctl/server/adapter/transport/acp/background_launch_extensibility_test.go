package acp

import (
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/acp/backgroundlaunch"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

type fakeVendorLaunchRecognizer struct{ agentID string }

func (f fakeVendorLaunchRecognizer) AgentID() string { return f.agentID }

func (f fakeVendorLaunchRecognizer) RecognizesDetachedLaunch(payload *streams.NormalizedPayload) bool {
	return payload != nil && payload.ShellExec() != nil && payload.ShellExec().Background
}

// AC-69(a): registering a second agent's recogniser through the public
// backgroundlaunch.Register API — with zero change to stampBackgroundShellWork
// or any other production code in this package — is sufficient for that
// agent's detached shell launches to attest. This is the observable half of
// "adding a vendor changes nothing else" (D7);
// TestPackageImports_ExcludeProbeProjectionOrchestratorAndFrontend in the
// backgroundlaunch package is the structural half.
func TestStampBackgroundShellWork_RecognizesASecondRegisteredAgent(t *testing.T) {
	const fakeAgentID = "acp-extensibility-test-agent"
	backgroundlaunch.Register(fakeVendorLaunchRecognizer{agentID: fakeAgentID})
	t.Cleanup(func() { backgroundlaunch.Unregister(fakeAgentID) })

	payload := streams.NewShellExec("sleep 300", "", "", 0, true)
	stampBackgroundShellWork(fakeAgentID, payload)

	bw := payload.BackgroundWork()
	if bw == nil || bw.Kind != streams.BackgroundWorkKindShell || !bw.Detached {
		t.Fatalf("expected the second registered agent's detached shell launch to attest, got %+v", bw)
	}
}

// AC-37: an agent with no registered recogniser is never attested.
func TestStampBackgroundShellWork_UnregisteredAgentDoesNotAttest(t *testing.T) {
	payload := streams.NewShellExec("sleep 300", "", "", 0, true)
	stampBackgroundShellWork("an-agent-nobody-registered-for-this-test", payload)

	if bw := payload.BackgroundWork(); bw != nil && bw.Detached {
		t.Fatalf("expected an unregistered agent to never attest, got %+v", bw)
	}
}

// Regression guard for the pre-registry behaviour: Claude's own recogniser
// (registered via init() in background_launch_recognizer.go) still attests
// through the same stampBackgroundShellWork entry point.
func TestStampBackgroundShellWork_ClaudeStillAttestsThroughTheRegistry(t *testing.T) {
	payload := streams.NewShellExec("sleep 300", "", "", 0, true)
	stampBackgroundShellWork(claudeAgentID, payload)

	bw := payload.BackgroundWork()
	if bw == nil || bw.Kind != streams.BackgroundWorkKindShell || !bw.Detached {
		t.Fatalf("expected Claude's registered recognizer to still attest, got %+v", bw)
	}
}

func TestStampBackgroundShellWork_ClaudeForegroundShellDoesNotAttest(t *testing.T) {
	payload := streams.NewShellExec("ls", "", "", 0, false)
	stampBackgroundShellWork(claudeAgentID, payload)

	if bw := payload.BackgroundWork(); bw != nil && bw.Detached {
		t.Fatalf("expected a foreground shell to not attest, got %+v", bw)
	}
}

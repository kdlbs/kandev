package controller

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/agent/settings/dto"
)

type localInstallTestAgent struct {
	testAgent
	installed bool
}

func (a *localInstallTestAgent) SupportsLocalInstall() bool { return true }

func (a *localInstallTestAgent) LocalInstall(_ context.Context, progress func(string)) error {
	progress("local install output\n")
	a.installed = true
	return nil
}

func TestAntigravityLocalInstallJob(t *testing.T) {
	ag := &localInstallTestAgent{testAgent: testAgent{id: "antigravity-acp", name: "antigravity-acp", enabled: true}}
	ctrl, hub := newInstallController(t, ag)

	snap, err := ctrl.EnqueueInstall(ag.ID())
	if err != nil {
		t.Fatalf("EnqueueInstall: %v", err)
	}
	done := waitForStatus(t, ctrl, snap.JobID, dto.InstallJobStatusSucceeded)
	if done.Output == "" {
		t.Error("local install job output is empty")
	}
	if !ag.installed {
		t.Error("local install action did not run")
	}
	actions := hub.waitForAction(t, "agent.install.finished")
	if len(actions) == 0 {
		t.Error("no install job broadcasts")
	}
}

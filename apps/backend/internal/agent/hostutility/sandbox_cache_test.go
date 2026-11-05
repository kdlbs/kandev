package hostutility

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	policy "github.com/kandev/kandev/internal/agent/sandbox"
	utility "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/stretchr/testify/require"
)

func TestProfileDiscoveryRejectsPolicyChangeDuringProbe(t *testing.T) {
	for _, modelOptions := range []bool{false, true} {
		name := "capabilities"
		if modelOptions {
			name = "model options"
		}
		t.Run(name, func(t *testing.T) {
			var changed atomic.Bool
			var mutate atomic.Bool
			manager, _ := newProfileProbeTestManager(t, agents.NewClaudeACP(), nil, func(w http.ResponseWriter, _ utility.ProbeRequest) {
				if mutate.Load() {
					changed.Store(true)
				}
				_, _ = w.Write([]byte(`{"success":true,"models":[{"id":"model","name":"Model"}],"config_options":[]}`))
			})
			manager.SetSandboxAdmission(func(_ context.Context, probe *ProfileProbeContext) error {
				probe.SandboxSelection, probe.SandboxDigest = nil, ""
				if changed.Load() {
					probe.SandboxSelection = &policy.SelectionContext{OrgID: "org", ExecutorID: "local"}
					probe.SandboxDigest = "new-selection"
				}
				return nil
			})
			call := func() error {
				scope := ProfileProbeContext{Scope: "operator:profile", ExecutorID: "local"}
				if modelOptions {
					_, err := manager.ResolveModelConfig(t.Context(), "claude-acp", ModelConfigResolutionRequest{Model: "model", ProfileContext: &scope, Refresh: true})
					return err
				}
				_, err := manager.ProbeProfileCapabilities(t.Context(), "claude-acp", ProfileCapabilityRequest{Context: scope, Refresh: true})
				return err
			}
			require.NoError(t, call(), "unchanged policy permits ordinary discovery")
			mutate.Store(true)
			require.ErrorIs(t, call(), errProfileContextChanged, "a policy change must invalidate the in-flight discovery result")
		})
	}
}

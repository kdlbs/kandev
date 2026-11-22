package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/agent/copilotplugins"
	"github.com/kandev/kandev/internal/task/models"
)

func TestCopilotPluginBootstrapFailurePreservesReason(t *testing.T) {
	pluginErr := &copilotplugins.Error{
		Reason:   copilotplugins.ReasonInvalidInventory,
		PluginID: "private-market/plugin-name",
	}
	failure := bootstrapFailureFor(copilotExecution(t, "profile-1"), pluginErr)
	if failure == nil {
		t.Fatal("bootstrapFailureFor returned nil")
	}
	if got, want := failure.SafeCode(), "plugin_preparation_failed"; got != want {
		t.Fatalf("safe code = %q, want %q", got, want)
	}
	if got, want := failure.SafeDetail(), "The selected Copilot plugins could not be prepared."; got != want {
		t.Fatalf("safe detail = %q, want constant %q", got, want)
	}
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
	if !ok {
		t.Fatal("expected normalized safe plugin cause")
	}
	if cause.Code != "plugin_preparation_failed" || cause.Reason != string(copilotplugins.ReasonInvalidInventory) {
		t.Fatalf("safe cause = %+v, want typed plugin reason", cause)
	}
	encoded, err := json.Marshal(cause)
	if err != nil {
		t.Fatal(err)
	}
	for _, private := range []string{"private-market", "plugin-name", pluginErr.Error()} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("safe plugin cause exposes private provider detail: %s", encoded)
		}
	}
}

func TestCopilotPluginPreparationFailureCapturesSelectedRuntimeForManualConflict(t *testing.T) {
	_, configDir, runner := copilotPluginFixture(t)
	manualRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(manualRoot, "plugin.json"), []byte(`{"name":"spark","version":"2.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	manager := newCopilotLifecycleManager(t, copilotLaunchProfile(), nil)
	manager.SetCopilotPluginRunner(runner)
	execution := copilotExecution(t, "profile-1")
	args := append(copilotBaseArgs(), "--plugin-dir", manualRoot)

	_, err := manager.prepareCopilotPluginArgs(
		context.Background(), execution,
		map[string]string{"COPILOT_HOME": configDir, "PATH": os.Getenv("PATH")},
		args,
	)
	if err == nil {
		t.Fatal("expected selected and manual plugin mounts to conflict")
	}
	var pluginErr *copilotplugins.Error
	if !errors.As(err, &pluginErr) || pluginErr.Reason != copilotplugins.ReasonPluginMountConflict {
		t.Fatalf("preparation error = %v, want typed manual mount conflict", err)
	}
	var preparationFailure *copilotPluginPreparationFailure
	if !errors.As(err, &preparationFailure) || preparationFailure.RuntimeVersion != "1.0.88" {
		t.Fatalf("preparation context = %+v, want captured runtime 1.0.88", preparationFailure)
	}
	failure := bootstrapFailureFor(execution, err)
	cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
	if !ok || cause.Reason != string(copilotplugins.ReasonPluginMountConflict) || cause.RuntimeVersion != "1.0.88" {
		t.Fatalf("safe startup cause = %+v, ok=%v; want captured runtime and conflict reason", cause, ok)
	}
}

func TestCopilotPluginBootstrapFailureRejectsForeignAndInvalidEvidence(t *testing.T) {
	pluginErr := &copilotplugins.Error{Reason: copilotplugins.ReasonInvalidInventory, PluginID: "private-plugin"}
	for _, test := range []struct {
		name      string
		execution *AgentExecution
		err       error
	}{
		{
			name:      "foreign provider",
			execution: &AgentExecution{AgentID: "claude-acp"},
			err:       pluginErr,
		},
		{
			name:      "unknown reason",
			execution: &AgentExecution{AgentID: copilotplugins.ProviderID},
			err: &copilotplugins.Error{
				Reason:   copilotplugins.ReasonCode("private_native_document"),
				PluginID: "private-plugin",
			},
		},
		{
			name:      "invalid captured runtime",
			execution: &AgentExecution{AgentID: copilotplugins.ProviderID},
			err:       preserveCopilotPluginRuntimeVersion(pluginErr, "latest"),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			failure := bootstrapFailureFor(test.execution, test.err)
			if failure.SafeCode() != models.AgentErrorCauseCodeUnknown {
				t.Fatalf("safe code = %q, want generic fallback", failure.SafeCode())
			}
			if got, want := failure.SafeDetail(), "The bootstrap operation could not be completed."; got != want {
				t.Fatalf("safe detail = %q, want generic fallback %q", got, want)
			}
		})
	}
}

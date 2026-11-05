package plugins

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestUpgradeRejectionReportsTheAffectedEnvironmentCount(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("initial Install(): %v", err)
	}
	for range 3 {
		row := pluginExecutorLifecycleRecord(t, "lambda", 1, "ready")
		row.Metadata["plugin_executor"].(map[string]any)["installation_id"] = installed.InstallationID
		reader.rows = append(reader.rows, row)
	}
	done := pluginExecutorLifecycleRecord(t, "lambda", 1, "absent")
	done.Metadata["plugin_executor"].(map[string]any)["installation_id"] = installed.InstallationID
	reader.rows = append(reader.rows, done)

	_, err = service.Install(context.Background(), testExecutorProviderPackage(t, "2.0.0", "lambda", []int{2}))
	if err == nil || !strings.Contains(err.Error(), "3 environments") {
		t.Fatalf("error = %v, want a count of the 3 live environments and not the destroyed one", err)
	}
	if strings.Contains(err.Error(), "resource_handle") {
		t.Fatalf("error exposes provider state: %v", err)
	}
}

// The lifecycle's allocation gate looks for exactly this method on the plugin
// service; losing it would turn the gate into a refusal for every provider that
// declares a capability.
var _ interface {
	ConfirmExecutorProviderCapabilities(context.Context, string, string, models.ExecutorProviderProfile) error
} = (*Service)(nil)

package plugins

import (
	"context"
	"strings"
	"testing"
)

const unsupportedHostCapabilityYAML = `    min_kandev_version: 0.99.0
    requires_host_capabilities:
      - {name: workspace_descriptor, version: 99}
`

func TestInstallRefusesAProviderRequiringAnUnsupportedHostCapability(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	_, err := service.Install(context.Background(), testExecutorProviderPackageRequiring(t, "1.0.0", "lambda", []int{1}, unsupportedHostCapabilityYAML))
	if err == nil || !strings.Contains(err.Error(), "workspace_descriptor") {
		t.Fatalf("Install() error = %v, want a refusal naming workspace_descriptor", err)
	}
	if records := service.List(); len(records) != 0 {
		t.Fatalf("a refused install left %d records", len(records))
	}
}

func TestUpgradeRefusedForAnUnsupportedHostCapabilityKeepsTheWorkingInstallationActive(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	_, err = service.Install(context.Background(), testExecutorProviderPackageRequiring(t, "1.1.0", "lambda", []int{1}, unsupportedHostCapabilityYAML))
	if err == nil || !strings.Contains(err.Error(), "workspace_descriptor") {
		t.Fatalf("upgrade error = %v, want a refusal naming workspace_descriptor", err)
	}
	record, getErr := service.Get(installed.ID)
	if getErr != nil || record.Version != "1.0.0" || record.Status != StatusActive || !runtime.Running(installed.ID) {
		t.Fatalf("after a refused upgrade: record=%+v err=%v running=%v, want 1.0.0 active and running", record, getErr, runtime.Running(installed.ID))
	}
}

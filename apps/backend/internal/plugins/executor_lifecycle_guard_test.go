package plugins

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	goruntime "runtime"
	"strings"
	"sync"
	"testing"
	"time"

	hcplugin "github.com/hashicorp/go-plugin"
	"github.com/kandev/kandev/internal/plugins/pkgtar/pkgtartest"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type pluginExecutorInventoryReaderFake struct {
	mu        sync.Mutex
	rows      []*models.ExecutorRunning
	err       error
	listCalls int
}

func (f *pluginExecutorInventoryReaderFake) ListExecutorsRunningPluginRemote(context.Context) ([]*models.ExecutorRunning, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	rows := append([]*models.ExecutorRunning(nil), f.rows...)
	return rows, f.err
}

func (f *pluginExecutorInventoryReaderFake) calls() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls
}

func (f *pluginExecutorInventoryReaderFake) setRows(rows []*models.ExecutorRunning) {
	f.mu.Lock()
	f.rows = append([]*models.ExecutorRunning(nil), rows...)
	f.mu.Unlock()
}

func TestPluginExecutorDisableRetention(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{rows: []*models.ExecutorRunning{
		pluginExecutorLifecycleRecord(t, "lambda", 1, "ready"),
	}}
	service.SetExecutorProviderInventoryReader(reader)
	vault := newFakeSecretRevealer()
	service.SetSecrets(vault)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	const secretID = "plugin:kandev-plugin-remote-test:provider-token"
	vault.set(secretID, "provider-secret")
	disableResult, err := service.DisableWithResult(installed.ID)
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}
	if !disableResult.Disabled || !disableResult.RemoteResourcesMayRemain {
		t.Fatalf("DisableWithResult() = %+v; want disabled with a remote-resource warning", disableResult)
	}
	if runtime.Running(installed.ID) {
		t.Fatal("provider runtime is still running after disable")
	}
	record, err := service.Get(installed.ID)
	if err != nil || record.Status != StatusDisabled || len(record.ExecutorProviders) != 1 {
		t.Fatalf("disabled provider record = %+v, %v; want provider declaration retained", record, err)
	}
	if secret, ok := vault.get(secretID); !ok || secret != "provider-secret" {
		t.Fatalf("provider credential after disable = (%q, %v); want it retained", secret, ok)
	}
	reader.mu.Lock()
	retainedRows := len(reader.rows)
	reader.mu.Unlock()
	if retainedRows != 1 {
		t.Fatalf("retained inventory rows after disable = %d, want 1", retainedRows)
	}
	providers, err := service.ListExecutorProviders(context.Background())
	if err != nil || len(providers) != 1 || providers[0].Available || providers[0].AvailabilityCause != "plugin_disabled" {
		t.Fatalf("disabled provider catalog = %+v, %v; want retained unavailable entry", providers, err)
	}
	if err := service.Enable(installed.ID); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}
	if secret, ok := vault.get(secretID); !ok || secret != "provider-secret" || !runtime.Running(installed.ID) {
		t.Fatalf("provider state after re-enable: secret=(%q, %v) runtime=%v", secret, ok, runtime.Running(installed.ID))
	}
	providers, err = service.ListExecutorProviders(context.Background())
	if err != nil || len(providers) != 1 || !providers[0].Available || providers[0].AvailabilityCause != "" {
		t.Fatalf("re-enabled provider catalog = %+v, %v; want available provider", providers, err)
	}
}

func TestPluginExecutorUninstallRace(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}

	// A provider dispatch holds this read lease through its RPC and inventory
	// callback. Uninstall must wait until that admitted operation has checkpointed.
	dispatchLock := service.dispatchLocks.lockFor(installed.ID)
	dispatchLock.RLock()
	callCtx, releaseDispatch, err := service.beginExecutorProviderDispatch(context.Background(), installed.ID)
	if err != nil {
		dispatchLock.RUnlock()
		t.Fatalf("beginExecutorProviderDispatch() error = %v", err)
	}
	uninstallDone := make(chan error, 1)
	go func() { uninstallDone <- service.Uninstall(context.Background(), installed.ID) }()

	select {
	case <-callCtx.Done():
	case <-time.After(2 * time.Second):
		releaseDispatch()
		dispatchLock.RUnlock()
		t.Fatal("uninstall did not cancel the admitted provider call")
	}
	deadline := time.Now().Add(2 * time.Second)
	for dispatchLock.TryRLock() {
		dispatchLock.RUnlock()
		if time.Now().After(deadline) {
			releaseDispatch()
			dispatchLock.RUnlock()
			t.Fatal("uninstall did not close dispatch admission before draining")
		}
		time.Sleep(time.Millisecond)
	}
	if reader.calls() != 0 || !runtime.Running(installed.ID) {
		releaseDispatch()
		dispatchLock.RUnlock()
		t.Fatal("uninstall inspected inventory or stopped the runtime before the admitted provider call drained")
	}
	reader.setRows([]*models.ExecutorRunning{pluginExecutorLifecycleRecord(t, "lambda", 1, "allocating")})
	releaseDispatch()
	dispatchLock.RUnlock()

	select {
	case err := <-uninstallDone:
		if err == nil || !strings.Contains(err.Error(), "normal task cleanup") {
			t.Fatalf("Uninstall() error = %v; want retained-resource cleanup guidance", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Uninstall() did not finish after the admitted provider call drained")
	}
	if !runtime.Running(installed.ID) {
		t.Fatal("uninstall stopped the provider before retained inventory was rejected")
	}
	if _, err := service.Get(installed.ID); err != nil {
		t.Fatalf("provider record was removed despite retained allocation: %v", err)
	}
}

func TestPluginExecutorUpgradeCompatibility(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("initial Install() error = %v", err)
	}
	row := pluginExecutorLifecycleRecord(t, "lambda", 1, "ready")
	row.Metadata["plugin_executor"].(map[string]any)["installation_id"] = installed.InstallationID
	reader.rows = []*models.ExecutorRunning{row}
	oldStartCalls := runtime.startCallCount(installed.ID)

	if _, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.1", "other", []int{1})); err == nil || !strings.Contains(err.Error(), "provider identity") {
		t.Fatalf("provider identity change error = %v; want retained-provider identity rejection", err)
	}
	if _, err := service.Install(context.Background(), testExecutorProviderPackage(t, "2.0.0", "lambda", []int{2})); err == nil || !strings.Contains(err.Error(), "state version 1") {
		t.Fatalf("incompatible Install() error = %v; want state-version rejection", err)
	}
	current, err := service.Get(installed.ID)
	if err != nil || current.Version != "1.0.0" || !runtime.Running(installed.ID) || runtime.startCallCount(installed.ID) != oldStartCalls {
		t.Fatalf("incompatible upgrade changed working install: record=%+v err=%v running=%v starts=%d", current, err, runtime.Running(installed.ID), runtime.startCallCount(installed.ID))
	}

	compatible, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.1.0", "lambda", []int{1, 2}))
	if err != nil {
		t.Fatalf("compatible Install() error = %v", err)
	}
	if compatible.Version != "1.1.0" || !runtime.Running(installed.ID) {
		t.Fatalf("compatible upgrade = %+v running=%v; want upgraded active provider", compatible, runtime.Running(installed.ID))
	}
}

func TestPluginExecutorRetainedResourceBlocksUninstall(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{rows: []*models.ExecutorRunning{
		pluginExecutorLifecycleRecord(t, "lambda", 1, "cleanup_pending"),
	}}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := service.Uninstall(context.Background(), installed.ID); err == nil || !strings.Contains(err.Error(), "normal task cleanup") {
		t.Fatalf("Uninstall() error = %v; want retained-resource cleanup guidance", err)
	}
	if !runtime.Running(installed.ID) {
		t.Fatal("retention guard stopped the provider before rejecting uninstall")
	}
	if _, err := service.Get(installed.ID); err != nil {
		t.Fatalf("retention guard removed provider record: %v", err)
	}
	if reader.calls() != 1 {
		t.Fatalf("inventory read count = %d; want one administrative read", reader.calls())
	}
}

func TestPluginExecutorLifecycleInventoryReadFailureIsFailClosed(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &pluginExecutorInventoryReaderFake{err: errors.New("database unavailable")}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install() error = %v", err)
	}
	if err := service.Uninstall(context.Background(), installed.ID); !errors.Is(err, ErrExecutorProviderInventoryUnavailable) {
		t.Fatalf("Uninstall() error = %v; want inventory-unavailable guard", err)
	}
	if !runtime.Running(installed.ID) {
		t.Fatal("inventory read failure stopped the provider before uninstall was verified")
	}
	if _, err := service.Get(installed.ID); err != nil {
		t.Fatalf("inventory read failure removed provider record: %v", err)
	}
}

func prepareExecutorProviderTestRuntime(t *testing.T, runtime *fakeRuntime) {
	t.Helper()
	client, _ := hcplugin.TestPluginGRPCConn(t, false, map[string]hcplugin.Plugin{
		pluginsdk.PluginMapKey: &pluginsdk.GRPCPlugin{Impl: &lifecycleExecutorProviderPlugin{}},
	})
	t.Cleanup(func() {
		_ = client.Close()
	})
	raw, err := client.Dispense(pluginsdk.PluginMapKey)
	if err != nil {
		t.Fatalf("Dispense() error = %v", err)
	}
	runtime.setRemote(raw.(*pluginsdk.RemotePlugin))
}

type lifecycleExecutorProviderPlugin struct{}

func (*lifecycleExecutorProviderPlugin) OnEvent(context.Context, *pluginsdk.Event) error {
	return nil
}

func (*lifecycleExecutorProviderPlugin) HandleWebhook(context.Context, *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error) {
	return &pluginsdk.WebhookResponse{Status: 200}, nil
}

func (*lifecycleExecutorProviderPlugin) ValidateExecutorProfile(context.Context, *pluginsdk.ValidateExecutorProfileRequest) (*pluginsdk.ValidateExecutorProfileResponse, error) {
	return &pluginsdk.ValidateExecutorProfileResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) ProvisionExecutorEnvironment(context.Context, *pluginsdk.ProvisionExecutorEnvironmentRequest) (*pluginsdk.ProvisionExecutorEnvironmentResponse, error) {
	return &pluginsdk.ProvisionExecutorEnvironmentResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) RecoverExecutorOperation(context.Context, *pluginsdk.RecoverExecutorOperationRequest) (*pluginsdk.RecoverExecutorOperationResponse, error) {
	return &pluginsdk.RecoverExecutorOperationResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) AttachExecutorEnvironment(context.Context, *pluginsdk.AttachExecutorEnvironmentRequest) (*pluginsdk.AttachExecutorEnvironmentResponse, error) {
	return &pluginsdk.AttachExecutorEnvironmentResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) InspectExecutorEnvironment(context.Context, *pluginsdk.InspectExecutorEnvironmentRequest) (*pluginsdk.InspectExecutorEnvironmentResponse, error) {
	return &pluginsdk.InspectExecutorEnvironmentResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) ResolveExecutorConnection(context.Context, *pluginsdk.ResolveExecutorConnectionRequest) (*pluginsdk.ResolveExecutorConnectionResponse, error) {
	return &pluginsdk.ResolveExecutorConnectionResponse{}, nil
}

func (*lifecycleExecutorProviderPlugin) DestroyExecutorEnvironment(context.Context, *pluginsdk.DestroyExecutorEnvironmentRequest) (*pluginsdk.DestroyExecutorEnvironmentResponse, error) {
	return &pluginsdk.DestroyExecutorEnvironmentResponse{}, nil
}

func pluginExecutorLifecycleRecord(t *testing.T, key string, stateVersion uint32, phase string) *models.ExecutorRunning {
	t.Helper()
	metadata, err := json.Marshal(map[string]any{
		"plugin_executor": map[string]any{
			"plugin_id":                "kandev-plugin-remote-test",
			"installation_id":          "installation-1",
			"provider_key":             key,
			"provider_identity":        fmt.Sprintf("plugin:kandev-plugin-remote-test:%s", key),
			"contract_version":         1,
			"supported_state_versions": []int{1, 2},
			"state_version":            stateVersion,
			"phase":                    phase,
			"environment_generation":   1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(metadata, &decoded); err != nil {
		t.Fatal(err)
	}
	return &models.ExecutorRunning{
		SessionID: "session-1", TaskID: "task-1", Runtime: "plugin_remote",
		AgentExecutionID: "execution-1", Metadata: decoded,
	}
}

func testExecutorProviderPackage(t *testing.T, version, key string, stateVersions []int) *bytes.Buffer {
	t.Helper()
	platform := goruntime.GOOS + "-" + goruntime.GOARCH
	versions := make([]string, 0, len(stateVersions))
	for _, value := range stateVersions {
		versions = append(versions, fmt.Sprint(value))
	}
	manifestYAML := fmt.Sprintf(`
id: kandev-plugin-remote-test
api_version: 1
version: %s
display_name: Remote Test Provider
capabilities:
  executor_provider: true
runtime:
  type: binary
  executables:
    %s: server/plugin
executor_providers:
  - key: %s
    display_name: Remote Test Provider
    description: Test remote compute provider
    contract_version: 1
    supported_state_versions: [%s]
    profile_schema:
      type: object
      properties:
        region: {type: string}
    resource_state_schema:
      type: object
      additionalProperties: false
      properties:
        handle: {type: string}
    capabilities:
      retention: persistent
`, version, platform, key, strings.Join(versions, ", "))
	var packageBytes bytes.Buffer
	if err := pkgtartest.WritePackage(&packageBytes, map[string][]byte{
		"manifest.yaml": []byte(manifestYAML),
		"server/plugin": []byte("#!/bin/sh\necho fake\n"),
	}); err != nil {
		t.Fatalf("WritePackage: %v", err)
	}
	return &packageBytes
}

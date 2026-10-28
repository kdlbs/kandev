package plugins

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type readabilityLockerReader struct {
	pluginExecutorInventoryReaderFake
	inside       bool
	lockedIDs    []string
	listedInLock []bool
}

func (f *readabilityLockerReader) WithPluginStateReadabilityLock(ctx context.Context, installationID string, fn func(context.Context) error) error {
	f.lockedIDs = append(f.lockedIDs, installationID)
	f.inside = true
	defer func() { f.inside = false }()
	return fn(ctx)
}

func (f *readabilityLockerReader) ListExecutorsRunningPluginRemote(ctx context.Context) ([]*models.ExecutorRunning, error) {
	f.listedInLock = append(f.listedInLock, f.inside)
	return f.pluginExecutorInventoryReaderFake.ListExecutorsRunningPluginRemote(ctx)
}

func TestUpgradeCountsRecordedStateUnderTheInstallationLock(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	reader := &readabilityLockerReader{}
	service.SetExecutorProviderInventoryReader(reader)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("initial Install(): %v", err)
	}
	reader.lockedIDs, reader.listedInLock = nil, nil

	if _, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.1.0", "lambda", []int{1, 2})); err != nil {
		t.Fatalf("upgrade: %v", err)
	}
	if len(reader.lockedIDs) != 1 || reader.lockedIDs[0] != installed.InstallationID {
		t.Fatalf("locked installations = %v, want [%s]", reader.lockedIDs, installed.InstallationID)
	}
	for _, inside := range reader.listedInLock {
		if !inside {
			t.Fatal("the recorded-state count ran outside the installation lock")
		}
	}
	if len(reader.listedInLock) == 0 {
		t.Fatal("the upgrade never counted recorded state")
	}
}

func TestStateWritableFollowsTheActivatedManifest(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install(): %v", err)
	}
	ctx := context.Background()
	if err := service.CheckExecutorStateWritable(ctx, installed.InstallationID, "lambda", 1); err != nil {
		t.Fatalf("readable version: %v", err)
	}
	for _, tc := range []struct {
		name         string
		installation string
		key          string
		version      uint32
	}{
		{"version outside the readable list", installed.InstallationID, "lambda", 2},
		{"unknown provider key", installed.InstallationID, "other", 1},
		{"unknown installation fails closed", "no-such-installation", "lambda", 1},
	} {
		if err := service.CheckExecutorStateWritable(ctx, tc.installation, tc.key, tc.version); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
			t.Fatalf("%s: err = %v, want capability_mismatch", tc.name, err)
		}
	}
}

func TestStateWritableRefusesAVersionTheInFlightUpgradeCannotRead(t *testing.T) {
	service, _, runtime := newTestService(t)
	prepareExecutorProviderTestRuntime(t, runtime)
	installed, err := service.Install(context.Background(), testExecutorProviderPackage(t, "1.0.0", "lambda", []int{1}))
	if err != nil {
		t.Fatalf("Install(): %v", err)
	}
	service.setReadableOverride(installed.InstallationID, map[string][]int{"lambda": {2}})
	if err := service.CheckExecutorStateWritable(context.Background(), installed.InstallationID, "lambda", 1); !errors.Is(err, models.ErrExecutorCapabilityMismatch) {
		t.Fatalf("err = %v; an activation that passed its count must make a later admission of the old version fail", err)
	}
	service.clearReadableOverride(installed.InstallationID)
	if err := service.CheckExecutorStateWritable(context.Background(), installed.InstallationID, "lambda", 1); err != nil {
		t.Fatalf("after the override is cleared: %v", err)
	}
}

package controller

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/settings/dto"
	"go.uber.org/zap"
)

func TestNativeOpenCodeUpdateJobsKeepTheNativeSelectionValid(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native runtime fixture uses a POSIX executable")
	}
	for _, tc := range []struct {
		name       string
		useDefault bool
		version    func(string) string
	}{
		{name: "ordinary update", version: func(_ string) string { return "1.18.33" }},
		{name: "repair", version: func(defaultVersion string) string { return defaultVersion }},
		{name: "use Kandev default", useDefault: true, version: func(defaultVersion string) string { return defaultVersion }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			settings, closeSettings := openInstallSettingsStore(t)
			t.Cleanup(closeSettings)
			selectionStore := managedruntime.NewStore(settings)
			openCode := agents.NewOpenCodeACP()
			spec, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
			if err != nil {
				t.Fatalf("resolve native v1 spec: %v", err)
			}
			selection := managedruntime.OpenCodeSelection{
				SchemaVersion:         1,
				Family:                managedruntime.OpenCodeFamilyV1,
				Source:                managedruntime.OpenCodeSourceNative,
				Package:               spec.Package,
				AppliedDefaultVersion: spec.DefaultVersionOrPinned(),
				Revision:              1,
			}
			if err := selectionStore.SaveOpenCodeSelection(context.Background(), 0, selection); err != nil {
				t.Fatalf("save native selection: %v", err)
			}
			binDir := t.TempDir()
			if err := os.WriteFile(filepath.Join(binDir, "opencode"), []byte("#!/bin/sh\nprintf '1.18.5\\n'\n"), 0o755); err != nil {
				t.Fatalf("write native OpenCode fixture: %v", err)
			}
			t.Setenv("PATH", binDir)

			target := tc.version(spec.DefaultVersionOrPinned())
			updater := &recoveryRuntimeUpdater{
				current:      hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: "1.18.5"},
				currentFound: true,
				metadata: RuntimeVersionMetadata{
					Versions: []string{"1.18.5", spec.DefaultVersionOrPinned(), "1.18.33"},
					Latest:   "1.18.33",
				},
				probeCaps: hostutility.AgentCapabilities{Status: hostutility.StatusOK, AgentVersion: target},
			}
			hub := newUpdateTerminalBroadcaster()
			controller := newTestController(map[string]agents.Agent{openCode.ID(): openCode})
			controller.managedRuntimeSelections = selectionStore
			controller.runtimeUpdater = updater
			controller.updateJobStore = NewAgentUpdateJobStore(
				hub, zap.NewNop(), updater, nil, nil, selectionStore,
			)
			controller.updateJobStore.SetOpenCodeSelectionReader(selectionStore)
			controller.updateJobStore.SetOpenCodeSelections(selectionStore)

			var job *dto.AgentUpdateJobDTO
			if tc.useDefault {
				job, err = controller.EnqueueAgentUpdateUseDefault(context.Background(), openCode.ID())
				if err != nil {
					t.Fatalf("EnqueueAgentUpdateUseDefault: %v", err)
				}
			} else {
				job, err = controller.EnqueueAgentUpdate(context.Background(), openCode.ID(), target)
				if err != nil {
					t.Fatalf("EnqueueAgentUpdate: %v", err)
				}
			}
			finished := waitForUpdateStatus(t, hub.completed, job.JobID, dto.AgentUpdateJobStatusSucceeded)
			persisted, found, err := selectionStore.GetOpenCodeSelection(context.Background())
			if err != nil || !found || persisted != selection {
				t.Fatalf("native selection after %s = %+v, found=%t err=%v; want unchanged %+v", tc.name, persisted, found, err, selection)
			}
			if finished.CurrentVersion != target || finished.EffectiveVersion != target {
				t.Fatalf("completed update versions = current %q/effective %q, want %q", finished.CurrentVersion, finished.EffectiveVersion, target)
			}
			if finished.ActiveVersion != "" {
				t.Fatalf("native selection active version = %q, want empty", finished.ActiveVersion)
			}
			if finished.RuntimeRevision != selection.Revision {
				t.Fatalf("runtime revision = %d, want unchanged %d", finished.RuntimeRevision, selection.Revision)
			}
			if got, want := updater.prepare, []string{"npm install -g " + spec.PackageSpec(target)}; !reflect.DeepEqual(got, want) {
				t.Fatalf("native install argv = %#v, want %#v", got, want)
			}
			if got, want := updater.probe, []string{"opencode acp --print-logs --log-level ERROR"}; !reflect.DeepEqual(got, want) {
				t.Fatalf("native probe commands = %#v, want %#v", got, want)
			}
		})
	}
}

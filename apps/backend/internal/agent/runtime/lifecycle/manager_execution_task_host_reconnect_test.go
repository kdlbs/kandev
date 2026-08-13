package lifecycle

import (
	"context"
	"errors"
	"testing"
)

func TestTaskHostExistingOnlyProbeSkipsDeletedLaunchProfile(t *testing.T) {
	for _, test := range []struct {
		name   string
		absent bool
	}{
		{name: "existing runtime", absent: false},
		{name: "proven absent runtime", absent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			missingProfile := errors.New("executor profile not found: deleted-profile")
			mgr, backend := newEnvironmentExecutionTestManager(t, &mockWorkspaceInfoProvider{
				envInfos: map[string]*WorkspaceInfo{
					"env-1": {
						TaskID: "task-1", TaskEnvironmentID: "env-1",
						WorkspacePath: "/workspace/task-1", ExecutorProfileID: "deleted-profile",
					},
				},
			})
			reader := &fakeExecutorProfileReader{profileErr: missingProfile}
			mgr.SetExecutorProfileReader(reader)
			backend.existingOnlyAbsent = test.absent

			proved, err := mgr.StopTaskHostForEnvironment(
				context.Background(), "env-1", "task_deleted",
			)
			if err != nil {
				t.Fatalf("stop task host through existing-only probe: %v", err)
			}
			if !proved {
				t.Fatal("existing-only task-host cleanup did not prove the process tree gone")
			}
			if len(reader.profileArgs) != 0 {
				t.Fatalf("existing-only probe loaded launch profiles: %v", reader.profileArgs)
			}
			if backend.lastRequest == nil || !backend.lastRequest.RequireExistingInstance {
				t.Fatalf("request=%#v, want existing-only physical probe", backend.lastRequest)
			}
			if len(backend.lastRequest.Env) != 0 {
				t.Fatalf("existing-only probe resolved launch environment: %v", backend.lastRequest.Env)
			}
			wantStops := int32(1)
			if test.absent {
				wantStops = 0
			}
			if stops := backend.stopCount.Load(); stops != wantStops {
				t.Fatalf("physical stop calls = %d, want %d", stops, wantStops)
			}

			if !test.absent {
				return
			}
			_, err = mgr.GetOrEnsureTaskHostForEnvironment(context.Background(), "env-1")
			if !errors.Is(err, missingProfile) {
				t.Fatalf("new task-host launch error = %v, want deleted profile failure", err)
			}
			if len(reader.profileArgs) != 1 || reader.profileArgs[0] != "deleted-profile" {
				t.Fatalf("new launch profile lookups = %v", reader.profileArgs)
			}
		})
	}
}

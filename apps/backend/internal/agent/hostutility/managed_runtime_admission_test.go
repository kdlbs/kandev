package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/stretchr/testify/require"
)

type admissionOpenCodeStore struct {
	mu                            sync.Mutex
	selection                     managedruntime.OpenCodeSelection
	inst                          *instance
	selectionReadWithoutAdmission bool
}

func (s *admissionOpenCodeStore) GetOpenCodeSelection(context.Context) (managedruntime.OpenCodeSelection, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.inst == nil || s.inst.operationGate == nil || s.inst.operationGate.TryAcquire(hostUtilityOperationCapacity) {
		s.selectionReadWithoutAdmission = true
		if s.inst != nil && s.inst.operationGate != nil {
			s.inst.operationGate.Release(hostUtilityOperationCapacity)
		}
	}
	return s.selection, true, nil
}

func (*admissionOpenCodeStore) Get(context.Context, string, string) (managedruntime.Selection, bool, error) {
	return managedruntime.Selection{}, false, nil
}

func (s *admissionOpenCodeStore) replace(selection managedruntime.OpenCodeSelection) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selection = selection
}

func (s *admissionOpenCodeStore) readWithoutAdmission() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selectionReadWithoutAdmission
}

func TestHostUtilityResolvesOpenCodeOnlyAfterOperationAdmission(t *testing.T) {
	for _, operation := range []string{"prompt", "profile prompt", "capability probe", "model config probe"} {
		t.Run(operation, func(t *testing.T) {
			log := newTestLogger(t)
			reg := registry.NewRegistry(log)
			openCode := agents.NewOpenCodeACP()
			require.NoError(t, reg.Register(openCode))
			commands := make(chan []string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/health":
					w.WriteHeader(http.StatusOK)
				case "/api/v1/inference/prompt":
					var req agentctlutil.PromptRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
					commands <- append([]string(nil), req.InferenceConfig.Command...)
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.PromptResponse{Success: true, Response: "ok"}))
				case "/api/v1/inference/probe":
					var req agentctlutil.ProbeRequest
					require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
					commands <- append([]string(nil), req.InferenceConfig.Command...)
					w.Header().Set("Content-Type", "application/json")
					require.NoError(t, json.NewEncoder(w).Encode(agentctlutil.ProbeResponse{
						Success: true, AgentVersion: "1.18.5", CurrentModelID: "opencode/test",
						ConfigOptions: []agentctlutil.ProbeConfigOption{{ID: "model", CurrentValue: "opencode/test"}},
					}))
				default:
					http.NotFound(w, r)
				}
			}))
			t.Cleanup(server.Close)
			host, port := serverHostPort(t, server)
			manager := NewManager(reg, host, port, nil, log)
			inst := &instance{
				agentType: openCode.ID(), workDir: t.TempDir(),
				client: agentctlclient.NewClient(host, port, log),
			}
			manager.instances[openCode.ID()] = inst
			manager.parentTmpDir = t.TempDir()
			selectionStore := &admissionOpenCodeStore{selection: managedruntime.OpenCodeSelection{
				SchemaVersion: 1, Family: managedruntime.OpenCodeFamilyV1, Source: managedruntime.OpenCodeSourceManaged,
				Package: "opencode-ai", SelectedVersion: "1.18.5", AppliedDefaultVersion: "1.18.32", Revision: 1,
			}, inst: inst}
			manager.SetManagedRuntimeSelectionStore(selectionStore)
			if operation == "profile prompt" {
				manager.SetProfileResolver(profilePromptResolver{profile: &settingsmodels.AgentProfile{
					ID: "profile", AgentID: openCode.ID(), Model: "opencode/test",
				}})
			}

			switch operation {
			case "prompt":
				_, err := manager.ExecutePrompt(context.Background(), openCode.ID(), "opencode/test", "", "hello")
				require.NoError(t, err)
			case "profile prompt":
				_, err := manager.ExecuteProfilePrompt(context.Background(), "profile", "hello")
				require.NoError(t, err)
			case "capability probe":
				caps, err := manager.Refresh(context.Background(), openCode.ID())
				require.NoError(t, err)
				require.Equal(t, StatusOK, caps.Status)
			case "model config probe":
				resolution, err := manager.ResolveModelConfig(context.Background(), openCode.ID(), ModelConfigResolutionRequest{Model: "opencode/test"})
				require.NoError(t, err)
				require.Equal(t, StatusOK, resolution.Status)
			}

			want, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
			require.NoError(t, err)
			select {
			case got := <-commands:
				require.Equal(t, want.ACPCommand("1.18.5").Args(), got)
			default:
				t.Fatal("utility request did not reach the subprocess boundary")
			}
			require.False(t, selectionStore.readWithoutAdmission(), "runtime selection was read before the shared operation lease")
		})
	}
}

func TestOpenCodeRuntimeCommandAdmissionOrdersUtilityAndMigration(t *testing.T) {
	openCode := agents.NewOpenCodeACP()
	v1Spec, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV1)
	require.NoError(t, err)
	v2Spec, err := openCode.ManagedNPMRuntimeForFamily(managedruntime.OpenCodeFamilyV2)
	require.NoError(t, err)
	newFixture := func() (*Manager, *instance, *admissionOpenCodeStore) {
		inst := &instance{agentType: openCode.ID()}
		store := &admissionOpenCodeStore{selection: managedruntime.OpenCodeSelection{
			SchemaVersion: 1, Family: managedruntime.OpenCodeFamilyV1, Source: managedruntime.OpenCodeSourceManaged,
			Package: v1Spec.Package, SelectedVersion: "1.18.5", AppliedDefaultVersion: "1.18.32", Revision: 1,
		}, inst: inst}
		manager := &Manager{managedRuntimeSelections: store}
		return manager, inst, store
	}
	v2Selection := managedruntime.OpenCodeSelection{
		SchemaVersion: 1, Family: managedruntime.OpenCodeFamilyV2, Source: managedruntime.OpenCodeSourceManaged,
		Package: v2Spec.Package, SelectedVersion: "2.0.18", AppliedDefaultVersion: "2.0.18", Revision: 2,
	}

	t.Run("utility admitted first", func(t *testing.T) {
		manager, inst, store := newFixture()
		first, releaseUtility, err := manager.acquireInferenceCommand(context.Background(), inst, openCode, agents.Command{})
		require.NoError(t, err)
		require.Equal(t, v1Spec.ACPCommand("1.18.5").Args(), first.Args())
		migrationStarted := make(chan struct{})
		migrationAcquired := make(chan func(), 1)
		go func() {
			close(migrationStarted)
			release, acquireErr := inst.acquireOperation(context.Background(), true)
			if acquireErr != nil {
				migrationAcquired <- nil
				return
			}
			migrationAcquired <- release
		}()
		<-migrationStarted
		releaseUtility()
		migrationRelease := <-migrationAcquired
		require.NotNil(t, migrationRelease)
		store.replace(v2Selection)
		migrationRelease()

		second, releaseUtility, err := manager.acquireInferenceCommand(context.Background(), inst, openCode, agents.Command{})
		require.NoError(t, err)
		defer releaseUtility()
		require.Equal(t, v2Spec.ACPCommand("2.0.18").Args(), second.Args())
		require.False(t, store.readWithoutAdmission())
	})

	t.Run("migration admitted first", func(t *testing.T) {
		manager, inst, store := newFixture()
		migrationRelease, err := inst.acquireOperation(context.Background(), true)
		require.NoError(t, err)
		started := make(chan struct{})
		resolved := make(chan agents.Command, 1)
		go func() {
			close(started)
			command, release, commandErr := manager.acquireInferenceCommand(context.Background(), inst, openCode, agents.Command{})
			if commandErr == nil {
				release()
			}
			resolved <- command
		}()
		<-started
		store.replace(v2Selection)
		migrationRelease()
		select {
		case command := <-resolved:
			require.Equal(t, v2Spec.ACPCommand("2.0.18").Args(), command.Args())
		case <-time.After(2 * time.Second):
			t.Fatal("utility command did not resume after migration released admission")
		}
		require.False(t, store.readWithoutAdmission())
	})
}

package backendapp

import (
	"context"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/api"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/server/instance"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
	"github.com/kandev/kandev/pkg/agent"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestNativeInferenceStopRealDeleteDeliversRetainedFinalProof(t *testing.T) {
	h := newNativeInferenceHarness(t, 1, true)
	ctx := context.Background()
	log := testLogger(t)
	manager := instance.NewManager(&config.Config{Ports: config.PortConfig{Base: 0, Max: 0}, Defaults: config.InstanceDefaults{Protocol: agent.ProtocolACP}}, log)
	t.Cleanup(func() { _ = manager.Shutdown(context.Background()) })
	manager.SetServerFactory(func(cfg *config.InstanceConfig, pm *process.Manager, _ *logger.Logger) http.Handler {
		pm.SetInferenceSnapshotter(func() utility.AttemptSnapshot {
			return utility.AttemptSnapshot{OwnerID: "owner", InstanceID: cfg.InstanceID, Known: true, Attempts: []utility.AttemptStatus{{OwnerID: "owner", InstanceID: cfg.InstanceID, AttemptID: "attempt", PreparationID: "preparation", Known: true, Settled: true, State: utility.AttemptTerminated}}}
		})
		return http.NotFoundHandler()
	})
	created, err := manager.CreateInstance(ctx, &instance.CreateRequest{WorkspacePath: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	key := "inference:owner:attempt"
	binding := sessioncapacity.InferenceBinding{Key: key, InstanceID: created.ID, OwnerID: "owner", AttemptID: "attempt", PreparationID: "preparation", Grade: "A", GradeKnown: true, SettlementKind: inferenceSettlementNone}
	if err := h.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error { active[key] = binding; return nil }); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(api.NewControlServer(&config.Config{}, manager, log).Router())
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	client := agentctlclient.NewControlClient(u.Hostname(), port, log)
	defer client.Close()
	consumer := &sharedInferenceConsumer{capacity: h.capacity}
	if err := client.DeleteInstance(agentctlclient.WithInferenceStopObserver(ctx, consumer), created.ID); err != nil {
		t.Fatal(err)
	}
	held, err := h.store.LoadInferenceBindings(ctx)
	if err != nil || len(held) != 0 {
		t.Fatalf("actual control delete lost final proof: held=%+v err=%v", held, err)
	}
	if _, found := manager.GetInstance(created.ID); found {
		t.Fatal("actual instance was not stopped")
	}
}

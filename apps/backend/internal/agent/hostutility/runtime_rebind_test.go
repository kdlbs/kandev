package hostutility

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctlutil "github.com/kandev/kandev/internal/agentctl/server/utility"
)

func TestHostUtilityRuntimeRebind(t *testing.T) {
	log := newTestLogger(t)
	var oldCreates, oldDeletes atomic.Int32
	var newCreates, newHealthChecks, newProbes atomic.Int32
	newToken := "new-runtime-secret"
	var oldPort, newPort int
	probeResponse := agentctlutil.ProbeResponse{
		Success: true,
		Models:  []agentctlutil.ProbeModel{{ID: "mock-fast", Name: "Mock Fast"}},
		ConfigOptions: []agentctlutil.ProbeConfigOption{{
			Type: "select",
			ID:   "effort",
			Name: "Effort",
			Options: []agentctlutil.ProbeConfigOptionChoice{{
				Value: "high",
				Name:  "High",
			}},
		}},
	}
	oldServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer old-runtime-secret" {
			t.Errorf("old server Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
			oldCreates.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{ID: "old-utility", Port: oldPort})
		case r.Method == http.MethodDelete:
			oldDeletes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/probe":
			_ = json.NewEncoder(w).Encode(probeResponse)
		case r.URL.Path == "/health":
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer oldServer.Close()
	oldHost, oldPort := serverHostPort(t, oldServer)

	newServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+newToken {
			t.Errorf("successor server Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
			newCreates.Add(1)
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(agentctlclient.CreateInstanceResponse{ID: "new-utility", Port: newPort})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/inference/probe":
			newProbes.Add(1)
			_ = json.NewEncoder(w).Encode(probeResponse)
		case r.URL.Path == "/health":
			newHealthChecks.Add(1)
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer newServer.Close()
	newHost, newPort := serverHostPort(t, newServer)

	owner := agentctlclient.NewRuntimeOwner(nil, log, "boot-one")
	t.Cleanup(owner.Stop)
	oldCandidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := oldCandidate.Configure(oldHost, oldPort, "old-runtime-secret", 101, nil); err != nil {
		t.Fatal(err)
	}
	if err := oldCandidate.Commit(); err != nil {
		t.Fatal(err)
	}

	reg := registry.NewRegistry(log)
	const agentType = "codex-acp"
	if err := reg.Register(&installedInferenceAgent{id: agentType}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(reg, "", 0, nil, log)
	manager.SetRuntimeOwner(owner)
	t.Cleanup(func() { manager.Stop(context.Background()) })
	if err := manager.Start(context.Background()); err != nil {
		t.Fatalf("start host utility manager: %v", err)
	}

	if !owner.MarkUnavailableEpoch(oldCandidate.Epoch(), agentctlclient.AvailabilityReasonAgentctlExited) {
		t.Fatal("retire original runtime")
	}
	newCandidate, err := owner.PrepareBinding()
	if err != nil {
		t.Fatal(err)
	}
	if err := newCandidate.Configure(newHost, newPort, newToken, 202, nil); err != nil {
		t.Fatal(err)
	}
	if err := newCandidate.Commit(); err != nil {
		t.Fatal(err)
	}

	newInstance, _, err := manager.getInstance(context.Background(), agentType)
	if err != nil {
		t.Fatalf("recreate utility instance on successor: %v", err)
	}
	if newInstance.runtimeEpoch != newCandidate.Epoch() || !newInstance.client.RuntimeCurrent() {
		t.Fatalf("new utility binding epoch=%d current=%v, want epoch %d",
			newInstance.runtimeEpoch, newInstance.client.RuntimeCurrent(), newCandidate.Epoch())
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		caps, ok := manager.cache.get(agentType)
		if ok && caps.Status == StatusOK && hasConfigOption(caps.ConfigOptions, "effort") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("successor capability probe did not restore effort option: ok=%v caps=%+v", ok, caps)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if oldCreates.Load() != 1 || newCreates.Load() != 1 || newHealthChecks.Load() == 0 || newProbes.Load() == 0 {
		t.Fatalf("create/health/probe counts old=%d new=%d health=%d probes=%d",
			oldCreates.Load(), newCreates.Load(), newHealthChecks.Load(), newProbes.Load())
	}
	if oldDeletes.Load() != 0 {
		t.Fatalf("old instance delete was sent to a different runtime generation: %d", oldDeletes.Load())
	}
	if newInstance.client.AuthToken() != newToken {
		t.Fatalf("new utility credential = %q, want current runtime credential", newInstance.client.AuthToken())
	}
}

func hasConfigOption(options []ConfigOption, id string) bool {
	for _, option := range options {
		if option.ID == id {
			return true
		}
	}
	return false
}

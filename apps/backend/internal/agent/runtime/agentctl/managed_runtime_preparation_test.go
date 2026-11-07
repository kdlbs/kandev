package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/agentctl/types"
)

func TestManagedRuntimePreparationProbeCapabilityIsRequiredOnlyForDirective(t *testing.T) {
	var identityRequests, probeRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			identityRequests++
			_, _ = w.Write([]byte(`{"capabilities":["agent-survival.v1"]}`))
		case "/api/v1/inference/probe":
			probeRequests++
			_, _ = w.Write([]byte(`{"success":true}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newClient(server.URL, newTestLogger())

	if _, err := client.Probe(context.Background(), &utility.ProbeRequest{}); err != nil {
		t.Fatalf("ordinary Probe: %v", err)
	}
	if identityRequests != 0 || probeRequests != 1 {
		t.Fatalf("ordinary Probe issued identity=%d probe=%d; want 0 and 1", identityRequests, probeRequests)
	}

	_, err := client.Probe(context.Background(), &utility.ProbeRequest{
		Preparation: &types.ManagedRuntimePreparationDirective{
			Condition:               types.ManagedRuntimeCacheRootManifestMissing,
			PackageSpec:             "@agentclientprotocol/claude-agent-acp@1.2.3",
			FailedProcessGeneration: 17,
			ProbeAttemptID:          "opaque-attempt",
		},
	})
	if err == nil || identityRequests != 1 || probeRequests != 1 {
		t.Fatalf("directive Probe error=%v identity=%d probe=%d; want capability refusal before second probe", err, identityRequests, probeRequests)
	}
}

func TestManagedRuntimePreparationStartNegotiatesOnlyDirectivePath(t *testing.T) {
	directive := types.ManagedRuntimePreparationDirective{
		Condition:               types.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:             "@agentclientprotocol/claude-agent-acp@1.2.3",
		FailedProcessGeneration: 17,
	}
	var posts int
	var received struct {
		Preparation types.ManagedRuntimePreparationDirective `json:"preparation"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/identity":
			_, _ = w.Write([]byte(`{"capabilities":["managed_runtime_private_preparation_v1"]}`))
		case "/api/v1/start":
			posts++
			if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
				t.Errorf("decode start directive: %v", err)
			}
			_, _ = w.Write([]byte(`{"success":true,"command":"npx claude-agent-acp","process_generation":18}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := newClient(server.URL, newTestLogger())
	command, err := client.StartWithManagedRuntimePreparation(context.Background(), directive)
	if err != nil {
		t.Fatalf("StartWithManagedRuntimePreparation: %v", err)
	}
	if command != "npx claude-agent-acp" || client.ProcessGeneration() != 18 {
		t.Fatalf("start result = (%q, generation %d), want successful generation 18", command, client.ProcessGeneration())
	}
	if posts != 1 || received.Preparation != directive {
		t.Fatalf("posts=%d, directive=%#v, want one exact directive", posts, received.Preparation)
	}
}

func TestManagedRuntimePreparationStartRejectsOlderPeerBeforePost(t *testing.T) {
	var posts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/identity" {
			_, _ = w.Write([]byte(`{"capabilities":["agent-survival.v1"]}`))
			return
		}
		posts++
		_, _ = w.Write([]byte(`{"success":true}`))
	}))
	defer server.Close()
	client := newClient(server.URL, newTestLogger())
	_, err := client.StartWithManagedRuntimePreparation(context.Background(), types.ManagedRuntimePreparationDirective{
		Condition:               types.ManagedRuntimeCacheRootManifestMissing,
		PackageSpec:             "@agentclientprotocol/claude-agent-acp@1.2.3",
		FailedProcessGeneration: 17,
	})
	if err == nil || posts != 0 {
		t.Fatalf("start error=%v posts=%d, want capability refusal before directive POST", err, posts)
	}
}

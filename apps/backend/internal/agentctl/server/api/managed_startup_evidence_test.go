package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const managedStartupAPIExitHelperEnv = "KANDEV_MANAGED_STARTUP_API_EXIT_HELPER"

func TestManagedStartupAPIExitHelper(t *testing.T) {
	if os.Getenv(managedStartupAPIExitHelperEnv) == "1" {
		os.Exit(1)
	}
}

func TestHandleWSInitializeCarriesExactProcessEvidence(t *testing.T) {
	s := newTestServer(t)
	s.cfg.Protocol = agent.ProtocolACP
	command := os.Args[0]
	args := []string{command, "-test.run=^TestManagedStartupAPIExitHelper$"}
	if err := s.procMgr.Configure(command, args, true, map[string]string{managedStartupAPIExitHelperEnv: "1"}, "", nil, false); err != nil {
		t.Fatalf("configure child process: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	t.Cleanup(func() { _ = s.procMgr.Stop(context.Background()) })
	if err := s.procMgr.Start(ctx); err != nil {
		t.Fatalf("start child process: %v", err)
	}
	generation := s.procMgr.ProcessGeneration()
	if generation == 0 {
		t.Fatal("successful process start returned generation 0")
	}
	evidence := s.procMgr.ManagedStartupEvidence(ctx, generation)
	if evidence == nil || evidence.ExitDisposition != types.ManagedStartupExitOrdinary || evidence.ExitCode == nil || *evidence.ExitCode != 1 {
		t.Fatalf("process evidence = %#v, want ordinary exit code 1", evidence)
	}

	msg, err := ws.NewRequest("req-evidence", "agent.initialize", InitializeRequest{
		ClientName:        "test",
		ClientVersion:     "1.0.0",
		ProcessGeneration: generation,
	})
	if err != nil {
		t.Fatalf("create initialize request: %v", err)
	}
	resp := s.handleWSInitialize(ctx, msg)
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("response type = %q, want error", resp.Type)
	}
	var payload ws.ErrorPayload
	if err := resp.ParsePayload(&payload); err != nil {
		t.Fatalf("parse initialize error: %v", err)
	}
	raw, ok := payload.Details["startup_evidence"]
	if !ok {
		t.Fatalf("initialize error details = %#v, want startup_evidence", payload.Details)
	}
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal startup evidence: %v", err)
	}
	var got types.ManagedStartupEvidence
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("decode startup evidence: %v", err)
	}
	if got.ProcessGeneration != generation || got.ExitDisposition != types.ManagedStartupExitOrdinary || got.ExitCode == nil || *got.ExitCode != 1 || !got.CollectionComplete {
		t.Fatalf("initialize startup evidence = %#v, want generation %d and complete ordinary exit code 1", got, generation)
	}
}

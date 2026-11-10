package api

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/startupdiag"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const startupProcessAPIHelperEnv = "KANDEV_STARTUP_PROCESS_API_HELPER"

func TestStartupProcessAPIHelper(t *testing.T) {
	switch os.Getenv(startupProcessAPIHelperEnv) {
	case "mise":
		_, _ = os.Stderr.WriteString("mise ERROR Config files in /private/workspace/mise.toml are not trusted.\n")
		os.Exit(1)
	case "unknown":
		_, _ = os.Stderr.WriteString("private startup failure at /private/workspace\n")
		os.Exit(2)
	}
}

func TestHandleWSInitializeProjectsEarlyProcessExit(t *testing.T) {
	tests := []struct {
		name       string
		mode       string
		exitCode   int
		reason     string
		hasReason  bool
		privateKey string
	}{
		{
			name: "recognized mise refusal", mode: "mise", exitCode: 1,
			reason: startupdiag.ReasonMiseConfigUntrusted, hasReason: true,
			privateKey: "/private/workspace",
		},
		{
			name: "ordinary unknown exit keeps status", mode: "unknown", exitCode: 2,
			privateKey: "private startup failure",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(t)
			s.cfg.Protocol = agent.ProtocolACP
			command := os.Args[0]
			args := []string{command, "-test.run=^TestStartupProcessAPIHelper$"}
			if err := s.procMgr.Configure(command, args, true, map[string]string{startupProcessAPIHelperEnv: tt.mode}, "", nil, false); err != nil {
				t.Fatalf("configure child process: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			t.Cleanup(func() { _ = s.procMgr.Stop(context.Background()) })
			generation, err := s.procMgr.StartWithGeneration(ctx)
			if err != nil {
				t.Fatalf("start child process: %v", err)
			}

			msg, err := ws.NewRequest("req-startup-process", "agent.initialize", InitializeRequest{
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
			if got.ProcessGeneration != generation || got.ExitDisposition != types.ManagedStartupExitOrdinary {
				t.Fatalf("initialize startup evidence = %#v, want generation %d ordinary exit", got, generation)
			}
			if got.ExitCode == nil || *got.ExitCode != tt.exitCode {
				t.Fatalf("exit code = %#v, want %d", got.ExitCode, tt.exitCode)
			}
			if tt.hasReason {
				if got.DiagnosticReason != tt.reason || !got.DiagnosticComplete {
					t.Fatalf("diagnostic = %#v, want complete %q", got, tt.reason)
				}
			} else if got.DiagnosticReason != "" {
				t.Fatalf("diagnostic reason = %q, want no trusted attribution", got.DiagnosticReason)
			}
			if tt.privateKey != "" && bytes.Contains(encoded, []byte(tt.privateKey)) {
				t.Fatalf("raw stderr leaked into startup evidence: %s", encoded)
			}
		})
	}
}

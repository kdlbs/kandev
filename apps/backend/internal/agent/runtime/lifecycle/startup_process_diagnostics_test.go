package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/startupdiag"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestStartupProcessBootstrapCause(t *testing.T) {
	const generation = 7
	tests := []struct {
		name       string
		failAction string
		evidence   map[string]any
		wantCode   string
		wantReason string
		wantExit   *int
	}{
		{
			name:       "recognized mise refusal",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 1, "diagnostic_reason": startupdiag.ReasonMiseConfigUntrusted,
				"diagnostic_complete": true, "collection_complete": true,
			},
			wantCode:   models.AgentErrorCauseCodeProcessExited,
			wantReason: models.AgentErrorCauseReasonMiseConfigUntrusted,
			wantExit:   intPtr(1),
		},
		{
			name:       "unknown ordinary exit keeps status",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 3, "diagnostic_complete": true, "collection_complete": true,
			},
			wantCode:   models.AgentErrorCauseCodeProcessExited,
			wantReason: models.AgentErrorCauseReasonUnclassified,
			wantExit:   intPtr(3),
		},
		{
			name:       "exit zero remains a truthful pre-readiness exit",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 0, "diagnostic_complete": true, "collection_complete": true,
			},
			wantCode:   models.AgentErrorCauseCodeProcessExited,
			wantReason: models.AgentErrorCauseReasonUnclassified,
			wantExit:   intPtr(0),
		},
		{
			name:       "unrecognized reason is not trusted",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 1, "diagnostic_reason": "private_reason", "diagnostic_complete": true,
			},
			wantCode:   models.AgentErrorCauseCodeProcessExited,
			wantReason: models.AgentErrorCauseReasonUnclassified,
			wantExit:   intPtr(1),
		},
		{
			name:       "incomplete diagnostic is not trusted",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 1, "diagnostic_reason": startupdiag.ReasonMiseConfigUntrusted,
				"diagnostic_complete": false,
			},
			wantCode:   models.AgentErrorCauseCodeProcessExited,
			wantReason: models.AgentErrorCauseReasonUnclassified,
			wantExit:   intPtr(1),
		},
		{
			name:       "mismatched generation is not attributed",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation + 1), "exit_disposition": "ordinary_exit",
				"exit_code": 1, "diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
		{
			name:       "signal does not invent an ordinary exit",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "signal_exit",
				"exit_code": 1, "diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
		{
			name:       "intentional stop keeps existing handling",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "intentional_stop",
				"diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
		{
			name:       "missing exit status is not invented",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
		{
			name:       "explicit null exit status is not invented",
			failAction: "agent.initialize",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": nil, "diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
		{
			name:       "later session phase cannot acquire exit evidence",
			failAction: "agent.session.new",
			evidence: map[string]any{
				"process_generation": uint64(generation), "exit_disposition": "ordinary_exit",
				"exit_code": 1, "diagnostic_complete": true,
			},
			wantCode: models.AgentErrorCauseCodeUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newMockAgentServer(t)
			t.Cleanup(mock.Close)
			mock.mu.Lock()
			mock.processGeneration = generation
			mock.mu.Unlock()
			mock.handler = func(msg ws.Message) *ws.Message {
				if msg.Action == tt.failAction {
					resp, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError,
						"peer disconnected before response", map[string]any{"startup_evidence": tt.evidence})
					return resp
				}
				return mock.defaultHandler(msg)
			}

			client := createTestClient(t, mock.server.URL)
			t.Cleanup(client.Close)
			if err := client.StreamUpdates(context.Background(), func(agentctl.AgentEvent) {}, nil, nil); err != nil {
				t.Fatalf("connect agent stream: %v", err)
			}
			waitForWSConnected(t, mock)
			if _, err := client.Start(context.Background()); err != nil {
				t.Fatalf("seed process generation: %v", err)
			}
			sessionManager := NewSessionManager(newSessionTestLogger(), newTestStopCh(t))
			agentConfig := &testAgent{
				id:      "test-agent",
				enabled: true,
				runtimeConfig: &agents.RuntimeConfig{
					Cmd:      agents.NewCommand("test-agent"),
					Protocol: agent.ProtocolACP,
				},
			}
			_, err := sessionManager.InitializeSession(
				context.Background(), nil, client, agentConfig, "", "/workspace", nil,
			)
			if err == nil {
				t.Fatal("expected injected initialize failure")
			}
			failure := bootstrapFailureFor(&AgentExecution{ID: "exec-1"}, err)
			if failure == nil {
				t.Fatal("bootstrap failure was not classified")
			}
			if failure.Code != tt.wantCode {
				t.Fatalf("bootstrap code = %q, want %q (cause %v)", failure.Code, tt.wantCode, err)
			}
			if tt.wantCode != models.AgentErrorCauseCodeProcessExited {
				return
			}
			if failure.Reason != tt.wantReason {
				t.Fatalf("reason = %q, want %q", failure.Reason, tt.wantReason)
			}
			if failure.ExitCode == nil || tt.wantExit == nil || *failure.ExitCode != *tt.wantExit {
				t.Fatalf("exit code = %#v, want %#v", failure.ExitCode, tt.wantExit)
			}
			cause, ok := failure.SafeAgentErrorCause(models.AgentErrorCauseOperationStart)
			if !ok || cause.ExitCode == nil || *cause.ExitCode != *tt.wantExit ||
				cause.Detail != models.AgentErrorCauseProcessExitedDetail {
				t.Fatalf("safe cause = %#v", cause)
			}
		})
	}
}

func TestStartupProcessBootstrapFailurePrecedence(t *testing.T) {
	evidence := &agentctltypes.ManagedStartupEvidence{
		ProcessGeneration: 7, ExitDisposition: agentctltypes.ManagedStartupExitOrdinary,
		ExitCode: intPtr(1), DiagnosticReason: startupdiag.ReasonMiseConfigUntrusted,
		DiagnosticComplete: true, CollectionComplete: true,
	}
	phaseErr := &SessionInitializationPhaseError{
		Phase: SessionInitializationPhaseACPInitialize, ProcessGeneration: 7,
		Cause: &agentctl.InitializeError{Message: "peer disconnected", StartupEvidence: evidence},
	}

	if err := wrapBootstrapFailure(nil, errors.Join(phaseErr, context.DeadlineExceeded)); err == nil {
		t.Fatal("expected deadline failure")
	} else {
		var failure *BootstrapFailure
		if !errors.As(err, &failure) || failure.Code != models.AgentErrorCauseCodeTimeout {
			t.Fatalf("deadline bootstrap failure = %#v, want timeout precedence", failure)
		}
	}
	cancelled := wrapBootstrapFailure(nil, errors.Join(phaseErr, context.Canceled))
	if !errors.Is(cancelled, context.Canceled) {
		t.Fatalf("cancelled failure = %v, want cancellation passthrough", cancelled)
	}
	var failure *BootstrapFailure
	if errors.As(cancelled, &failure) {
		t.Fatalf("cancellation became a bootstrap failure: %#v", failure)
	}
}

func intPtr(value int) *int { return &value }

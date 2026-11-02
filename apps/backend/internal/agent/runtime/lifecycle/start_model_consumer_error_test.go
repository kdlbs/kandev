package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agent/agents"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/agent"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func modelFailureConsumerClient(t *testing.T, kind string) (*agentctl.Client, *mockAgentServer) {
	t.Helper()
	mock := newMockAgentServer(t)
	t.Cleanup(mock.Close)
	var client *agentctl.Client
	mock.handler = func(msg ws.Message) *ws.Message {
		switch msg.Action {
		case "agent.session.set_model":
			if kind == "success" {
				return mock.defaultHandler(msg)
			}
			if kind == "transport" {
				client.Close()
				return nil
			}
			message := acp.NewInternalError(map[string]any{"error": "connection closed by model service"}).Error()
			reply, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, message, nil)
			return reply
		case "agent.session.new", "agent.session.load":
			reply, _ := ws.NewResponse(msg.ID, msg.Action, agentctl.NewSessionResponse{
				Success: true, SessionID: "native", ModelState: &streams.SessionModelState{
					CurrentModelID: "saved-model", Models: []streams.SessionModelInfo{{ModelID: "saved-model"}},
				},
			})
			return reply
		}
		return mock.defaultHandler(msg)
	}
	client = createTestClient(t, mock.server.URL)
	disconnected := connectAgentStream(t, mock, client)
	t.Cleanup(func() {
		client.Close()
		select {
		case <-disconnected:
		case <-time.After(5 * time.Second):
			t.Error("agent stream did not finish draining during cleanup")
		}
	})
	return client, mock
}

func TestStartModelConsumersPreserveProviderReplyCause(t *testing.T) {
	for _, path := range []string{"startup", "native resume", "context reset", "Kubernetes reapply"} {
		for _, providerReply := range []bool{false, true} {
			name := "transport"
			if providerReply {
				name = "provider reply"
			}
			t.Run(path+"/"+name, func(t *testing.T) {
				client, mock := modelFailureConsumerClient(t, name)
				execution := &AgentExecution{ID: "execution", SessionID: "session", agentctl: client, WorkspacePath: "/workspace"}
				execution.SetModelState(modelState("saved-model"))
				log := newSessionTestLogger()
				manager := &Manager{logger: log}
				var err error
				switch path {
				case "startup", "native resume":
					err = initializeModelFailureSession(t, execution, path == "native resume")
				case "context reset":
					err = manager.reapplySessionModelAfterResetWithClient(context.Background(), execution, client, "native", "saved-model")
				case "Kubernetes reapply":
					// The real session response seeds the replacement client's fresh catalog.
					if _, newErr := client.NewSession(context.Background(), "/workspace", nil); newErr != nil {
						t.Fatal(newErr)
					}
					err = manager.applyRestartedKubernetesModel(context.Background(), execution, client, "native", "saved-model")
				}
				assertModelFailureConsumerCause(t, execution, err, providerReply)
				calls := 0
				for _, action := range mock.getActionLog() {
					if action == "agent.session.set_model" {
						calls++
					}
					if action == "agent.prompt" {
						t.Fatal("failed model application dispatched a prompt")
					}
					if path == "native resume" && action == "agent.session.new" {
						t.Fatal("resume replaced the native conversation")
					}
				}
				if calls != 1 {
					t.Fatalf("SetModel calls = %d, want 1", calls)
				}
			})
		}
	}
}

func initializeModelFailureSession(t *testing.T, execution *AgentExecution, resume bool) error {
	t.Helper()
	log := newSessionTestLogger()
	sm := NewSessionManager(log, newTestStopCh(t))
	sm.SetDependencies(NewEventPublisher(&MockEventBusWithTracking{}, log), nil, nil, nil)
	if resume {
		execution.ACPSessionID = "native"
		execution.isResumedSession = true
	}
	config := &testAgent{id: "test-agent", enabled: true, runtimeConfig: &agents.RuntimeConfig{
		Cmd: agents.NewCommand("test-agent"), Protocol: agent.ProtocolACP,
		SessionConfig: agents.SessionConfig{NativeSessionResume: true},
	}}
	return sm.InitializeAndPromptWithLayers(context.Background(), execution, config, "do work", nil, nil,
		func(string) error { t.Error("failed startup became ready"); return nil },
		"saved-model", "", nil, "", "", nil, StartModelPolicy{RequireExactModel: true})
}

func assertModelFailureConsumerCause(t *testing.T, execution *AgentExecution, err error, providerReply bool) {
	t.Helper()
	if err == nil {
		t.Fatal("model call unexpectedly succeeded")
	}
	var failure *BootstrapFailure
	if !errors.As(wrapBootstrapFailure(execution, err), &failure) {
		t.Fatalf("missing bootstrap cause: %v", err)
	}
	cause, ok := failure.SafeAgentErrorCause("")
	if !ok {
		t.Fatal("invalid cause")
	}
	want := models.AgentErrorCauseCodeUnknown
	if providerReply {
		want = models.AgentErrorCauseCodeModelSelectionFailed
	}
	if cause.Code != want {
		t.Fatalf("cause = %+v, want %q", cause, want)
	}
	if providerReply && (cause.Reason != models.AgentErrorCauseReasonApplicationFailed || cause.AttemptedModel != "saved-model") {
		t.Fatalf("provider evidence lost: %+v", cause)
	}
	if !providerReply && (cause.RequestedModel != "" || cause.AttemptedModel != "") {
		t.Fatalf("transport acquired model evidence: %+v", cause)
	}
	if strings.Contains(cause.Detail, "connection closed") {
		t.Fatalf("raw provider detail escaped: %+v", cause)
	}
	wantOperation := models.AgentErrorCauseOperationStart
	if execution.isResumedSession {
		wantOperation = models.AgentErrorCauseOperationResume
	}
	if cause.Operation != wantOperation {
		t.Fatalf("operation = %q, want %q", cause.Operation, wantOperation)
	}
}

func TestStartModelStartupSuccessDispatchesPrompt(t *testing.T) {
	client, _ := modelFailureConsumerClient(t, "success")
	execution := &AgentExecution{ID: "execution", SessionID: "session", agentctl: client, WorkspacePath: "/workspace", promptDoneCh: make(chan PromptCompletionSignal, 1)}
	dispatched := make(chan struct{})
	execution.setInitialPromptDispatchCallbacks(nil, func() {
		execution.promptDoneCh <- PromptCompletionSignal{
			StopReason: "test-complete", PromptGeneration: execution.promptGenerationSnapshot(),
		}
		close(dispatched)
	}, nil)
	if err := initializeModelFailureSession(t, execution, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-dispatched:
	case <-time.After(5 * time.Second):
		t.Fatal("successful model application did not dispatch the prompt")
	}
	if !execution.isSessionInitialized() {
		t.Fatal("successful model application did not initialize the session")
	}
}

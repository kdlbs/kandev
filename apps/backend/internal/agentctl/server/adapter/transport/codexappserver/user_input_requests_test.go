package codexappserver

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	agenttypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/logger"
	protocol "github.com/kandev/kandev/pkg/codexappserver"
)

const userInputRequestParams = `{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","isBlocking":true,"questions":[{"id":"q1","header":"Mode","question":"Choose a mode","isOther":false,"isSecret":false,"options":[{"label":"Fast","description":"Quick"},{"label":"Safe","description":"Careful"}]}]}`

func TestToolUserInputMapsClarificationAnswerToProviderLabel(t *testing.T) {
	adapter := newUserInputTestAdapter(t)
	adapter.SetUserInputRequestHandler(func(_ context.Context, request *agenttypes.UserInputRequest) (*agenttypes.UserInputResponse, error) {
		if request.ThreadID != "thread-1" || request.TurnID != "turn-1" || request.ItemID != "item-1" {
			t.Fatalf("request identity = %+v", request)
		}
		if len(request.Questions) != 1 || request.Questions[0].ID != "q1" || request.Questions[0].Options[1].OptionID != "codex-option-1" {
			t.Fatalf("normalized questions = %+v", request.Questions)
		}
		return &agenttypes.UserInputResponse{Answers: map[string]agenttypes.UserInputAnswer{
			"q1": {OptionIDs: []string{"codex-option-1"}},
		}}, nil
	})

	got, err := adapter.handleServerRequest(context.Background(), userInputRequest())
	if err != nil {
		t.Fatalf("handleServerRequest: %v", err)
	}
	response, ok := got.(protocol.ToolRequestUserInputResponse)
	if !ok {
		t.Fatalf("response type = %T", got)
	}
	if values := response.Answers["q1"].Answers; len(values) != 1 || values[0] != "Safe" {
		t.Fatalf("provider answers = %#v, want Safe", response.Answers)
	}
}

func TestChildThreadUserInputUsesTheParentKandevSessionRoute(t *testing.T) {
	adapter := newUserInputTestAdapter(t)
	adapter.children["child-thread"] = childBinding{parentThreadID: "thread-1", toolCallID: "collab-call"}
	adapter.SetUserInputRequestHandler(func(_ context.Context, request *agenttypes.UserInputRequest) (*agenttypes.UserInputResponse, error) {
		if request.ThreadID != "child-thread" || request.ItemID != "child-item" {
			t.Fatalf("child request identity = %+v", request)
		}
		return &agenttypes.UserInputResponse{Answers: map[string]agenttypes.UserInputAnswer{
			"child-q": {OptionIDs: []string{"codex-option-0"}},
		}}, nil
	})
	request := protocol.ServerRequest{
		ID:     json.RawMessage(`"child-input-id"`),
		Method: protocol.ServerRequestToolUserInput,
		Params: json.RawMessage(`{"threadId":"child-thread","turnId":"child-turn","itemId":"child-item","questions":[{"id":"child-q","header":"Mode","question":"Choose a mode","isOther":false,"isSecret":false,"options":[{"label":"Fast","description":"Quick"},{"label":"Safe","description":"Careful"}]}]}`),
	}
	got, err := adapter.handleServerRequest(context.Background(), request)
	if err != nil {
		t.Fatalf("child handleServerRequest: %v", err)
	}
	response := got.(protocol.ToolRequestUserInputResponse)
	if values := response.Answers["child-q"].Answers; len(values) != 1 || values[0] != "Fast" {
		t.Fatalf("child response answers = %#v", response.Answers)
	}
}

func TestToolUserInputRejectsSecretQuestions(t *testing.T) {
	adapter := newUserInputTestAdapter(t)
	called := false
	adapter.SetUserInputRequestHandler(func(context.Context, *agenttypes.UserInputRequest) (*agenttypes.UserInputResponse, error) {
		called = true
		return nil, nil
	})
	request := userInputRequest()
	request.Params = json.RawMessage(`{"threadId":"thread-1","turnId":"turn-1","itemId":"item-1","questions":[{"id":"q1","header":"Token","question":"Enter a token","isOther":true,"isSecret":true,"options":[]}]}`)
	_, err := adapter.handleServerRequest(context.Background(), request)
	var rpcErr *protocol.RPCError
	if !errors.As(err, &rpcErr) || rpcErr.Code != -32602 || !strings.Contains(rpcErr.Message, "secret") {
		t.Fatalf("secret request error = %#v, want explicit unsupported error", err)
	}
	if called {
		t.Fatal("secret question reached clarification handler")
	}
}

func TestToolUserInputCancellationReachesClarificationHandler(t *testing.T) {
	adapter := newUserInputTestAdapter(t)
	started := make(chan struct{})
	adapter.SetUserInputRequestHandler(func(ctx context.Context, _ *agenttypes.UserInputRequest) (*agenttypes.UserInputResponse, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := adapter.handleServerRequest(ctx, userInputRequest())
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("clarification handler was not called")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("native request did not stop after provider resolution")
	}
}

func TestToolUserInputRejectedResponseReturnsEmptyAnswerForEveryQuestion(t *testing.T) {
	providerQuestions := []protocol.ToolRequestUserInputQuestion{
		{ID: "q1"},
		{ID: "q2"},
	}
	normalized := []agenttypes.UserInputQuestion{{ID: "q1"}, {ID: "q2"}}
	got, err := makeUserInputResponse(providerQuestions, normalized, &agenttypes.UserInputResponse{Rejected: true})
	if err != nil {
		t.Fatalf("makeUserInputResponse: %v", err)
	}
	response := got.(protocol.ToolRequestUserInputResponse)
	if len(response.Answers) != 2 || len(response.Answers["q1"].Answers) != 0 || len(response.Answers["q2"].Answers) != 0 {
		t.Fatalf("rejected response = %#v, want empty answer for each question", response.Answers)
	}
}

func newUserInputTestAdapter(t *testing.T) *Adapter {
	t.Helper()
	adapter := NewAdapter(&shared.Config{}, logger.Default())
	adapter.threadID = "thread-1"
	t.Cleanup(func() { _ = adapter.Close() })
	return adapter
}

func userInputRequest() protocol.ServerRequest {
	return protocol.ServerRequest{
		ID:     json.RawMessage(`"input-id"`),
		Method: protocol.ServerRequestToolUserInput,
		Params: json.RawMessage(userInputRequestParams),
	}
}

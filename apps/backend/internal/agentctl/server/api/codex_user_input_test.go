package api

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/adapter"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	mcp "github.com/kandev/kandev/internal/mcp/server"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestCodexUserInputBridgePreservesQuestionsAndAnswers(t *testing.T) {
	backend := mcp.NewChannelBackendClient(logger.Default())
	defer backend.Close()
	handler := newCodexUserInputRequestHandler(&config.InstanceConfig{SessionID: "kandev-session", TaskID: "task-1"}, backend, logger.Default())
	request := &adapter.UserInputRequest{
		ThreadID: "codex-thread",
		TurnID:   "codex-turn",
		ItemID:   "codex-item",
		Questions: []adapter.UserInputQuestion{
			{
				ID: "q-choice", Header: "Mode", Prompt: "Choose a mode",
				Options: []adapter.UserInputOption{
					{OptionID: "codex-option-0", Label: "Fast", Description: "Quick"},
					{OptionID: "codex-option-1", Label: "Safe", Description: "Careful"},
				},
			},
			{ID: "q-text", Header: "Reason", Prompt: "What should change?", IsOther: true},
		},
	}
	resultCh := make(chan struct {
		response *adapter.UserInputResponse
		err      error
	}, 1)
	go func() {
		response, err := handler(context.Background(), request)
		resultCh <- struct {
			response *adapter.UserInputResponse
			err      error
		}{response, err}
	}()

	message := receiveBackendRequest(t, backend.GetRequestChannel())
	if message.Action != ws.ActionMCPAskUserQuestion {
		t.Fatalf("action = %q, want %q", message.Action, ws.ActionMCPAskUserQuestion)
	}
	var payload struct {
		SessionID         string                   `json:"session_id"`
		TaskID            string                   `json:"task_id"`
		Questions         []clarification.Question `json:"questions"`
		AllowFreeTextOnly bool                     `json:"allow_free_text_only"`
	}
	if err := json.Unmarshal(message.Payload, &payload); err != nil {
		t.Fatalf("unmarshal clarification request: %v", err)
	}
	if payload.SessionID != "kandev-session" || payload.TaskID != "task-1" || !payload.AllowFreeTextOnly {
		t.Fatalf("clarification identity/options = %+v", payload)
	}
	if len(payload.Questions) != 2 || payload.Questions[0].ID != "q-choice" || payload.Questions[0].Options[1].ID != "codex-option-1" {
		t.Fatalf("clarification questions = %+v", payload.Questions)
	}
	if payload.Questions[0].AllowCustomText == nil || *payload.Questions[0].AllowCustomText {
		t.Fatalf("choice question custom text policy = %+v, want false", payload.Questions[0].AllowCustomText)
	}
	if payload.Questions[1].AllowCustomText == nil || !*payload.Questions[1].AllowCustomText || len(payload.Questions[1].Options) != 0 {
		t.Fatalf("text-only question = %+v", payload.Questions[1])
	}
	respondToBackendRequest(t, backend, message, clarification.Response{Answers: []clarification.Answer{
		{QuestionID: "q-choice", SelectedOptions: []string{"codex-option-1"}},
		{QuestionID: "q-text", CustomText: "Change the timeout"},
	}})
	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("clarification bridge: %v", result.err)
		}
		if got := result.response.Answers["q-choice"].OptionIDs; len(got) != 1 || got[0] != "codex-option-1" {
			t.Fatalf("choice answer = %v", got)
		}
		if got := result.response.Answers["q-text"].CustomText; got != "Change the timeout" {
			t.Fatalf("text answer = %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("clarification bridge did not finish")
	}
}

func TestCodexUserInputBridgeCancelsPendingClarificationOnProviderResolution(t *testing.T) {
	backend := mcp.NewChannelBackendClient(logger.Default())
	defer backend.Close()
	handler := newCodexUserInputRequestHandler(&config.InstanceConfig{SessionID: "kandev-session"}, backend, logger.Default())
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := handler(ctx, &adapter.UserInputRequest{
			Questions: []adapter.UserInputQuestion{{
				ID: "q1", Header: "Mode", Prompt: "Choose a mode", IsOther: true,
				Options: []adapter.UserInputOption{{OptionID: "codex-option-0", Label: "Fast"}, {OptionID: "codex-option-1", Label: "Safe"}},
			}},
		})
		done <- err
	}()
	questionRequest := receiveBackendRequest(t, backend.GetRequestChannel())
	if questionRequest.Action != ws.ActionMCPAskUserQuestion {
		t.Fatalf("first action = %q", questionRequest.Action)
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("handler error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("question handler did not stop after provider resolution")
	}
	cancelRequest := receiveBackendRequest(t, backend.GetRequestChannel())
	if cancelRequest.Action != ws.ActionMCPClarificationTimeout {
		t.Fatalf("cancellation action = %q, want %q", cancelRequest.Action, ws.ActionMCPClarificationTimeout)
	}
	var payload map[string]string
	if err := json.Unmarshal(cancelRequest.Payload, &payload); err != nil || payload["session_id"] != "kandev-session" {
		t.Fatalf("cancellation payload = %s, err = %v", cancelRequest.Payload, err)
	}
	respondToBackendRequest(t, backend, cancelRequest, map[string]any{"ok": true})
}

func TestNormalizeClarificationResponseRejectsMissingAndUnofferedAnswers(t *testing.T) {
	question := adapter.UserInputQuestion{
		ID: "q1",
		Options: []adapter.UserInputOption{
			{OptionID: "offered", Label: "Fast"},
		},
	}
	for _, test := range []struct {
		name     string
		response clarification.Response
	}{
		{name: "missing", response: clarification.Response{}},
		{name: "unoffered", response: clarification.Response{Answers: []clarification.Answer{{QuestionID: "q1", SelectedOptions: []string{"other"}}}}},
		{name: "unknown question", response: clarification.Response{Answers: []clarification.Answer{{QuestionID: "q2", CustomText: "answer"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := normalizeClarificationResponse([]adapter.UserInputQuestion{question}, test.response); err == nil {
				t.Fatal("expected malformed clarification response to fail")
			}
		})
	}
}

func receiveBackendRequest(t *testing.T, requests <-chan *ws.Message) *ws.Message {
	t.Helper()
	select {
	case request := <-requests:
		return request
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for backend request")
		return nil
	}
}

func respondToBackendRequest(t *testing.T, backend *mcp.ChannelBackendClient, request *ws.Message, payload any) {
	t.Helper()
	response, err := ws.NewResponse(request.ID, request.Action, payload)
	if err != nil {
		t.Fatalf("create backend response: %v", err)
	}
	backend.HandleResponse(response)
}

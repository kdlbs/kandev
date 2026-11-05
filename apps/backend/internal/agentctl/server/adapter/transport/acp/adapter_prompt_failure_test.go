package acp

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	sdk "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// staticPromptErrorAgent answers every prompt with one fixed SDK error.
type staticPromptErrorAgent struct {
	burstAgent
	err error
}

func (a *staticPromptErrorAgent) Prompt(context.Context, sdk.PromptRequest) (sdk.PromptResponse, error) {
	return sdk.PromptResponse{}, a.err
}

// remoteACPRequestError returns an error decoded from a real error response
// envelope, so it carries positive remote-response provenance.
func remoteACPRequestError(t *testing.T, code int, message string, data any) error {
	t.Helper()

	c2aR, c2aW := io.Pipe()
	a2cR, a2cW := io.Pipe()
	client := sdk.NewClientSideConnection(&burstClient{}, c2aW, a2cR)
	_ = sdk.NewAgentSideConnection(&staticPromptErrorAgent{
		err: &sdk.RequestError{Code: code, Message: message, Data: data},
	}, a2cW, c2aR)
	t.Cleanup(func() {
		_ = c2aR.Close()
		_ = c2aW.Close()
		_ = a2cR.Close()
		_ = a2cW.Close()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := client.Prompt(ctx, sdk.PromptRequest{
		SessionId: sdk.SessionId("origin-session"),
		Prompt:    []sdk.ContentBlock{sdk.TextBlock("hello")},
	})
	if err == nil {
		t.Fatal("expected a remote prompt error")
	}
	var requestErr *sdk.RequestError
	if !errors.As(err, &requestErr) || !requestErr.IsRemoteResponse() {
		t.Fatalf("error %v did not carry remote-response provenance", err)
	}
	return err
}

func TestRetainableACPApplicationErrorRequiresClassifiedProviderCapacity(t *testing.T) {
	remoteCases := []struct {
		name     string
		provider string
		message  string
		data     any
	}{
		{name: "opencode generic internal", provider: opencodeAgentID, message: "Internal error: Failed to execute statement", data: map[string]any{"details": "SQLiteError: database is locked"}},
		{name: "opencode database error", provider: opencodeAgentID, message: "database is locked"},
		{name: "claude generic internal", provider: claudeAgentID, message: "Internal error"},
		{name: "codex authentication", provider: codexAgentID, message: "Authentication required"},
		{name: "cursor invalid model", provider: "cursor-acp", message: "invalid model id"},
		{name: "unlisted provider quota", provider: "custom-acp", message: "insufficient_quota"},
		{name: "empty message", provider: claudeAgentID, message: ""},
	}

	for _, test := range remoteCases {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapterForAgent(test.provider)
			err := remoteACPRequestError(t, -32603, test.message, test.data)
			providerError, ok := a.retainedACPApplicationError(err)
			if !ok {
				t.Fatalf("remote error %v was not retained", err)
			}
			if providerError == nil || !providerError.Valid() {
				t.Fatalf("provider error = %+v, want a valid sanitized diagnostic", providerError)
			}
			if providerError.Source != streams.ProviderErrorSourceACPPrompt {
				t.Fatalf("provider error source = %q, want the generic ACP prompt source", providerError.Source)
			}
			if providerError.Message == "" {
				t.Fatal("provider error has no sanitized message")
			}
		})
	}

	localCases := []struct {
		name     string
		provider string
		err      error
	}{
		{name: "local internal constructor", provider: opencodeAgentID, err: sdk.NewInternalError(map[string]any{"details": "database is locked"})},
		{name: "local auth constructor", provider: claudeAgentID, err: sdk.NewAuthRequired(nil)},
		{name: "local cancellation", provider: codexAgentID, err: sdk.NewRequestCancelled(nil)},
		{name: "plain error", provider: "custom-acp", err: errors.New("Internal error: Failed to execute statement")},
	}
	for _, test := range localCases {
		t.Run(test.name, func(t *testing.T) {
			a := newTestAdapterForAgent(test.provider)
			if providerError, ok := a.retainedACPApplicationError(test.err); ok {
				t.Fatalf("local error %v produced retained evidence %+v", test.err, providerError)
			}
		})
	}
}

package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	promptmodels "github.com/kandev/kandev/internal/prompts/models"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type sharedPromptReaderStub struct {
	prompts       []*promptmodels.Prompt
	prompt        *promptmodels.Prompt
	listErr       error
	getErr        error
	requestedName string
	requestedCtx  context.Context
}

type sharedPromptContextKey struct{}

func (s *sharedPromptReaderStub) ListPrompts(ctx context.Context) ([]*promptmodels.Prompt, error) {
	s.requestedCtx = ctx
	return s.prompts, s.listErr
}

func (s *sharedPromptReaderStub) GetPromptByName(ctx context.Context, name string) (*promptmodels.Prompt, error) {
	s.requestedCtx = ctx
	s.requestedName = name
	return s.prompt, s.getErr
}

func TestRegisterHandlers_PromptReaderControlsSavedPromptActions(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reader PromptReader
		want   bool
	}{
		{name: "reader available", reader: &sharedPromptReaderStub{}, want: true},
		{name: "reader unavailable", reader: nil, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := &Handlers{logger: testLogger(t)}
			h.SetPromptReader(tc.reader)
			dispatcher := ws.NewDispatcher()
			h.RegisterHandlers(dispatcher)

			assert.Equal(t, tc.want, dispatcher.HasHandler(ws.ActionMCPListSharedPrompts))
			assert.Equal(t, tc.want, dispatcher.HasHandler(ws.ActionMCPGetSharedPrompt))
		})
	}
}

func TestHandleListSharedPrompts_MapsSummariesWithoutContent(t *testing.T) {
	reader := &sharedPromptReaderStub{prompts: []*promptmodels.Prompt{
		{Name: "code-review", Content: "review é", Builtin: true},
		{Name: "custom", Content: "custom prompt", Builtin: false},
	}}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	resp, err := h.handleListSharedPrompts(context.Background(), makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]interface{}{}))
	require.NoError(t, err)
	require.NotNil(t, resp)

	var payload struct {
		SharedPrompts []struct {
			Name         string `json:"name"`
			Builtin      bool   `json:"builtin"`
			ContentBytes int    `json:"content_bytes"`
			Content      string `json:"content"`
		} `json:"shared_prompts"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	require.Len(t, payload.SharedPrompts, 2)
	assert.Equal(t, "code-review", payload.SharedPrompts[0].Name)
	assert.True(t, payload.SharedPrompts[0].Builtin)
	assert.Equal(t, len("review é"), payload.SharedPrompts[0].ContentBytes)
	assert.Empty(t, payload.SharedPrompts[0].Content)
	assert.Equal(t, "custom", payload.SharedPrompts[1].Name)
	assert.False(t, payload.SharedPrompts[1].Builtin)
	assert.Equal(t, 2, payload.Total)
	assert.NotContains(t, string(resp.Payload), "review é")
}

func TestHandleGetSharedPrompt_TrimsNameAndOmitsInternalID(t *testing.T) {
	createdAt := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.FixedZone("test", 3600))
	updatedAt := createdAt.Add(time.Hour)
	reader := &sharedPromptReaderStub{prompt: &promptmodels.Prompt{
		ID:        "internal-prompt-id",
		Name:      "code-review",
		Content:   "Review the change.",
		Builtin:   true,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	ctx := context.WithValue(context.Background(), sharedPromptContextKey{}, "request-value")
	resp, err := h.handleGetSharedPrompt(ctx, makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]interface{}{
		"name": "  code-review  ",
	}))
	require.NoError(t, err)
	assert.Equal(t, "code-review", reader.requestedName)
	assert.Equal(t, "request-value", reader.requestedCtx.Value(sharedPromptContextKey{}))
	assert.NotContains(t, string(resp.Payload), "internal-prompt-id")

	var payload map[string]interface{}
	require.NoError(t, json.Unmarshal(resp.Payload, &payload))
	assert.Equal(t, "code-review", payload["name"])
	assert.Equal(t, "Review the change.", payload["content"])
	assert.Equal(t, true, payload["builtin"])
	assert.Equal(t, float64(len("Review the change.")), payload["content_bytes"])
	assert.Equal(t, "2025-01-02T02:04:05Z", payload["created_at"])
	assert.Equal(t, "2025-01-02T03:04:05Z", payload["updated_at"])
	assert.NotContains(t, payload, "id")
}

func TestHandleGetSharedPrompt_RejectsBlankAndMapsNotFound(t *testing.T) {
	reader := &sharedPromptReaderStub{getErr: promptservice.ErrPromptNotFound}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	blank, err := h.handleGetSharedPrompt(context.Background(), makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]string{
		"name": " \t",
	}))
	require.NoError(t, err)
	assertWSError(t, blank, ws.ErrorCodeValidation)
	assert.Empty(t, reader.requestedName)

	missing, err := h.handleGetSharedPrompt(context.Background(), makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]string{
		"name": "missing",
	}))
	require.NoError(t, err)
	assertWSError(t, missing, ws.ErrorCodeNotFound)
	assert.NotContains(t, string(missing.Payload), "content")
}

func TestHandleGetSharedPrompt_MapsNilResultToNotFound(t *testing.T) {
	reader := &sharedPromptReaderStub{}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	resp, err := h.handleGetSharedPrompt(context.Background(), makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]string{
		"name": "missing",
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeNotFound)
	assert.NotContains(t, string(resp.Payload), "content")
}

func TestHandleGetSharedPrompt_HidesRepositoryErrors(t *testing.T) {
	reader := &sharedPromptReaderStub{
		prompt: &promptmodels.Prompt{Content: "secret prompt content"},
		getErr: errors.New("database details"),
	}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	resp, err := h.handleGetSharedPrompt(context.Background(), makeWSMessage(t, ws.ActionMCPGetSharedPrompt, map[string]string{
		"name": "code-review",
	}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assert.NotContains(t, string(resp.Payload), "database details")
	assert.NotContains(t, string(resp.Payload), "secret prompt content")
}

func TestHandleSharedPrompts_HidesRepositoryErrors(t *testing.T) {
	reader := &sharedPromptReaderStub{listErr: errors.New("database details")}
	h := &Handlers{promptReader: reader, logger: testLogger(t)}

	resp, err := h.handleListSharedPrompts(context.Background(), makeWSMessage(t, ws.ActionMCPListSharedPrompts, map[string]interface{}{}))
	require.NoError(t, err)
	assertWSError(t, resp, ws.ErrorCodeInternalError)
	assert.NotContains(t, string(resp.Payload), "database details")
}

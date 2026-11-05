package backendapp

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/kandev/kandev/internal/agent/runtime"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/clarification/protocol"
	"github.com/kandev/kandev/internal/common/imagepreview"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestQuestionImagesMetadataRoundTrip(t *testing.T) {
	var question clarification.Question
	require.NoError(t, json.Unmarshal([]byte(`{"id":"q","prompt":"Approve?","images":[{"id":"screen","path":"screen.png","alt":"Layout"}]}`), &question))
	data, err := json.Marshal(clarificationQuestionData(question, nil))
	require.NoError(t, err)
	require.Contains(t, string(data), `"path":"screen.png"`)
	var restored clarification.Question
	require.NoError(t, json.Unmarshal(data, &restored))
	again, err := json.Marshal(restored)
	require.NoError(t, err)
	require.Contains(t, string(again), `"path":"screen.png"`)
	legacy := clarificationQuestionData(clarification.Question{}, nil)
	require.NotContains(t, legacy, "images")
}

func TestClarificationChoiceImageMetadata(t *testing.T) {
	var question clarification.Question
	require.NoError(t, json.Unmarshal([]byte(`{"id":"q","prompt":"Choose","options":[{"option_id":"wide","label":"Wide","description":"Roomy","images":[{"id":"wide","path":"wide.png","alt":"Wide preview"}]},{"option_id":"compact","label":"Compact"}]}`), &question))
	data, err := json.Marshal(clarificationQuestionData(question, clarificationOptionsData(question.Options)))
	require.NoError(t, err)
	var restored clarification.Question
	require.NoError(t, json.Unmarshal(data, &restored))
	require.Equal(t, question, restored)
	require.Len(t, restored.Options[0].Images, 1)
	require.NotContains(t, clarificationOptionsData(question.Options)[1], "images")
}

type imageExecutionLookup struct {
	executions map[string]*runtime.AgentExecution
	err        error
}

func (l imageExecutionLookup) GetOrEnsureExecution(_ context.Context, id string) (*runtime.AgentExecution, error) {
	return l.executions[id], l.err
}

func TestQuestionImageSourceSession(t *testing.T) {
	executions := map[string]*runtime.AgentExecution{}
	for _, id := range []string{"source", "selected"} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/api/v1/workspace/image-preview", r.URL.Path)
			require.Equal(t, "same.png", r.URL.Query().Get("path"))
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte(id))
		}))
		t.Cleanup(server.Close)
		parsed, err := url.Parse(server.URL)
		require.NoError(t, err)
		port, err := strconv.Atoi(parsed.Port())
		require.NoError(t, err)
		execution := &runtime.AgentExecution{}
		execution.SetAgentCtlClientForTesting(agentctl.NewClient(parsed.Hostname(), port, logger.Default()))
		executions[id] = execution
	}
	adapter := &questionImageReaderAdapter{lifecycle: imageExecutionLookup{executions: executions}}
	result, err := adapter.ReadQuestionImage(t.Context(), "source", protocol.QuestionImage{Path: "same.png"})
	require.NoError(t, err)
	require.Equal(t, "source", string(result.Bytes))
	adapter.lifecycle = imageExecutionLookup{err: errors.New("session.exec denied")}
	result, err = adapter.ReadQuestionImage(t.Context(), "source", protocol.QuestionImage{Path: "same.png"})
	require.ErrorIs(t, err, imagepreview.Unavailable)
	require.Nil(t, result)
}

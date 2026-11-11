package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	mcplib "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.1
func TestAskUserQuestionAnswerLabels(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{
		"answers": []interface{}{
			map[string]interface{}{"question_id": "q3", "selected_options": []interface{}{"q3_opt2"}},
			map[string]interface{}{"question_id": "database", "selected_options": []interface{}{"custom-choice"}},
			map[string]interface{}{"question_id": "flow", "selected_options": []interface{}{"q1_opt1"}},
		},
	}}
	s := newTaskModeServer(t, backend, "task-current")
	result := callTool(t, s, "ask_user_question_kandev", map[string]interface{}{
		"questions": []map[string]interface{}{
			{"id": "flow", "prompt": "Choose a flow", "options": []map[string]interface{}{
				{"label": "A", "description": "First flow"},
				{"label": "B", "description": "Second flow"},
				{"label": "C", "description": "Third flow"},
			}},
			{"id": "database", "prompt": "Choose a database", "options": []map[string]interface{}{
				{"option_id": "custom-choice", "label": "SQLite \"local\"", "description": "Embedded"},
				{"option_id": "other", "label": "A", "description": "Another database"},
			}},
			{"prompt": "Choose a duplicate label", "options": []map[string]interface{}{
				{"label": "A", "description": "First"},
				{"label": "A", "description": "Second"},
			}},
		},
	})
	assertQuestionLabelResult(t, result, `{
  "flow":{"selected_option":"q1_opt1","selected_label":"A"},
  "database":{"selected_option":"custom-choice","selected_label":"SQLite \"local\""},
  "q3":{"selected_option":"q3_opt2","selected_label":"A"}
 }`)
}

// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.2
// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.3
func TestExtractQuestionAnswerLabels(t *testing.T) {
	tests := []struct {
		name     string
		selected string
		custom   string
		rejected bool
		options  []map[string]interface{}
		want     string
	}{
		{name: "selection and text", selected: "q1_opt1", custom: "Use this", want: `{"selected_option":"q1_opt1","selected_label":"A","custom_text":"Use this"}`},
		{name: "text only", custom: "Neither", want: `{"custom_text":"Neither"}`},
		{name: "unanswered", want: `{"answered":false}`},
		{name: "rejected", rejected: true, want: `{"answered":false,"rejected":true}`},
		{name: "partial rejection", rejected: true, selected: "q1_opt1", want: `{"selected_option":"q1_opt1","selected_label":"A"}`},
		{name: "unknown ID", selected: "q2_opt1", custom: "Keep text", want: `{"selected_option":"q2_opt1","custom_text":"Keep text"}`},
		{name: "duplicate ID", selected: "same", options: []map[string]interface{}{
			{"option_id": "same", "label": "A"}, {"option_id": "same", "label": "B"},
		}, want: `{"selected_option":"same"}`},
		{name: "missing label", selected: "same", options: []map[string]interface{}{
			{"option_id": "same"},
		}, want: `{"selected_option":"same"}`},
		{name: "empty label", selected: "same", options: []map[string]interface{}{
			{"option_id": "same", "label": ""},
		}, want: `{"selected_option":"same"}`},
		{name: "duplicate ID missing label", selected: "same", options: []map[string]interface{}{
			{"option_id": "same", "label": "A"}, {"option_id": "same"},
		}, want: `{"selected_option":"same"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			options := tt.options
			if options == nil {
				options = []map[string]interface{}{{"option_id": "q1_opt1", "label": "A"}}
			}
			questions := []map[string]interface{}{
				{"id": "flow", "options": options},
				{"id": "other", "options": []map[string]interface{}{{"option_id": "q2_opt1", "label": "Wrong question"}}},
			}
			result := map[string]interface{}{"answers": []interface{}{map[string]interface{}{
				"question_id": "flow", "selected_options": []interface{}{tt.selected}, "custom_text": tt.custom,
			}}}
			want := `{"flow":` + tt.want + `,"other":{"answered":false}}`
			if tt.rejected {
				result["rejected"] = true
				result["reject_reason"] = "Later"
				want = `{"flow":` + tt.want + `,"other":{"answered":false,"rejected":true},"rejected":true,"reject_reason":"Later"}`
			}
			assertQuestionLabelResult(t, extractQuestionAnswers(result, questions), want)
		})
	}
}

func assertQuestionLabelResult(t *testing.T, result *mcplib.CallToolResult, want string) {
	t.Helper()
	require.False(t, result.IsError)
	content, ok := result.Content[0].(mcplib.TextContent)
	require.True(t, ok)
	assert.JSONEq(t, want, content.Text)
	structured, err := json.Marshal(result.StructuredContent)
	require.NoError(t, err)
	assert.JSONEq(t, want, string(structured))
}

// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.4
func TestAskUserQuestionAnswerLabelsSchema(t *testing.T) {
	s := newTaskModeServer(t, &testBackend{}, "task-current")
	tool := s.mcpServer.ListTools()["ask_user_question_kandev"].Tool
	assert.Contains(t, tool.Description, "selected_label")
	assertQuestionLabelSchema(t, tool.RawOutputSchema)
}

func assertQuestionLabelSchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal(raw, &schema))
	answer := schema["additionalProperties"].(map[string]interface{})
	props := answer["properties"].(map[string]interface{})
	for _, field := range []string{"selected_option", "selected_label"} {
		prop, ok := props[field].(map[string]interface{})
		require.True(t, ok, "missing schema field %s", field)
		assert.Equal(t, "string", prop["type"])
		assert.NotEmpty(t, prop["description"])
		required, _ := answer["required"].([]interface{})
		assert.NotContains(t, required, field)
	}
	assert.Equal(t, false, answer["additionalProperties"])
}

// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.1
// @covers AC-TASKS-QUESTION-ANSWER-LABELS-001.4
func TestAskUserQuestionAnswerLabelsTransport(t *testing.T) {
	backend := &testBackend{response: map[string]interface{}{"answers": []interface{}{
		map[string]interface{}{"question_id": "flow", "selected_options": []interface{}{"q1_opt1"}},
	}}}
	s := newTaskModeServer(t, backend, "task-current")
	router := gin.New()
	s.RegisterRoutes(router)
	ts := httptest.NewServer(router)
	t.Cleanup(ts.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	init := postQuestionLabelRPC(t, ctx, ts.URL, "", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0.0"}}}`)
	listing := postQuestionLabelRPC(t, ctx, ts.URL, init.sessionID, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	var catalog struct {
		Result struct {
			Tools []struct {
				Name         string          `json:"name"`
				OutputSchema json.RawMessage `json:"outputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractDataLine(listing.body)), &catalog))
	found := false
	for _, tool := range catalog.Result.Tools {
		if tool.Name == "ask_user_question_kandev" {
			found = true
			assertQuestionLabelSchema(t, tool.OutputSchema)
		}
	}
	require.True(t, found)
	response := postQuestionLabelRPC(t, ctx, ts.URL, init.sessionID, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ask_user_question_kandev","arguments":{"questions":[{"id":"flow","prompt":"Which?","options":[{"label":"A","description":"First"},{"label":"B","description":"Second"},{"label":"C","description":"Third"}]}]}}}`)
	var frame struct {
		Error  json.RawMessage `json:"error"`
		Result struct {
			IsError bool `json:"isError"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			StructuredContent json.RawMessage `json:"structuredContent"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal([]byte(extractDataLine(response.body)), &frame))
	require.Empty(t, frame.Error)
	require.False(t, frame.Result.IsError)
	require.Len(t, frame.Result.Content, 1)
	const want = `{"flow":{"selected_option":"q1_opt1","selected_label":"A"}}`
	assert.JSONEq(t, want, frame.Result.Content[0].Text)
	assert.JSONEq(t, want, string(frame.Result.StructuredContent))
}

func postQuestionLabelRPC(t *testing.T, ctx context.Context, url, sessionID, body string) httpResp {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url+"/mcp", strings.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if sessionID != "" {
		req.Header.Set("Mcp-Session-Id", sessionID)
	}
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	t.Cleanup(func() { _ = resp.Body.Close() })
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode, "%s", data)
	return httpResp{statusCode: resp.StatusCode, body: string(data), sessionID: resp.Header.Get("Mcp-Session-Id")}
}

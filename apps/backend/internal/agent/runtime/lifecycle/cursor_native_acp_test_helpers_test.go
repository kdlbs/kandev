package lifecycle

import (
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type cursorNativeSmokeACPClient struct {
	input                    io.WriteCloser
	decoder                  *json.Decoder
	output                   strings.Builder
	assistantOutput          strings.Builder
	fixturePermissionAllowed bool
	initializeResult         json.RawMessage
	sessionIDs               []string
	toolEvidence             []cursorNativeSmokeToolEvidence
}

func (c *cursorNativeSmokeACPClient) initializeSummary() string {
	var result struct {
		AgentCapabilities struct {
			LoadSession         bool `json:"loadSession"`
			SessionCapabilities struct {
				Resume json.RawMessage `json:"resume"`
			} `json:"sessionCapabilities"`
		} `json:"agentCapabilities"`
	}
	if json.Unmarshal(c.initializeResult, &result) != nil {
		return "unavailable"
	}
	resume := result.AgentCapabilities.SessionCapabilities.Resume
	resumeSupported := len(resume) > 0 && string(resume) != "null"
	if result.AgentCapabilities.LoadSession && resumeSupported {
		return "loadSession=true,resume=true"
	}
	if result.AgentCapabilities.LoadSession {
		return "loadSession=true,resume=false"
	}
	if resumeSupported {
		return "loadSession=false,resume=true"
	}
	return "loadSession=false,resume=false"
}

func (c *cursorNativeSmokeACPClient) hasCompletedReadForPathSince(start int, path string) bool {
	for _, evidence := range c.toolEvidence[start:] {
		if evidence.Kind == "read" && evidence.Status == "completed" && evidence.Path == path {
			return true
		}
	}
	return false
}

func (p *cursorNativeSmokeACPProcess) promptAndCancelAfterAssistantMarker(
	t *testing.T,
	requestID int,
	prompt string,
	marker string,
) (json.RawMessage, bool) {
	t.Helper()
	p.client.send(t, map[string]any{"jsonrpc": "2.0", "id": requestID, "method": "session/prompt", "params": map[string]any{
		"sessionId": p.sessionID,
		"prompt":    []map[string]string{{"type": "text", "text": prompt}},
	}})
	cancelSent := false
	for {
		var frame cursorNativeSmokeACPFrame
		err := p.client.decoder.Decode(&frame)
		require.NoError(t, err, "Cursor ACP closed before the output prompt settled")
		if frame.Method != "" {
			p.client.captureSessionUpdate(frame)
			if !cancelSent && frame.Method == "session/update" && strings.Contains(p.client.assistantOutput.String(), marker) {
				p.client.send(t, map[string]any{"jsonrpc": "2.0", "method": "session/cancel", "params": map[string]string{"sessionId": p.sessionID}})
				cancelSent = true
			}
			p.client.handleServerRequest(t, frame)
			continue
		}
		if !cursorNativeSmokeJSONIDMatches(frame.ID, requestID) {
			continue
		}
		if len(frame.Error) > 0 {
			return nil, cancelSent
		}
		return frame.Result, cancelSent
	}
}

type cursorNativeSmokeACPFrame struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

func (c *cursorNativeSmokeACPClient) request(t *testing.T, id int, method string, params any) json.RawMessage {
	t.Helper()
	c.send(t, map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	for {
		var frame cursorNativeSmokeACPFrame
		err := c.decoder.Decode(&frame)
		require.NoError(t, err, "Cursor ACP closed before replying to %s", method)
		if frame.Method != "" {
			c.captureAgentMessageChunk(frame)
			c.handleServerRequest(t, frame)
			continue
		}
		if !cursorNativeSmokeJSONIDMatches(frame.ID, id) {
			continue
		}
		require.Empty(t, frame.Error, "Cursor ACP returned an error for %s", method)
		return frame.Result
	}
}

func (c *cursorNativeSmokeACPClient) captureAgentMessageChunk(frame cursorNativeSmokeACPFrame) {
	c.captureSessionUpdate(frame)
}

func (c *cursorNativeSmokeACPClient) captureSessionUpdate(frame cursorNativeSmokeACPFrame) {
	if frame.Method != "session/update" {
		return
	}
	var params struct {
		SessionID string `json:"sessionId"`
		Update    struct {
			SessionUpdate string          `json:"sessionUpdate"`
			Content       json.RawMessage `json:"content"`
			Kind          string          `json:"kind"`
			Status        string          `json:"status"`
			ToolCallID    string          `json:"toolCallId"`
			RawInput      map[string]any  `json:"rawInput"`
			RawOutput     json.RawMessage `json:"rawOutput"`
		} `json:"update"`
	}
	if json.Unmarshal(frame.Params, &params) != nil {
		return
	}
	if params.SessionID != "" {
		c.sessionIDs = append(c.sessionIDs, params.SessionID)
	}
	if params.Update.SessionUpdate == "agent_message_chunk" || params.Update.SessionUpdate == "user_message_chunk" {
		var content struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(params.Update.Content, &content) == nil && content.Type == "text" {
			c.output.WriteString(content.Text)
			if params.Update.SessionUpdate == "agent_message_chunk" {
				c.assistantOutput.WriteString(content.Text)
			}
		}
	}
	if params.Update.SessionUpdate == "tool_call" || params.Update.SessionUpdate == "tool_call_update" {
		c.captureToolUpdate(params.Update.Kind, params.Update.Status, params.Update.ToolCallID,
			params.Update.RawInput, params.Update.Content, params.Update.RawOutput)
	}
}

func (c *cursorNativeSmokeACPClient) captureToolUpdate(
	kind, status, id string,
	rawInput map[string]any,
	content, rawOutput json.RawMessage,
) {
	path, _ := rawInput["path"].(string)
	evidence := cursorNativeSmokeToolEvidence{Kind: kind, Status: status, ID: id, Path: path}
	updated := false
	for i := len(c.toolEvidence) - 1; i >= 0; i-- {
		previous := c.toolEvidence[i]
		if evidence.ID == "" || previous.ID != evidence.ID {
			continue
		}
		evidence = mergeCursorNativeToolEvidence(evidence, previous)
		c.toolEvidence[i] = evidence
		updated = true
		break
	}
	if !updated {
		c.toolEvidence = append(c.toolEvidence, evidence)
	}
	if len(content) > 0 {
		c.output.Write(content)
	}
	if len(rawOutput) > 0 {
		c.output.Write(rawOutput)
	}
}

func mergeCursorNativeToolEvidence(current, previous cursorNativeSmokeToolEvidence) cursorNativeSmokeToolEvidence {
	if current.Kind == "" {
		current.Kind = previous.Kind
	}
	if current.Path == "" {
		current.Path = previous.Path
	}
	if current.Status == "" {
		current.Status = previous.Status
	}
	return current
}

func (c *cursorNativeSmokeACPClient) send(t *testing.T, frame any) {
	t.Helper()
	data, err := json.Marshal(frame)
	require.NoError(t, err)
	_, err = c.input.Write(append(data, '\n'))
	require.NoError(t, err)
}

func (c *cursorNativeSmokeACPClient) handleServerRequest(t *testing.T, frame cursorNativeSmokeACPFrame) {
	t.Helper()
	if len(frame.ID) == 0 {
		return
	}
	if frame.Method != "session/request_permission" {
		c.send(t, map[string]any{"jsonrpc": "2.0", "id": frame.ID,
			"error": map[string]any{"code": -32601, "message": "Unsupported fixture request"}})
		return
	}
	var request struct {
		ToolCall struct {
			Title    string `json:"title"`
			RawInput struct {
				ProviderIdentifier string `json:"providerIdentifier"`
				ToolName           string `json:"toolName"`
			} `json:"rawInput"`
		} `json:"toolCall"`
		Options []struct {
			OptionID string `json:"optionId"`
			Kind     string `json:"kind"`
		} `json:"options"`
	}
	require.NoError(t, json.Unmarshal(frame.Params, &request))
	fixtureTool := request.ToolCall.RawInput.ProviderIdentifier == cursorNativeSmokeID &&
		request.ToolCall.RawInput.ToolName == "fixture_ping"
	fixtureTool = fixtureTool || strings.Contains(request.ToolCall.Title, cursorNativeSmokeID+"-fixture_ping")
	var outcome map[string]any
	if fixtureTool {
		for _, option := range request.Options {
			if option.Kind == "allow_once" && option.OptionID != "" {
				c.fixturePermissionAllowed = true
				outcome = map[string]any{"outcome": "selected", "optionId": option.OptionID}
				break
			}
		}
	}
	if outcome == nil {
		outcome = map[string]any{"outcome": "cancelled"}
	}
	c.send(t, map[string]any{"jsonrpc": "2.0", "id": frame.ID, "result": map[string]any{"outcome": outcome}})
}

func cursorNativeSmokeJSONIDMatches(raw json.RawMessage, want int) bool {
	var got int
	return json.Unmarshal(raw, &got) == nil && got == want
}

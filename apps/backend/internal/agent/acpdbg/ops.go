package acpdbg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
)

// IsAuthErrorMessage is a coarse substring heuristic mirroring
// hostutility.isAuthError. ACP auth failures bubble up as plain-text
// JSON-RPC errors without a distinct code, so we match obvious markers so
// callers can route the user to the login flow; anything unmatched stays
// as a generic failure.
func IsAuthErrorMessage(s string) bool {
	lower := strings.ToLower(s)
	for _, needle := range []string{"auth", "login", "unauthorized", "credential", "api key", "token"} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// ProbeResult is the parsed summary of a successful probe. It mirrors the
// high-level fields we extract from `initialize` and `session/new` so the
// CLI's `matrix` summary and the skill don't have to re-parse JSONL.
type ProbeResult struct {
	ProtocolVersion int
	AgentName       string
	AgentVersion    string
	AuthMethods     []string
	Models          []string
	CurrentModelID  string
	Modes           []string
	CurrentModeID   string
	SessionID       string
}

// Probe runs the minimum ACP handshake: initialize → session/new → close.
// Returns a parsed summary, or an error describing where the handshake
// broke. The runner's JSONL file captures the full wire payload regardless
// of the outcome.
func Probe(ctx context.Context, r *Runner) (*ProbeResult, error) {
	initResp, err := sendInitialize(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}

	newResp, err := sendSessionNew(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("session/new: %w", err)
	}

	return buildProbeResult(initResp, newResp), nil
}

// PromptOptions configures the Prompt operation.
type PromptOptions struct {
	Model  string // if non-empty, set via session/set_model (or unstable variant)
	Mode   string // if non-empty, set via session/set_mode
	Prompt string // required — text body of the session/prompt request
}

// PromptResult holds the text response collected from session/update
// message_chunks while session/prompt is running.
type PromptResult struct {
	ProbeResult
	Text string
}

// Prompt runs the full prompt round-trip: initialize → session/new →
// [set_model] → [set_mode] → session/prompt → drain updates until the
// prompt response arrives.
func Prompt(ctx context.Context, r *Runner, opts PromptOptions) (*PromptResult, error) {
	if opts.Prompt == "" {
		return nil, errors.New("prompt is required")
	}
	probe, err := Probe(ctx, r)
	if err != nil {
		return nil, err
	}

	if opts.Model != "" {
		if err := sendSetModel(ctx, r, probe.SessionID, opts.Model); err != nil {
			return nil, fmt.Errorf("session/set_model: %w", err)
		}
	}
	if opts.Mode != "" {
		if err := sendSetMode(ctx, r, probe.SessionID, opts.Mode); err != nil {
			return nil, fmt.Errorf("session/set_mode: %w", err)
		}
	}

	text, err := sendPromptAndCollect(ctx, r, probe.SessionID, opts.Prompt)
	if err != nil {
		return nil, fmt.Errorf("session/prompt: %w", err)
	}
	return &PromptResult{ProbeResult: *probe, Text: text}, nil
}

// SessionLoad runs initialize → session/load.
func SessionLoad(ctx context.Context, r *Runner, sessionID string) (*ProbeResult, error) {
	if sessionID == "" {
		return nil, errors.New("session-id is required")
	}
	initResp, err := sendInitialize(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	req, _ := r.Framer().NewRequest("session/load", map[string]any{
		"sessionId":  sessionID,
		"mcpServers": []any{},
	})
	resp, err := r.Request(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("session/load: %w", err)
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("session/load error: %v", errMap["message"])
	}
	return buildProbeResult(initResp, resp), nil
}

// --- JSON-RPC helpers ---

func sendInitialize(ctx context.Context, r *Runner) (Frame, error) {
	req, _ := r.Framer().NewRequest("initialize", map[string]any{
		"protocolVersion": 1,
		"clientInfo": map[string]any{
			"name":    "acpdbg",
			"version": "0.1.0",
		},
		"clientCapabilities": map[string]any{
			"fs":       map[string]any{"readTextFile": false, "writeTextFile": false},
			"terminal": false,
		},
	})
	resp, err := r.Request(ctx, req)
	if err != nil {
		return nil, err
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("%v", errMap["message"])
	}
	return resp, nil
}

func sendSessionNew(ctx context.Context, r *Runner) (Frame, error) {
	req, _ := r.Framer().NewRequest("session/new", map[string]any{
		"cwd":        r.cfg.Workdir,
		"mcpServers": []any{},
	})
	resp, err := r.Request(ctx, req)
	if err != nil {
		return nil, err
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("%v", errMap["message"])
	}
	return resp, nil
}

func sendSetModel(ctx context.Context, r *Runner, sessionID, modelID string) error {
	// Try the unstable form first (matches the acp-go-sdk fork we use).
	// If that returns method-not-found, fall back to the stable name.
	req, _ := r.Framer().NewRequest("session/set_model", map[string]any{
		"sessionId": sessionID,
		"modelId":   modelID,
	})
	resp, err := r.Request(ctx, req)
	if err != nil {
		return err
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return fmt.Errorf("%v", errMap["message"])
	}
	return nil
}

func sendSetMode(ctx context.Context, r *Runner, sessionID, modeID string) error {
	req, _ := r.Framer().NewRequest("session/set_mode", map[string]any{
		"sessionId": sessionID,
		"modeId":    modeID,
	})
	resp, err := r.Request(ctx, req)
	if err != nil {
		return err
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return fmt.Errorf("%v", errMap["message"])
	}
	return nil
}

func sendPromptAndCollect(ctx context.Context, r *Runner, sessionID, prompt string) (string, error) {
	req, promptID := r.Framer().NewRequest("session/prompt", map[string]any{
		"sessionId": sessionID,
		"prompt": []any{
			map[string]any{"type": "text", "text": prompt},
		},
	})

	// We can't use r.Request here because we want to drain update
	// notifications while waiting for the response. Write directly and
	// loop on the OOB channel + a pending response future.
	if err := r.rec.Sent(req); err != nil {
		return "", err
	}

	respCh := make(chan Frame, 1)
	r.mu.Lock()
	r.pending[promptID] = respCh
	r.mu.Unlock()

	if err := r.framer.Write(req); err != nil {
		return "", err
	}

	var text strings.Builder
	for {
		select {
		case resp := <-respCh:
			if errMap, ok := resp["error"].(map[string]any); ok {
				return text.String(), fmt.Errorf("%v", errMap["message"])
			}
			return text.String(), nil
		case frame, ok := <-r.oob:
			if !ok {
				return text.String(), io.EOF
			}
			// Agent-initiated request during a prompt: auto-reply so it
			// doesn't hang.
			if frame.Method() != "" && frame.ID() != nil {
				reply := NewMethodNotFound(frame.ID(), frame.Method())
				_ = r.rec.Sent(reply)
				_ = r.framer.Write(reply)
				continue
			}
			// Notification: extract text chunks from session/update if
			// present.
			if frame.Method() == "session/update" {
				if chunk := extractTextChunk(frame); chunk != "" {
					text.WriteString(chunk)
				}
			}
		case <-ctx.Done():
			return text.String(), ctx.Err()
		}
	}
}

// extractTextChunk pulls an `agentMessageChunk.content.text.text` value out
// of a session/update notification, if present. ACP's session/update is a
// tagged variant; we walk the structure defensively.
func extractTextChunk(frame Frame) string {
	params, ok := frame["params"].(map[string]any)
	if !ok {
		return ""
	}
	update, ok := params["update"].(map[string]any)
	if !ok {
		return ""
	}
	// Variant 1: { "sessionUpdate": "agent_message_chunk", "content": {...} }
	if kind, _ := update["sessionUpdate"].(string); kind == "agent_message_chunk" {
		if content, ok := update["content"].(map[string]any); ok {
			if t, ok := content["text"].(string); ok {
				return t
			}
		}
	}
	// Variant 2 (older SDKs): { "agentMessageChunk": { "content": {...} } }
	return extractVariant2Text(update)
}

func extractVariant2Text(update map[string]any) string {
	chunk, ok := update["agentMessageChunk"].(map[string]any)
	if !ok {
		return ""
	}
	content, ok := chunk["content"].(map[string]any)
	if !ok {
		return ""
	}
	if inner, ok := content["text"].(map[string]any); ok {
		if t, ok := inner["text"].(string); ok {
			return t
		}
	}
	if t, ok := content["text"].(string); ok {
		return t
	}
	return ""
}

func buildProbeResult(initResp, newResp Frame) *ProbeResult {
	out := &ProbeResult{}
	if initResult, ok := initResp["result"].(map[string]any); ok {
		parseInitResult(initResult, out)
	}
	if newResult, ok := newResp["result"].(map[string]any); ok {
		parseSessionResult(newResult, out)
	}
	return out
}

func parseInitResult(initResult map[string]any, out *ProbeResult) {
	if pv, ok := initResult["protocolVersion"].(float64); ok {
		out.ProtocolVersion = int(pv)
	}
	if info, ok := initResult["agentInfo"].(map[string]any); ok {
		out.AgentName, _ = info["name"].(string)
		out.AgentVersion, _ = info["version"].(string)
	}
	if methods, ok := initResult["authMethods"].([]any); ok {
		out.AuthMethods = extractStringField(methods, "id")
	}
}

func parseSessionResult(newResult map[string]any, out *ProbeResult) {
	out.SessionID, _ = newResult["sessionId"].(string)
	if models, ok := newResult["models"].(map[string]any); ok {
		out.CurrentModelID, _ = models["currentModelId"].(string)
		if avail, ok := models["availableModels"].([]any); ok {
			out.Models = extractStringField(avail, "modelId")
		}
	}
	if modes, ok := newResult["modes"].(map[string]any); ok {
		out.CurrentModeID, _ = modes["currentModeId"].(string)
		if avail, ok := modes["availableModes"].([]any); ok {
			out.Modes = extractStringField(avail, "id")
		}
	}
}

// extractStringField pulls a string field from each map entry in the list.
func extractStringField(list []any, field string) []string {
	var out []string
	for _, item := range list {
		mm, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := mm[field].(string); ok {
			out = append(out, v)
		}
	}
	return out
}

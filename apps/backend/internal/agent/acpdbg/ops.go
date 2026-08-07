package acpdbg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// methodSessionUpdate is the ACP notification an agent streams for every
// message chunk, tool call, and replayed history entry.
const methodSessionUpdate = "session/update"

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
	MCPHTTP         bool
	MCPSSE          bool
}

// MCPProbeResult describes what was delivered to an agent separately from
// what the injected sentinel observed. An unobserved sentinel is normal for
// agents which defer MCP connections or do not support the supplied transport.
type MCPProbeResult struct {
	ProbeResult
	Delivered bool
	Sentinel  MCPSentinelSummary
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

// MCPProbe runs initialize and injects a short-lived HTTP sentinel into
// session/new. The caller's timeout bounds both ACP delivery and observation.
func MCPProbe(ctx context.Context, r *Runner) (*MCPProbeResult, error) {
	initResp, err := sendInitialize(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	sentinel := NewMCPSentinel(r.rec)
	defer sentinel.Close()
	newResp, err := sendSessionNewWithMCP(ctx, r, []any{map[string]any{
		"name": "acpdbg-sentinel",
		"type": "http",
		"url":  sentinel.URL(),
	}})
	if err != nil {
		return nil, fmt.Errorf("session/new: %w", err)
	}
	result := &MCPProbeResult{
		ProbeResult: *buildProbeResult(initResp, newResp),
		Delivered:   true,
	}
	// Give an asynchronously connecting agent a small, bounded opportunity to
	// reach the sentinel without turning missing traffic into a failure.
	wait := time.NewTimer(750 * time.Millisecond)
	defer wait.Stop()
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-wait.C:
	}
	result.Sentinel = sentinel.Summary()
	return result, nil
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

// SessionLoadOptions configures a session/load operation and an optional
// prompt used to verify the loaded session's effective workspace.
type SessionLoadOptions struct {
	SessionID string
	Prompt    string
}

// SessionLoadResult holds the loaded session metadata and optional response.
type SessionLoadResult struct {
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

// SessionLoad runs initialize → session/load → [session/prompt].
func SessionLoad(ctx context.Context, r *Runner, opts SessionLoadOptions) (*SessionLoadResult, error) {
	if opts.SessionID == "" {
		return nil, errors.New("session-id is required")
	}
	initResp, err := sendInitialize(ctx, r)
	if err != nil {
		return nil, fmt.Errorf("initialize: %w", err)
	}
	req, _ := r.Framer().NewRequest("session/load", sessionLoadParams(opts.SessionID, r.cfg.Workdir))
	resp, err := r.Request(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("session/load: %w", err)
	}
	if errMap, ok := resp["error"].(map[string]any); ok {
		return nil, fmt.Errorf("session/load error: %v", errMap["message"])
	}
	result := &SessionLoadResult{ProbeResult: *buildProbeResult(initResp, resp)}
	if opts.Prompt == "" {
		return result, nil
	}
	result.Text, err = sendPromptAndCollect(ctx, r, opts.SessionID, opts.Prompt)
	if err != nil {
		return nil, fmt.Errorf("session/prompt: %w", err)
	}
	return result, nil
}

func sessionLoadParams(sessionID, workdir string) map[string]any {
	return map[string]any{
		"sessionId":  sessionID,
		"cwd":        workdir,
		"mcpServers": []any{},
	}
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
	return sendSessionNewWithMCP(ctx, r, []any{})
}

func sendSessionNewWithMCP(ctx context.Context, r *Runner, mcpServers []any) (Frame, error) {
	req, _ := r.Framer().NewRequest("session/new", map[string]any{
		"cwd":        r.cfg.Workdir,
		"mcpServers": mcpServers,
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

	respCh, err := r.registerPending(promptID)
	if err != nil {
		return "", err
	}
	// Every exit path drops the waiter, including the ones that give up before
	// a response arrives, so the map does not accumulate dead entries.
	defer func() {
		r.mu.Lock()
		delete(r.pending, promptID)
		r.mu.Unlock()
	}()

	var text strings.Builder

	// drain consumes everything currently queued. Frames that arrived before
	// the prompt was written are answered but never collected: a session/load
	// replay leaves the whole conversation history queued, and folding that
	// into the prompt's text would return the transcript instead of the reply.
	drain := func(collect bool) {
		for {
			frame, ok, _ := r.popOOB()
			if !ok {
				return
			}
			// Agent-initiated request: auto-reply so it doesn't hang.
			if frame.Method() != "" && frame.ID() != nil {
				reply := NewMethodNotFound(frame.ID(), frame.Method())
				_ = r.rec.Sent(reply)
				_ = r.framer.Write(reply)
				continue
			}
			if collect && frame.Method() == methodSessionUpdate {
				if chunk := extractTextChunk(frame); chunk != "" {
					text.WriteString(chunk)
				}
			}
		}
	}

	drain(false)
	if err := r.framer.Write(req); err != nil {
		return "", err
	}

	for {
		drain(true)

		select {
		case resp, ok := <-respCh:
			if !ok {
				// The read loop closed our waiter: the child died or the
				// runner is shutting down. Without this check the nil frame
				// would read as a successful, empty response.
				r.mu.Lock()
				readErr := r.readLoopEr
				r.mu.Unlock()
				if readErr == nil {
					readErr = io.EOF
				}
				return text.String(), fmt.Errorf("read loop exited: %w", readErr)
			}
			// The read loop is serial and the agent emits every chunk before
			// the response, so anything left queued belongs to this prompt.
			drain(true)
			if errMap, ok := resp["error"].(map[string]any); ok {
				return text.String(), fmt.Errorf("%v", errMap["message"])
			}
			return text.String(), nil
		case <-r.oobWake:
		case <-ctx.Done():
			return text.String(), ctx.Err()
		}

		if r.oobFinished() {
			return text.String(), io.EOF
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
	if capabilities, ok := initResult["capabilities"].(map[string]any); ok {
		if mcp, ok := capabilities["mcpCapabilities"].(map[string]any); ok {
			out.MCPHTTP, _ = mcp["http"].(bool)
			out.MCPSSE, _ = mcp["sse"].(bool)
		}
	}
}

func parseSessionResult(newResult map[string]any, out *ProbeResult) {
	out.SessionID, _ = newResult["sessionId"].(string)
	if models, ok := newResult["models"].(map[string]any); ok {
		out.CurrentModelID, _ = models["currentModelId"].(string)
		if avail, ok := models["availableModels"].([]any); ok {
			out.Models = extractStringField(avail, "modelId")
		}
	} else {
		out.CurrentModelID, out.Models = extractSelectConfigOption(newResult, "model")
	}
	if modes, ok := newResult["modes"].(map[string]any); ok {
		out.CurrentModeID, _ = modes["currentModeId"].(string)
		if avail, ok := modes["availableModes"].([]any); ok {
			out.Modes = extractStringField(avail, "id")
		}
	} else {
		out.CurrentModeID, out.Modes = extractSelectConfigOption(newResult, "mode")
	}
}

func extractSelectConfigOption(newResult map[string]any, category string) (string, []string) {
	opts, ok := newResult["configOptions"].([]any)
	if !ok {
		return "", nil
	}
	for _, item := range opts {
		opt, ok := item.(map[string]any)
		if !ok || opt["category"] != category || opt["type"] != "select" {
			continue
		}
		current, _ := opt["currentValue"].(string)
		return current, extractConfigOptionValues(opt["options"])
	}
	return "", nil
}

func extractConfigOptionValues(raw any) []string {
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if v, ok := m["value"].(string); ok {
			out = append(out, v)
			continue
		}
		if nested, ok := m["options"].([]any); ok {
			out = append(out, extractStringField(nested, "value")...)
		}
	}
	return out
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

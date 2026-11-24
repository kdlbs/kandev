package acpdbg

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

func initializeV2Params(cfg RunConfig) map[string]any {
	params := map[string]any{
		"protocolVersion": 2,
		"info":            acpdbgClientInfo(cfg.AgentID, acpcompat.AntigravityRuntimeForCommand(cfg.Command)),
	}
	if len(cfg.ClientCapabilities) > 0 {
		params["capabilities"] = cfg.ClientCapabilities
	}
	return params
}

func validateV2Initialize(result map[string]any) error {
	switch result["protocolVersion"] {
	case float64(1):
		return errors.New("peer selected ACP v1")
	case float64(2):
		if validImplementation(result["info"]) {
			return nil
		}
		if result["info"] == nil && validImplementation(result["agentInfo"]) {
			return errors.New("peer returned a legacy v1 envelope with protocolVersion 2")
		}
		return errors.New("ACP v2 initialize response is missing valid info")
	default:
		return errors.New("initialize response did not select ACP v2")
	}
}

func validImplementation(raw any) bool {
	info, _ := raw.(map[string]any)
	name, _ := info["name"].(string)
	version, _ := info["version"].(string)
	return name != "" && version != ""
}

func parseV2InitResult(result map[string]any, out *ProbeResult) {
	info, _ := result["info"].(map[string]any)
	out.AgentName, _ = info["name"].(string)
	out.AgentVersion, _ = info["version"].(string)
	methods, _ := result["authMethods"].([]any)
	out.AuthMethods = extractStringField(methods, "methodId")
	caps, _ := result["capabilities"].(map[string]any)
	session, _ := caps["session"].(map[string]any)
	mcp, _ := session["mcp"].(map[string]any)
	_, out.MCPHTTP = mcp["http"].(map[string]any)
}

func extractV2ConfigID(result map[string]any, category string) string {
	opts, _ := result["configOptions"].([]any)
	for _, item := range opts {
		opt, _ := item.(map[string]any)
		if opt["category"] == category && opt["type"] == "select" {
			id, _ := opt["configId"].(string)
			return id
		}
	}
	return ""
}

func applyV2Selections(ctx context.Context, r *Runner, probe *ProbeResult, opts PromptOptions) error {
	for _, selection := range []struct{ category, configID, value string }{
		{"model", probe.modelConfigID, opts.Model},
		{"mode", probe.modeConfigID, opts.Mode},
	} {
		if selection.value == "" {
			continue
		}
		if selection.configID == "" {
			return fmt.Errorf("ACP v2 session has no %s selector", selection.category)
		}
		_, err := rpcRequest(ctx, r, "session/set_config_option", map[string]any{
			"sessionId": probe.SessionID, "configId": selection.configID, "type": "id", "value": selection.value,
		})
		if err != nil {
			return fmt.Errorf("%s selection: %w", selection.category, err)
		}
	}
	return nil
}

func v2SessionUpdate(frame Frame, sessionID string) map[string]any {
	params, _ := frame["params"].(map[string]any)
	if frame.Method() != methodSessionUpdate || params["sessionId"] != sessionID {
		return nil
	}
	update, _ := params["update"].(map[string]any)
	return update
}

func awaitV2Replay(ctx context.Context, r *Runner, sessionID string) error {
	err := r.DrainOOBUntil(ctx, func(frame Frame) bool {
		return v2SessionUpdate(frame, sessionID)["sessionUpdate"] == "replay_complete"
	})
	if err == nil {
		_ = r.rec.Meta("replay_complete", nil)
	}
	return err
}

func promptV2(ctx context.Context, r *Runner, sessionID, prompt string) (string, error) {
	if err := drainAvailable(r, nil); err != nil {
		return "", err
	}
	req, id := r.Framer().NewRequest("session/prompt", map[string]any{
		"sessionId": sessionID, "idempotencyKey": rand.Text(),
		"prompt": []any{map[string]any{"type": "text", "text": prompt}},
	})
	responses, err := r.registerPending(id)
	if err != nil {
		return "", err
	}
	defer func() {
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
	}()
	if err := r.rec.Sent(req); err != nil {
		return "", err
	}
	if err := r.framer.Write(req); err != nil {
		return "", err
	}
	return collectV2Turn(ctx, r, sessionID, responses)
}

func collectV2Turn(ctx context.Context, r *Runner, sessionID string, responses <-chan Frame) (string, error) {
	var turn v2Turn
	for {
		if err := drainAvailable(r, func(frame Frame) { turn.update(v2SessionUpdate(frame, sessionID)) }); err != nil {
			return turn.text(), err
		}
		if responses == nil {
			if turn.completed {
				return turn.text(), turn.outcome()
			}
			if r.oobFinished() {
				return turn.text(), errors.New("agent disconnected after accepting the prompt")
			}
		}
		select {
		case response, ok := <-responses:
			if !ok {
				return turn.text(), errors.New("agent disconnected before acknowledging the prompt")
			}
			if err := validateV2PromptReply(response); err != nil {
				return turn.text(), err
			}
			_ = r.rec.Meta("prompt_acknowledged", nil)
			responses = nil
		case <-r.oobWake:
		case <-ctx.Done():
			return turn.text(), ctx.Err()
		}
	}
}

func drainAvailable(r *Runner, collect func(Frame)) error {
	for {
		frame, ok, _ := r.popOOB()
		if !ok {
			return nil
		}
		if isHumanQuestionFrame(r.cfg.AgentID, frame) {
			return ErrHumanQuestionPending
		}
		if frame.Method() != "" && frame.ID() != nil {
			reply := NewMethodNotFound(frame.ID(), frame.Method())
			_ = r.rec.Sent(reply)
			_ = r.framer.Write(reply)
		} else if collect != nil {
			collect(frame)
		}
	}
}

func validateV2PromptReply(resp Frame) error {
	if rpcErr, ok := resp["error"].(map[string]any); ok {
		return fmt.Errorf("%v", rpcErr["message"])
	}
	result, _ := resp["result"].(map[string]any)
	id, _ := result["messageId"].(string)
	if id == "" {
		return errors.New("prompt acknowledgement is missing messageId")
	}
	return nil
}

type v2Turn struct {
	active    bool
	completed bool
	reason    string
	ids       []string
	messages  map[string]string
}

func (turn *v2Turn) update(update map[string]any) {
	if turn.completed {
		return
	}
	switch update["sessionUpdate"] {
	case "state_update":
		switch update["state"] {
		case "running", "requires_action":
			turn.active = true
		case "idle":
			reason, _ := update["stopReason"].(string)
			turn.reason = reason
			turn.completed = turn.active || reason != ""
		}
	case "agent_message_chunk", "agent_message":
		turn.updateMessage(update)
	}
}

func (turn *v2Turn) updateMessage(update map[string]any) {
	id, _ := update["messageId"].(string)
	content, exists := update["content"]
	if id == "" || !exists {
		return
	}
	if turn.messages == nil {
		turn.messages = make(map[string]string)
	}
	if _, seen := turn.messages[id]; !seen {
		turn.ids = append(turn.ids, id)
	}
	if update["sessionUpdate"] == "agent_message_chunk" {
		turn.messages[id] += blockText(content)
		return
	}
	var text strings.Builder
	blocks, _ := content.([]any)
	for _, block := range blocks {
		text.WriteString(blockText(block))
	}
	turn.messages[id] = text.String()
}

func blockText(raw any) string {
	block, _ := raw.(map[string]any)
	if block["type"] != "text" {
		return ""
	}
	text, _ := block["text"].(string)
	return text
}

func (turn *v2Turn) text() string {
	var text strings.Builder
	for _, id := range turn.ids {
		text.WriteString(turn.messages[id])
	}
	return text.String()
}

func (turn *v2Turn) outcome() error {
	switch turn.reason {
	case "end_turn", "refusal", "max_tokens", "max_turn_requests":
		return nil
	case "":
		return errors.New("foreground stopped with an unknown outcome")
	default:
		return fmt.Errorf("foreground stopped: %s", turn.reason)
	}
}

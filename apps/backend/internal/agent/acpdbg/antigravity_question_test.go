package acpdbg

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/acpcompat"
)

const antigravityQuestionFrame = `{"jsonrpc":"2.0","id":77,"method":"session/request_permission","params":{"sessionId":"session-1","toolCall":{"toolCallId":"interaction_q1","title":"Which approach?","kind":"other"},"options":[{"optionId":"option-a","name":"Option A","kind":"allow_once"},{"optionId":"option-b","name":"Option B","kind":"allow_once"}]}}`

// promptQuestionPeer answers initialize/session/new, asks a native question on
// session/prompt, and records whether any response reached it.
const promptQuestionPeer = `
marker="$1"
IFS= read -r init
printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"oauth-personal","name":"Google"}]}}'
IFS= read -r auth
auth_id=$(printf '%s' "$auth" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$auth_id"
IFS= read -r session
session_id=$(printf '%s' "$session" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"session-1"}}\n' "$session_id"
IFS= read -r prompt
printf '%s\n' '` + antigravityQuestionFrame + `'
IFS= read -r reply || exit 0
printf 'answered\n' > "$marker"
prompt_id=$(printf '%s' "$prompt" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}\n' "$prompt_id"
`

// probeQuestionPeer asks the native question immediately after session/new so
// the NextOOB path observes it without a prompt.

// promptQuestionPeerWithoutAuth keeps the pre-auth handshake for another agent.
const promptQuestionPeerWithoutAuth = `
marker="$1"
IFS= read -r init
printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1}}'
IFS= read -r session
session_id=$(printf '%s' "$session" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"session-1"}}\n' "$session_id"
IFS= read -r prompt
printf '%s\n' '` + antigravityQuestionFrame + `'
IFS= read -r reply || exit 0
printf 'answered\n' > "$marker"
prompt_id=$(printf '%s' "$prompt" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"stopReason":"end_turn"}}\n' "$prompt_id"
`

const probeQuestionPeer = `
marker="$1"
IFS= read -r init
printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"protocolVersion":1,"authMethods":[{"id":"oauth-personal","name":"Google"}]}}'
IFS= read -r auth
auth_id=$(printf '%s' "$auth" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{}}\n' "$auth_id"
IFS= read -r session
session_id=$(printf '%s' "$session" | sed -n 's/.*"id":\([0-9]*\).*/\1/p')
printf '{"jsonrpc":"2.0","id":%s,"result":{"sessionId":"session-1"}}\n' "$session_id"
printf '%s\n' '` + antigravityQuestionFrame + `'
IFS= read -r reply || exit 0
printf 'answered\n' > "$marker"
`

func newScriptedQuestionRunner(t *testing.T, agentID, script string) (*Runner, string) {
	t.Helper()
	dir := t.TempDir()
	marker := filepath.Join(dir, "answered.marker")
	runner, err := NewRunner(context.Background(), filepath.Join(dir, "frames.jsonl"), RunConfig{
		AgentID: agentID,
		Command: []string{"sh", "-c", script, "acpdbg-test", marker},
	})
	if err != nil {
		t.Fatalf("NewRunner: %v", err)
	}
	return runner, marker
}

func sentFrames(t *testing.T, path string) []Frame {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read capture: %v", err)
	}
	var frames []Frame
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		if line == "" {
			continue
		}
		var entry Entry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("decode capture line: %v", err)
		}
		if entry.Direction == DirSent {
			frames = append(frames, entry.Frame)
		}
	}
	return frames
}

func findFrameWithID(frames []Frame, id float64) Frame {
	for _, frame := range frames {
		if got, ok := frame["id"].(float64); ok && got == id {
			return frame
		}
	}
	return nil
}

func assertNoAnswerWritten(t *testing.T, path, marker string) {
	t.Helper()
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("debugger answered the native question")
	} else if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stat answer marker: %v", err)
	}
	if frame := findFrameWithID(sentFrames(t, path), 77); frame != nil {
		t.Fatalf("debugger wrote a response to the native question: %+v", frame)
	}
}

// TestAntigravityDebuggerHumanQuestion pins that a registered-agent capture
// stops at a native Antigravity question without writing any response. A
// diagnostic must never invent the user's answer, while every other agent
// keeps the existing method-not-found auto-reply.
func TestAntigravityDebuggerHumanQuestion(t *testing.T) {
	t.Run("prompt drain stops without a response", func(t *testing.T) {
		runner, marker := newScriptedQuestionRunner(t, acpcompat.AntigravityAgentID, promptQuestionPeer)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := Prompt(ctx, runner, PromptOptions{Prompt: "Which approach?"})
		runner.Close("test complete")
		if !errors.Is(err, ErrHumanQuestionPending) {
			t.Fatalf("Prompt error = %v, want ErrHumanQuestionPending", err)
		}
		assertNoAnswerWritten(t, runner.Path(), marker)
	})

	t.Run("next out-of-band frame stops without a response", func(t *testing.T) {
		runner, marker := newScriptedQuestionRunner(t, acpcompat.AntigravityAgentID, probeQuestionPeer)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if _, err := Probe(ctx, runner); err != nil {
			runner.Close("test complete")
			t.Fatalf("Probe: %v", err)
		}
		err := runner.DrainOOBUntil(ctx, func(Frame) bool { return false })
		runner.Close("test complete")
		if !errors.Is(err, ErrHumanQuestionPending) {
			t.Fatalf("DrainOOBUntil error = %v, want ErrHumanQuestionPending", err)
		}
		assertNoAnswerWritten(t, runner.Path(), marker)
	})

	t.Run("other agent keeps the diagnostic auto-reply", func(t *testing.T) {
		runner, marker := newScriptedQuestionRunner(t, "exec-other-agent", promptQuestionPeerWithoutAuth)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		_, err := Prompt(ctx, runner, PromptOptions{Prompt: "Which approach?"})
		runner.Close("test complete")
		if err != nil {
			t.Fatalf("Prompt: %v", err)
		}
		if _, statErr := os.Stat(marker); statErr != nil {
			t.Fatalf("other agent did not receive the diagnostic reply: %v", statErr)
		}
		frame := findFrameWithID(sentFrames(t, runner.Path()), 77)
		if frame == nil {
			t.Fatal("other agent's permission request received no auto-reply")
		}
		errMap, ok := frame["error"].(map[string]any)
		if !ok {
			t.Fatalf("auto-reply = %+v, want a method-not-found error", frame)
		}
		if code, _ := errMap["code"].(float64); code != -32601 {
			t.Fatalf("auto-reply code = %v, want -32601", errMap["code"])
		}
	})
}

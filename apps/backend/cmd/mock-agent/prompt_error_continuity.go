package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// promptErrorCmdRe matches `/prompt-error` and its deterministic variants. Each
// variant is a plain JSON-RPC error response: no capacity marker, no transient
// keyword, and no disconnect-shaped Data. The runtime must therefore keep the
// conversation usable purely from response provenance.
//
// `/prompt-error` (or `-storage`)  Internal error: Failed to execute statement
// `/prompt-error-auth`            Authentication required
// `/prompt-error-quota`           insufficient_quota
// `/prompt-error-model`           invalid model id
// `/prompt-error-empty`           empty diagnostic message
// An optional `:N` suffix fails N consecutive prompts (default 1).
var promptErrorCmdRe = regexp.MustCompile(
	`(?i)^/(?:e2e:)?prompt-error(?:-(internal|storage|auth|quota|model|empty))?(?::(\d+))?$`,
)

// parsePromptErrorCmd reports whether the prompt is a /prompt-error scenario
// and the number of consecutive prompts that should fail.
func parsePromptErrorCmd(prompt string) (kind string, failTimes int, ok bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	m := promptErrorCmdRe.FindStringSubmatch(cmd)
	if m == nil {
		return "", 0, false
	}
	kind = strings.ToLower(m[1])
	if kind == "" || kind == "internal" {
		kind = "storage"
	}
	failTimes = 1
	if m[2] != "" {
		if n, err := strconv.Atoi(m[2]); err == nil && n >= 0 {
			failTimes = n
		}
	}
	return kind, failTimes, true
}

func promptErrorRequest(kind string) *acp.RequestError {
	switch kind {
	case "auth":
		return &acp.RequestError{Code: -32000, Message: "Authentication required"}
	case "quota":
		return &acp.RequestError{Code: -32603, Message: "insufficient_quota"}
	case "model":
		return &acp.RequestError{Code: -32603, Message: "invalid model id"}
	case "empty":
		return &acp.RequestError{Code: -32603, Message: ""}
	default:
		return &acp.RequestError{
			Code:    -32603,
			Message: "Internal error: Failed to execute statement",
			Data:    map[string]any{"details": "SQLiteError: database is locked"},
		}
	}
}

// handlePromptErrorContinuity returns a real prompt-time ACP error for the
// first N /prompt-error prompts of a session, then recovers with normal output.
func (a *mockAgent) handlePromptErrorContinuity(
	ctx context.Context,
	sid acp.SessionId,
	prompt string,
) (acp.PromptResponse, error, bool) {
	kind, failTimes, ok := parsePromptErrorCmd(prompt)
	if !ok {
		return acp.PromptResponse{}, nil, false
	}
	attempt := nextPromptErrorAttempt(sid)
	if attempt <= failTimes {
		_, _ = fmt.Fprintf(logOutput, "mock-agent[%d]: emitting %s prompt error for session %s (failure %d/%d)\n",
			os.Getpid(), kind, sid, attempt, failTimes)
		return acp.PromptResponse{}, promptErrorRequest(kind), true
	}
	_ = os.Remove(promptErrorCounterPath(sid))
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	e.text(fmt.Sprintf("Agent recovered after a %s prompt error. Here is your response.", kind))
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

func promptErrorCounterPath(sid acp.SessionId) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(string(sid))
	return filepath.Join(os.TempDir(), "kandev-mock-prompt-error-"+safe+".count")
}

func nextPromptErrorAttempt(sid acp.SessionId) int {
	path := promptErrorCounterPath(sid)
	prev := 0
	if raw, err := os.ReadFile(path); err == nil {
		if n, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil {
			prev = n
		}
	}
	next := prev + 1
	_ = os.WriteFile(path, []byte(strconv.Itoa(next)), 0o600)
	return next
}

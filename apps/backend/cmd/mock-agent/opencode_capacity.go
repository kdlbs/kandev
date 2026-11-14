package main

import (
	"context"
	"encoding/json"
	"os"
	"regexp"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

// openCodeCapacityMessage is the exact terminal diagnostic captured from
// OpenCode 2.2.0 after its own admission retries were exhausted. The
// classifier's bounded signature and the host's retry-hint floor both read
// this text, so the fixture must not simplify it.
const openCodeCapacityMessage = "Internal error: Upstream request failed: [ttft_admission_refused] the cell is at capacity; retry shortly (after 21 s)"

const (
	openCodeCapacityScenarioSuffix = "opencode"
	openCodeCapacityCompletedTools = "completed-tools"
	openCodeCapacityAfterTools     = "after-tools"
)

var openCodeCapacityCmdRe = regexp.MustCompile(`(?i)^/(?:e2e:)?opencode-capacity-(after-tools|completed-tools|retry|cancel|exhaust)(?::(\d+))?$`)

type openCodeCapacityScenario struct {
	name      string
	failTimes int
}

type openCodeCapacityScenarioState struct {
	Name      string `json:"name"`
	FailTimes int    `json:"fail_times"`
}

func parseOpenCodeCapacityCmd(prompt string) (openCodeCapacityScenario, bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	match := openCodeCapacityCmdRe.FindStringSubmatch(cmd)
	if match == nil {
		return openCodeCapacityScenario{}, false
	}
	scenario := openCodeCapacityScenario{name: strings.ToLower(match[1]), failTimes: 1}
	switch scenario.name {
	case "cancel", "exhaust":
		scenario.failTimes = 9
	}
	if match[2] != "" {
		if count, err := strconv.Atoi(match[2]); err == nil && count >= 0 {
			scenario.failTimes = count
		}
	}
	return scenario, true
}

// The scenario state file keeps the "-" suffix so the existing per-session
// cleanup glob removes it with the counters.
func openCodeCapacityStatePath(sid acp.SessionId) string {
	return retainedCapacityCounterPrefix(sid) + "-" + openCodeCapacityScenarioSuffix + "-scenario.json"
}

func openCodeCapacityCounterPath(sid acp.SessionId, scenario string) string {
	return retainedCapacityCounterPrefix(sid) + "-" + openCodeCapacityScenarioSuffix + "-" + scenario + ".count"
}

func saveOpenCodeCapacityScenario(sid acp.SessionId, scenario openCodeCapacityScenario) {
	data, err := json.Marshal(openCodeCapacityScenarioState{Name: scenario.name, FailTimes: scenario.failTimes})
	if err != nil {
		return
	}
	_ = os.WriteFile(openCodeCapacityStatePath(sid), data, 0600)
}

func loadOpenCodeCapacityScenario(sid acp.SessionId) (openCodeCapacityScenario, bool) {
	data, err := os.ReadFile(openCodeCapacityStatePath(sid))
	if err != nil {
		return openCodeCapacityScenario{}, false
	}
	var state openCodeCapacityScenarioState
	if err := json.Unmarshal(data, &state); err != nil || state.Name == "" {
		return openCodeCapacityScenario{}, false
	}
	return openCodeCapacityScenario{name: state.Name, failTimes: state.FailTimes}, true
}

// emitOpenCodeNativeRetry reproduces the advisory `_meta["opencode/retry"]`
// update and its clearing null update. OpenCode emits these while its own
// prompt is still active; neither frame is a terminal settlement.
func emitOpenCodeNativeRetry(e *emitter, attempt int) {
	e.sessionInfo(map[string]any{
		"opencode/retry": map[string]any{
			"attempt": attempt,
			"error": map[string]any{
				"message":  openCodeCapacityMessage,
				"status":   429,
				"type":     "provider.rate-limit",
				"response": map[string]any{"body": `{"error":{"type":"server_error","message":"` + openCodeCapacityMessage + `"}}`},
			},
			"nextRetryAt": "2026-10-28T22:26:22.452Z",
		},
	})
	e.sessionInfo(map[string]any{"opencode/retry": nil})
}

func (a *mockAgent) handleOpenCodeCapacity(ctx context.Context, sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	scenario, ok := parseOpenCodeCapacityCmd(prompt)
	continuation := strings.Contains(strings.ToLower(prompt), retainedCapacityContinuationMarker)
	if !ok && continuation {
		scenario, ok = loadOpenCodeCapacityScenario(sid)
	}
	if !ok {
		return acp.PromptResponse{}, nil, false
	}
	if !continuation {
		previous, exists := loadOpenCodeCapacityScenario(sid)
		if !exists || previous != scenario {
			_ = os.Remove(openCodeCapacityCounterPath(sid, scenario.name))
		}
		saveOpenCodeCapacityScenario(sid, scenario)
	}
	attempt := nextRetainedCapacityAttempt(sid, openCodeCapacityScenarioSuffix+"-"+scenario.name)
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	emitOpenCodeNativeRetry(e, attempt)
	if attempt <= scenario.failTimes {
		if attempt == 1 && scenario.name == openCodeCapacityCompletedTools {
			e.text("Updated fixture.txt before the provider admission refusal.\n")
			e.startTool("opencode-capacity-edit", "Write fixture.txt", acp.ToolKindEdit, map[string]any{"path": "fixture.txt", "content": "updated"})
			e.completeTool("opencode-capacity-edit", "fixture.txt updated.")
			traceACP("completed_side_effect", string(sid), map[string]string{"scenario": "opencode-completed-tools", "effect": "fixture.txt"})
		}
		if attempt == 1 && scenario.name == openCodeCapacityAfterTools {
			e.text("Reviewed fixture.txt before the provider admission refusal.\n")
			e.startTool("opencode-capacity-read", "Read fixture.txt", acp.ToolKindRead, map[string]any{"path": "fixture.txt"})
			e.completeTool("opencode-capacity-read", "Fixture contents read.")
			e.startTool("opencode-capacity-pending", "Update related fixture", acp.ToolKindExecute, map[string]any{"path": "related.txt"})
		}
		return acp.PromptResponse{}, &acp.RequestError{Code: -32603, Message: openCodeCapacityMessage}, true
	}
	_ = os.Remove(openCodeCapacityCounterPath(sid, scenario.name))
	_ = os.Remove(openCodeCapacityStatePath(sid))
	if continuation {
		e.text("OpenCode provider continued the unfinished request without repeating completed work.")
	} else {
		e.text("OpenCode provider recovered after the admission refusal.")
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

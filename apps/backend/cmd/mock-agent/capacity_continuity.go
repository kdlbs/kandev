package main

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const retainedCapacityMessage = "Selected model is at capacity. Please try a different model."

var retainedCapacityCmdRe = regexp.MustCompile(`(?i)^/(?:e2e:)?capacity-(after-tools|completed-tools|known-failed-command|retry|cancel|exhaust)(?::(\d+))?$`)

const retainedCapacityContinuationMarker = "previous turn stopped because the selected model was at capacity"

type retainedCapacityScenario struct {
	name      string
	failTimes int
	withTools bool
}

type retainedCapacityScenarioState struct {
	Name      string `json:"name"`
	FailTimes int    `json:"fail_times"`
	WithTools bool   `json:"with_tools"`
}

func parseRetainedCapacityCmd(prompt string) (retainedCapacityScenario, bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	match := retainedCapacityCmdRe.FindStringSubmatch(cmd)
	if match == nil {
		return retainedCapacityScenario{}, false
	}
	scenario := retainedCapacityScenario{name: strings.ToLower(match[1]), failTimes: 1}
	switch scenario.name {
	case "after-tools", "completed-tools", "known-failed-command":
		scenario.withTools = true
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

func retainedCapacityCounterPrefix(sid acp.SessionId) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("kandev-mock-retained-capacity-%x", sha256.Sum256([]byte(sid))))
}

func retainedCapacityCounterPath(sid acp.SessionId, scenario string) string {
	return retainedCapacityCounterPrefix(sid) + "-" + scenario + ".count"
}

func retainedCapacityScenarioPath(sid acp.SessionId) string {
	return retainedCapacityCounterPrefix(sid) + ".scenario"
}

func clearRetainedCapacityCounters(sid acp.SessionId) {
	paths, _ := filepath.Glob(retainedCapacityCounterPrefix(sid) + "-*")
	for _, path := range paths {
		_ = os.Remove(path)
	}
	_ = os.Remove(retainedCapacityScenarioPath(sid))
}

func nextRetainedCapacityAttempt(sid acp.SessionId, scenario string) int {
	path := retainedCapacityCounterPath(sid, scenario)
	previous := 0
	if raw, err := os.ReadFile(path); err == nil {
		previous, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}
	next := previous + 1
	_ = os.WriteFile(path, []byte(strconv.Itoa(next)), 0600)
	return next
}

func saveRetainedCapacityScenario(sid acp.SessionId, scenario retainedCapacityScenario) {
	data, err := json.Marshal(retainedCapacityScenarioState{
		Name: scenario.name, FailTimes: scenario.failTimes, WithTools: scenario.withTools,
	})
	if err != nil {
		return
	}
	_ = os.WriteFile(retainedCapacityScenarioPath(sid), data, 0600)
}

func loadRetainedCapacityScenario(sid acp.SessionId) (retainedCapacityScenario, bool) {
	data, err := os.ReadFile(retainedCapacityScenarioPath(sid))
	if err != nil {
		return retainedCapacityScenario{}, false
	}
	var state retainedCapacityScenarioState
	if err := json.Unmarshal(data, &state); err != nil || state.Name == "" {
		return retainedCapacityScenario{}, false
	}
	return retainedCapacityScenario{name: state.Name, failTimes: state.FailTimes, withTools: state.WithTools}, true
}

func (a *mockAgent) handleRetainedCapacity(ctx context.Context, sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	scenario, ok := parseRetainedCapacityCmd(prompt)
	continuation := strings.Contains(strings.ToLower(prompt), retainedCapacityContinuationMarker)
	if !ok && continuation {
		scenario, ok = loadRetainedCapacityScenario(sid)
	}
	if !ok {
		return acp.PromptResponse{}, nil, false
	}
	if !continuation {
		previous, exists := loadRetainedCapacityScenario(sid)
		if !exists || previous != scenario {
			_ = os.Remove(retainedCapacityCounterPath(sid, scenario.name))
		}
		saveRetainedCapacityScenario(sid, scenario)
	}
	attempt := nextRetainedCapacityAttempt(sid, scenario.name)
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	if attempt <= scenario.failTimes {
		if scenario.withTools && attempt == 1 && scenario.name == "after-tools" {
			e.text("Reviewed fixture.txt before the provider reported capacity.\n")
			e.startTool("capacity-read", "Read fixture.txt", acp.ToolKindRead, map[string]any{"path": "fixture.txt"})
			e.completeTool("capacity-read", "Fixture contents read.")
			e.startTool("capacity-pending", "Update related fixture", acp.ToolKindExecute, map[string]any{"path": "related.txt"})
		}
		if scenario.withTools && attempt == 1 && scenario.name == "completed-tools" {
			e.text("Updated fixture.txt before the provider reported capacity.\n")
			e.startTool("capacity-edit", "Write fixture.txt", acp.ToolKindEdit, map[string]any{"path": "fixture.txt", "content": "updated"})
			e.completeTool("capacity-edit", "fixture.txt updated.")
			traceACP("completed_side_effect", string(sid), map[string]string{"scenario": scenario.name, "effect": "fixture.txt"})
		}
		if scenario.withTools && attempt == 1 && scenario.name == "known-failed-command" {
			emitKnownFailedCommandCapacityFrames(e, sid)
		}
		return acp.PromptResponse{}, &acp.RequestError{
			Code:    -32603,
			Message: retainedCapacityMessage,
			Data: map[string]any{
				"kandevMock": map[string]any{"retainedProviderCapacity": true},
			},
		}, true
	}
	_ = os.Remove(retainedCapacityCounterPath(sid, scenario.name))
	_ = os.Remove(retainedCapacityScenarioPath(sid))
	if continuation {
		e.text("Mock provider continued the unfinished request without repeating completed work.")
	} else {
		e.text(fmt.Sprintf("Mock provider recovered after %d retained capacity error(s).", scenario.failTimes))
	}
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

// emitKnownFailedCommandCapacityFrames models the measured Codex shape: one
// successful read and one command that ended with a matching terminal identity,
// integer nonzero exit, and no signal. The command stays failed in history.
func emitKnownFailedCommandCapacityFrames(e *emitter, sid acp.SessionId) {
	const commandID acp.ToolCallId = "capacity-known-command"
	const readID acp.ToolCallId = "capacity-known-read"
	e.text("Ran the focused fixture read and command before the provider reported capacity.\n")
	e.startTool(readID, "Read fixture.txt", acp.ToolKindRead, map[string]any{"path": "fixture.txt"})
	e.completeTool(readID, "Fixture contents read.")
	e.startTool(commandID, "Run focused fixture command", acp.ToolKindExecute, map[string]any{"command": "go test ./focused"})
	e.completeToolWithMeta(commandID, acp.ToolCallStatusFailed,
		map[string]any{
			"exit_code": 1, "stdout": "FAIL focused package\n",
			"stderr": "", "formatted_output": "FAIL focused package\n",
		},
		map[string]any{
			"terminal_output": map[string]any{"data": "FAIL focused package\n", "terminal_id": string(commandID)},
			"terminal_exit": map[string]any{
				"exit_code": 1, "signal": nil, "terminal_id": string(commandID),
			},
		},
	)
	traceACP("known_command_failure", string(sid), map[string]string{
		"scenario": "known-failed-command", "exit_code": "1",
	})
}

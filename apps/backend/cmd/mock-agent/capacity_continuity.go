package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	acp "github.com/coder/acp-go-sdk"
)

const retainedCapacityMessage = "Selected model is at capacity. Please try a different model."

var retainedCapacityCmdRe = regexp.MustCompile(`(?i)^/(?:e2e:)?capacity-(after-tools|retry|cancel|exhaust)(?::(\d+))?$`)

type retainedCapacityScenario struct {
	name      string
	failTimes int
	withTools bool
}

func parseRetainedCapacityCmd(prompt string) (retainedCapacityScenario, bool) {
	cmd := stripKandevSystem(strings.TrimSpace(prompt))
	match := retainedCapacityCmdRe.FindStringSubmatch(cmd)
	if match == nil {
		return retainedCapacityScenario{}, false
	}
	scenario := retainedCapacityScenario{name: strings.ToLower(match[1]), failTimes: 1}
	switch scenario.name {
	case "after-tools":
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

func retainedCapacityCounterPath(sid acp.SessionId) string {
	return filepath.Join(os.TempDir(), fmt.Sprintf("kandev-mock-retained-capacity-%x.count", sha256.Sum256([]byte(sid))))
}

func nextRetainedCapacityAttempt(sid acp.SessionId) int {
	path := retainedCapacityCounterPath(sid)
	previous := 0
	if raw, err := os.ReadFile(path); err == nil {
		previous, _ = strconv.Atoi(strings.TrimSpace(string(raw)))
	}
	next := previous + 1
	_ = os.WriteFile(path, []byte(strconv.Itoa(next)), 0600)
	return next
}

func (a *mockAgent) handleRetainedCapacity(ctx context.Context, sid acp.SessionId, prompt string) (acp.PromptResponse, error, bool) {
	scenario, ok := parseRetainedCapacityCmd(prompt)
	if !ok {
		return acp.PromptResponse{}, nil, false
	}
	attempt := nextRetainedCapacityAttempt(sid)
	e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
	if attempt <= scenario.failTimes {
		if scenario.withTools && attempt == 1 {
			e.text("Reviewed fixture.txt before the provider reported capacity.\n")
			e.startTool("capacity-read", "Read fixture.txt", acp.ToolKindRead, map[string]any{"path": "fixture.txt"})
			e.completeTool("capacity-read", "Fixture contents read.")
		}
		return acp.PromptResponse{}, &acp.RequestError{
			Code:    -32603,
			Message: retainedCapacityMessage,
			Data: map[string]any{
				"kandevMock": map[string]any{"retainedProviderCapacity": true},
			},
		}, true
	}
	_ = os.Remove(retainedCapacityCounterPath(sid))
	e.text(fmt.Sprintf("Mock provider recovered after %d retained capacity error(s).", scenario.failTimes))
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

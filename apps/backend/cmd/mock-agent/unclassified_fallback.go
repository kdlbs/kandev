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

const (
	dynamicUnclassifiedFallbackStableDiagnostic    = "Mock unclassified terminal failure: stable diagnostic"
	dynamicUnclassifiedFallbackAlternateDiagnostic = "Mock unclassified terminal failure: alternate diagnostic"
	dynamicUnclassifiedFallbackSuccess             = "Dynamic fallback successor response."
)

var dynamicUnclassifiedFallbackCommand = regexp.MustCompile(
	`(?i)/(?:e2e:)?dynamic-unclassified:(same|mixed|output|tool)(?::(\d+))?`,
)

type dynamicUnclassifiedFallbackScenario struct {
	variant  string
	failures int
}

func parseDynamicUnclassifiedFallbackScenario(prompt string) (dynamicUnclassifiedFallbackScenario, bool) {
	command := stripKandevSystem(strings.TrimSpace(prompt))
	matches := dynamicUnclassifiedFallbackCommand.FindStringSubmatch(command)
	if matches == nil {
		return dynamicUnclassifiedFallbackScenario{}, false
	}

	failures := 3
	if matches[2] != "" {
		parsed, err := strconv.Atoi(matches[2])
		if err != nil || parsed < 0 {
			return dynamicUnclassifiedFallbackScenario{}, false
		}
		failures = parsed
	}
	return dynamicUnclassifiedFallbackScenario{variant: strings.ToLower(matches[1]), failures: failures}, true
}

func dynamicUnclassifiedFallbackDiagnostic(variant string, attempt int) string {
	if variant == "mixed" && attempt == 2 {
		return dynamicUnclassifiedFallbackAlternateDiagnostic
	}
	return dynamicUnclassifiedFallbackStableDiagnostic
}

func (a *mockAgent) handleDynamicUnclassifiedFallback(
	ctx context.Context,
	sid acp.SessionId,
	prompt string,
) (acp.PromptResponse, error, bool) {
	scenario, ok := parseDynamicUnclassifiedFallbackScenario(prompt)
	if !ok {
		return acp.PromptResponse{}, nil, false
	}

	a.mu.Lock()
	if a.dynamicFallbackCounterSessions == nil {
		a.dynamicFallbackCounterSessions = make(map[acp.SessionId]acp.SessionId)
	}
	counterSessionID := a.dynamicFallbackCounterSessions[sid]
	if counterSessionID == "" {
		if raw, err := os.ReadFile(dynamicUnclassifiedFallbackBindingPath(sid)); err == nil {
			counterSessionID = acp.SessionId(strings.TrimSpace(string(raw)))
		}
	}
	if counterSessionID == "" {
		counterSessionID = acp.SessionId(extractRegexMatch(sessionIDRegex, prompt))
		if counterSessionID == "" {
			counterSessionID = sid
		}
	}
	a.dynamicFallbackCounterSessions[sid] = counterSessionID
	a.mu.Unlock()
	_ = os.WriteFile(dynamicUnclassifiedFallbackBindingPath(sid), []byte(counterSessionID), 0o600)
	attempt := nextDynamicUnclassifiedFallbackAttempt(counterSessionID)
	if attempt <= scenario.failures {
		e := &emitter{ctx: ctx, conn: a.conn, sid: sid}
		switch scenario.variant {
		case "output":
			e.text("Partial output before a terminal provider failure.")
		case "tool":
			e.startTool(acp.ToolCallId(fmt.Sprintf("unclassified-effect-%d", attempt)), "Run a mock effect", acp.ToolKindExecute, map[string]any{
				"command": "mock-effect",
			})
			e.completeTool(acp.ToolCallId(fmt.Sprintf("unclassified-effect-%d", attempt)), map[string]any{
				"result": "effect observed",
			})
		}
		return acp.PromptResponse{}, &acp.RequestError{
			Code:    -32603,
			Message: dynamicUnclassifiedFallbackDiagnostic(scenario.variant, attempt),
			Data:    map[string]any{"errorKind": "server_error"},
		}, true
	}

	_ = os.Remove(dynamicUnclassifiedFallbackCounterPath(counterSessionID))
	_ = os.Remove(dynamicUnclassifiedFallbackBindingPath(sid))
	a.mu.Lock()
	delete(a.dynamicFallbackCounterSessions, sid)
	a.mu.Unlock()
	(&emitter{ctx: ctx, conn: a.conn, sid: sid}).text(dynamicUnclassifiedFallbackSuccess)
	return acp.PromptResponse{StopReason: acp.StopReasonEndTurn}, nil, true
}

func dynamicUnclassifiedFallbackBindingPath(sid acp.SessionId) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(string(sid))
	return filepath.Join(os.TempDir(), "kandev-mock-dynamic-unclassified-"+safe+".session")
}

func dynamicUnclassifiedFallbackCounterPath(sid acp.SessionId) string {
	safe := strings.NewReplacer("/", "_", "\\", "_", "..", "_").Replace(string(sid))
	return filepath.Join(os.TempDir(), "kandev-mock-dynamic-unclassified-"+safe+".count")
}

func nextDynamicUnclassifiedFallbackAttempt(sid acp.SessionId) int {
	path := dynamicUnclassifiedFallbackCounterPath(sid)
	previous := 0
	if raw, err := os.ReadFile(path); err == nil {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(string(raw))); parseErr == nil {
			previous = parsed
		}
	}
	next := previous + 1
	_ = os.WriteFile(path, []byte(strconv.Itoa(next)), 0o600)
	return next
}

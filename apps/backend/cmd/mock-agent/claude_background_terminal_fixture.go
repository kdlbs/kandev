package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// mockClaudeBridgeVersionEnv makes the mock advertise the exact official
// bridge identity inspected for null-code terminal-exit evidence. It is set
// only by the scenario that replays those captured frames.
const mockClaudeBridgeVersionEnv = "E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION"

// mockClaudeTerminalSignalEnv gates live output and terminal frames on file
// contents written by the browser test. Unset, the fixture falls back to its
// delay for manual use.
const mockClaudeTerminalSignalEnv = "E2E_MOCK_AGENT_CLAUDE_TERMINAL_SIGNAL_FILE"

const (
	mockClaudeBridgeName           = "@agentclientprotocol/claude-agent-acp"
	mockClaudeBridgeLegacyVersion  = "0.97.0"
	mockClaudeBridgeMonitorVersion = "0.107.0"
	mockClaudeBridgeVersion        = "0.109.0"
)

func mockClaudeBridgeAgentInfo() *acp.Implementation {
	version := strings.TrimSpace(os.Getenv(mockClaudeBridgeVersionEnv))
	if version != mockClaudeBridgeVersion && version != mockClaudeBridgeMonitorVersion && version != mockClaudeBridgeLegacyVersion {
		return nil
	}
	return &acp.Implementation{
		Name:    mockClaudeBridgeName,
		Version: version,
	}
}

type claudeBackgroundTerminalJob struct {
	toolCallID string
	command    string
	title      string
	outputPath string
	initialOut string
	liveOut    string
	finalOut   string
}

// emitClaudeBackgroundTerminalFixture replays the qualified Claude
// detached-shell lifecycle: two launches complete their Bash tool calls while
// the jobs keep running, then each job's own terminal frame reports a null
// exit. The foreground prompt returns before either terminal frame.
func emitClaudeBackgroundTerminalFixture(e *emitter, cmd string) {
	firstDelay, gap := 8*time.Second, 8*time.Second
	parts := strings.Fields(cmd)
	if len(parts) >= 2 {
		firstDelay = parseDurationArg(parts[1], firstDelay)
	}
	if len(parts) >= 3 {
		gap = parseDurationArg(parts[2], gap)
	}

	jobs := []claudeBackgroundTerminalJob{
		{
			toolCallID: string(nextToolID()),
			command:    "go test ./...",
			title:      "Run the full test suite",
			initialOut: "Mock test suite started\n",
			liveOut:    "Mock test suite is still running\n",
			finalOut:   "Mock test suite finished\n",
		},
		{
			toolCallID: string(nextToolID()),
			command:    "gh pr checks --watch",
			title:      "Watch pull request checks",
			initialOut: "Mock PR check watch started\n",
			liveOut:    "Mock PR checks are still running\n",
			finalOut:   "Mock PR check watch finished\n",
		},
	}
	for index := range jobs {
		jobs[index].outputPath = mockClaudeBackgroundShellOutputPath(string(e.sid), jobs[index].toolCallID)
		_ = os.MkdirAll(filepath.Dir(jobs[index].outputPath), 0o700)
		_ = os.WriteFile(jobs[index].outputPath, []byte(jobs[index].initialOut), 0o600)
	}
	for _, job := range jobs {
		e.startClaudeBackgroundShell(job)
		e.completeClaudeBackgroundShell(job)
	}
	e.text("Two background commands are running.")

	backgroundCtx, cancel := context.WithCancel(context.Background())
	release := func() {}
	if e.registerDetachedWork != nil {
		release = e.registerDetachedWork(e.sid, cancel)
	}
	signalFile := strings.TrimSpace(os.Getenv(mockClaudeTerminalSignalEnv))
	backgroundEmitter := &emitter{ctx: context.Background(), conn: e.conn, sid: e.sid}
	go func() {
		defer release()
		defer cancel()
		if signalFile != "" {
			if !waitClaudeTerminalSignal(backgroundCtx, signalFile, "append-live-output", "release-terminal-frames") {
				return
			}
		} else if !waitClaudeTerminalFixture(backgroundCtx, firstDelay) {
			return
		}
		appendClaudeBackgroundShellOutput(jobs[0], jobs[0].liveOut)
		if signalFile != "" && !waitClaudeTerminalSignal(backgroundCtx, signalFile, "release-terminal-frames") {
			return
		}
		appendClaudeBackgroundShellOutput(jobs[0], jobs[0].finalOut)
		backgroundEmitter.emitClaudeBackgroundTerminalExit(jobs[0].toolCallID)
		if !waitClaudeTerminalFixture(backgroundCtx, gap) {
			return
		}
		appendClaudeBackgroundShellOutput(jobs[1], jobs[1].liveOut)
		appendClaudeBackgroundShellOutput(jobs[1], jobs[1].finalOut)
		backgroundEmitter.emitClaudeBackgroundTerminalExit(jobs[1].toolCallID)
	}()
}

func mockClaudeBackgroundShellOutputPath(sessionID, workID string) string {
	return filepath.Join(os.TempDir(), "claude-mock-agent", sessionID, "tasks", workID+".output")
}

func appendClaudeBackgroundShellOutput(job claudeBackgroundTerminalJob, output string) {
	file, err := os.OpenFile(job.outputPath, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	_, _ = file.WriteString(output)
	_ = file.Close()
}

func waitClaudeTerminalSignal(ctx context.Context, path string, expectedValues ...string) bool {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if value, err := os.ReadFile(path); err == nil {
			current := strings.TrimSpace(string(value))
			for _, expected := range expectedValues {
				if current == expected {
					return true
				}
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return false
		}
	}
}

func waitClaudeTerminalFixture(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func withClaudeStartMeta(meta map[string]any) acp.ToolCallStartOpt {
	return func(tc *acp.SessionUpdateToolCall) { tc.Meta = meta }
}

func withClaudeUpdateMeta(meta map[string]any) acp.ToolCallUpdateOpt {
	return func(tu *acp.SessionToolCallUpdate) { tu.Meta = meta }
}

func (e *emitter) startClaudeBackgroundShell(job claudeBackgroundTerminalJob) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.StartToolCall(acp.ToolCallId(job.toolCallID), job.title,
			acp.WithStartKind(acp.ToolKindExecute),
			acp.WithStartStatus(acp.ToolCallStatusPending),
			acp.WithStartRawInput(map[string]any{
				"command":           job.command,
				"description":       job.title,
				"run_in_background": true,
			}),
			withClaudeStartMeta(map[string]any{
				"claudeCode": map[string]any{"toolName": "Bash"},
				"terminal_info": map[string]any{
					"terminal_id": job.toolCallID,
				},
			}),
		),
	})
}

func (e *emitter) completeClaudeBackgroundShell(job claudeBackgroundTerminalJob) {
	banner := "Command running in background with ID: " + job.toolCallID +
		". Output is being written to: " + job.outputPath
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.UpdateToolCall(acp.ToolCallId(job.toolCallID),
			acp.WithUpdateStatus(acp.ToolCallStatusCompleted),
			acp.WithUpdateTitle(job.title),
			acp.WithUpdateRawOutput(map[string]any{"output": banner}),
			withClaudeUpdateMeta(map[string]any{
				"terminal_output": map[string]any{
					"data":        banner,
					"terminal_id": job.toolCallID,
				},
			}),
		),
	})
}

// emitClaudeBackgroundTerminalExit emits the bridge's separate terminal frame.
// It deliberately carries no tool status and no outcome: the null exit fields
// are the only evidence that the command ended.
func (e *emitter) emitClaudeBackgroundTerminalExit(toolCallID string) {
	_ = e.conn.SessionUpdate(e.ctx, acp.SessionNotification{
		SessionId: e.sid,
		Update: acp.UpdateToolCall(acp.ToolCallId(toolCallID),
			withClaudeUpdateMeta(map[string]any{
				"terminal_exit": map[string]any{
					"exit_code":   nil,
					"signal":      nil,
					"terminal_id": toolCallID,
				},
			}),
		),
	})
}

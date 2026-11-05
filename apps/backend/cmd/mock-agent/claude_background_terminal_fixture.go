package main

import (
	"context"
	"os"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

// mockClaudeBridgeVersionEnv makes the mock advertise the exact official
// bridge identity inspected for null-code terminal-exit evidence. It is set
// only by the scenario that replays those captured frames.
const mockClaudeBridgeVersionEnv = "E2E_MOCK_AGENT_CLAUDE_BRIDGE_VERSION"

// mockClaudeTerminalSignalEnv gates the captured terminal frames on a file the
// browser test creates after it has observed both running jobs. Unset, the
// fixture falls back to its delay for manual use.
const mockClaudeTerminalSignalEnv = "E2E_MOCK_AGENT_CLAUDE_TERMINAL_SIGNAL_FILE"

const (
	mockClaudeBridgeName    = "@agentclientprotocol/claude-agent-acp"
	mockClaudeBridgeVersion = "0.97.0"
)

func mockClaudeBridgeAgentInfo() *acp.Implementation {
	if strings.TrimSpace(os.Getenv(mockClaudeBridgeVersionEnv)) != mockClaudeBridgeVersion {
		return nil
	}
	return &acp.Implementation{
		Name:    mockClaudeBridgeName,
		Version: mockClaudeBridgeVersion,
	}
}

type claudeBackgroundTerminalJob struct {
	toolCallID string
	command    string
	title      string
}

// emitClaudeBackgroundTerminalFixture replays the captured Claude 0.97.0
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
		},
		{
			toolCallID: string(nextToolID()),
			command:    "gh pr checks --watch",
			title:      "Watch pull request checks",
		},
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
			if !waitClaudeTerminalSignal(backgroundCtx, signalFile) {
				return
			}
		} else if !waitClaudeTerminalFixture(backgroundCtx, firstDelay) {
			return
		}
		backgroundEmitter.emitClaudeBackgroundTerminalExit(jobs[0].toolCallID)
		if !waitClaudeTerminalFixture(backgroundCtx, gap) {
			return
		}
		backgroundEmitter.emitClaudeBackgroundTerminalExit(jobs[1].toolCallID)
	}()
}

func waitClaudeTerminalSignal(ctx context.Context, path string) bool {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if _, err := os.Stat(path); err == nil {
			return true
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
	banner := "Command running in background with ID: " + job.toolCallID
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

package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	acp "github.com/coder/acp-go-sdk"
)

const (
	mockClaudeMonitorSignalFileEnv = "E2E_MOCK_AGENT_CLAUDE_MONITOR_SIGNAL_FILE"
	mockClaudeNativeVersion        = "2.1.328"
)

type mockClaudeMonitor struct {
	taskID     string
	toolCallID string
	command    string
}

// emitClaudeMonitorLifecycleFixture leaves two watches active after the
// foreground response. Test-owned signal lines append native completion
// records to the exact session transcript so the ACP adapter reads them via
// its production completion source.
func emitClaudeMonitorLifecycleFixture(e *emitter) {
	monitors := []mockClaudeMonitor{
		{taskID: "monitor-task-first", toolCallID: string(nextToolID()), command: "gh pr checks --watch"},
		{taskID: "monitor-task-second", toolCallID: string(nextToolID()), command: "go test ./..."},
	}
	for _, monitor := range monitors {
		e.startMonitorTool(acp.ToolCallId(monitor.toolCallID), monitor.taskID, monitor.command)
		state.monitorTools[monitor.taskID] = acp.ToolCallId(monitor.toolCallID)
	}
	e.text("Two Claude Monitors are watching.")

	signalFile := strings.TrimSpace(os.Getenv(mockClaudeMonitorSignalFileEnv))
	if signalFile == "" {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	release := func() {}
	if e.registerDetachedWork != nil {
		release = e.registerDetachedWork(e.sid, cancel)
	}
	go func() {
		defer release()
		defer cancel()
		firstAction := waitClaudeMonitorFirstAction(ctx, signalFile)
		if firstAction == "" {
			return
		}
		if firstAction == "complete-first" {
			if err := appendClaudeMonitorNativeCompletion(string(e.sid), monitors[0]); err != nil {
				return
			}
		}
		if !waitClaudeMonitorLifecycleSignal(ctx, signalFile, "complete-second") {
			return
		}
		if err := appendClaudeMonitorNativeCompletion(string(e.sid), monitors[1]); err != nil {
			return
		}
	}()
}

func emitClaudeMonitorTaskStopFirst(e *emitter) {
	const taskID = "monitor-task-first"
	_, ok := state.monitorTools[taskID]
	if !ok {
		e.text("Script error: TaskStop for unknown Monitor taskId " + taskID)
		return
	}
	delete(state.monitorTools, taskID)
	taskStopToolCallID := nextToolID()
	e.startClaudeControlTool(taskStopToolCallID, "TaskStop", "TaskStop", map[string]any{})
	e.completeClaudeTaskStopTool(taskStopToolCallID, taskID)
	e.text("The first Monitor was stopped.")

	signalFile := strings.TrimSpace(os.Getenv(mockClaudeMonitorSignalFileEnv))
	if signalFile == "" {
		return
	}
	file, err := os.OpenFile(signalFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		e.text("Script error: could not signal the Monitor fixture")
		return
	}
	_, writeErr := file.WriteString("taskstop-first\n")
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		e.text("Script error: could not signal the Monitor fixture")
	}
}

func waitClaudeMonitorFirstAction(ctx context.Context, path string) string {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		if content, err := os.ReadFile(path); err == nil {
			text := string(content)
			if strings.Contains(text, "taskstop-first") {
				return "taskstop-first"
			}
			if strings.Contains(text, "complete-first") {
				return "complete-first"
			}
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return ""
		}
	}
}

func waitClaudeMonitorLifecycleSignal(ctx context.Context, path, want string) bool {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		content, err := os.ReadFile(path)
		if err == nil && strings.Contains(string(content), want) {
			return true
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			return false
		}
	}
}

func appendClaudeMonitorNativeCompletion(sessionID string, monitor mockClaudeMonitor) error {
	configDir := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR"))
	if configDir == "" {
		return os.ErrNotExist
	}
	workDir, err := os.Getwd()
	if err != nil {
		return err
	}
	projectName := encodeMockClaudeProjectName(filepath.Clean(workDir))
	if projectName == "" || len(projectName) > 200 {
		return os.ErrInvalid
	}
	projectDir := filepath.Join(configDir, "projects", projectName)
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		return err
	}
	content := strings.Join([]string{
		"[SYSTEM NOTIFICATION - NOT USER INPUT]",
		"<task-notification>",
		"<task-id>" + monitor.taskID + "</task-id>",
		"<tool-use-id>" + monitor.toolCallID + "</tool-use-id>",
		"<output-file>/tmp/mock-claude-monitor.output</output-file>",
		"<status>completed</status>",
		"<summary>Mock Monitor completed</summary>",
		"</task-notification>",
	}, "\n")
	record := map[string]any{
		"type":      "attachment",
		"sessionId": sessionID,
		"version":   mockClaudeNativeVersion,
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"attachment": map[string]any{
			"type": "queued_command",
			"origin": map[string]string{
				"kind":     "task-notification",
				"producer": "session-task",
			},
			"prompt": content,
		},
	}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(filepath.Join(projectDir, sessionID+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(encoded, '\n'))
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func encodeMockClaudeProjectName(workDir string) string {
	var b strings.Builder
	b.Grow(len(workDir))
	for _, char := range workDir {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') {
			b.WriteRune(char)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

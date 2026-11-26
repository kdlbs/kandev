package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	acp "github.com/coder/acp-go-sdk"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-BACKGROUND-WORK-010.10
func TestClaudeBackgroundShellOutputStreamsFromLaunchFile(t *testing.T) {
	a := claudeTerminalAdapter(t)
	a.agentInfo.Version = verifiedClaudeTerminalBridgeCurrentVersion
	a.backgroundWorkEnabled = true
	a.normalizer = NewNormalizerWithBackgroundWork(claudeAgentID, true)
	a.claudeShellOutput.interval = time.Hour

	toolCallID := "toolu_background_output"
	outputPath := filepath.Join(t.TempDir(), "session-claude", "tasks", "task-output.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(outputPath), 0o700))
	require.NoError(t, os.WriteFile(outputPath, []byte("live output\n"), 0o600))

	startClaudeBackgroundShell(t, a, "session-claude", claudeBackgroundLaunch{
		toolCallID: toolCallID,
		command:    "go test ./...",
		title:      "Run the test suite",
	})
	completed := acp.ToolCallStatusCompleted
	launch := a.convertToolCallResultUpdate("session-claude", &acp.SessionToolCallUpdate{
		ToolCallId:    acp.ToolCallId(toolCallID),
		Status:        &completed,
		SessionUpdate: "tool_call_update",
		Meta:          map[string]any{"claudeCode": map[string]any{"toolName": "Bash"}},
		RawInput: map[string]any{
			"command":           "go test ./...",
			"description":       "Run the test suite",
			"run_in_background": true,
		},
		RawOutput: map[string]any{"output": "Command running in background with ID: task-output. Output is being written to: " + outputPath},
	})
	require.NotNil(t, launch)
	require.NotNil(t, launch.NormalizedPayload.BackgroundWork())
	require.NotNil(t, launch.NormalizedPayload.BackgroundWork().Capabilities)
	require.Equal(t, "stream", launch.NormalizedPayload.BackgroundWork().Capabilities.Output)

	a.claudeShellOutput.pollOnce()
	require.Zero(t, len(a.updatesCh), "shell output must wait until its launch update is queued")

	a.sendUpdate(*launch)
	a.claudeShellOutput.pollOnce()
	require.Eventually(t, func() bool { return len(a.updatesCh) >= 2 }, time.Second, time.Millisecond,
		"the delayed consumer should have both the launch and first output event queued")
	queuedLaunch := receiveClaudeShellAgentEvent(t, a.updatesCh)
	require.Equal(t, streams.EventTypeToolUpdate, queuedLaunch.Type)
	liveOutput := receiveClaudeShellAgentEvent(t, a.updatesCh)
	require.Equal(t, streams.EventTypeBackgroundWorkOutput, liveOutput.Type)
	require.Equal(t, toolCallID, liveOutput.BackgroundWorkOutput.WorkID)
	require.Equal(t, int64(0), liveOutput.BackgroundWorkOutput.Offset)
	require.Equal(t, "live output\n", liveOutput.BackgroundWorkOutput.Chunk)

	require.NoError(t, os.WriteFile(outputPath, []byte("live output\nfinal output"), 0o600))
	terminal := a.convertToolCallResultUpdate("session-claude", claudeTerminalExitUpdate(toolCallID, nil, nil))
	require.NotNil(t, terminal)
	a.sendUpdate(*terminal)
	finalOutput := receiveClaudeShellAgentEvent(t, a.updatesCh)
	terminalEvent := receiveClaudeShellAgentEvent(t, a.updatesCh)
	require.Equal(t, streams.EventTypeBackgroundWorkOutput, finalOutput.Type)
	require.Equal(t, int64(len("live output\n")), finalOutput.BackgroundWorkOutput.Offset)
	require.Equal(t, "final output", finalOutput.BackgroundWorkOutput.Chunk)
	require.Equal(t, streams.EventTypeToolUpdate, terminalEvent.Type)
	require.Equal(t, streams.RunStateEnded, terminalEvent.BackgroundWork.State)
}

func TestClaudeShellOutputPath(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "user with spaces", "session-claude", "tasks", "task-output.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(valid), 0o700))
	require.NoError(t, os.WriteFile(valid, []byte("output\n"), 0o600))

	unclean := strings.Replace(
		valid,
		string(filepath.Separator)+"session-claude",
		string(filepath.Separator)+string(filepath.Separator)+"session-claude",
		1,
	)
	for _, testCase := range []struct {
		name   string
		stdout string
	}{
		{
			name:   "background launch with a spaced path",
			stdout: "Command running in background with ID: task-output. Output is being written to: " + valid,
		},
		{
			name: "timeout moved with a spaced path and sentence period",
			stdout: "Command did not complete within its 30s timeout and was moved to the background " +
				"(ID: task-output). Output is being written to: " + valid + ".",
		},
		{
			name:   "cleans repeated separators before validating shape",
			stdout: "Output is being written to: " + unclean,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			path, ok := claudeShellOutputPath(testCase.stdout, "session-claude")
			require.True(t, ok)
			require.Equal(t, valid, path)
		})
	}
	t.Run("extracts a Windows path with spaces", func(t *testing.T) {
		windowsPath := `C:\Users\Jane Doe\AppData\Local\Temp\claude\123\project\session-claude\tasks\task-output.output`
		path, ok := claudeShellOutputPathFromLine(windowsPath + ".")
		require.True(t, ok)
		require.Equal(t, windowsPath, path)
	})

	missing := filepath.Join(root, "session-claude", "tasks", "created-lazily.output")
	path, ok := claudeShellOutputPath("Output is being written to: "+missing, "session-claude")
	require.True(t, ok, "Claude can create the output file after reporting its path")
	require.Equal(t, missing, path)

	nonRegular := filepath.Join(filepath.Dir(valid), "directory.output")
	require.NoError(t, os.Mkdir(nonRegular, 0o700))
	foreign := filepath.Join(root, "other-session", "tasks", "foreign.output")
	wrongShape := filepath.Join(root, "session-claude", "output", "wrong.output")
	for _, candidate := range []struct {
		name string
		path string
	}{
		{name: "relative", path: "session-claude/tasks/relative.output"},
		{name: "foreign session", path: foreign},
		{name: "wrong shape", path: wrongShape},
		{name: "non regular", path: nonRegular},
		{name: "wrong extension", path: filepath.Join(root, "session-claude", "tasks", "wrong.txt")},
	} {
		t.Run(candidate.name, func(t *testing.T) {
			_, ok := claudeShellOutputPath("Output is being written to: "+candidate.path, "session-claude")
			require.False(t, ok)
		})
	}
}

func TestClaudeShellOutputFollowerEmitsProgressWithoutNewline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	events := make(chan AgentEvent, 2)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	t.Cleanup(func() {
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "progress.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	content := "progress 10%\rprogress 20%\r"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.True(t, follower.follow("session-claude", "work-progress", path))
	require.True(t, follower.activate("session-claude", "work-progress"))
	follower.pollOnce()

	select {
	case event := <-events:
		require.Equal(t, content, event.BackgroundWorkOutput.Chunk)
	case <-time.After(time.Second):
		t.Fatal("progress output without a newline should be emitted before exit")
	}
}

func TestClaudeShellOutputFollowerActivationDoesNotWaitForSend(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	sendStarted := make(chan struct{})
	releaseSend := make(chan struct{})
	pollDone := make(chan struct{})
	follower := newClaudeShellOutputFollower(ctx, func(AgentEvent) {
		close(sendStarted)
		<-releaseSend
	})
	follower.interval = time.Hour
	t.Cleanup(func() {
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "blocked-send.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("output\n"), 0o600))
	require.True(t, follower.follow("session-claude", "work-blocked-send", path))
	require.True(t, follower.activate("session-claude", "work-blocked-send"))

	go func() {
		follower.pollOnce()
		close(pollDone)
	}()
	select {
	case <-sendStarted:
	case <-time.After(time.Second):
		t.Fatal("poller did not reach the blocked send")
	}

	activationDone := make(chan bool, 1)
	go func() {
		activationDone <- follower.activate("session-claude", "work-blocked-send")
	}()
	select {
	case activated := <-activationDone:
		if !activated {
			close(releaseSend)
			<-pollDone
			t.Fatal("activation failed while the job remained active")
		}
	case <-time.After(time.Second):
		close(releaseSend)
		<-pollDone
		<-activationDone
		t.Fatal("activation waited for send to release the job mutex")
	}

	close(releaseSend)
	<-pollDone
}

func TestClaudeShellOutputFollowerCapsGrowthBetweenPolls(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	events := make(chan AgentEvent, 2)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	t.Cleanup(func() {
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "growth.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	initial := "started\n"
	require.NoError(t, os.WriteFile(path, []byte(initial), 0o600))
	require.True(t, follower.follow("session-claude", "work-growth", path))
	require.True(t, follower.activate("session-claude", "work-growth"))
	follower.pollOnce()
	require.Equal(t, initial, receiveClaudeShellOutputEvent(t, events).Chunk)

	content := initial + strings.Repeat("x", claudeShellOutputMaxBytes*2) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	follower.pollOnce()
	chunk := receiveClaudeShellOutputEvent(t, events)
	require.True(t, chunk.Truncated)
	require.Equal(t, int64(len(initial)), chunk.Offset)
	require.LessOrEqual(t, len(chunk.Chunk), claudeShellOutputMaxBytes)
	require.Equal(t, strings.Repeat("x", claudeShellOutputMaxBytes-1)+"\n", chunk.Chunk)
}

func TestClaudeShellOutputFollowerCarriesSplitUTF8Rune(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	events := make(chan AgentEvent, 2)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	t.Cleanup(func() {
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "utf8.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	partial := []byte("status \xe2\x82")
	require.NoError(t, os.WriteFile(path, partial, 0o600))
	require.True(t, follower.follow("session-claude", "work-utf8", path))
	require.True(t, follower.activate("session-claude", "work-utf8"))
	follower.pollOnce()
	first := receiveClaudeShellOutputEvent(t, events)
	require.Equal(t, "status ", first.Chunk)
	require.Equal(t, int64(len("status ")), first.Offset+int64(len(first.Chunk)))

	complete := []byte("status \xe2\x82\xac done")
	require.NoError(t, os.WriteFile(path, complete, 0o600))
	follower.pollOnce()
	second := receiveClaudeShellOutputEvent(t, events)
	require.Equal(t, "€ done", second.Chunk)
	require.Equal(t, int64(len("status ")), second.Offset)
}

func TestClaudeShellOutputFollowerKeepsJSONOffsetsConsistentForInvalidUTF8(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(cancel)
	events := make(chan AgentEvent, 2)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	t.Cleanup(func() {
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "invalid-utf8.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte{0xff, 'A'}, 0o600))
	require.True(t, follower.follow("session-claude", "work-invalid-utf8", path))
	require.True(t, follower.activate("session-claude", "work-invalid-utf8"))
	follower.pollOnce()
	first := jsonRoundTripClaudeOutput(t, receiveClaudeShellOutputEvent(t, events))
	require.Equal(t, "\uFFFDA", first.Chunk)
	require.Zero(t, first.Offset)

	require.NoError(t, os.WriteFile(path, []byte{0xff, 'A', 'B'}, 0o600))
	follower.pollOnce()
	second := jsonRoundTripClaudeOutput(t, receiveClaudeShellOutputEvent(t, events))
	require.Equal(t, "B", second.Chunk)
	require.Equal(t, first.Offset+int64(len(first.Chunk)), second.Offset)
}

func jsonRoundTripClaudeOutput(t *testing.T, chunk *streams.WorkloadOutputChunk) *streams.WorkloadOutputChunk {
	t.Helper()
	encoded, err := json.Marshal(chunk)
	require.NoError(t, err)
	var delivered streams.WorkloadOutputChunk
	require.NoError(t, json.Unmarshal(encoded, &delivered))
	return &delivered
}

func receiveClaudeShellOutputEvent(t *testing.T, events <-chan AgentEvent) *streams.WorkloadOutputChunk {
	t.Helper()
	select {
	case event := <-events:
		require.NotNil(t, event.BackgroundWorkOutput)
		return event.BackgroundWorkOutput
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Claude shell output")
		return nil
	}
}

func TestClaudeShellOutputFollowerLimits(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	events := make(chan AgentEvent, 32)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	follower.now = func() time.Time { return time.Unix(0, now.Load()) }
	t.Cleanup(func() {
		cancel()
		follower.clear()
		follower.wait()
	})

	root := t.TempDir()
	firstPath := filepath.Join(root, "session-claude", "tasks", "first.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(firstPath), 0o700))
	require.NoError(t, os.WriteFile(firstPath, []byte("first output\n"), 0o600))
	require.True(t, follower.follow("session-claude", "work-0", firstPath))
	require.True(t, follower.activate("session-claude", "work-0"))
	done := follower.done
	follower.pollOnce()
	require.Equal(t, "first output\n", receiveClaudeShellOutputEvent(t, events).Chunk)
	follower.pollOnce()
	require.Empty(t, events, "an unchanged file must not emit the same bytes again")

	for index := 1; index < claudeShellOutputMaxJobs; index++ {
		workID := fmt.Sprintf("work-%d", index)
		path := filepath.Join(root, "session-claude", "tasks", workID+".output")
		require.True(t, follower.follow("session-claude", workID, path))
	}
	require.False(t, follower.follow("session-claude", "work-21", firstPath), "the follower must reject a 21st job")

	now.Add(int64(31 * time.Minute))
	follower.pollOnce()
	follower.mu.Lock()
	remaining := len(follower.jobs)
	follower.mu.Unlock()
	require.Zero(t, remaining, "jobs with no growth for 30 minutes must leave the map")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the poller should exit when its job map becomes empty")
	}
}

func TestClaudeShellOutputFollowerExpiresUnactivatedJobs(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	events := make(chan AgentEvent, 1)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	follower.interval = time.Hour
	var now atomic.Int64
	now.Store(time.Now().UnixNano())
	follower.now = func() time.Time { return time.Unix(0, now.Load()) }
	t.Cleanup(func() {
		cancel()
		follower.clear()
		follower.wait()
	})

	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "unactivated.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	require.NoError(t, os.WriteFile(path, []byte("not yet visible\n"), 0o600))
	require.True(t, follower.follow("session-claude", "work-unactivated", path))
	done := follower.done

	follower.pollOnce()
	require.Empty(t, events, "output must wait for the launch update")
	now.Add(int64(31 * time.Minute))
	follower.pollOnce()
	follower.mu.Lock()
	remaining := len(follower.jobs)
	follower.mu.Unlock()
	require.Zero(t, remaining, "a job without a launch update must still expire")
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("the poller should exit after an unactivated job expires")
	}
}

func TestClaudeShellOutputFollowerTruncatesInitialRead(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	events := make(chan AgentEvent, 1)
	follower := newClaudeShellOutputFollower(ctx, func(event AgentEvent) { events <- event })
	path := filepath.Join(t.TempDir(), "session-claude", "tasks", "large.output")
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o700))
	content := strings.Repeat("x", claudeShellOutputMaxBytes+1024) + "\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	require.True(t, follower.follow("session-claude", "work-large", path))
	require.True(t, follower.activate("session-claude", "work-large"))
	follower.pollOnce()
	chunk := receiveClaudeShellOutputEvent(t, events)
	require.True(t, chunk.Truncated)
	require.Zero(t, chunk.Offset)
	require.Equal(t, content[len(content)-claudeShellOutputMaxBytes:], chunk.Chunk)
	follower.clear()
	follower.wait()
}

func receiveClaudeShellAgentEvent(t *testing.T, events <-chan AgentEvent) AgentEvent {
	t.Helper()
	select {
	case event, ok := <-events:
		if !ok {
			t.Fatal("Claude shell output event channel closed unexpectedly")
		}
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for Claude shell output update")
		return AgentEvent{}
	}
}

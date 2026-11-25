package acp

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClaudeMonitorReaderExitCancelsOwnedContext(t *testing.T) {
	const sessionID = "native-session-1"
	configDir, workDir := t.TempDir(), t.TempDir()
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, sessionID))
	if err := os.MkdirAll(filepath.Dir(project), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(project, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	a := newClaudeMonitorCompletionTestAdapter(t, configDir, workDir, sessionID, true)
	reader, err := newClaudeMonitorCompletionReader(configDir, workDir, sessionID, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	readerCtx, cancel := context.WithCancel(a.lifetimeCtx)
	t.Cleanup(cancel)
	state := &claudeMonitorReaderState{reader: reader, ctx: readerCtx, cancel: cancel, started: true}
	state.generation.Store(1)
	a.mu.Lock()
	a.claudeMonitorReaders[sessionID] = state
	a.mu.Unlock()

	finished := make(chan struct{})
	a.claudeMonitorReaderWg.Add(1)
	go func() {
		a.runClaudeMonitorCompletionReader(readerCtx, sessionID, state)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("reader did not stop after its last prompt and Monitor ended")
	}
	select {
	case <-readerCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("reader returned without canceling its owned context")
	}
}

//go:build !windows

package acp

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestClaudeMonitorSessionFileDescriptorIsNonblocking(t *testing.T) {
	configDir, workDir := t.TempDir(), t.TempDir()
	sessionID := "native-session-1"
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, sessionID))
	require.NoError(t, os.MkdirAll(filepath.Dir(project), 0o700))
	require.NoError(t, os.WriteFile(project, nil, 0o600))

	reader, err := newClaudeMonitorCompletionReader(configDir, workDir, sessionID, time.Now().UTC())
	require.NoError(t, err)
	t.Cleanup(func() { _ = reader.Close() })
	require.NotNil(t, reader.file)

	flags, err := unix.FcntlInt(reader.file.Fd(), unix.F_GETFL, 0)
	require.NoError(t, err)
	require.NotZero(t, flags&unix.O_NONBLOCK,
		"the contained descriptor must remain nonblocking after validation")
}

func TestClaudeMonitorAdapterCloseDrainsReaderAfterFIFOReplacement(t *testing.T) {
	configDir, workDir := t.TempDir(), t.TempDir()
	const sessionID, taskID, toolCallID = "native-session-1", "monitor-task-1", "toolu_monitor_1"
	project := filepath.Join(configDir, claudeMonitorSessionPath(workDir, sessionID))
	require.NoError(t, os.MkdirAll(filepath.Dir(project), 0o700))
	require.NoError(t, os.WriteFile(project, nil, 0o600))
	a := newClaudeMonitorCompletionTestAdapter(t, configDir, workDir, sessionID, true)
	payload := newMonitorGeneric()
	payload.SetMonitorIdentity(taskID, false)
	seedMonitor(t, a, sessionID, taskID, toolCallID)
	a.activeToolCalls[toolCallID] = payload
	require.NotNil(t, a.beginClaudeMonitorPrompt(sessionID))

	backup := project + ".regular"
	require.NoError(t, os.Rename(project, backup))
	require.NoError(t, unix.Mkfifo(project, 0o600))

	closed := make(chan error, 1)
	go func() { closed <- a.Close() }()
	select {
	case err := <-closed:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("adapter close blocked while the native session path named a FIFO")
	}
}

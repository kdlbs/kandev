package toolretention

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIncrementalShellRetentionRemoval(t *testing.T) {
	for _, bytes := range []int{8192, 7, 0} {
		t.Run(strconv.Itoa(bytes), func(t *testing.T) { testIncrementalShellRetentionRemoval(t, bytes) })
	}
}

func testIncrementalShellRetentionRemoval(t *testing.T, bytes int) {
	t.Helper()
	s := testService(t)
	seedAnalysis(t, s)
	metadata := map[string]any{"tool_call_id": "tool1", "status": "complete", "shell_output_storage": 1, "normalized": map[string]any{"kind": "shell_exec", "shell_exec": map[string]any{"command": "cmd", "output": map[string]any{"has_output": bytes > 0, "stdout_bytes": bytes, "stderr_bytes": 0}}}}
	raw, err := json.Marshal(metadata)
	require.NoError(t, err)
	_, err = s.pool.Writer().Exec(`CREATE TABLE task_message_shell_output(message_id TEXT PRIMARY KEY); INSERT INTO task_message_shell_output VALUES('message')`)
	require.NoError(t, err)
	_, err = s.pool.Writer().Exec(`UPDATE task_session_messages SET metadata=? WHERE id='message'`, string(raw))
	require.NoError(t, err)
	rev := approveTestPolicy(t, s)
	_, err = s.Run(context.Background(), rev)
	require.NoError(t, err)
	for range 10 {
		require.NoError(t, s.stepScan(context.Background()))
	}
	var count int
	require.NoError(t, s.pool.Reader().Get(&count, `SELECT COUNT(*) FROM task_message_shell_output`))
	require.Zero(t, count)
}

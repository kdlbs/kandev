package handlers

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/system/logbundle"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
	"github.com/stretchr/testify/require"
)

type diagnosticMaterializerFake struct {
	taskID    string
	sessionID string
	bytes     int64
}

func (f *diagnosticMaterializerFake) MaterializeDiagnosticBundle(
	_ context.Context,
	taskID, sessionID, _ string,
	source io.Reader,
) (lifecycle.DiagnosticMaterialization, error) {
	f.taskID = taskID
	f.sessionID = sessionID
	count, err := io.Copy(io.Discard, source)
	f.bytes = count
	return lifecycle.DiagnosticMaterialization{Path: ".kandev/diagnostics/bundle.zip", Bytes: count}, err
}

func TestGetDiagnosticBundleUsesImmutableTaskSessionAndMaterializesArchive(t *testing.T) {
	taskService, repository := newTestTaskService(t)
	_, task, session := seedTaskWithSession(
		t, taskService, repository, models.TaskSessionStateWaitingForInput,
	)
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "logs"), 0o700))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "logs", "backend-logs.log"), []byte("task_id="+task.ID), 0o600,
	))
	bundles := logbundle.New(logbundle.Config{HomeDir: home, Log: testLogger(t)})
	ctx, cancel := context.WithCancel(context.Background())
	bundles.Start(ctx)
	t.Cleanup(func() {
		cancel()
		bundles.Stop()
	})
	materializer := &diagnosticMaterializerFake{}
	handler := &Handlers{
		taskSvc: taskService, sessionRepo: repository, logger: testLogger(t).WithFields(),
	}
	handler.SetDiagnosticBundleServices(bundles, materializer)
	message := makeWSMessage(t, ws.ActionMCPGetDiagnosticBundle, map[string]interface{}{
		"source": "backend", "task_id": task.ID, "session_id": session.ID,
	})
	response, err := handler.handleGetDiagnosticBundle(context.Background(), message)
	require.NoError(t, err)
	require.Equal(t, ws.MessageTypeResponse, response.Type)
	require.Equal(t, task.ID, materializer.taskID)
	require.Equal(t, session.ID, materializer.sessionID)
	require.Positive(t, materializer.bytes)
}

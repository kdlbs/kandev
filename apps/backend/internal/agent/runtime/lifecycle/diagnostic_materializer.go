package lifecycle

import (
	"context"
	"fmt"
	"io"
)

type DiagnosticMaterialization struct {
	Path  string `json:"path"`
	Bytes int64  `json:"bytes"`
}

func (m *Manager) MaterializeDiagnosticBundle(
	ctx context.Context,
	taskID, sessionID, bundleID string,
	source io.Reader,
) (DiagnosticMaterialization, error) {
	execution, exists := m.executionStore.GetBySessionID(sessionID)
	if !exists || execution == nil {
		return DiagnosticMaterialization{}, fmt.Errorf("active task execution not found")
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return DiagnosticMaterialization{}, fmt.Errorf("active task execution not found")
	}
	if execution.TaskID != taskID || execution.SessionID != sessionID {
		return DiagnosticMaterialization{}, fmt.Errorf("task and session do not match the active execution")
	}
	executionID := execution.ID
	result, err := client.MaterializeDiagnosticBundle(ctx, bundleID, source)
	if err != nil {
		return DiagnosticMaterialization{}, err
	}
	current, stillCurrent := m.executionStore.GetBySessionID(sessionID)
	if !stillCurrent || current.ID != executionID {
		return DiagnosticMaterialization{}, fmt.Errorf("task execution changed during diagnostic transfer")
	}
	return DiagnosticMaterialization{Path: result.Path, Bytes: result.Bytes}, nil
}

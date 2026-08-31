package lifecycle

import (
	"context"
	"fmt"
	"io"
)

// ExportACPDebug fetches the selected execution's exact ACP conversation
// export. The caller must perform task/session authorization before invoking
// this method; lifecycle only resolves the in-memory execution seam.
func (m *Manager) ExportACPDebug(
	ctx context.Context, taskSessionID string, maxBytes int64,
) (io.ReadCloser, error) {
	if taskSessionID == "" {
		return nil, fmt.Errorf("task session ID is required")
	}
	execution, ok := m.GetExecutionBySessionID(taskSessionID)
	if !ok || execution == nil || execution.ACPSessionID == "" {
		return nil, fmt.Errorf("ACP executor is unavailable")
	}
	client, release := execution.acquireAgentctlClient()
	defer release()
	if client == nil {
		return nil, fmt.Errorf("ACP executor is unavailable")
	}
	return client.ExportACPDebug(ctx, execution.ACPSessionID, maxBytes)
}

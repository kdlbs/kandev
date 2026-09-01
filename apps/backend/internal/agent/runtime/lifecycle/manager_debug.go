package lifecycle

import (
	"context"
	"fmt"
	"io"
	"sync"
)

type releaseOnCloseReadCloser struct {
	io.ReadCloser
	releaseOnce sync.Once
	release     func()
}

func (r *releaseOnCloseReadCloser) Close() error {
	err := r.ReadCloser.Close()
	r.releaseOnce.Do(r.release)
	return err
}

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
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, fmt.Errorf("ACP executor is unavailable")
	}
	body, err := client.ExportACPDebug(ctx, execution.ACPSessionID, maxBytes)
	if err != nil {
		release()
		return nil, err
	}
	if body == nil {
		release()
		return nil, fmt.Errorf("ACP debug export returned no body")
	}
	return &releaseOnCloseReadCloser{ReadCloser: body, release: release}, nil
}

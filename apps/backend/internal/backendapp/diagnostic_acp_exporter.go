package backendapp

import (
	"context"
	"errors"
	"io"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/system/logbundle"
)

type diagnosticACPExporter struct {
	manager *lifecycle.Manager
}

func newDiagnosticACPExporter(manager *lifecycle.Manager) *diagnosticACPExporter {
	return &diagnosticACPExporter{manager: manager}
}

func (e *diagnosticACPExporter) ExportACP(
	ctx context.Context, session logbundle.DiagnosticSession, maxBytes int64,
) (io.ReadCloser, error) {
	if e == nil || e.manager == nil {
		return nil, errors.New("ACP exporter unavailable")
	}
	return e.manager.ExportACPDebug(ctx, session.SessionID, maxBytes)
}

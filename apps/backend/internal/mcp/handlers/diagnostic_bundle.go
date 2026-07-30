package handlers

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/system/logbundle"
	userstore "github.com/kandev/kandev/internal/user/store"
	ws "github.com/kandev/kandev/pkg/websocket"
)

type DiagnosticBundleProvider interface {
	Create(owner string, sources []string) (logbundle.JobView, bool, error)
	Get(owner, id string) (logbundle.JobView, error)
	OpenArchive(owner, id string) (*os.File, logbundle.JobView, error)
	ManifestSummary(owner, id string) (map[string]any, error)
}

type DiagnosticBundleMaterializer interface {
	MaterializeDiagnosticBundle(
		ctx context.Context,
		taskID, sessionID, bundleID string,
		source io.Reader,
	) (lifecycle.DiagnosticMaterialization, error)
}

type diagnosticBundleRequest struct {
	Source    string `json:"source"`
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

func (h *Handlers) SetDiagnosticBundleServices(
	provider DiagnosticBundleProvider,
	materializer DiagnosticBundleMaterializer,
) {
	h.diagnosticBundles = provider
	h.diagnosticMaterializer = materializer
}

func (h *Handlers) handleGetDiagnosticBundle(
	ctx context.Context,
	message *ws.Message,
) (*ws.Message, error) {
	if h.diagnosticBundles == nil || h.diagnosticMaterializer == nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError,
			"Diagnostic bundles are unavailable", nil)
	}
	request, sources, requestErr := decodeDiagnosticRequest(message.Payload)
	if requestErr != nil {
		return ws.NewError(message.ID, message.Action, requestErr.code, requestErr.message, nil)
	}
	owner, authorizationErr := h.authorizeDiagnosticRequest(ctx, request)
	if authorizationErr != nil {
		return ws.NewError(message.ID, message.Action,
			authorizationErr.code, authorizationErr.message, nil)
	}
	job, _, err := h.diagnosticBundles.Create(owner, sources)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	job, err = h.waitForDiagnosticBundle(ctx, owner, job)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	archive, _, err := h.diagnosticBundles.OpenArchive(owner, job.ID)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	defer func() { _ = archive.Close() }()
	manifest, err := h.diagnosticBundles.ManifestSummary(owner, job.ID)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	materialized, err := h.diagnosticMaterializer.MaterializeDiagnosticBundle(
		ctx, request.TaskID, request.SessionID, job.ID, archive,
	)
	if err != nil {
		return ws.NewError(message.ID, message.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(message.ID, message.Action, map[string]any{
		"path": materialized.Path, "bytes": materialized.Bytes,
		"bundle_id": job.ID, "status": job.Status, "sources": job.Sources,
		"warnings": job.Warnings, "manifest": manifest,
	})
}

type diagnosticRequestError struct {
	code    string
	message string
}

func decodeDiagnosticRequest(payload json.RawMessage) (
	diagnosticBundleRequest,
	[]string,
	*diagnosticRequestError,
) {
	var request diagnosticBundleRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		return request, nil, &diagnosticRequestError{ws.ErrorCodeBadRequest, "Invalid payload"}
	}
	sources, ok := diagnosticSources(request.Source)
	if !ok || request.TaskID == "" || request.SessionID == "" {
		return request, nil, &diagnosticRequestError{
			ws.ErrorCodeValidation, "source, task_id, and session_id are required",
		}
	}
	return request, sources, nil
}

func (h *Handlers) authorizeDiagnosticRequest(
	ctx context.Context,
	request diagnosticBundleRequest,
) (string, *diagnosticRequestError) {
	task, err := h.taskSvc.GetTask(ctx, request.TaskID)
	if err != nil || task == nil {
		return "", &diagnosticRequestError{ws.ErrorCodeNotFound, "Task not found"}
	}
	session, err := h.sessionRepo.GetTaskSession(ctx, request.SessionID)
	if err != nil || session == nil || session.TaskID != request.TaskID {
		return "", &diagnosticRequestError{ws.ErrorCodeValidation, "Session does not belong to task"}
	}
	if identity, present := authn.IdentityFromContext(ctx); present && identity.UserID != "" {
		return identity.UserID, nil
	}
	return userstore.DefaultUserID, nil
}

func (h *Handlers) waitForDiagnosticBundle(
	ctx context.Context,
	owner string,
	job logbundle.JobView,
) (logbundle.JobView, error) {
	for job.Status == logbundle.StatusCollecting || job.Status == logbundle.StatusBuilding {
		timer := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return logbundle.JobView{}, ctx.Err()
		case <-timer.C:
		}
		var err error
		job, err = h.diagnosticBundles.Get(owner, job.ID)
		if err != nil {
			return logbundle.JobView{}, err
		}
	}
	if job.Status != logbundle.StatusReady && job.Status != logbundle.StatusPartial {
		return logbundle.JobView{}, &diagnosticStateError{status: job.Status}
	}
	return job, nil
}

type diagnosticStateError struct{ status logbundle.Status }

func (e *diagnosticStateError) Error() string {
	return "diagnostic bundle ended in state " + string(e.status)
}

func diagnosticSources(source string) ([]string, bool) {
	switch source {
	case "backend":
		return []string{"backend"}, true
	case "frontend":
		return []string{"frontend"}, true
	case "all":
		return []string{"backend", "frontend"}, true
	default:
		return nil, false
	}
}

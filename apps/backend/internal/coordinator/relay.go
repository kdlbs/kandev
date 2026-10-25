package coordinator

import (
	"context"
	"errors"
	"expvar"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/clarification"
	"github.com/kandev/kandev/internal/common/logger"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var relayReadFailedTotal = expvar.NewInt("coordinator_relay_read_failed_total")

// RelayReader is the durable-interaction read seam the relay read needs.
type RelayReader interface {
	clarification.BundleMessageReader
	ListUnresolvedClarificationBundles(ctx context.Context, opts taskmodels.ListClarificationBundlesOptions) (*taskmodels.ClarificationBundlePage, error)
	ListPendingInteractions(ctx context.Context, filter taskmodels.PendingInteractionFilter) ([]*taskmodels.Message, error)
}

// RelayTasks resolves a task and its primary session.
type RelayTasks interface {
	GetTask(ctx context.Context, id string) (*taskmodels.Task, error)
	GetPrimarySessionIDsForTasks(ctx context.Context, taskIDs []string) (map[string]string, error)
}

// RelayClarification is the answerable clarification bundle of a session.
type RelayClarification struct {
	PendingID string        `json:"pending_id"`
	Context   string        `json:"context"`
	Messages  []*v1.Message `json:"messages"`
}

// RelayPermission is the answerable pending permission request of a session.
type RelayPermission struct {
	Message *v1.Message `json:"message"`
}

// RelayResult is the relay read response: the task's primary session and its
// answerable interactions, each nil when there is none.
type RelayResult struct {
	TaskID        string              `json:"task_id"`
	SessionID     string              `json:"session_id"`
	Clarification *RelayClarification `json:"clarification"`
	Permission    *RelayPermission    `json:"permission"`
}

// SetRelayDeps wires the readers behind the relay read route.
func (s *Service) SetRelayDeps(reader RelayReader, tasks RelayTasks) {
	s.relayReader = reader
	s.relayTasks = tasks
}

// ReadRelay returns the answerable clarification bundle and permission
// request of a task's primary session. A task or coordinator outside the
// workspace, or phase 3 not effective, is ErrNotFound.
func (s *Service) ReadRelay(ctx context.Context, workspaceID, coordinatorID, taskID string) (*RelayResult, error) {
	if !s.phase3 || s.relayReader == nil || s.relayTasks == nil {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID); err != nil {
		return nil, err
	}
	task, err := s.relayTasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return nil, ErrNotFound
		}
		return nil, s.relayReadFailed(err)
	}
	if task == nil || task.WorkspaceID != workspaceID {
		return nil, ErrNotFound
	}
	sessions, err := s.relayTasks.GetPrimarySessionIDsForTasks(ctx, []string{taskID})
	if err != nil {
		return nil, s.relayReadFailed(err)
	}
	result := &RelayResult{TaskID: taskID, SessionID: sessions[taskID]}
	if result.SessionID == "" {
		return result, nil
	}
	if result.Clarification, err = s.relayClarification(ctx, result.SessionID); err != nil {
		return nil, s.relayReadFailed(err)
	}
	if result.Permission, err = s.relayPermission(ctx, result.SessionID); err != nil {
		return nil, s.relayReadFailed(err)
	}
	return result, nil
}

func (s *Service) relayReadFailed(err error) error {
	relayReadFailedTotal.Add(1)
	return fmt.Errorf("relay read: %w", err)
}

func (s *Service) relayClarification(ctx context.Context, sessionID string) (*RelayClarification, error) {
	page, err := s.relayReader.ListUnresolvedClarificationBundles(ctx, taskmodels.ListClarificationBundlesOptions{
		Unscoped:  true,
		SessionID: sessionID,
		Limit:     1,
	})
	if err != nil {
		return nil, err
	}
	if page == nil || len(page.Bundles) == 0 {
		return nil, nil
	}
	summary := page.Bundles[0]
	content, err := clarification.HydrateBundle(ctx, s.relayReader, summary)
	if err != nil || content == nil {
		return nil, err
	}
	return &RelayClarification{PendingID: summary.PendingID, Context: content.Context, Messages: content.Messages}, nil
}

// relayPermission returns the newest pending permission row when it can be
// answered: it must carry a request id, a pending id and at least one option.
func (s *Service) relayPermission(ctx context.Context, sessionID string) (*RelayPermission, error) {
	rows, err := s.relayReader.ListPendingInteractions(ctx, taskmodels.PendingInteractionFilter{
		SessionIDs: []string{sessionID},
		Kinds:      []string{string(taskmodels.InteractionKindPermission)},
	})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	newest := rows[len(rows)-1]
	if newest == nil || !permissionAnswerable(newest.Metadata) {
		return nil, nil
	}
	return &RelayPermission{Message: newest.ToAPI()}, nil
}

func permissionAnswerable(meta map[string]any) bool {
	requestID, _ := meta["request_id"].(string)
	pendingID, _ := meta["pending_id"].(string)
	options, _ := meta["options"].([]any)
	return requestID != "" && pendingID != "" && len(options) > 0
}

// httpGetRelay backs
// GET /api/v1/workspaces/:id/coordinators/:cid/relay/:taskId.
func (h *Handlers) httpGetRelay(c *gin.Context) {
	result, err := h.service.ReadRelay(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("taskId"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// RegisterRelayRoutes mounts the relay read route. Called only when phase 3
// is effective.
func RegisterRelayRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	h := NewHandlers(svc, log)
	router.Group("/api/v1/workspaces/:id").GET("/coordinators/:cid/relay/:taskId", h.httpGetRelay)
}

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
	if err := s.checkRelayTask(ctx, workspaceID, taskID); err != nil {
		return nil, err
	}
	targets, err := s.readTargets(ctx, taskID, false)
	if err != nil {
		return nil, s.relayReadFailed(err)
	}
	result := &RelayResult{TaskID: taskID, SessionID: targets.sessionID}
	if targets.bundle != nil {
		content, err := clarification.HydrateBundle(ctx, s.relayReader, *targets.bundle)
		if err != nil {
			return nil, s.relayReadFailed(err)
		}
		if content != nil {
			result.Clarification = &RelayClarification{PendingID: targets.bundle.PendingID, Context: content.Context, Messages: content.Messages}
		}
	}
	if targets.permission != nil {
		result.Permission = &RelayPermission{Message: targets.permission.ToAPI()}
	}
	return result, nil
}

// checkRelayTask returns ErrNotFound for a task that does not exist or lives
// in another workspace.
func (s *Service) checkRelayTask(ctx context.Context, workspaceID, taskID string) error {
	task, err := s.relayTasks.GetTask(ctx, taskID)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskNotFound) {
			return ErrNotFound
		}
		return s.relayReadFailed(err)
	}
	if task == nil || task.WorkspaceID != workspaceID {
		return ErrNotFound
	}
	return nil
}

func (s *Service) relayReadFailed(err error) error {
	relayReadFailedTotal.Add(1)
	return fmt.Errorf("relay read: %w", err)
}

// relayTargets is one read of a task's answerable interactions: the primary
// session, the newest unresolved clarification bundle (with its task-model
// messages when asked for) and the newest answerable permission row.
type relayTargets struct {
	sessionID  string
	bundle     *taskmodels.ClarificationBundleSummary
	messages   []*taskmodels.Message
	permission *taskmodels.Message
}

// readTargets is the one read behind ReadRelay and AnswerTargets. A task with
// no primary session has no targets.
func (s *Service) readTargets(ctx context.Context, taskID string, withMessages bool) (*relayTargets, error) {
	sessions, err := s.relayTasks.GetPrimarySessionIDsForTasks(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	out := &relayTargets{sessionID: sessions[taskID]}
	if out.sessionID == "" {
		return out, nil
	}
	if out.bundle, err = s.relayBundle(ctx, out.sessionID); err != nil {
		return nil, err
	}
	if out.bundle != nil && withMessages {
		byPending, err := s.relayReader.FindMessagesByPendingIDs(ctx, []string{out.bundle.PendingID})
		if err != nil {
			return nil, err
		}
		out.messages = byPending[out.bundle.PendingID]
	}
	out.permission, err = s.relayPermission(ctx, out.sessionID)
	return out, err
}

func (s *Service) relayBundle(ctx context.Context, sessionID string) (*taskmodels.ClarificationBundleSummary, error) {
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
	return &summary, nil
}

// relayPermission returns the newest pending permission row when it can be
// answered: it must carry a request id, a pending id and at least one option.
func (s *Service) relayPermission(ctx context.Context, sessionID string) (*taskmodels.Message, error) {
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
	return newest, nil
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

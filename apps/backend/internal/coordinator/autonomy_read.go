package coordinator

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
)

// stopFailingAfter is how long an open turn's requested stop may stay
// unconfirmed before the read reports it as failing.
const stopFailingAfter = 5 * time.Minute

const (
	stopStateFailing    = "stop_failing"
	codeCoordinatorGone = "coordinator_not_found"
	codeRunGone         = "run_not_found"
	codeChangeGone      = "change_not_found"
	codeReadError       = "read_error"
	autonomyReadTimeFmt = time.RFC3339
)

var (
	// ErrRunNotFound is a run id that names no row of the coordinator.
	ErrRunNotFound = errors.New("coordinator run not found")
	// errReadFailed marks a store or wiring read that failed.
	errReadFailed = errors.New("coordinator read failed")
)

func readFailed(what string, err error) error {
	if err == nil {
		return fmt.Errorf("%w: %s", errReadFailed, what)
	}
	return fmt.Errorf("%w: %s: %w", errReadFailed, what, err)
}

// TurnReadDTO is one unattended turn row on the wire.
type TurnReadDTO struct {
	ID                 string  `json:"id"`
	CoordinatorID      string  `json:"coordinator_id"`
	ConversationTaskID string  `json:"conversation_task_id"`
	SessionID          string  `json:"session_id"`
	StartedAt          string  `json:"started_at"`
	FinishedAt         *string `json:"finished_at"`
	Outcome            *string `json:"outcome"`
	WakeCount          int     `json:"wake_count"`
	DeniedPermissions  int     `json:"denied_permissions"`
	CostSubcents       *int64  `json:"cost_subcents"`
	StopRequestedAt    *string `json:"stop_requested_at"`
	StopState          *string `json:"stop_state"`
}

// RunWakeDTO is one wake of a run.
type RunWakeDTO struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	TaskID         string  `json:"task_id"`
	TaskIdentifier *string `json:"task_identifier"`
	TaskTitle      *string `json:"task_title"`
}

// RunReadDTO is the run read body: a turn plus its wakes.
type RunReadDTO struct {
	TurnReadDTO
	Wakes []RunWakeDTO `json:"wakes"`
}

// AdmissionDTO is the read-only admission result.
type AdmissionDTO struct {
	OK     bool    `json:"ok"`
	Reason string  `json:"reason,omitempty"`
	Detail string  `json:"detail"`
	Until  *string `json:"until,omitempty"`
}

// ContainmentConditionDTO is one containment condition.
type ContainmentConditionDTO struct {
	Name   string `json:"name"`
	Met    bool   `json:"met"`
	Detail string `json:"detail"`
}

// AutonomyContainmentDTO holds the four conditions in check order.
type AutonomyContainmentDTO struct {
	Conditions []ContainmentConditionDTO `json:"conditions"`
}

// AutonomySpendDTO is the one spend wire shape of the three readings.
type AutonomySpendDTO struct {
	Measurable      bool   `json:"measurable"`
	Degraded        bool   `json:"degraded"`
	WindowSubcents  *int64 `json:"window_subcents"`
	MeanDaily7d     *int64 `json:"mean_daily_subcents_7d"`
	MeanKnown       bool   `json:"mean_known"`
	CeilingSubcents *int64 `json:"ceiling_subcents"`
}

// AutonomyReadDTO is the autonomy read body.
type AutonomyReadDTO struct {
	ServerTime      string                 `json:"server_time"`
	AutonomyEnabled bool                   `json:"autonomy_enabled"`
	Admission       *AdmissionDTO          `json:"admission,omitempty"`
	PendingWakes    int                    `json:"pending_wakes"`
	OldestPendingAt *string                `json:"oldest_pending_at"`
	LastWokeAt      *string                `json:"last_woke_at"`
	LastTurn        *TurnReadDTO           `json:"last_turn"`
	Containment     AutonomyContainmentDTO `json:"containment"`
	Spend           AutonomySpendDTO       `json:"spend"`
	PauseView
}

func timeStr(t time.Time) string { return t.UTC().Format(autonomyReadTimeFmt) }

func timeStrPtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := timeStr(*t)
	return &s
}

func newTurnReadDTO(r *turnReadRow, now time.Time) TurnReadDTO {
	dto := TurnReadDTO{
		ID: r.ID, CoordinatorID: r.CoordinatorID, ConversationTaskID: r.ConvTaskID, SessionID: r.SessionID,
		StartedAt: timeStr(r.StartedAt), FinishedAt: timeStrPtr(r.FinishedAt), Outcome: r.Outcome,
		WakeCount: r.WakeCount, DeniedPermissions: r.DeniedPerms, CostSubcents: r.CostSubcents,
		StopRequestedAt: timeStrPtr(r.StopRequestedAt),
	}
	if r.FinishedAt == nil && r.StopRequestedAt != nil && now.Sub(*r.StopRequestedAt) > stopFailingAfter {
		state := stopStateFailing
		dto.StopState = &state
	}
	return dto
}

// AutonomyRead answers the autonomy read for one coordinator. The fields are
// read one after another; the pending pair alone comes from a single query.
func (s *Service) AutonomyRead(ctx context.Context, workspaceID, coordinatorID string) (*AutonomyReadDTO, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	coord, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, readFailed("coordinator", err)
	}
	now := s.store.now().UTC()
	out := &AutonomyReadDTO{ServerTime: timeStr(now), AutonomyEnabled: coord.AutonomyEnabled, PauseView: s.pauseView(ctx, coord)}
	if coord.AutonomyEnabled {
		if err := s.fillAdmission(ctx, coord, out); err != nil {
			return nil, err
		}
	}
	if err := s.fillTurnFields(ctx, coord.ID, now, out); err != nil {
		return nil, err
	}
	if s.containment == nil {
		return nil, readFailed("containment is not wired", nil)
	}
	out.Containment = newContainmentDTO(s.containment.Check(ctx, coord))
	out.Spend = s.spendDTO(ctx, coord, now)
	return out, nil
}

func (s *Service) fillAdmission(ctx context.Context, coord *Coordinator, out *AutonomyReadDTO) error {
	adm := s.Admit(ctx, coord.ID, AdmitReadOnly)
	if adm.Detail == admitDetailReadError {
		return readFailed("admission", nil)
	}
	if adm.Reason == admitAutonomyOff {
		if adm.Detail == admitDetailNotFound {
			return ErrNotFound
		}
		out.AutonomyEnabled = false
		return nil
	}
	dto := &AdmissionDTO{OK: adm.OK, Reason: adm.Reason, Detail: adm.Detail}
	if adm.Reason == admitCooldown {
		dto.Until = timeStrPtr(adm.Until)
	}
	out.Admission = dto
	return nil
}

func (s *Service) fillTurnFields(ctx context.Context, coordinatorID string, now time.Time, out *AutonomyReadDTO) error {
	pending, oldest, err := s.store.pendingWakeSummary(ctx, coordinatorID)
	if err != nil {
		return readFailed("pending wakes", err)
	}
	out.PendingWakes, out.OldestPendingAt = pending, timeStrPtr(oldest)
	lastWoke, err := s.store.lastWokeAt(ctx, coordinatorID)
	if err != nil {
		return readFailed("last woke", err)
	}
	out.LastWokeAt = timeStrPtr(lastWoke)
	newest, err := s.store.newestTurnRead(ctx, coordinatorID)
	if err != nil {
		return readFailed("last turn", err)
	}
	if newest != nil {
		dto := newTurnReadDTO(newest, now)
		out.LastTurn = &dto
	}
	return nil
}

func newContainmentDTO(res ContainmentResult) AutonomyContainmentDTO {
	conds := make([]ContainmentConditionDTO, 0, len(res.Conditions))
	for _, c := range res.Conditions {
		conds = append(conds, ContainmentConditionDTO(c))
	}
	return AutonomyContainmentDTO{Conditions: conds}
}

// spendDTO maps the spend reading to the three-row wire table; a failed read
// is a failed row, never an error.
func (s *Service) spendDTO(ctx context.Context, coord *Coordinator, now time.Time) AutonomySpendDTO {
	dto := AutonomySpendDTO{CeilingSubcents: coord.CostCeilingSubcents}
	reading, err := s.Spend(ctx, coord, now)
	if err != nil {
		return dto
	}
	dto.Measurable, dto.Degraded, dto.MeanKnown = reading.Measurable, reading.Degraded, reading.Mean7dKnown
	if reading.Measurable {
		w := reading.WindowSubcents
		dto.WindowSubcents = &w
	}
	if reading.Mean7dKnown {
		m := reading.Mean7dSubcents
		dto.MeanDaily7d = &m
	}
	return dto
}

// RunRead answers the run read for one turn row of the coordinator.
func (s *Service) RunRead(ctx context.Context, workspaceID, coordinatorID, runID string) (*RunReadDTO, error) {
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	coord, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if errors.Is(err, ErrNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, readFailed("coordinator", err)
	}
	row, err := s.store.turnReadByID(ctx, coord.ID, runID)
	if err != nil {
		return nil, readFailed("run", err)
	}
	if row == nil {
		return nil, ErrRunNotFound
	}
	wakes, err := s.store.wakesOfTurn(ctx, row.ID)
	if err != nil {
		return nil, readFailed("run wakes", err)
	}
	out := &RunReadDTO{TurnReadDTO: newTurnReadDTO(row, s.store.now().UTC()), Wakes: make([]RunWakeDTO, 0, len(wakes))}
	titles := map[string]*RunWakeDTO{}
	for _, w := range wakes {
		dto := RunWakeDTO{ID: w.ID, Kind: w.Kind, TaskID: w.TaskID}
		if known, ok := titles[w.TaskID]; ok {
			dto.TaskIdentifier, dto.TaskTitle = known.TaskIdentifier, known.TaskTitle
		} else {
			s.fillWakeTask(ctx, &dto)
			titles[w.TaskID] = &dto
		}
		out.Wakes = append(out.Wakes, dto)
	}
	return out, nil
}

// fillWakeTask reads the wake's task now; an unreadable task leaves both
// fields null.
func (s *Service) fillWakeTask(ctx context.Context, dto *RunWakeDTO) {
	if s.conversationTasks == nil {
		return
	}
	task, err := s.conversationTasks.GetTask(ctx, dto.TaskID)
	if err != nil || task == nil {
		return
	}
	title := task.Title
	dto.TaskTitle = &title
	if task.Identifier != "" {
		id := task.Identifier
		dto.TaskIdentifier = &id
	}
}

// httpGetAutonomy backs GET /api/v1/workspaces/:id/coordinators/:cid/autonomy.
func (h *Handlers) httpGetAutonomy(c *gin.Context) {
	out, err := h.service.AutonomyRead(c.Request.Context(), c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// httpGetRun backs GET /api/v1/workspaces/:id/coordinators/:cid/runs/:runId.
func (h *Handlers) httpGetRun(c *gin.Context) {
	out, err := h.service.RunRead(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("runId"))
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

// respondReadError maps the two read routes' errors to their coded bodies.
func (h *Handlers) respondReadError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrRunNotFound):
		c.JSON(http.StatusNotFound, NewErrorResponse(codeRunGone))
	case errors.Is(err, ErrNotFound):
		c.JSON(http.StatusNotFound, NewErrorResponse(codeCoordinatorGone))
	case errors.Is(err, ErrChangeNotFound):
		c.JSON(http.StatusNotFound, NewErrorResponse(codeChangeGone))
	case errors.As(err, new(*ChangeConflictError)):
		var conflict *ChangeConflictError
		_ = errors.As(err, &conflict)
		c.JSON(http.StatusConflict, changeConflictBody{Error: "conflict", Reason: conflict.Reason, Change: conflict.Change})
	case errors.Is(err, errReadFailed):
		h.logger.Warn("coordinator read failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewErrorResponse(codeReadError))
	default:
		h.respondError(c, err)
	}
}

// changeConflictBody is the 409 body of an Apply or Discard that lost.
type changeConflictBody struct {
	Error  string         `json:"error"`
	Reason string         `json:"reason"`
	Change *PendingChange `json:"change"`
}

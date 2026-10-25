package coordinator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/authz"
)

const (
	beforeField    = "before"
	bodyUnreadable = "request body is unreadable or too large"
)

// dreamPageSize is the fixed size of one page of the reports list.
const dreamPageSize = 20

// Ratings an item may carry.
const (
	RatingUseful    = "useful"
	RatingNotUseful = "not_useful"
	RatingHarmful   = "harmful"
)

// LearningHealth is the health line of the Learning section.
type LearningHealth struct {
	State     string `json:"state"`
	Condition string `json:"condition,omitempty"`
	Detail    string `json:"detail,omitempty"`
}

// LearningHealthReader computes the health of one coordinator's dream. The
// dream package satisfies it.
type LearningHealthReader interface {
	Health(ctx context.Context, c *Coordinator) (LearningHealth, error)
}

// SetLearningHealth wires the reader behind the Learning route.
func (s *Service) SetLearningHealth(r LearningHealthReader) {
	s.observerMu.Lock()
	defer s.observerMu.Unlock()
	s.learningHealth = r
}

// LearningView is the response of the Learning route.
type LearningView struct {
	ShadowDream bool           `json:"shadow_dream"`
	Health      LearningHealth `json:"health"`
}

// DreamSummary is one row of the reports list.
type DreamSummary struct {
	ID           string     `json:"id"`
	Status       string     `json:"status"`
	Reason       string     `json:"reason,omitempty"`
	WindowStart  time.Time  `json:"window_start"`
	WindowEnd    time.Time  `json:"window_end"`
	TurnCount    int        `json:"turn_count"`
	ItemCount    int        `json:"item_count"`
	CostSubcents *int64     `json:"cost_subcents"`
	StartedAt    time.Time  `json:"started_at"`
	FinishedAt   *time.Time `json:"finished_at"`
}

// DreamsPage is the response of the reports list.
type DreamsPage struct {
	Dreams     []DreamSummary `json:"dreams"`
	NextBefore string         `json:"next_before,omitempty"`
}

// DreamItemView is one item of a report.
type DreamItemView struct {
	ID           string   `json:"id"`
	Position     int      `json:"position"`
	Kind         string   `json:"kind"`
	Text         string   `json:"text"`
	TargetID     string   `json:"target_id,omitempty"`
	CitedTurnIDs []string `json:"cited_turn_ids"`
	Gate         string   `json:"gate"`
	ReplayID     string   `json:"replay_id,omitempty"`
	ReplayResult string   `json:"replay_verdict,omitempty"`
	ReplayReason string   `json:"replay_reason,omitempty"`
	Rating       string   `json:"rating,omitempty"`
}

// DreamDetail is the response of the report detail.
type DreamDetail struct {
	DreamSummary
	Considered []string        `json:"considered"`
	Items      []DreamItemView `json:"items"`
}

func (s *Service) learningReadable(ctx context.Context, workspaceID, coordinatorID string, scope authz.Scope) (*Coordinator, error) {
	if !s.phase31 {
		return nil, ErrNotFound
	}
	if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	c, err := s.store.GetCoordinator(ctx, workspaceID, coordinatorID)
	if err != nil {
		return nil, err
	}
	if scope != authz.ScopeWorkspaceRead {
		if err := s.authz.AuthorizeWorkspaceScope(ctx, workspaceID, scope); err != nil {
			return nil, err
		}
	}
	return c, nil
}

// Learning backs GET .../learning.
func (s *Service) Learning(ctx context.Context, workspaceID, coordinatorID string) (*LearningView, error) {
	c, err := s.learningReadable(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	on, err := s.store.ShadowDreamEnabled(ctx, c.ID)
	if err != nil {
		return nil, readFailed("learning", err)
	}
	view := &LearningView{ShadowDream: on, Health: LearningHealth{State: "off"}}
	s.observerMu.RLock()
	reader := s.learningHealth
	s.observerMu.RUnlock()
	if reader != nil {
		if view.Health, err = reader.Health(ctx, c); err != nil {
			return nil, readFailed("learning health", err)
		}
	}
	return view, nil
}

// SetShadowDream backs PUT .../learning. It publishes coordinator.updated and
// changes neither the conversation nor the policy revision.
func (s *Service) SetShadowDream(ctx context.Context, workspaceID, coordinatorID string, body []byte) (*LearningView, error) {
	c, err := s.learningReadable(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return nil, err
	}
	var req struct {
		ShadowDream *bool `json:"shadow_dream"`
	}
	if err := json.Unmarshal(body, &req); err != nil || req.ShadowDream == nil {
		return nil, &FieldError{Field: "shadow_dream", Message: "shadow_dream must be true or false"}
	}
	if err := s.store.SetShadowDreamEnabled(ctx, workspaceID, c.ID, *req.ShadowDream); err != nil {
		return nil, err
	}
	s.publishCoordinatorUpdated(ctx, workspaceID, c.ID)
	return s.Learning(ctx, workspaceID, coordinatorID)
}

func parseDreamCursor(raw string) (*DreamCursor, error) {
	if raw == "" {
		return nil, nil
	}
	at, id, ok := strings.Cut(raw, ",")
	t, err := time.Parse(time.RFC3339Nano, at)
	if !ok || id == "" || err != nil {
		return nil, &FieldError{Field: beforeField, Message: "before is not a cursor from a previous page"}
	}
	return &DreamCursor{StartedAt: t, ID: id}, nil
}

func dreamCursor(d Dream) string {
	return d.StartedAt.UTC().Format(time.RFC3339Nano) + "," + d.ID
}

// ListDreams backs GET .../dreams.
func (s *Service) ListDreams(ctx context.Context, workspaceID, coordinatorID, before string) (*DreamsPage, error) {
	c, err := s.learningReadable(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	cursor, err := parseDreamCursor(before)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.ListDreams(ctx, c.ID, cursor, dreamPageSize+1)
	if err != nil {
		return nil, readFailed("dreams", err)
	}
	page := &DreamsPage{Dreams: []DreamSummary{}}
	if len(rows) > dreamPageSize {
		rows = rows[:dreamPageSize]
		page.NextBefore = dreamCursor(rows[len(rows)-1])
	}
	ids := make([]string, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.ID)
	}
	counts, err := s.store.DreamItemCounts(ctx, ids)
	if err != nil {
		return nil, readFailed("dream items", err)
	}
	for _, r := range rows {
		page.Dreams = append(page.Dreams, dreamSummary(r, counts[r.ID]))
	}
	return page, nil
}

func dreamSummary(d Dream, items int) DreamSummary {
	return DreamSummary{
		ID: d.ID, Status: d.Status, Reason: d.Reason, WindowStart: d.WindowStart, WindowEnd: d.WindowEnd,
		TurnCount: len(d.TurnIDs), ItemCount: items, CostSubcents: d.CostSubcents,
		StartedAt: d.StartedAt, FinishedAt: d.FinishedAt,
	}
}

// GetDream backs GET .../dreams/:dreamId.
func (s *Service) GetDream(ctx context.Context, workspaceID, coordinatorID, dreamID string) (*DreamDetail, error) {
	c, err := s.learningReadable(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceRead)
	if err != nil {
		return nil, err
	}
	d, err := s.store.GetDream(ctx, c.ID, dreamID)
	if errors.Is(err, ErrDreamNotFound) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, readFailed("dream", err)
	}
	items, err := s.store.ListDreamItems(ctx, d.ID)
	if err != nil {
		return nil, readFailed("dream items", err)
	}
	out := &DreamDetail{DreamSummary: dreamSummary(*d, len(items)), Considered: d.Considered, Items: []DreamItemView{}}
	if out.Considered == nil {
		out.Considered = []string{}
	}
	for _, it := range items {
		out.Items = append(out.Items, DreamItemView{
			ID: it.ID, Position: it.Position, Kind: it.Kind, Text: it.Text, TargetID: it.TargetID,
			CitedTurnIDs: it.CitedTurnIDs, Gate: it.Gate, ReplayID: it.ReplayID,
			ReplayResult: it.Verdict, ReplayReason: it.ReplayReason, Rating: it.Rating,
		})
	}
	return out, nil
}

// RateDreamItem backs PUT .../dreams/:dreamId/items/:itemId/rating.
func (s *Service) RateDreamItem(ctx context.Context, workspaceID, coordinatorID, dreamID, itemID string, body []byte) error {
	c, err := s.learningReadable(ctx, workspaceID, coordinatorID, authz.ScopeWorkspaceManage)
	if err != nil {
		return err
	}
	var req struct {
		Rating string `json:"rating"`
	}
	if err := json.Unmarshal(body, &req); err != nil ||
		(req.Rating != RatingUseful && req.Rating != RatingNotUseful && req.Rating != RatingHarmful) {
		return &FieldError{Field: "rating", Message: "rating must be useful, not_useful or harmful"}
	}
	err = s.store.RateDreamItem(ctx, c.ID, dreamID, itemID, decidingUserID(ctx), req.Rating)
	if errors.Is(err, ErrDreamNotFound) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("rate dream item: %w", err)
	}
	return nil
}

func registerLearningRoutes(workspace *gin.RouterGroup, h *Handlers) {
	workspace.GET("/coordinators/:cid/learning", h.httpGetLearning)
	workspace.PUT("/coordinators/:cid/learning", h.httpPutLearning)
	workspace.GET("/coordinators/:cid/dreams", h.httpListDreams)
	workspace.GET("/coordinators/:cid/dreams/:dreamId", h.httpGetDream)
	workspace.PUT("/coordinators/:cid/dreams/:dreamId/items/:itemId/rating", h.httpPutDreamRating)
}

func (h *Handlers) httpGetLearning(c *gin.Context) {
	out, err := h.service.Learning(c.Request.Context(), c.Param("id"), c.Param("cid"))
	h.respondLearning(c, out, err)
}

func (h *Handlers) httpPutLearning(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSettingsBodyBytes))
	if err != nil {
		h.respondError(c, &FieldError{Field: "shadow_dream", Message: bodyUnreadable})
		return
	}
	out, err := h.service.SetShadowDream(c.Request.Context(), c.Param("id"), c.Param("cid"), body)
	h.respondLearning(c, out, err)
}

func (h *Handlers) respondLearning(c *gin.Context, out any, err error) {
	if err != nil {
		h.respondReadError(c, err)
		return
	}
	c.JSON(http.StatusOK, out)
}

func (h *Handlers) httpListDreams(c *gin.Context) {
	out, err := h.service.ListDreams(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Query(beforeField))
	h.respondLearning(c, out, err)
}

func (h *Handlers) httpGetDream(c *gin.Context) {
	out, err := h.service.GetDream(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("dreamId"))
	h.respondLearning(c, out, err)
}

func (h *Handlers) httpPutDreamRating(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSettingsBodyBytes))
	if err != nil {
		h.respondError(c, &FieldError{Field: "rating", Message: bodyUnreadable})
		return
	}
	if err := h.service.RateDreamItem(c.Request.Context(), c.Param("id"), c.Param("cid"), c.Param("dreamId"), c.Param("itemId"), body); err != nil {
		h.respondReadError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

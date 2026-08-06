package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
)

// maxUserStateBodyBytes caps the request body PUT
// /api/plugins/:id/user-state/... will read before persisting it, mirroring
// maxWebhookBodyBytes's rationale: without a cap, an authenticated-but-
// arbitrary browser write could exhaust backend memory. 256 KiB comfortably
// covers a rich-text scratchpad document (the driving use case, see
// docs/decisions/2026-08-01-per-user-plugin-storage.md) while bounding
// worst-case memory per request.
const maxUserStateBodyBytes = 256 << 10 // 256 KiB

// userStateSegmentPattern validates the :scopeId and :key path segments of
// the per-user storage routes (AC18): non-empty, alphanumeric plus a small
// set of separators, capped at 128 characters.
var userStateSegmentPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)

// userStateScopes is the closed set of valid :scope values (AC18).
var userStateScopes = map[string]bool{
	"instance":   true,
	"workspace":  true,
	"task":       true,
	"session":    true,
	"repository": true,
}

// registerUserStateRoutes wires the authenticated, per-user plugin storage
// surface (Approach D1: AC15-AC20, AC24, AC27, AC28). Registered before the
// public /:id/webhooks/:key wildcard, mirroring the /settings ordering
// caveat already documented in RegisterRoutes — these are static-ish
// sub-paths of /:id that must not be shadowed by a broader wildcard
// registered first.
func registerUserStateRoutes(api *gin.RouterGroup, ctrl *Controller) {
	api.GET("/:id/user-state/:scope/:scopeId", ctrl.userStateList)
	api.GET("/:id/user-state/:scope/:scopeId/:key", ctrl.userStateGet)
	api.PUT("/:id/user-state/:scope/:scopeId/:key", ctrl.userStateSet)
	api.DELETE("/:id/user-state/:scope/:scopeId/:key", ctrl.userStateDelete)
}

// userStateSetRequest is the PUT .../user-state/:scope/:scopeId/:key body.
// Value is wrapped rather than being the bare request body so an arbitrary
// JSON value (array, string, number — not just an object) round-trips
// unambiguously alongside the sibling writerId/ifUnmodifiedSince fields.
type userStateSetRequest struct {
	Value             json.RawMessage `json:"value"`
	WriterID          string          `json:"writerId,omitempty"`
	IfUnmodifiedSince *time.Time      `json:"ifUnmodifiedSince,omitempty"`
}

// pluginUserStateUpdatedEvent is the bus event payload published on a
// successful PUT/DELETE (Approach F1, AC24).
//
// UserID is exported (json:"user_id") so it survives the JSON marshal a
// NATS-backed EventBus performs before publishing
// (internal/events/bus/nats.go): an unexported field disappears from that
// wire representation, so a NATS subscriber decodes event.Data as a map with
// no "user_id" at all, and UserEventBroadcaster silently drops the
// notification. It never reaches the browser regardless of transport —
// UserEventBroadcaster.subscribe strips "user_id" from the outgoing WS
// payload before building the message the client receives (routing is the
// broadcaster's concern; the client already knows its own identity).
type pluginUserStateUpdatedEvent struct {
	UserID    string    `json:"user_id"`
	PluginID  string    `json:"pluginId"`
	Scope     string    `json:"scope"`
	ScopeID   string    `json:"scopeId"`
	Key       string    `json:"key"`
	UpdatedAt time.Time `json:"updatedAt"`
	WriterID  string    `json:"writerId,omitempty"`
	Deleted   bool      `json:"deleted,omitempty"`
}

// userStateIdentity resolves the caller's identity and the plugin record,
// enforces the record is active and declares the user_state capability, and
// validates :scope. It writes the appropriate error response and returns
// ok=false on any failure, so callers can return immediately.
func (c *Controller) userStateIdentity(ctx *gin.Context) (userID string, record *store.Record, scope string, ok bool) {
	identity, hasIdentity := authn.FromGin(ctx)
	if !hasIdentity {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return "", nil, "", false
	}

	rec, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return "", nil, "", false
	}
	if rec.Status != StatusActive {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %q is not active", rec.ID)})
		return "", nil, "", false
	}
	if !rec.Capabilities.UserState {
		ctx.JSON(http.StatusForbidden, gin.H{
			"error": fmt.Sprintf("plugin %q does not declare the user_state capability", rec.ID),
		})
		return "", nil, "", false
	}

	scope = ctx.Param("scope")
	if !userStateScopes[scope] {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid scope %q", scope)})
		return "", nil, "", false
	}

	if c.svc.UserState() == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "user state store not configured"})
		return "", nil, "", false
	}

	return identity.UserID, rec, scope, true
}

// userStateSegments validates the :scopeId and (for single-key routes) :key
// path params against userStateSegmentPattern (AC18).
func (c *Controller) userStateSegments(ctx *gin.Context) (scopeID, key string, ok bool) {
	scopeID = ctx.Param("scopeId")
	if !userStateSegmentPattern.MatchString(scopeID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid scopeId"})
		return "", "", false
	}
	key = ctx.Param("key")
	if !userStateSegmentPattern.MatchString(key) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid key"})
		return "", "", false
	}
	return scopeID, key, true
}

// userStateList serves GET /api/plugins/:id/user-state/:scope/:scopeId
// (AC27): every entry under that scope, ordered by key.
func (c *Controller) userStateList(ctx *gin.Context) {
	userID, record, scope, ok := c.userStateIdentity(ctx)
	if !ok {
		return
	}
	scopeID := ctx.Param("scopeId")
	if !userStateSegmentPattern.MatchString(scopeID) {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid scopeId"})
		return
	}

	entries, err := c.svc.UserState().List(ctx.Request.Context(), record.ID, userID, scope, scopeID)
	if err != nil {
		c.log.Warn("plugin user-state list error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if entries == nil {
		entries = []state.UserStateEntry{}
	}
	ctx.JSON(http.StatusOK, gin.H{"entries": entries})
}

// userStateGet serves GET /api/plugins/:id/user-state/:scope/:scopeId/:key
// (AC15's positive path).
func (c *Controller) userStateGet(ctx *gin.Context) {
	userID, record, scope, ok := c.userStateIdentity(ctx)
	if !ok {
		return
	}
	scopeID, key, ok := c.userStateSegments(ctx)
	if !ok {
		return
	}

	value, updatedAt, found, err := c.svc.UserState().Get(ctx.Request.Context(), record.ID, userID, scope, scopeID, key)
	if err != nil {
		c.log.Warn("plugin user-state get error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if !found {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"value": value, "updatedAt": updatedAt})
}

// userStateSet serves PUT /api/plugins/:id/user-state/:scope/:scopeId/:key
// (AC15's write path, AC28's conditional-write path, AC24's publish).
func (c *Controller) userStateSet(ctx *gin.Context) {
	userID, record, scope, ok := c.userStateIdentity(ctx)
	if !ok {
		return
	}
	scopeID, key, ok := c.userStateSegments(ctx)
	if !ok {
		return
	}

	body, err := readCappedUserStateBody(ctx)
	if err != nil {
		return // readCappedUserStateBody already wrote the error response
	}

	var req userStateSetRequest
	if err := json.Unmarshal(body, &req); err != nil || len(req.Value) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload: value required"})
		return
	}

	updatedAt, err := c.svc.UserState().Set(
		ctx.Request.Context(), record.ID, userID, scope, scopeID, key, req.Value, req.IfUnmodifiedSince,
	)
	if err != nil {
		if errors.Is(err, state.ErrConflict) {
			ctx.JSON(http.StatusConflict, gin.H{"error": "conflict: value was modified since ifUnmodifiedSince"})
			return
		}
		c.log.Warn("plugin user-state set error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.publishUserStateUpdated(ctx, record.ID, userID, scope, scopeID, key, updatedAt, req.WriterID, false)
	ctx.JSON(http.StatusOK, gin.H{"updatedAt": updatedAt})
}

// userStateDelete serves DELETE
// /api/plugins/:id/user-state/:scope/:scopeId/:key. writerId, if the caller
// wants echo suppression on delete too, is passed as a query param since
// DELETE carries no body in this API.
func (c *Controller) userStateDelete(ctx *gin.Context) {
	userID, record, scope, ok := c.userStateIdentity(ctx)
	if !ok {
		return
	}
	scopeID, key, ok := c.userStateSegments(ctx)
	if !ok {
		return
	}

	if err := c.svc.UserState().Delete(ctx.Request.Context(), record.ID, userID, scope, scopeID, key); err != nil {
		c.log.Warn("plugin user-state delete error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.publishUserStateUpdated(ctx, record.ID, userID, scope, scopeID, key, time.Now().UTC(), ctx.Query("writerId"), true)
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// publishUserStateUpdated best-effort publishes the realtime notification
// (Approach F1) after a successful write/delete. A nil/unconfigured event
// bus is a silent no-op — the write itself already succeeded.
func (c *Controller) publishUserStateUpdated(
	ctx *gin.Context, pluginID, userID, scope, scopeID, key string, updatedAt time.Time, writerID string, deleted bool,
) {
	eventBus := c.svc.EventBus()
	if eventBus == nil {
		return
	}
	payload := pluginUserStateUpdatedEvent{
		UserID:    userID,
		PluginID:  pluginID,
		Scope:     scope,
		ScopeID:   scopeID,
		Key:       key,
		UpdatedAt: updatedAt,
		WriterID:  writerID,
		Deleted:   deleted,
	}
	evt := bus.NewEvent(events.PluginUserStateUpdated, "plugins-service", payload)
	if err := eventBus.Publish(ctx.Request.Context(), events.PluginUserStateUpdated, evt); err != nil {
		c.log.Warn("plugin user-state event publish failed", zap.Error(err))
	}
}

// readCappedUserStateBody reads ctx.Request.Body bounded at
// maxUserStateBodyBytes via http.MaxBytesReader, writing the 413 response
// itself (and returning a non-nil error as a sentinel to the caller) when
// the body exceeds the cap — mirrors readCappedWebhookBody's approach for
// the same reason (unbounded io.ReadAll on an authenticated-but-arbitrary
// write is still a memory-exhaustion vector).
func readCappedUserStateBody(ctx *gin.Context) ([]byte, error) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxUserStateBodyBytes)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("request body exceeds max size of %d bytes", maxUserStateBodyBytes),
			})
			return nil, err
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return nil, err
	}
	return body, nil
}

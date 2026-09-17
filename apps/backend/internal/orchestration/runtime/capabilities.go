package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// CapabilityReader reads bounded pages without starting agents or remote probes.
type CapabilityReader interface {
	ReadCapabilities(context.Context, models.CapabilityQuery) (models.CapabilityPage, error)
}

type capabilityCursor struct {
	Scope, Generation, After string
}

func (h *Handler) capabilities(c *gin.Context) {
	var b *models.AssistantBinding
	var ok bool
	sessionID := c.Query("session_id")
	if _, runtime := c.Get("agent_claims"); runtime {
		claims, binding, valid := h.runtimeAssistant(c)
		b, ok = binding, valid
		if valid && sessionID == "" {
			sessionID = claims.SessionID
		}
	} else {
		b, ok = h.humanAssistant(c)
	}
	if !ok {
		return
	}
	query, scope, err := capabilityRequest(c, b, sessionID)
	if err != nil {
		c.AbortWithStatus(400)
		return
	}
	if h.Service.Capabilities == nil {
		c.AbortWithStatus(503)
		return
	}
	ctx := authn.WithIdentity(c.Request.Context(), authn.Identity{UserID: b.OwnerUserID, Role: authn.RoleMember})
	page, err := h.Service.Capabilities.ReadCapabilities(ctx, query)
	if errors.Is(err, models.ErrCapabilityGeneration) {
		c.AbortWithStatusJSON(409, gin.H{errorResponseKey: "capability_generation_changed"})
		return
	}
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	if page.After != "" {
		raw, _ := json.Marshal(capabilityCursor{Scope: scope, Generation: page.Generation, After: page.After})
		page.NextCursor = base64.RawURLEncoding.EncodeToString(raw)
	}
	c.JSON(200, page)
}

func capabilityRequest(c *gin.Context, b *models.AssistantBinding, session string) (models.CapabilityQuery, string, error) {
	kind := c.Query("kind")
	if !slices.Contains([]string{"", "native", "profile", "workflow", "executor", "integration", "plugin", "mcp"}, kind) || len(session) > 200 {
		return models.CapabilityQuery{}, "", fmt.Errorf("invalid capability filter")
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return models.CapabilityQuery{}, "", fmt.Errorf("invalid limit")
	}
	scope := cursorScope("capabilities-v1", b.OwnerUserID, b.WorkspaceID, b.ID, strconv.FormatInt(b.Version, 10), session, kind)
	q := models.CapabilityQuery{OwnerID: b.OwnerUserID, WorkspaceID: b.WorkspaceID, ConversationID: b.ConversationID, SessionID: session, Kind: kind, Limit: limit}
	if raw := c.Query("after"); raw != "" {
		var cursor capabilityCursor
		decoded, err := base64.RawURLEncoding.DecodeString(raw)
		if len(raw) > 2048 || err != nil || json.Unmarshal(decoded, &cursor) != nil || cursor.Scope != scope || cursor.Generation == "" || cursor.After == "" || len(cursor.After) > 200 {
			return q, scope, fmt.Errorf("invalid capability cursor")
		}
		q.After, q.Generation = cursor.After, cursor.Generation
	}
	return q, scope, nil
}

package runtime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
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
	if raw, present := c.Get("agent_claims"); present {
		if claims, valid := raw.(*runtimeauth.AgentClaims); valid && claims.Capabilities == workspaceCoordinatorAudience {
			h.workspaceCapabilities(c)
			return
		}
	}
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

func (h *Handler) workspaceCapabilities(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	limit, ok := boundedPageLimit(c)
	if !ok {
		return
	}
	kind := c.Query("kind")
	if kind != "" && kind != "native" {
		c.JSON(400, gin.H{errorResponseKey: "workspace capabilities supports kind=native; use workspace for resource IDs"})
		return
	}
	scope := cursorScope("workspace-controls-v1", claims.TaskID, claims.WorkspaceID)
	after, err := decodeScopedCursor(c.Query("after"), scope)
	if err != nil {
		fail(c, err)
		return
	}
	tools := models.WorkspaceBrokerTools()
	slices.SortFunc(tools, func(a, b models.WorkspaceBrokerTool) int { return strings.Compare(a.Name, b.Name) })
	page := models.CapabilityPage{Entries: []models.Capability{}, Generation: "workspace-controls-v1"}
	for _, tool := range tools {
		if tool.Name <= after {
			continue
		}
		if len(page.Entries) == limit {
			page.NextCursor = encodeScopedCursor(scope, page.Entries[len(page.Entries)-1].Name)
			break
		}
		const workspaceReadEffect = "read"
		effect := workspaceReadEffect
		if tool.Method != http.MethodGet {
			effect = "write"
		}
		page.Entries = append(page.Entries, models.Capability{ID: "native/" + tool.Name, Kind: "native", Name: tool.Name,
			WorkspaceID: claims.WorkspaceID, Effect: effect, Surfaces: []string{"conversation"}, Health: "ready",
			Configured: true, Attached: true, InspectAllowed: true, Reason: tool.Description, Revision: page.Generation,
			InputSchema: json.RawMessage(`{"type":"object"}`), SchemaPartial: true})
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

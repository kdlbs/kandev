package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type builtinMCPHTTP struct {
	service *mcpconfig.BuiltinPolicyService
}

type builtinMCPWrite struct {
	Version                int                             `json:"version"`
	ExpectedRevision       *int64                          `json:"expected_revision"`
	ExpectedPolicyRevision *int64                          `json:"expected_workspace_policy_revision,omitempty"`
	Packages               mcpconfig.BuiltinPackageChoices `json:"packages"`
}

// RegisterBuiltinMCPRoutes installs the human management API separately from
// agent tools. Writers enforce human identity and resource scopes in the service.
func RegisterBuiltinMCPRoutes(router *gin.Engine, service *mcpconfig.BuiltinPolicyService, interlock gin.HandlerFunc) {
	h := &builtinMCPHTTP{service: service}
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/mcp-built-in", h.read(service.ReadWorkspace))
	api.GET("/tasks/:id/mcp-built-in", h.read(service.ReadTask))
	mutations := api.Group("")
	if interlock != nil {
		mutations.Use(interlock)
	}
	mutations.PUT("/workspaces/:id/mcp-built-in/policy", h.updateWorkspace)
	mutations.PUT("/tasks/:id/mcp-built-in/preferences", h.updateTask)
}

func (h *builtinMCPHTTP) read(read func(context.Context, string) (*mcpconfig.BuiltinManagementView, error)) gin.HandlerFunc {
	return func(c *gin.Context) {
		view, err := read(c.Request.Context(), c.Param("id"))
		if err != nil {
			writeBuiltinMCPError(c, err)
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

func (h *builtinMCPHTTP) updateWorkspace(c *gin.Context) {
	body, err := decodeBuiltinMCPWrite(c, false)
	if err != nil {
		writeBuiltinMCPError(c, err)
		return
	}
	policy, err := h.service.UpdateWorkspace(c.Request.Context(), c.Param("id"), *body.ExpectedRevision, body.Packages)
	if err != nil {
		writeBuiltinMCPError(c, err)
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *builtinMCPHTTP) updateTask(c *gin.Context) {
	body, err := decodeBuiltinMCPWrite(c, true)
	if err != nil {
		writeBuiltinMCPError(c, err)
		return
	}
	snapshot, err := h.service.UpdateTask(c.Request.Context(), c.Param("id"), *body.ExpectedRevision, *body.ExpectedPolicyRevision, body.Packages)
	if err != nil {
		writeBuiltinMCPError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func decodeBuiltinMCPWrite(c *gin.Context, task bool) (*builtinMCPWrite, error) {
	decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 8192))
	decoder.DisallowUnknownFields()
	var body builtinMCPWrite
	if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	if body.Version != 1 || body.ExpectedRevision == nil || *body.ExpectedRevision < 0 || body.Packages == nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	if task && (body.ExpectedPolicyRevision == nil || *body.ExpectedPolicyRevision < 0) {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	if !task && body.ExpectedPolicyRevision != nil {
		return nil, mcpconfig.ErrBuiltinPolicyInvalid
	}
	return &body, nil
}

func writeBuiltinMCPError(c *gin.Context, err error) {
	var conflict *mcpconfig.BuiltinPolicyConflictError
	switch {
	case errors.As(err, &conflict):
		c.JSON(http.StatusConflict, gin.H{"error_code": "builtin_mcp_conflict", "current": conflict.Current})
	case errors.Is(err, mcpconfig.ErrBuiltinHumanRequired), errors.Is(err, mcpconfig.ErrMCPWorkspaceAccess):
		c.JSON(http.StatusForbidden, gin.H{"error_code": "builtin_mcp_forbidden"})
	case errors.Is(err, repoerrors.ErrTaskNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error_code": "builtin_mcp_not_found"})
	case errors.Is(err, mcpconfig.ErrBuiltinPackageBlocked):
		c.JSON(http.StatusForbidden, gin.H{"error_code": "builtin_mcp_blocked"})
	case errors.Is(err, mcpconfig.ErrBuiltinPolicyInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error_code": "builtin_mcp_invalid"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error_code": "builtin_mcp_unavailable"})
	}
}

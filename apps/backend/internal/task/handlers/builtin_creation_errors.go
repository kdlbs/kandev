package handlers

import (
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	ws "github.com/kandev/kandev/pkg/websocket"
	"net/http"
)

func builtinCreationError(err error) (int, string, string) {
	switch {
	case errors.Is(err, mcpconfig.ErrBuiltinPolicyConflict):
		return http.StatusConflict, ws.ErrorCodeConflict, "builtin_mcp_conflict"
	case errors.Is(err, mcpconfig.ErrBuiltinHumanRequired):
		return http.StatusForbidden, ws.ErrorCodeForbidden, "builtin_mcp_forbidden"
	case errors.Is(err, mcpconfig.ErrBuiltinPackageBlocked):
		return http.StatusForbidden, ws.ErrorCodeForbidden, "builtin_mcp_blocked"
	case errors.Is(err, mcpconfig.ErrBuiltinPolicyInvalid):
		return http.StatusBadRequest, ws.ErrorCodeValidation, "builtin_mcp_invalid"
	default:
		return 0, "", ""
	}
}

func handleBuiltinCreationHTTPError(c *gin.Context, err error) bool {
	status, _, code := builtinCreationError(err)
	if status == 0 {
		return false
	}
	c.JSON(status, gin.H{"error": err.Error(), "error_code": code})
	return true
}

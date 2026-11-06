package handlers

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestBuiltinCreationHTTPRejectsPolicyErrorsAsClientErrors(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
	}{
		{"denied", mcpconfig.ErrBuiltinHumanRequired, 403},
		{"blocked", mcpconfig.ErrBuiltinPackageBlocked, 403},
		{"invalid", mcpconfig.ErrBuiltinPolicyInvalid, 400},
		{"stale", &mcpconfig.BuiltinPolicyConflictError{}, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			ctx, _ := gin.CreateTestContext(recorder)
			handleNotFound(ctx, newTestLogger(t), fmt.Errorf("initial package consent: %w", tc.err), "task not created")
			require.Equal(t, tc.status, recorder.Code)
			require.Contains(t, recorder.Body.String(), "builtin_mcp_")
		})
	}
}

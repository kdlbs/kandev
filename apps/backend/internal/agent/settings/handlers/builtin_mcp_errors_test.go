package handlers

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"testing"
)

func TestBuiltinManagementHTTPPreservesDomainDenials(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{{taskservice.ErrForbidden, 403}, {models.ErrTaskSessionNotFound, 404}} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		writeBuiltinMCPError(ctx, tc.err)
		require.Equal(t, tc.status, recorder.Code)
	}
}

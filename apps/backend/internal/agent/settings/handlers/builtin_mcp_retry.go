package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
)

type builtinSessionRetrier interface {
	RetryBuiltinSessionMCP(context.Context, string, string, int64) (*mcpconfig.BuiltinSessionView, error)
}

func builtinMCPRetryHandler(retrier builtinSessionRetrier) gin.HandlerFunc {
	return func(c *gin.Context) {
		if _, err := mcpconfig.BuiltinHumanActor(c.Request.Context()); err != nil {
			writeBuiltinMCPError(c, err)
			return
		}
		var body struct {
			Version    int    `json:"version"`
			RequestID  string `json:"request_id"`
			RowVersion int64  `json:"expected_row_version"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(c.Writer, c.Request.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || decoder.Decode(new(any)) != io.EOF || body.Version != 1 || body.RequestID == "" || body.RowVersion <= 0 {
			writeBuiltinMCPError(c, mcpconfig.ErrBuiltinPolicyInvalid)
			return
		}
		view, err := retrier.RetryBuiltinSessionMCP(c.Request.Context(), c.Param("id"), body.RequestID, body.RowVersion)
		if err != nil {
			writeBuiltinMCPError(c, err)
			return
		}
		c.JSON(http.StatusOK, view)
	}
}

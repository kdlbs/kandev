package backendapp

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/httpmw"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	mcpserver "github.com/kandev/kandev/internal/mcp/server"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/stretchr/testify/require"
)

// @covers AC-AGENTS-MCP-PROTOCOL-001.9
func TestExternalMCPNotification_AuthenticatesBeforeBodyRead(t *testing.T) {
	cfg := &config.Config{}
	cfg.Features.Auth = true
	authService := newEnabledAuthService(t, cfg)
	_, token, err := authService.MintToken(context.Background(), userstore.DefaultUserID, "notification", 0)
	require.NoError(t, err)
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	require.NoError(t, err)
	srv := mcpserver.NewExternal(nil, log, "")
	t.Cleanup(func() { require.NoError(t, srv.Close(context.Background())) })
	router := gin.New()
	router.Use(httpmw.Middleware(authService))
	srv.RegisterBackendRoutes(router.Group("", externalMCPAuthMiddleware(authService)))
	for _, credential := range []string{"", "invalid", token} {
		body := &notificationCountingBody{Reader: strings.NewReader(
			`{"jsonrpc":"2.0","method":"notifications/roots/list_changed"}`,
		)}
		request := httptest.NewRequest(http.MethodPost, "http://localhost/mcp", body)
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json, text/event-stream")
		request.Header.Set("Mcp-Protocol-Version", externalModernProtocolVersion)
		request.Header.Set("Mcp-Method", "notifications/roots/list_changed")
		if credential != "" {
			request.Header.Set("Authorization", "Bearer "+credential)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if credential == token {
			require.Equal(t, http.StatusAccepted, response.Code, "body = %s", response.Body.String())
			require.Empty(t, response.Body.String())
			require.Positive(t, body.reads)
		} else {
			require.Equal(t, http.StatusUnauthorized, response.Code)
			require.Zero(t, body.reads)
		}
	}
}

type notificationCountingBody struct {
	io.Reader
	reads int
}

func (b *notificationCountingBody) Read(p []byte) (int, error) {
	b.reads++
	return b.Reader.Read(p)
}

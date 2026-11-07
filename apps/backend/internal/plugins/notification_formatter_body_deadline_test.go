package plugins

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/stretchr/testify/require"
)

func formatterDeadlineServer(t *testing.T, consume func(*gin.Context)) *httptest.Server {
	t.Helper()
	router := gin.New()
	router.POST("/", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Millisecond)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		finish := boundFormatterManagement(c, manifest.ActionScopeNotificationProvider)
		defer finish()
		consume(c)
	})
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	return server
}

func TestFormatterManagementDeadlineInterruptsPartialBodyRead(t *testing.T) {
	done := make(chan struct{})
	server := formatterDeadlineServer(t, func(c *gin.Context) {
		_, _ = io.ReadAll(c.Request.Body)
		close(done)
	})
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n{")
	require.NoError(t, err)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("management body read outlived its request deadline")
	}
}

func TestFormatterManagementDeadlineSurvivesEarlyInvalidBodyReturn(t *testing.T) {
	server := formatterDeadlineServer(t, func(c *gin.Context) {
		var body any
		_ = json.NewDecoder(c.Request.Body).Decode(&body)
		c.Status(http.StatusBadRequest)
	})
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
	_, err = io.WriteString(conn, "POST / HTTP/1.1\r\nHost: localhost\r\nContent-Length: 100\r\n\r\n!")
	require.NoError(t, err)
	response, err := http.ReadResponse(bufio.NewReader(conn), nil)
	require.NoError(t, err, "server body drain must retain the management read deadline")
	defer func() { _ = response.Body.Close() }()
	require.Equal(t, http.StatusBadRequest, response.StatusCode)
}

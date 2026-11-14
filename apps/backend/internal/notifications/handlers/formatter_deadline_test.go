package handlers

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

func TestFormatterHTTPDeadlineBoundsBodyReadsAndDrain(t *testing.T) {
	for _, route := range []struct{ method, suffix string }{
		{http.MethodPut, "/formatter"},
		{http.MethodPost, "/formatter/preview"},
	} {
		for _, body := range []string{"{", "!"} {
			t.Run(route.method+route.suffix+body, func(t *testing.T) {
				router := newProviderAPI(t)
				provider := createProviderAs(t, router, "alice", "Mail", "slack://synthetic")
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx, cancel := context.WithTimeout(r.Context(), 50*time.Millisecond)
					defer cancel()
					router.ServeHTTP(w, r.WithContext(ctx))
				}))
				t.Cleanup(server.Close)
				conn, err := net.Dial("tcp", server.Listener.Addr().String())
				require.NoError(t, err)
				t.Cleanup(func() { _ = conn.Close() })
				require.NoError(t, conn.SetReadDeadline(time.Now().Add(2*time.Second)))
				_, err = fmt.Fprintf(conn, "%s /api/v1/notification-providers/%s%s HTTP/1.1\r\nHost: localhost\r\nX-Test-User: alice\r\nContent-Length: 100\r\n\r\n%s", route.method, provider.ID, route.suffix, body)
				require.NoError(t, err)
				response, err := http.ReadResponse(bufio.NewReader(conn), nil)
				require.NoError(t, err, "body read and unread-body drain must honor the earlier request deadline")
				t.Cleanup(func() { _ = response.Body.Close() })
				require.Equal(t, http.StatusBadRequest, response.StatusCode)
			})
		}
	}
}

func TestFormatterHTTPManagementEstablishesFiveSecondDeadline(t *testing.T) {
	for _, route := range []struct{ method, suffix string }{
		{http.MethodPut, "/formatter"},
		{http.MethodPost, "/formatter/preview"},
	} {
		t.Run(route.suffix, func(t *testing.T) {
			var deadline time.Time
			var bounded bool
			router := newProviderAPI(t, func(c *gin.Context) {
				c.Next()
				deadline, bounded = c.Request.Context().Deadline()
			})
			provider := createProviderAs(t, router, "alice", "Mail", "slack://synthetic")
			started := time.Now()
			response := callAs(t, router, "alice", route.method, "/api/v1/notification-providers/"+provider.ID+route.suffix, "!")
			require.Equal(t, http.StatusBadRequest, response.Code)
			require.True(t, bounded, "management deadline must start before body decoding")
			require.True(t, deadline.After(started))
			require.LessOrEqual(t, time.Until(deadline), 5*time.Second)
		})
	}
}

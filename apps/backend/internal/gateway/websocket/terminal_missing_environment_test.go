package websocket

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/stretchr/testify/require"
)

type unavailableTerminalWorkspace struct {
	lifecycle.WorkspaceInfoProvider
	err error
}

func (p unavailableTerminalWorkspace) GetWorkspaceInfoForEnvironment(context.Context, string) (*lifecycle.WorkspaceInfo, error) {
	return nil, p.err
}

func TestMissingEnvironmentTerminalStopsReconnect(t *testing.T) {
	for _, missing := range []bool{true, false} {
		t.Run(fmt.Sprintf("missing=%t", missing), func(t *testing.T) {
			log := testTerminalLogger(t)
			manager := lifecycle.NewManager(nil, bus.NewMemoryEventBus(log), nil, nil, nil, nil, lifecycle.ExecutorFallbackDeny, t.TempDir(), log)
			workspaceErr := fmt.Errorf("database unavailable")
			if missing {
				workspaceErr = fmt.Errorf("workspace lookup: %w", repoerrors.ErrTaskEnvironmentNotFound)
			}
			manager.SetWorkspaceInfoProvider(unavailableTerminalWorkspace{err: workspaceErr})
			handler := NewTerminalHandler(manager, nil, nil, log)
			router := gin.New()
			router.GET("/terminal/*target", handler.HandleTerminalWS)
			server := httptest.NewServer(router)
			t.Cleanup(server.Close)
			path := "/terminal/environment/missing?terminalId=shell-old"
			conn, response, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http")+path, nil)
			if !missing {
				require.Error(t, err)
				require.Equal(t, http.StatusServiceUnavailable, response.StatusCode)
				require.NoError(t, response.Body.Close())
				return
			}
			require.NoError(t, err, "missing targets must send an explicit non-retryable close")
			t.Cleanup(func() { _ = conn.Close() })
			require.NoError(t, conn.SetReadDeadline(time.Now().Add(time.Second)))
			_, _, err = conn.ReadMessage()
			var closed *gorillaws.CloseError
			require.ErrorAs(t, err, &closed)
			require.Equal(t, gorillaws.CloseNormalClosure, closed.Code)
			require.Equal(t, "environment_not_found", closed.Text)
			plain, err := http.Get(server.URL + path)
			require.NoError(t, err)
			require.Equal(t, http.StatusNotFound, plain.StatusCode)
			require.NoError(t, plain.Body.Close())
		})
	}
}

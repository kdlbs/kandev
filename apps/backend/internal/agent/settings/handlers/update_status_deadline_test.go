package handlers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/httpmw"
)

// slowHandlerRuntimeUpdater makes every latest-version lookup outlast the
// test server's write timeout, standing in for a native check that runs up
// to its own timeout.
type slowHandlerRuntimeUpdater struct {
	handlerRuntimeUpdater
}

func (u *slowHandlerRuntimeUpdater) ResolveTarget(ctx context.Context, name string) (string, error) {
	select {
	case <-time.After(300 * time.Millisecond):
	case <-ctx.Done():
		return "", ctx.Err()
	}
	return u.handlerRuntimeUpdater.ResolveTarget(ctx, name)
}

// The registered status and preview routes must extend their write deadline,
// so a lookup slower than the server-wide write timeout still delivers its
// response instead of a dropped connection.
func TestUpdateStatusRoutesOutliveServerWriteTimeout(t *testing.T) {
	t.Setenv("KANDEV_E2E_MOCK", "true")
	t.Setenv("PATH", t.TempDir())
	router, _, _ := newAgentUpdateRouter(t, &slowHandlerRuntimeUpdater{})
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	for _, path := range []string{"/api/v1/agent-update/status", "/api/v1/agent-update/claude-acp/preview"} {
		request, err := http.NewRequest(http.MethodGet, server.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set(httpmw.InterimSettingsInterlockHeader, "test-interlock")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("GET %s lost its response: %v", path, err)
		}
		body, readErr := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if readErr != nil || response.StatusCode != http.StatusOK || !json.Valid(body) {
			t.Errorf("GET %s = %d %q (read error %v)", path, response.StatusCode, body, readErr)
		}
	}
}

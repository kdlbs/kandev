package handlers

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

// A native update check can legitimately run up to its own timeout, which
// equals the server's default write timeout. The status and preview routes
// must extend their write deadline so the timed-out check's reason reaches the
// client instead of a dropped connection.
func TestUpdateStatusRoutesOutliveServerWriteTimeout(t *testing.T) {
	if controller.UpdateStatusResponseBudget <= 30*time.Second {
		t.Fatalf("response budget %s does not exceed the 30s check timeout plus cleanup", controller.UpdateStatusResponseBudget)
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	slow := func(c *gin.Context) {
		time.Sleep(300 * time.Millisecond)
		c.JSON(http.StatusOK, gin.H{"check_reason": "timeout"})
	}
	router.GET("/extended", func(c *gin.Context) {
		extendUpdateStatusWriteDeadline(c)
		slow(c)
	})
	router.GET("/plain", slow)
	server := httptest.NewUnstartedServer(router)
	server.Config.WriteTimeout = 100 * time.Millisecond
	server.Start()
	t.Cleanup(server.Close)

	response, err := http.Get(server.URL + "/extended")
	if err != nil {
		t.Fatalf("extended route lost its response: %v", err)
	}
	body, _ := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK || string(body) != `{"check_reason":"timeout"}` {
		t.Errorf("extended route = %d %q", response.StatusCode, body)
	}

	if plain, err := http.Get(server.URL + "/plain"); err == nil {
		_ = plain.Body.Close()
		t.Error("control route succeeded; the test server is not enforcing its write timeout")
	}
}

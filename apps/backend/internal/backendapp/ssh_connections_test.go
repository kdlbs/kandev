package backendapp

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	sshhandlers "github.com/kandev/kandev/internal/ssh"
	"github.com/kandev/kandev/internal/sshconnections"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

func TestSSHConnectionsCompositionHonorsFeatureFlag(t *testing.T) {
	cfg := &config.Config{}
	if got := newSSHConnections(cfg, nil); got != nil {
		t.Fatal("disabled manager constructed dependencies")
	}
	cfg.Features.SSHConnectionManager = true
	if got := newSSHConnections(cfg, &Repositories{}); got == nil {
		t.Fatal("enabled manager missing")
	}
}

func TestSSHManagementAndDiscoveryRoutesCoexist(t *testing.T) {
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	sshhandlers.RegisterRoutes(router, ws.NewDispatcher(), nil, nil, nil, nil, log, nil, nil)
	sshconnections.RegisterRoutes(router, sshconnections.NewService(true, nil, nil))
	for _, path := range []string{"/api/v1/ssh/identities", "/api/v1/ssh/managed/identities"} {
		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("%s status = %d, want unauthorized", path, response.Code)
		}
	}
}

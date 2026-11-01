package backendapp

import (
	"fmt"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/runtimeprobe"
)

// registerRuntimeProbeRoute mounts the worker-to-host reachability probe. The
// route answers GET only and authenticates by probe token inside its handler.
func registerRuntimeProbeRoute(router *gin.Engine, probes *runtimeprobe.Registry) {
	if router == nil || probes == nil {
		return
	}
	router.GET(runtimeprobe.PingPath, probes.Handler())
}

// pluginHostAPIURL is the backend's own listener, the base URL a provider
// forwards to the worker. It is empty when the listener port is unknown.
func pluginHostAPIURL(cfg *config.Config) string {
	if cfg == nil || cfg.Server.Port == 0 {
		return ""
	}
	return fmt.Sprintf("http://127.0.0.1:%d", cfg.Server.Port)
}

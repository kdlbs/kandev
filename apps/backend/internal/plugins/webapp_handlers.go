package plugins

import (
	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/plugins/webapp"
)

// RegisterWebAppRuntimeRoutes mounts only the capability-bound static
// runtime. Callers must gate this registration with features.canvases.
func RegisterWebAppRuntimeRoutes(router *gin.Engine, runtime *webapp.Runtime) {
	if router == nil || runtime == nil {
		return
	}
	handler := func(c *gin.Context) {
		runtime.Serve(c.Writer, c.Request, c.Param("token"), c.Param("path"))
	}
	router.GET("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.HEAD("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.OPTIONS("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.POST("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.PATCH("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.PUT("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
	router.DELETE("/api/v1/plugins/web-apps/runtime/:token/*path", handler)
}

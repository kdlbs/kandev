package debug

import (
	"net/http"
	"net/http/pprof"
	"runtime"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
)

// RegisterPprofRoutes mounts the standard net/http/pprof handlers on the Gin router
// at /debug/pprof/. These endpoints are only registered when debug.pprofEnabled is true.
func RegisterPprofRoutes(router *gin.Engine, log *logger.Logger) {
	g := router.Group("/debug/pprof")
	g.GET("/", gin.WrapF(pprof.Index))
	g.GET("/cmdline", gin.WrapF(pprof.Cmdline))
	g.GET("/profile", gin.WrapF(pprof.Profile))
	g.GET("/symbol", gin.WrapF(pprof.Symbol))
	g.POST("/symbol", gin.WrapF(pprof.Symbol))
	g.GET("/trace", gin.WrapF(pprof.Trace))

	for _, name := range []string{"allocs", "block", "goroutine", "heap", "mutex", "threadcreate"} {
		g.GET("/"+name, gin.WrapH(pprof.Handler(name)))
	}

	log.Info("pprof endpoints registered at /debug/pprof/")
}

// RegisterMemoryRoute registers GET /api/v1/debug/memory which returns a JSON summary
// of runtime memory statistics. Useful for quick health checks without pulling a full
// pprof heap profile.
func RegisterMemoryRoute(router *gin.Engine, log *logger.Logger) {
	router.GET("/api/v1/debug/memory", handleMemoryStats())
	log.Info("Memory stats endpoint registered at /api/v1/debug/memory")
}

func handleMemoryStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		var m runtime.MemStats
		runtime.ReadMemStats(&m)

		c.JSON(http.StatusOK, gin.H{
			"heap_alloc_mb":  float64(m.HeapAlloc) / (1024 * 1024),
			"heap_inuse_mb":  float64(m.HeapInuse) / (1024 * 1024),
			"heap_sys_mb":    float64(m.HeapSys) / (1024 * 1024),
			"heap_objects":   m.HeapObjects,
			"goroutines":     runtime.NumGoroutine(),
			"num_gc":         m.NumGC,
			"sys_mb":         float64(m.Sys) / (1024 * 1024),
			"stack_inuse_mb": float64(m.StackInuse) / (1024 * 1024),
		})
	}
}

package plugins

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/marketplace"
)

// registerMarketplaceRoutes wires the marketplace catalog + source-management
// surface onto the /api/plugins group. Kept separate from RegisterRoutes so
// the marketplace HTTP surface stays self-contained. guard is the per-boot
// operator-token middleware (see RegisterRoutes); every mutating route is
// gated by it, while the read-only catalog/source lists stay open.
func (c *Controller) registerMarketplaceRoutes(api *gin.RouterGroup, guard gin.HandlerFunc) {
	api.GET("/marketplace", c.marketplaceCatalog)
	api.POST("/marketplace/refresh", guard, c.marketplaceRefresh)
	api.GET("/marketplace/sources", c.listSources)
	api.POST("/marketplace/sources", guard, c.addSource)
	api.PATCH("/marketplace/sources/:sid", guard, c.updateSource)
	api.DELETE("/marketplace/sources/:sid", guard, c.deleteSource)
}

// addSourceRequest is the POST /api/plugins/marketplace/sources body.
type addSourceRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

// updateSourceRequest is the PATCH body; nil fields are left unchanged.
type updateSourceRequest struct {
	Name    *string `json:"name"`
	Enabled *bool   `json:"enabled"`
}

// mkt returns the attached marketplace service, writing a 503 and returning
// nil when the subsystem is unavailable.
func (c *Controller) mkt(ctx *gin.Context) *marketplace.Service {
	m := c.svc.Marketplace()
	if m == nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "marketplace is unavailable"})
		return nil
	}
	return m
}

// marketplaceCatalog serves GET /api/plugins/marketplace: the merged catalog
// across all enabled sources, filtered/sorted by the query params
// (?q, ?category, ?sort), each entry annotated with install_state.
func (c *Controller) marketplaceCatalog(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	result, err := m.Catalog(ctx.Request.Context(), c.svc.InstalledForMarketplace())
	if err != nil {
		c.log.Warn("marketplace catalog error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	result.Plugins = marketplace.ApplyQuery(result.Plugins, marketplace.Query{
		Text:     ctx.Query("q"),
		Category: ctx.Query("category"),
		Sort:     ctx.Query("sort"),
	})
	ctx.JSON(http.StatusOK, result)
}

// marketplaceRefresh serves POST /api/plugins/marketplace/refresh: drops the
// cached index documents so the next catalog fetch re-hits every source.
func (c *Controller) marketplaceRefresh(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	m.Refresh()
	ctx.JSON(http.StatusOK, gin.H{"refreshed": true})
}

func (c *Controller) listSources(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	sources, err := m.Sources()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"sources": sources})
}

func (c *Controller) addSource(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	var req addSourceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	rec, err := m.AddSource(req.Name, req.URL)
	if err != nil {
		if errors.Is(err, marketplace.ErrDuplicateSource) {
			ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		// Remaining Add errors are input validation (bad/empty URL) with clean,
		// non-leaking messages.
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, rec)
}

func (c *Controller) updateSource(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	var req updateSourceRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	rec, err := m.UpdateSource(ctx.Param("sid"), req.Name, req.Enabled)
	if err != nil {
		c.writeSourceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, rec)
}

func (c *Controller) deleteSource(ctx *gin.Context) {
	m := c.mkt(ctx)
	if m == nil {
		return
	}
	if err := m.DeleteSource(ctx.Param("sid")); err != nil {
		c.writeSourceError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

// writeSourceError maps source CRUD errors to HTTP statuses: unknown id -> 404,
// deleting the built-in source -> 409, anything else -> 500.
func (c *Controller) writeSourceError(ctx *gin.Context, err error) {
	switch {
	case errors.Is(err, marketplace.ErrSourceNotFound):
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	case errors.Is(err, marketplace.ErrBuiltinImmutable):
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	default:
		c.log.Warn("marketplace source error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

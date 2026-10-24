package plugins

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/themes"
)

func (c *Controller) registerThemeRoutes(api *gin.RouterGroup) {
	api.GET("/themes", func(ctx *gin.Context) { ctx.JSON(http.StatusOK, c.svc.ThemeInventory()) })
	api.GET("/themes/schema/v1", func(ctx *gin.Context) { ctx.Data(http.StatusOK, "application/schema+json", themes.Schema) })
	api.GET("/themes/:packageId/:key", c.themeContent)
	api.POST("/themes/prepare", authn.RequireAdmin(), c.prepareTheme)
	api.POST("/themes/commit", authn.RequireAdmin(), c.commitTheme)
}

func (c *Controller) themeContent(ctx *gin.Context) {
	data, err := c.svc.ThemeContent(ctx.Param("packageId"), ctx.Param("key"), ctx.Query("digest"))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "theme_unavailable"})
		return
	}
	ctx.Header("Cache-Control", "no-store")
	ctx.Data(http.StatusOK, "application/json", data)
}

func (c *Controller) prepareTheme(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, pkgtar.MaxThemePackageBytes+(1<<20))
	data, base, expected, err := c.themeUpload(ctx)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	review, err := c.svc.PrepareTheme(ctx.Request.Context(), data, base)
	if err == nil && expected != nil && (review.Manifest.ID != expected.ID || review.Manifest.Version != expected.Version || review.Digest != strings.ToLower(expected.Digest)) {
		c.svc.themeReviews.mu.Lock()
		delete(c.svc.themeReviews.entries, review.ID)
		c.svc.themeReviews.mu.Unlock()
		err = fmt.Errorf("theme_identity: archive does not match selected release")
	}
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, review)
}

type themeURLRequest struct {
	URL     string `json:"url"`
	ID      string `json:"id"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (c *Controller) themeUpload(ctx *gin.Context) ([]byte, string, *themeURLRequest, error) {
	if strings.HasPrefix(ctx.GetHeader("Content-Type"), "application/json") {
		var request themeURLRequest
		if err := ctx.ShouldBindJSON(&request); err != nil {
			return nil, "", nil, err
		}
		data, err := c.svc.downloadTheme(ctx.Request.Context(), themeRelease(request))
		return data, "", &request, err
	}
	file, _, err := ctx.Request.FormFile("package")
	if ctx.Request.MultipartForm != nil {
		defer func() { _ = ctx.Request.MultipartForm.RemoveAll() }()
	}
	if err != nil {
		return nil, "", nil, err
	}
	defer func() { _ = file.Close() }()
	data, err := io.ReadAll(io.LimitReader(file, pkgtar.MaxThemePackageBytes+1))
	if len(data) > pkgtar.MaxThemePackageBytes {
		return nil, "", nil, fmt.Errorf("theme_size: package limit exceeded")
	}
	return data, ctx.PostForm("base"), nil, err
}

func (c *Controller) commitTheme(ctx *gin.Context) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 4096)
	var request struct {
		ID string `json:"id"`
	}
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "theme_review_id_required"})
		return
	}
	rec, err := c.svc.CommitTheme(ctx.Request.Context(), request.ID)
	if err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, ErrThemeReviewConflict) {
			code = http.StatusConflict
		}
		if errors.Is(err, ErrThemeReviewExpired) {
			code = http.StatusGone
		}
		ctx.JSON(code, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, InstallResponse{Plugin: rec})
}

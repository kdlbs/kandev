package backendapp

import (
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	canvasservice "github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

func registerCanvasDistributionRoutes(router *gin.Engine, handler *canvasHTTPHandler) {
	if router == nil || handler == nil || handler.distribution == nil {
		return
	}
	router.POST("/api/v1/canvases/:canvasID/exports", handler.prepareCanvasExport)
	router.GET("/api/v1/canvases/exports/:preparationID", handler.getCanvasExport)
	router.GET("/api/v1/canvases/exports/:preparationID/bundle", handler.downloadCanvasBundle)
	router.GET("/api/v1/canvases/exports/:preparationID/source", handler.downloadCanvasSource)
	router.DELETE("/api/v1/canvases/exports/:preparationID", handler.cancelCanvasExport)
	router.POST("/api/v1/canvases/install-preparations", handler.prepareCanvasInstall)
	router.GET("/api/v1/canvases/install-preparations/:preparationID", handler.getCanvasInstall)
	router.POST("/api/v1/canvases/install-preparations/:preparationID/confirm", handler.confirmCanvasInstall)
	router.DELETE("/api/v1/canvases/install-preparations/:preparationID", handler.cancelCanvasInstall)
}

type canvasExportRequest struct {
	WorkspaceID       string                       `json:"workspace_id"`
	ExpectedReleaseID string                       `json:"expected_release_id"`
	Metadata          canvasservice.ExportMetadata `json:"metadata"`
	PackageID         string                       `json:"package_id,omitempty"`
	Version           string                       `json:"version,omitempty"`
	DisplayName       string                       `json:"display_name,omitempty"`
	Description       string                       `json:"description,omitempty"`
	Author            string                       `json:"author,omitempty"`
	License           string                       `json:"license,omitempty"`
	SourceMode        string                       `json:"source_mode,omitempty"`
	MinKandevVersion  string                       `json:"min_kandev_version,omitempty"`
	RepoURL           string                       `json:"repo_url,omitempty"`
}

type canvasInstallRequest struct {
	WorkspaceID     string `json:"workspace_id"`
	OriginKind      string `json:"origin_kind,omitempty"`
	SourceID        string `json:"source_id,omitempty"`
	RepositoryURL   string `json:"repository_url,omitempty"`
	PackageID       string `json:"package_id,omitempty"`
	ExpectedVersion string `json:"expected_version,omitempty"`
	ExpectedDigest  string `json:"expected_sha256,omitempty"`
	BundleURL       string `json:"bundle_url,omitempty"`
}

func (r canvasInstallRequest) serviceRequest(userID string, bundle []byte) canvasservice.InstallRequest {
	return canvasservice.InstallRequest{
		UserID: userID, WorkspaceID: strings.TrimSpace(r.WorkspaceID), OriginKind: strings.TrimSpace(r.OriginKind),
		SourceID: strings.TrimSpace(r.SourceID), RepositoryURL: strings.TrimSpace(r.RepositoryURL),
		PackageID: strings.TrimSpace(r.PackageID), ExpectedVersion: strings.TrimSpace(r.ExpectedVersion),
		ExpectedArchiveDigest: strings.TrimSpace(r.ExpectedDigest), Bundle: bundle,
	}
}

type canvasInstallConfirmationRequest struct {
	canvasservice.InstallConfirmation
}

func (r canvasExportRequest) metadata() canvasservice.ExportMetadata {
	metadata := r.Metadata
	if metadata.PackageID == "" {
		metadata = canvasservice.ExportMetadata{
			PackageID: r.PackageID, Version: r.Version, DisplayName: r.DisplayName,
			Description: r.Description, Author: r.Author, License: r.License,
			SourceMode: r.SourceMode, MinKandevVersion: r.MinKandevVersion, RepoURL: r.RepoURL,
		}
	}
	return metadata
}

func (h *canvasHTTPHandler) prepareCanvasExport(c *gin.Context) {
	var request canvasExportRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	request.WorkspaceID = strings.TrimSpace(request.WorkspaceID)
	if request.WorkspaceID == "" || strings.TrimSpace(request.ExpectedReleaseID) == "" {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	review, err := h.distribution.PrepareExport(c.Request.Context(), canvasservice.ExportRequest{
		UserID: canvasRuntimeUser(c), WorkspaceID: request.WorkspaceID, CanvasID: c.Param("canvasID"),
		ExpectedReleaseID: request.ExpectedReleaseID, Metadata: request.metadata(),
	})
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, review)
}

func (h *canvasHTTPHandler) getCanvasExport(c *gin.Context) {
	review, err := h.distribution.GetExport(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID"))
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, review)
}

func (h *canvasHTTPHandler) downloadCanvasBundle(c *gin.Context) {
	h.downloadCanvasExport(c, canvasservice.ExportBundle)
}

func (h *canvasHTTPHandler) downloadCanvasSource(c *gin.Context) {
	h.downloadCanvasExport(c, canvasservice.ExportSource)
}

func (h *canvasHTTPHandler) downloadCanvasExport(c *gin.Context, kind canvasservice.ExportKind) {
	download, err := h.distribution.DownloadExport(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID"), kind)
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Content-Disposition", `attachment; filename="`+download.Filename+`"`)
	c.Data(http.StatusOK, download.ContentType, download.Data)
}

func (h *canvasHTTPHandler) cancelCanvasExport(c *gin.Context) {
	if err := h.distribution.CancelExport(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID")); err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *canvasHTTPHandler) prepareCanvasInstall(c *gin.Context) {
	if strings.HasPrefix(c.ContentType(), "multipart/form-data") {
		h.prepareCanvasInstallUpload(c)
		return
	}
	var request canvasInstallRequest
	if err := c.ShouldBindJSON(&request); err != nil || (strings.TrimSpace(request.BundleURL) == "" && (strings.TrimSpace(request.SourceID) == "" || strings.TrimSpace(request.PackageID) == "" || strings.TrimSpace(request.ExpectedVersion) == "" || strings.TrimSpace(request.ExpectedDigest) == "")) {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	if strings.TrimSpace(request.WorkspaceID) == "" {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	serviceRequest := request.serviceRequest(canvasRuntimeUser(c), nil)
	var review canvasservice.InstallReview
	var err error
	if strings.TrimSpace(request.BundleURL) != "" {
		review, err = h.distribution.PrepareInstallFromURL(c.Request.Context(), serviceRequest, request.BundleURL)
	} else {
		review, err = h.distribution.PrepareInstallFromCatalog(c.Request.Context(), serviceRequest)
	}
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, review)
}

func (h *canvasHTTPHandler) prepareCanvasInstallUpload(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, webapp.MaxCompressedBytes+1<<20)
	fileHeader, err := c.FormFile("package")
	if err != nil || fileHeader.Size > webapp.MaxCompressedBytes {
		writeCanvasDistributionError(c, webapp.ErrCompressedTooLarge)
		return
	}
	file, err := fileHeader.Open()
	if err != nil {
		writeCanvasDistributionError(c, canvasservice.ErrInstallInvalid)
		return
	}
	defer func() { _ = file.Close() }()
	bundle, err := io.ReadAll(io.LimitReader(file, webapp.MaxCompressedBytes+1))
	if err != nil || int64(len(bundle)) > webapp.MaxCompressedBytes {
		writeCanvasDistributionError(c, webapp.ErrCompressedTooLarge)
		return
	}
	request := canvasInstallRequest{
		WorkspaceID: c.PostForm("workspace_id"), OriginKind: "upload", SourceID: c.PostForm("source_id"),
		RepositoryURL: c.PostForm("repository_url"), PackageID: c.PostForm("package_id"),
		ExpectedVersion: c.PostForm("expected_version"), ExpectedDigest: c.PostForm("expected_sha256"),
	}
	if strings.TrimSpace(request.WorkspaceID) == "" {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	review, err := h.distribution.PrepareInstall(c.Request.Context(), request.serviceRequest(canvasRuntimeUser(c), bundle))
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, review)
}

func (h *canvasHTTPHandler) getCanvasInstall(c *gin.Context) {
	review, err := h.distribution.GetInstallReview(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID"))
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, review)
}

func (h *canvasHTTPHandler) confirmCanvasInstall(c *gin.Context) {
	var request canvasInstallConfirmationRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		writeCanvasError(c, http.StatusBadRequest, "invalid_request", nil)
		return
	}
	result, err := h.distribution.ConfirmInstall(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID"), request.InstallConfirmation)
	if err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	writeCanvasJSON(c, http.StatusOK, result)
}

func (h *canvasHTTPHandler) cancelCanvasInstall(c *gin.Context) {
	if err := h.distribution.CancelInstall(c.Request.Context(), canvasRuntimeUser(c), c.Param("preparationID")); err != nil {
		writeCanvasDistributionError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func writeCanvasDistributionError(c *gin.Context, err error) {
	status, code := canvasDistributionErrorStatus(err)
	writeCanvasError(c, status, code, nil)
}

func canvasDistributionErrorStatus(err error) (int, string) {
	switch {
	case errors.Is(err, canvasservice.ErrCanvasNotFound):
		return http.StatusNotFound, "canvas_not_found"
	case errors.Is(err, canvasservice.ErrPreparationNotFound):
		return http.StatusNotFound, "review_not_found"
	case errors.Is(err, canvasservice.ErrPreparationExpired):
		return http.StatusConflict, "review_expired"
	case errors.Is(err, canvasservice.ErrExportStale):
		return http.StatusConflict, "review_stale"
	case errors.Is(err, canvasservice.ErrExportInvalid):
		return http.StatusBadRequest, "invalid_request"
	case isCanvasInvalidInstallError(err):
		return http.StatusBadRequest, "invalid_install"
	case errors.Is(err, canvasservice.ErrInstallIncompatible):
		return http.StatusConflict, "incompatible_install"
	case errors.Is(err, canvasservice.ErrInstallURLBlocked):
		return http.StatusBadRequest, "install_url_blocked"
	case errors.Is(err, webapp.ErrCompressedTooLarge):
		return http.StatusRequestEntityTooLarge, "package_too_large"
	case errors.Is(err, canvasservice.ErrInstallUnavailable):
		return http.StatusServiceUnavailable, "install_unavailable"
	case errors.Is(err, canvasservice.ErrInstallReceiptNotFound):
		return http.StatusNotFound, "install_not_found"
	case errors.Is(err, canvasservice.ErrPreparationLimit):
		return http.StatusConflict, "storage_limit_reached"
	case isCanvasSourceUnavailableError(err):
		return http.StatusConflict, "source_unavailable"
	default:
		return http.StatusInternalServerError, "internal_error"
	}
}

func isCanvasInvalidInstallError(err error) bool {
	return errors.Is(err, canvasservice.ErrInstallInvalid) ||
		errors.Is(err, canvasservice.ErrInstallNotApproved) ||
		errors.Is(err, canvasservice.ErrPackageDigestMismatch) ||
		errors.Is(err, canvasservice.ErrInstallURLInvalid)
}

func isCanvasSourceUnavailableError(err error) bool {
	return errors.Is(err, webapp.ErrSourceUnavailable) || errors.Is(err, webapp.ErrArtifactUnavailable)
}

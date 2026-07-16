package plugins

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// maxWebhookBodyBytes caps the request body the webhook relay
// (POST/GET /api/plugins/:id/webhooks/:key) will read before relaying it to
// a plugin's live subprocess. Without a cap, io.ReadAll(ctx.Request.Body)
// is an unbounded read an external, unauthenticated webhook caller could
// use to exhaust backend memory. 4 MiB comfortably covers realistic webhook
// payloads (GitHub/Slack/Jira event bodies are KB-sized) while bounding
// worst-case memory use per request.
const maxWebhookBodyBytes = 4 << 20 // 4 MiB

// Controller holds the plugin HTTP handlers: operator-facing management
// (install/list/get/config/uninstall/enable/disable), the tools listing,
// the bundle/UI static-file serving (from the extracted package on disk),
// and the external webhook relay (HTTP -> Host RPC over the live
// subprocess).
type Controller struct {
	svc *Service
	log *logger.Logger
}

// RegisterRoutes wires the plugin HTTP surface. deliverer is accepted for
// parity with the backendapp wiring (svc.SetDeliverer(deliverer) happens
// alongside this call) — no handler in this file calls it directly, since
// Service already notifies it on every install/status change.
func RegisterRoutes(router *gin.Engine, svc *Service, _ Deliverer, log *logger.Logger) {
	ctrl := &Controller{svc: svc, log: log}

	api := router.Group("/api/plugins")
	api.POST("/install", ctrl.install)
	api.POST("/sync", ctrl.sync)
	api.GET("", ctrl.list)
	api.GET("/tools", ctrl.listTools)
	api.GET("/:id", ctrl.get)
	api.PATCH("/:id", ctrl.updateConfig)
	api.DELETE("/:id", ctrl.uninstall)
	api.POST("/:id/enable", ctrl.enable)
	api.POST("/:id/disable", ctrl.disable)

	api.GET("/:id/bundle", ctrl.bundle)
	api.GET("/:id/ui/*path", ctrl.ui)
	api.POST("/:id/webhooks/:key", ctrl.webhook)
	api.GET("/:id/webhooks/:key", ctrl.webhook)
}

// --- Management ---

// install serves POST /api/plugins/install: JSON {"url": "..."} or a
// multipart/form-data upload with a "package" field.
func (c *Controller) install(ctx *gin.Context) {
	rec, err := c.installFromRequest(ctx)
	if err != nil {
		if rec == nil {
			c.writeInstallError(ctx, err)
			return
		}
		ctx.JSON(http.StatusCreated, InstallResponse{Plugin: rec, Warning: err.Error()})
		return
	}
	ctx.JSON(http.StatusCreated, InstallResponse{Plugin: rec})
}

// installFromRequest dispatches to Service.Install (multipart upload) or
// Service.InstallFromURL (JSON body), based on the request's Content-Type.
func (c *Controller) installFromRequest(ctx *gin.Context) (*store.Record, error) {
	if strings.HasPrefix(ctx.ContentType(), "multipart/form-data") {
		fileHeader, err := ctx.FormFile("package")
		if err != nil {
			return nil, errBadRequest("missing multipart field \"package\"")
		}
		f, err := fileHeader.Open()
		if err != nil {
			return nil, errBadRequest("failed to read uploaded package")
		}
		defer func() { _ = f.Close() }()
		return c.svc.Install(ctx.Request.Context(), f)
	}

	var req InstallRequest
	if err := ctx.ShouldBindJSON(&req); err != nil || req.URL == "" {
		return nil, errBadRequest("invalid payload: url required (or a multipart \"package\" upload)")
	}
	return c.svc.InstallFromURL(ctx.Request.Context(), req.URL)
}

// errBadRequest is a sentinel-ish wrapper writeInstallError recognizes to
// always map to 400, for installFromRequest's own input-validation errors
// (as opposed to pkgtar's package-content errors).
type errBadRequest string

func (e errBadRequest) Error() string { return string(e) }

// writeInstallError maps an Install/InstallFromURL error to the right HTTP
// status: pkgtar.ErrVersionExists -> 409 (matches the frozen contract's
// "ErrVersionExists -> 409 semantics"), every other pkgtar validation
// error and errBadRequest -> 400, anything else -> 500.
func (c *Controller) writeInstallError(ctx *gin.Context, err error) {
	var badReq errBadRequest
	switch {
	case errors.Is(err, pkgtar.ErrVersionExists):
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
	case errors.As(err, &badReq),
		errors.Is(err, pkgtar.ErrManifestInvalid),
		errors.Is(err, pkgtar.ErrMissingChecksums),
		errors.Is(err, pkgtar.ErrUnlistedFile),
		errors.Is(err, pkgtar.ErrChecksumMismatch),
		errors.Is(err, pkgtar.ErrPathTraversal),
		errors.Is(err, pkgtar.ErrPlatformNotSupported):
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	default:
		c.log.Warn("plugin install error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	}
}

// sync serves POST /api/plugins/sync: runs Service.Sync (dir sideloads,
// dropped tarballs, missing-install detection) and returns the resulting
// SyncResult.
func (c *Controller) sync(ctx *gin.Context) {
	result, err := c.svc.Sync(ctx.Request.Context())
	if err != nil {
		c.log.Warn("plugin sync error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, result)
}

func (c *Controller) list(ctx *gin.Context) {
	ctx.JSON(http.StatusOK, gin.H{"plugins": c.svc.List()})
}

func (c *Controller) get(ctx *gin.Context) {
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, record)
}

func (c *Controller) updateConfig(ctx *gin.Context) {
	var req UpdateConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.svc.UpdateConfig(ctx.Param("id"), req.Config); err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"updated": true})
}

func (c *Controller) uninstall(ctx *gin.Context) {
	if err := c.svc.Uninstall(ctx.Param("id")); err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"deleted": true})
}

func (c *Controller) enable(ctx *gin.Context) {
	if err := c.svc.Enable(ctx.Param("id")); err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"enabled": true})
}

func (c *Controller) disable(ctx *gin.Context) {
	if err := c.svc.Disable(ctx.Param("id")); err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"disabled": true})
}

// writeLookupError maps common Service errors to HTTP status codes shared
// by most management handlers.
func (c *Controller) writeLookupError(ctx *gin.Context, err error) {
	if errors.Is(err, store.ErrNotFound) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var invalidErr *ErrInvalidTransition
	if errors.As(err, &invalidErr) {
		ctx.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	c.log.Warn("plugin handler error", zap.Error(err))
	ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// --- Tools listing ---

// listTools aggregates the declared tools of every StatusActive plugin
// (GET /api/plugins/tools). Listing only — wiring these into the agent's
// invocable tool set is out of scope for this task.
func (c *Controller) listTools(ctx *gin.Context) {
	var tools []PluginToolDTO
	for _, rec := range c.svc.List() {
		if rec.Status != StatusActive {
			continue
		}
		for _, tool := range rec.Tools {
			tools = append(tools, PluginToolDTO{
				PluginID:    rec.ID,
				Name:        tool.Name,
				DisplayName: tool.DisplayName,
				Description: tool.Description,
				InputSchema: tool.InputSchema,
			})
		}
	}
	if tools == nil {
		tools = []PluginToolDTO{}
	}
	ctx.JSON(http.StatusOK, gin.H{"tools": tools})
}

// --- Bundle / UI static file serving ---

// activeRecord resolves id to a StatusActive plugin record, writing the
// appropriate error response and returning ok=false otherwise. Bundle/UI
// serving only applies to active plugins: there's no extracted-and-running
// process to trust the files of otherwise (disabled/error/uninstalled).
func (c *Controller) activeRecord(ctx *gin.Context) (*store.Record, bool) {
	record, err := c.svc.Get(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return nil, false
	}
	if record.Status != StatusActive {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "plugin is not active"})
		return nil, false
	}
	return record, true
}

// bundle serves GET /api/plugins/:id/bundle: the plugin's declared
// ui.bundle file, read from disk under rec.InstallPath, forcing
// Content-Type: text/javascript so the SPA's dynamic import() always sees
// a JS module.
func (c *Controller) bundle(ctx *gin.Context) {
	record, ok := c.activeRecord(ctx)
	if !ok {
		return
	}
	if record.UI.Bundle == "" {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "plugin has no UI bundle"})
		return
	}
	serveInstalledFile(ctx, record.InstallPath, record.UI.Bundle, "text/javascript; charset=utf-8")
}

// ui serves GET /api/plugins/:id/ui/*path: the remainder of the path,
// verbatim, from disk under rec.InstallPath (mirrors ui.bundle/ui.styles'
// root-relative path convention — e.g. requesting ".../ui/ui/style.css"
// resolves rec.InstallPath + "/ui/style.css", the manifest's declared
// ui.styles entry).
func (c *Controller) ui(ctx *gin.Context) {
	record, ok := c.activeRecord(ctx)
	if !ok {
		return
	}
	subPath := ctx.Param("path")
	if subPath == "" {
		subPath = "/"
	}
	serveInstalledFile(ctx, record.InstallPath, subPath, "")
}

// serveInstalledFile serves relPath from disk under root via
// http.FileServer(http.Dir(root)), which rejects ".."-containing paths
// itself (net/http.Dir.Open cleans the path and refuses to escape root) —
// this is the "path-traversal safe via http.FileServer on a rooted FS"
// requirement. contentType, if non-empty, is set before serving so
// FileServer's extension-based sniffing is skipped (net/http only
// auto-detects when the header is unset).
func serveInstalledFile(ctx *gin.Context, root, relPath, contentType string) {
	if contentType != "" {
		ctx.Writer.Header().Set("Content-Type", contentType)
	}
	req := ctx.Request.Clone(ctx.Request.Context())
	req.URL.Path = relPath
	http.FileServer(http.Dir(root)).ServeHTTP(ctx.Writer, req)
}

// --- External webhook relay ---

// webhook serves POST/GET /api/plugins/:id/webhooks/:key: validates :key
// against the plugin's manifest-declared webhooks (404 for an undeclared
// key — this endpoint must not blindly relay an arbitrary caller-supplied
// key to the subprocess), reads the body capped at maxWebhookBodyBytes (413
// if exceeded), builds a pluginsdk.WebhookRequest from the inbound HTTP
// request, relays it to the plugin's live subprocess via
// Service.InvokeWebhook, then writes back the plugin's WebhookResponse
// verbatim.
func (c *Controller) webhook(ctx *gin.Context) {
	id := ctx.Param("id")
	record, err := c.svc.Get(id)
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}

	key := ctx.Param("key")
	if !manifestDeclaresWebhookKey(record, key) {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %q has no webhook %q", id, key)})
		return
	}

	body, err := readCappedWebhookBody(ctx)
	if err != nil {
		return // readCappedWebhookBody already wrote the error response
	}

	req := &pluginsdk.WebhookRequest{
		WebhookKey: key,
		Method:     ctx.Request.Method,
		Query:      ctx.Request.URL.RawQuery,
		Headers:    flattenHeaders(ctx.Request.Header),
		Body:       body,
	}

	resp, err := c.svc.InvokeWebhook(ctx.Request.Context(), id, req)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	writeWebhookResponse(ctx, resp)
}

// webhookStatusForResponse validates a plugin-supplied WebhookResponse.Status
// against the HTTP status code range net/http's WriteHeader accepts
// ([100, 599] — RFC 9110 informational/success/redirection/client
// error/server error classes). ok is false for anything outside that range,
// which gin's ResponseWriter.WriteHeader panics on (recovered into a bare
// 500 with no useful body by gin's recovery middleware) — a single
// misbehaving plugin should get a clear 502 instead of taking down the
// whole request with a panic.
func webhookStatusForResponse(status int32) (int, bool) {
	if status < 100 || status > 599 {
		return 0, false
	}
	return int(status), true
}

// writeWebhookResponse turns a plugin's WebhookResponse into the outbound
// HTTP response: an out-of-range Status is rejected as 502 (see
// webhookStatusForResponse) before ever reaching ctx.Writer.WriteHeader;
// otherwise headers, status, and body are relayed verbatim.
func writeWebhookResponse(ctx *gin.Context, resp *pluginsdk.WebhookResponse) {
	status, ok := webhookStatusForResponse(resp.Status)
	if !ok {
		ctx.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("plugin returned invalid webhook status %d", resp.Status),
		})
		return
	}
	for k, v := range resp.Headers {
		ctx.Writer.Header().Set(k, v)
	}
	ctx.Writer.WriteHeader(status)
	_, _ = ctx.Writer.Write(resp.Body)
}

// manifestDeclaresWebhookKey reports whether record's manifest declares a
// webhooks[] entry with the given key.
func manifestDeclaresWebhookKey(record *store.Record, key string) bool {
	for _, wh := range record.Webhooks {
		if wh.Key == key {
			return true
		}
	}
	return false
}

// readCappedWebhookBody reads ctx.Request.Body bounded at
// maxWebhookBodyBytes via http.MaxBytesReader, writing the 413 response
// itself (and returning a non-nil error as a sentinel to the caller) when
// the body exceeds the cap, so a single external webhook POST cannot
// exhaust backend memory via an unbounded io.ReadAll.
func readCappedWebhookBody(ctx *gin.Context) ([]byte, error) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxWebhookBodyBytes)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("webhook body exceeds max size of %d bytes", maxWebhookBodyBytes),
			})
			return nil, err
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "failed to read request body"})
		return nil, err
	}
	return body, nil
}

// flattenHeaders converts a net/http.Header (map[string][]string) into the
// single-valued map[string]string WebhookRequest.Headers expects,
// per §3 of docs/plans/plugins/GRPC-CONTRACT.md: multi-valued headers are
// joined by ", ".
func flattenHeaders(h http.Header) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		out[k] = strings.Join(v, ", ")
	}
	return out
}

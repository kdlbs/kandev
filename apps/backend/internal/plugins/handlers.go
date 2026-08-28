package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth"
	"github.com/kandev/kandev/internal/auth/authn"
	commonhttpmw "github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/plugins/manifest"
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
const maxWebhookBodyBytes = manifest.DefaultWebhookMaxBodyBytes

const (
	// maxPluginActionEnvelopeBytes bounds the complete browser request before
	// its declared action-specific body cap is available. The small allowance
	// covers the resource selectors and JSON envelope around the largest legal
	// action body without allowing arbitrary envelope growth.
	maxPluginActionEnvelopeBytes = manifest.MaxActionBodyBytes + 4096
	maxPluginActionResponseBytes = 1 << 20 // 1 MiB
	contentTypeHeader            = "Content-Type"
)

var pluginActionTimeout = 15 * time.Second

// actionInvoker is deliberately narrower than Service so the HTTP boundary
// can be tested without a subprocess while production dispatch remains the
// Service's runtime-mediated RPC call.
type actionInvoker interface {
	InvokeAction(
		context.Context, string, pluginDispatchGeneration, *pluginsdk.PluginActionRequest,
	) (*pluginsdk.PluginActionResponse, error)
}

type webhookInvoker interface {
	InvokeWebhook(context.Context, string, *pluginsdk.WebhookRequest) (*pluginsdk.WebhookResponse, error)
}

// Controller holds the plugin HTTP handlers: operator-facing management
// (install/list/get/config/uninstall/enable/disable), the bundle/UI
// static-file serving (from the extracted package on disk), and the
// external webhook relay (HTTP -> Host RPC over the live subprocess).
type Controller struct {
	svc            *Service
	log            *logger.Logger
	actionInvoker  actionInvoker
	webhookInvoker webhookInvoker
}

// RegisterRoutes wires the plugin HTTP surface. deliverer is accepted for
// parity with the backendapp wiring (svc.SetDeliverer(deliverer) happens
// alongside this call) — no handler in this file calls it directly, since
// Service already notifies it on every install/status change.
func RegisterRoutes(router *gin.Engine, svc *Service, _ Deliverer, log *logger.Logger) {
	ctrl := &Controller{svc: svc, log: log, actionInvoker: svc, webhookInvoker: svc}

	api := router.Group("/api/plugins")
	api.POST("/install", authn.RequireAdmin(), ctrl.install)
	api.POST("/sync", authn.RequireAdmin(), ctrl.sync)
	// Register the static /marketplace and /settings routes before the /:id
	// wildcard, matching the /install and /sync ordering — some gin/httprouter
	// tree versions reject a static sibling added after an existing wildcard for
	// the same method.
	ctrl.registerMarketplaceRoutes(api)
	api.GET("/settings", ctrl.getSettings)
	api.PUT("/settings", authn.RequireAdmin(), ctrl.updateSettings)
	api.GET("", ctrl.list)
	api.GET("/:id", ctrl.get)
	api.GET("/:id/config", ctrl.getConfig)
	api.PATCH("/:id", authn.RequireAdmin(), ctrl.updateConfig)
	api.PUT("/:id/auto-update", authn.RequireAdmin(), ctrl.setAutoUpdate)
	api.DELETE("/:id", authn.RequireAdmin(), ctrl.uninstall)
	api.POST("/:id/enable", authn.RequireAdmin(), ctrl.enable)
	api.POST("/:id/disable", authn.RequireAdmin(), ctrl.disable)

	api.GET("/:id/bundle", ctrl.bundle)
	api.GET("/:id/ui/*path", ctrl.ui)
	api.POST("/:id/actions/:key", ctrl.action)
	// Registered before the /:id/webhooks/:key wildcard for the same reason
	// /settings is registered before /:id above: some gin/httprouter tree
	// versions reject a static-ish sibling added after an existing wildcard.
	registerUserStateRoutes(api, ctrl)
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

// getConfig serves GET /api/plugins/:id/config: the stored operator config
// with secret values (per the manifest's config_schema) masked — cleartext
// secrets never leave the backend on this surface.
func (c *Controller) getConfig(ctx *gin.Context) {
	config, err := c.svc.GetMaskedConfig(ctx.Param("id"))
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"config": config})
}

func (c *Controller) updateConfig(ctx *gin.Context) {
	var req UpdateConfigRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.svc.UpdateConfig(ctx.Request.Context(), ctx.Param("id"), req.Config); err != nil {
		if errors.Is(err, ErrConfigInvalid) {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"updated": true})
}

func (c *Controller) uninstall(ctx *gin.Context) {
	if err := c.svc.Uninstall(ctx.Request.Context(), ctx.Param("id")); err != nil {
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

// --- Auto-update settings ---

// getSettings serves GET /api/plugins/settings: the instance-wide plugin
// preferences (currently just the auto-update default).
func (c *Controller) getSettings(ctx *gin.Context) {
	def, err := c.svc.AutoUpdateDefault()
	if err != nil {
		c.log.Warn("plugin settings read error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, Settings{AutoUpdateDefault: def})
}

// updateSettings serves PUT /api/plugins/settings: sets the instance-wide
// auto-update default. Turning it on opts every plugin without a per-plugin
// override into auto-update.
func (c *Controller) updateSettings(ctx *gin.Context) {
	var req UpdateSettingsRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := c.svc.SetAutoUpdateDefault(req.AutoUpdateDefault); err != nil {
		c.log.Warn("plugin settings write error", zap.Error(err))
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	ctx.JSON(http.StatusOK, Settings(req))
}

// setAutoUpdate serves PUT /api/plugins/:id/auto-update: sets or clears the
// per-plugin auto-update override. A null/omitted auto_update clears the
// override so the plugin inherits the instance-wide default; true/false force
// it on/off for this plugin.
func (c *Controller) setAutoUpdate(ctx *gin.Context) {
	var req SetAutoUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	rec, err := c.svc.SetPluginAutoUpdate(ctx.Param("id"), req.AutoUpdate)
	if err != nil {
		c.writeLookupError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, rec)
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
		ctx.Writer.Header().Set(contentTypeHeader, contentType)
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
	id, key := ctx.Param("id"), ctx.Param("key")
	record, lookupErr := c.svc.Get(id)
	declaration, declared := findWebhookDeclaration(record, key)
	public := declared && declaration.EffectiveAccess(record.APIVersion) == manifest.WebhookAccessPublic
	if !webhookCallerAuthorized(ctx, public) {
		return
	}
	if lookupErr != nil {
		c.writeLookupError(ctx, lookupErr)
		return
	}
	if !declared {
		ctx.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("plugin %q has no webhook %q", id, key)})
		return
	}

	body, err := readCappedWebhookBody(ctx, declaration.EffectiveMaxBodyBytes())
	if err != nil {
		return
	}

	req := &pluginsdk.WebhookRequest{
		WebhookKey: key,
		Method:     ctx.Request.Method,
		Query:      ctx.Request.URL.RawQuery,
		Headers:    flattenHeaders(ctx.Request.Header, c.svc.sessionCookieName(), public),
		Body:       body,
	}

	leasedRecord, release, err := c.svc.beginPluginDispatch(id, dispatchGeneration(record))
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	defer release()

	resp, err := c.webhookInvoker.InvokeWebhook(ctx.Request.Context(), id, req)
	if err != nil {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}
	c.writeWebhookResponse(ctx, leasedRecord, resp)
}

// webhookCallerAuthorized requires a caller identity unless the declaration
// explicitly opts into public access. It runs before lookup errors to avoid
// revealing installed plugin IDs or declared webhook keys to anonymous callers.
func webhookCallerAuthorized(ctx *gin.Context, public bool) bool {
	if public {
		return true
	}
	identity, ok := authn.FromGin(ctx)
	if !ok {
		ctx.Header("WWW-Authenticate", "Bearer")
		ctx.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return false
	}
	if identity.SessionID != "" && !webhookSameOriginRequest(ctx) {
		ctx.AbortWithStatusJSON(http.StatusForbidden, gin.H{
			"error": "a same-origin request is required for session-authenticated webhooks",
		})
		return false
	}
	return true
}

// webhookSameOriginRequest reports whether a session-authenticated webhook
// call is a genuine same-origin request rather than cross-site forgery.
//
// A session identity rides on an ambient cookie, so a page on another site can
// make the browser attach it; a PAT or the synthetic auth-disabled identity
// cannot be borrowed that way, which is why only sessions reach this check.
// The signal is the request's origin, never its method: Kandev does not
// enforce webhooks[].method and cannot see whether a plugin's GET handler has
// side effects, so exempting safe verbs would reopen exactly the hole this
// gate closes.
//
// The two signals, in order:
//
//   - Origin present: decided by httpmw.AllowedOrigin, the shared origin trust
//     policy. Every cross-origin request carries Origin, including the SPA's
//     own fetch in a split-origin or desktop install (frontend and backend on
//     different ports, see apps/web/lib/plugins/host-api.ts), which the browser
//     labels Sec-Fetch-Site: same-site. Origin therefore has to decide alone
//     here; also demanding same-origin fetch metadata would break those installs.
//
//   - Origin absent: decided by Sec-Fetch-Site. Browsers do not send Origin on
//     a same-origin GET or HEAD (per Fetch, it is attached to cross-origin
//     requests and to same-origin requests whose method is neither GET nor
//     HEAD), so an absent Origin is the normal state of a plugin panel polling
//     its own webhook, not evidence of a cross-origin caller. Requiring Origin
//     unconditionally refused every such poll on any auth-enabled instance.
//
// same-origin and none (a user-initiated request with no initiator: address
// bar, bookmark) are accepted; cross-site and same-site are refused. Refusing
// cross-site closes the one ambient-credential vector left on this route: the
// SameSite=Lax session cookie is not sent on a cross-site subresource request
// or POST, but it *is* sent on a cross-site top-level GET navigation, which
// carries no Origin either.
//
// Neither header is refused. A session cookie with no origin signal at all is
// indistinguishable from a cross-site top-level GET navigation made by a
// browser that predates Fetch Metadata (or behind something stripping it),
// which SameSite=Lax does attach the cookie to, so accepting it would reopen
// the CSRF path this gate exists to close. That costs nothing anyone has
// today: such a request is already refused before this change, so refusing it
// still is not a regression, and the deliberate non-browser caller (curl, a
// CLI, a server-side integration) has a credential built for exactly this,
// the PAT, which is not ambient and is not gated here at all.
func webhookSameOriginRequest(ctx *gin.Context) bool {
	if origin := ctx.GetHeader("Origin"); origin != "" {
		return commonhttpmw.AllowedOrigin(origin, ctx.Request.Host)
	}
	// Fetch Metadata values are lowercase per spec; match leniently, which
	// cannot admit a value outside the accepted set. An absent header falls to
	// the default and is refused along with cross-site and same-site.
	switch strings.ToLower(strings.TrimSpace(ctx.GetHeader("Sec-Fetch-Site"))) {
	case "same-origin", "none":
		return true
	default:
		return false
	}
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
// webhookStatusForResponse) before ever reaching ctx.Writer.WriteHeader.
//
// Before relaying, it consumes the reserved SSO login directive (an
// auth-capable plugin asserting a validated external identity — OIDC/SAML)
// and, when accepted, mints and sets the session cookie host-side so the
// plugin never handles the raw token. Plugin-supplied Set-Cookie headers are
// dropped: a relayed webhook has no legitimate reason to set browser cookies,
// and dropping them stops a plugin from overwriting the session cookie the
// host just minted or fixating any other. Remaining headers, status, and body
// are relayed verbatim.
func (c *Controller) writeWebhookResponse(ctx *gin.Context, record *store.Record, resp *pluginsdk.WebhookResponse) {
	status, ok := webhookStatusForResponse(resp.Status)
	if !ok {
		ctx.JSON(http.StatusBadGateway, gin.H{
			"error": fmt.Sprintf("plugin returned invalid webhook status %d", resp.Status),
		})
		return
	}
	if raw, present, ambiguous := takeAuthLoginDirective(resp.Headers); present {
		if ambiguous {
			c.log.Warn("plugin sent multiple auth login directives", zap.String("plugin", record.ID))
			ctx.JSON(http.StatusForbidden, gin.H{"error": "auth login rejected"})
			return
		}
		if err := c.applyAuthLogin(ctx, record, raw); err != nil {
			// The webhook endpoint is public; keep the response body generic so
			// a raw store/auth error can't be disclosed to an anonymous caller.
			// The real cause is logged for the operator.
			c.log.Warn("plugin auth login rejected", zap.String("plugin", record.ID), zap.Error(err))
			ctx.JSON(http.StatusForbidden, gin.H{"error": "auth login rejected"})
			return
		}
	}
	for k, v := range resp.Headers {
		if http.CanonicalHeaderKey(k) == "Set-Cookie" {
			continue
		}
		ctx.Writer.Header().Set(k, v)
	}
	ctx.Writer.WriteHeader(status)
	_, _ = ctx.Writer.Write(resp.Body)
}

// applyAuthLogin establishes a browser session from a plugin's external-identity
// assertion. It requires the plugin to declare the `auth` capability and the
// SSO login bridge to be wired (auth enabled); a plugin that emits the directive
// without the capability is rejected rather than silently ignored.
func (c *Controller) applyAuthLogin(ctx *gin.Context, record *store.Record, raw string) error {
	if !record.Capabilities.Auth {
		return fmt.Errorf("plugin %q used the auth login directive without the 'auth' capability", record.ID)
	}
	bridge := c.svc.authLoginBridge()
	if bridge == nil {
		return errors.New("authentication is not enabled")
	}
	var a externalLoginAssertion
	if err := json.Unmarshal([]byte(raw), &a); err != nil {
		return errors.New("invalid auth login assertion")
	}
	return bridge.LoginExternal(ctx, a.Provider, a.Subject, a.Email, a.DisplayName)
}

func findWebhookDeclaration(record *store.Record, key string) (manifest.Webhook, bool) {
	if record == nil {
		return manifest.Webhook{}, false
	}
	for _, wh := range record.Webhooks {
		if wh.Key == key {
			return wh, true
		}
	}
	return manifest.Webhook{}, false
}

// readCappedWebhookBody reads ctx.Request.Body bounded at
// maxWebhookBodyBytes via http.MaxBytesReader, writing the 413 response
// itself (and returning a non-nil error as a sentinel to the caller) when
// the body exceeds the cap, so a single external webhook POST cannot
// exhaust backend memory via an unbounded io.ReadAll.
func readCappedWebhookBody(ctx *gin.Context, maxBytes int64) ([]byte, error) {
	ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, maxBytes)
	body, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			ctx.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": fmt.Sprintf("webhook body exceeds max size of %d bytes", maxBytes),
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
func flattenHeaders(h http.Header, sessionCookieName string, public bool) map[string]string {
	out := make(map[string]string, len(h))
	for k, v := range h {
		switch http.CanonicalHeaderKey(k) {
		case "Cookie":
			if !public {
				continue
			}
			stripped, keep := stripSessionCookies(v, sessionCookieName)
			if !keep {
				continue
			}
			out[k] = stripped
		case "Authorization":
			if !public {
				continue
			}
			if containsKandevPATCredential(v) {
				continue
			}
			out[k] = strings.Join(v, ", ")
		default:
			out[k] = strings.Join(v, ", ")
		}
	}
	return out
}

func stripSessionCookies(headers []string, sessionCookieName string) (string, bool) {
	kept := make([]string, 0, len(headers))
	for _, header := range headers {
		stripped, keep := stripSessionCookie(header, sessionCookieName)
		if keep {
			kept = append(kept, stripped)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	return strings.Join(kept, "; "), true
}

func containsKandevPATCredential(values []string) bool {
	for _, value := range values {
		if isKandevPATCredential(value) || strings.Contains(value, auth.PATPrefix) {
			return true
		}
	}
	return false
}

// isKandevPATCredential reports whether an Authorization header value carries
// a kandev_pat_* token, with or without a bearer scheme prefix. RFC 9110 makes
// the auth scheme case-insensitive, so "bearer kandev_pat_..." has to be
// stripped just like "Bearer kandev_pat_...": httpmw.BearerToken would not have
// authenticated such a request, but the credential still must not be relayed to
// a plugin subprocess (which a public webhook would otherwise receive it as).
func isKandevPATCredential(value string) bool {
	const bearerPrefix = "Bearer "
	token := strings.TrimSpace(value)
	if len(token) > len(bearerPrefix) && strings.EqualFold(token[:len(bearerPrefix)], bearerPrefix) {
		token = strings.TrimSpace(token[len(bearerPrefix):])
	}
	return strings.HasPrefix(token, auth.PATPrefix)
}

// stripSessionCookie removes the sessionCookieName cookie from a Cookie
// header value ("a=1; b=2"). keep is false when no cookies remain, so the
// caller can omit the header entirely rather than send an empty one.
func stripSessionCookie(header, sessionCookieName string) (stripped string, keep bool) {
	if sessionCookieName == "" {
		return header, true
	}
	parts := strings.Split(header, ";")
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		name, _, found := strings.Cut(trimmed, "=")
		if found && name == sessionCookieName {
			continue
		}
		if trimmed != "" {
			kept = append(kept, trimmed)
		}
	}
	if len(kept) == 0 {
		return "", false
	}
	return strings.Join(kept, "; "), true
}

package backendapp

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	canvasservice "github.com/kandev/kandev/internal/canvas"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/webapp"
	userstore "github.com/kandev/kandev/internal/user/store"
)

var errCanvasWebAppNotDeclared = errors.New("canvas web application is not declared")

func (h *canvasHTTPHandler) runtimeBinding(c *gin.Context, canvas canvasservice.Canvas, request canvasRuntimeRequest) (string, instances.Release, manifest.WebApp, webapp.CapabilityBinding, error) {
	instance, release, appManifest, err := h.loadRuntimeRelease(c, canvas)
	if err != nil {
		return "", instances.Release{}, manifest.WebApp{}, webapp.CapabilityBinding{}, err
	}
	app, placement, err := selectRuntimeWebApp(appManifest, request, canvas.ScopeKind)
	if err != nil {
		return "", instances.Release{}, manifest.WebApp{}, webapp.CapabilityBinding{}, err
	}
	grants, err := h.plugins.Instances().ListGrants(c.Request.Context(), instance.ID)
	if err != nil {
		return "", instances.Release{}, manifest.WebApp{}, webapp.CapabilityBinding{}, err
	}
	binding := webapp.CapabilityBinding{
		PluginID:        instance.PluginID,
		UserID:          canvasRuntimeUser(c),
		InstanceID:      instance.ID,
		ReleaseID:       release.ID,
		WebAppKey:       app.Key,
		Placement:       placement,
		ScopeKind:       instance.ScopeKind,
		WorkspaceID:     instance.WorkspaceID,
		TaskID:          instance.TaskID,
		SessionID:       instance.SessionID,
		RepositoryID:    instance.RepositoryID,
		GrantGeneration: instance.GrantGeneration,
		Permissions:     runtimeGrantedPermissions(appManifest, instance.ScopeKind, grants),
		Artifact: webapp.Artifact{
			Digest:       release.PackageDigest,
			RelativePath: release.ArtifactPath,
			Bytes:        release.ArtifactBytes,
			Available:    true,
		},
		Entry:          app.Entry,
		NetworkOrigins: runtimeNetworkOrigins(app, instance.ScopeKind, grants),
	}
	runtime := h.plugins.WebRuntime()
	if runtime == nil {
		return "", instances.Release{}, manifest.WebApp{}, webapp.CapabilityBinding{}, errors.New("canvas runtime is not configured")
	}
	path, err := runtime.IssueCapabilityPath(binding, webapp.RuntimeTokenTTL)
	if err != nil {
		return "", instances.Release{}, manifest.WebApp{}, webapp.CapabilityBinding{}, err
	}
	return path, release, app, binding, nil
}

func (h *canvasHTTPHandler) loadRuntimeRelease(c *gin.Context, canvas canvasservice.Canvas) (instances.Instance, instances.Release, *manifest.Manifest, error) {
	store := h.plugins.Instances()
	if store == nil {
		return instances.Instance{}, instances.Release{}, nil, errors.New("canvas runtime is not configured")
	}
	instance, err := store.Get(c.Request.Context(), canvas.PluginInstanceID)
	if err != nil {
		return instances.Instance{}, instances.Release{}, nil, err
	}
	release, err := store.GetRelease(c.Request.Context(), instance.ActiveReleaseID)
	if err != nil {
		return instances.Instance{}, instances.Release{}, nil, err
	}
	appManifest, err := manifest.Parse(release.ManifestJSON)
	if err != nil {
		return instances.Instance{}, instances.Release{}, nil, err
	}
	return instance, release, appManifest, nil
}

func selectRuntimeWebApp(appManifest *manifest.Manifest, request canvasRuntimeRequest, scope string) (manifest.WebApp, string, error) {
	placement := request.Placement
	if placement == "" {
		placement = appPlacement(manifest.WebApp{}, "", scope)
	}
	var app manifest.WebApp
	for _, candidate := range appManifest.UI.WebApps {
		if request.WebAppKey != "" && candidate.Key != request.WebAppKey {
			continue
		}
		if placement != "" && !containsString(candidate.Placements, placement) {
			continue
		}
		app = candidate
		break
	}
	if app.Key == "" {
		return manifest.WebApp{}, "", errCanvasWebAppNotDeclared
	}
	return app, appPlacement(app, request.Placement, scope), nil
}

func runtimeNetworkOrigins(app manifest.WebApp, scope string, grants []instances.Grant) []string {
	declared, err := webapp.NormalizeNetworkOrigins(app.NetworkOrigins)
	if err != nil || len(declared) == 0 {
		return nil
	}
	origins := make([]string, 0, len(grants))
	for _, origin := range declared {
		for _, grant := range grants {
			if grant.NetworkOrigin == origin && runtimeGrantScopeCovers(grant.ScopeCeiling, scope) && !containsString(origins, grant.NetworkOrigin) {
				origins = append(origins, grant.NetworkOrigin)
			}
		}
	}
	return origins
}

func (h *canvasHTTPHandler) writeError(c *gin.Context, err error) {
	status := http.StatusInternalServerError
	code := "canvas_error"
	switch {
	case errors.Is(err, canvasservice.ErrCanvasNotFound), errors.Is(err, instances.ErrNotFound):
		status, code = http.StatusNotFound, "canvas_not_found"
	case errors.Is(err, canvasservice.ErrTaskCanvasLimit), errors.Is(err, canvasservice.ErrWorkspaceCanvasLimit), errors.Is(err, instances.ErrTaskCanvasLimit), errors.Is(err, instances.ErrWorkspaceCanvasLimit):
		status, code = http.StatusConflict, "canvas_limit_exceeded"
	case errors.Is(err, instances.ErrWorkspaceStorageLimit), errors.Is(err, instances.ErrInstallationStorageLimit):
		status, code = http.StatusConflict, "canvas_storage_limit_exceeded"
	case errors.Is(err, instances.ErrInvalidRelease):
		status, code = http.StatusConflict, "invalid_release"
	case errors.Is(err, instances.ErrStaleCanvasPublish), errors.Is(err, canvasservice.ErrStaleCanvasPublish):
		status, code = http.StatusConflict, "canvas_publish_stale"
	case errors.Is(err, instances.ErrInvalidLifecycleState), errors.Is(err, canvasservice.ErrInvalidLifecycleState):
		status, code = http.StatusBadRequest, canvasErrorCodeInvalid
	case errors.Is(err, canvasservice.ErrStalePromotionReview):
		status, code = http.StatusConflict, "promotion_review_stale"
	case errors.Is(err, canvasservice.ErrStaleCanvasEdit):
		status, code = http.StatusConflict, "canvas_edit_stale"
	case errors.Is(err, canvasservice.ErrInvalidCanvas), errors.Is(err, canvasservice.ErrInvalidCanvasState):
		status, code = http.StatusBadRequest, canvasErrorCodeInvalid
	case errors.Is(err, errCanvasWebAppNotDeclared):
		status, code = http.StatusBadRequest, "web_app_not_declared"
	default:
	}
	writeCanvasError(c, status, code, nil)
}

func mapCanvasResponses(values []canvasservice.Canvas) []canvasHTTPResponse {
	result := make([]canvasHTTPResponse, 0, len(values))
	for _, value := range values {
		result = append(result, canvasResponse(value))
	}
	return result
}

func canvasIncludeArchived(c *gin.Context) (bool, bool) {
	value := c.Query("include_archived")
	if value == "" {
		return false, true
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		writeCanvasError(c, http.StatusBadRequest, "invalid_include_archived", nil)
		return false, false
	}
	return parsed, true
}

func canvasRuntimeUser(c *gin.Context) string {
	if identity, ok := authn.FromGin(c); ok && identity.UserID != "" {
		return identity.UserID
	}
	return userstore.DefaultUserID
}

func appPlacement(app manifest.WebApp, requested, scope string) string {
	if requested != "" {
		return requested
	}
	wanted := manifest.WebAppPlacementWorkspace
	if scope == instances.ScopeTask {
		wanted = manifest.WebAppPlacementTask
	}
	if containsString(app.Placements, wanted) {
		return wanted
	}
	if len(app.Placements) > 0 {
		return app.Placements[0]
	}
	return wanted
}

func declaredPermissions(appManifest *manifest.Manifest) []string {
	if appManifest == nil {
		return nil
	}
	permissions := make([]string, 0, len(appManifest.Capabilities.APIRead)+len(appManifest.Capabilities.APIWrite)+len(appManifest.Capabilities.Events)+1)
	for _, resource := range appManifest.Capabilities.APIRead {
		permissions = appendUnique(permissions, "api_read:"+resource)
	}
	for _, resource := range appManifest.Capabilities.APIWrite {
		permissions = appendUnique(permissions, "api_write:"+resource)
	}
	for _, event := range appManifest.Capabilities.Events {
		permissions = appendUnique(permissions, "events:"+event)
	}
	if appManifest.Capabilities.State {
		permissions = appendUnique(permissions, "state")
	}
	return permissions
}

func runtimeGrantedPermissions(appManifest *manifest.Manifest, scope string, grants []instances.Grant) []string {
	declared := declaredPermissions(appManifest)
	permissions := make([]string, 0, len(declared))
	for _, permission := range declared {
		parts := strings.SplitN(permission, ":", 2)
		kind, resource := parts[0], ""
		if len(parts) == 2 {
			resource = parts[1]
		}
		for _, grant := range grants {
			if grant.PermissionKind == kind && grant.Resource == resource && runtimeGrantScopeCovers(grant.ScopeCeiling, scope) {
				permissions = append(permissions, permission)
				break
			}
		}
	}
	return permissions
}

func runtimeGrantScopeCovers(ceiling, scope string) bool {
	if ceiling == instances.ScopeInstance {
		return true
	}
	return ceiling == scope
}

func appendUnique(values []string, value string) []string {
	if value == "" || containsString(values, value) {
		return values
	}
	return append(values, value)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func canvasHostError(status string) string {
	switch status {
	case instances.ValidationPendingPermission:
		return "permission_review_required"
	case instances.ValidationInvalid:
		return "invalid_release"
	case instances.ValidationUnavailable:
		return "runtime_unavailable"
	default:
		return "canvas_release_unavailable"
	}
}

func writeCanvasJSON(c *gin.Context, status int, value interface{}) {
	c.Header("Cache-Control", "no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.JSON(status, value)
}

func writeCanvasError(c *gin.Context, status int, code string, details map[string]interface{}) {
	body := gin.H{"error": code, "error_code": code}
	for key, value := range details {
		body[key] = value
	}
	writeCanvasJSON(c, status, body)
}

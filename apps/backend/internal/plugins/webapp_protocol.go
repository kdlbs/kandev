package plugins

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/webapp"
	userstore "github.com/kandev/kandev/internal/user/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	webAppProtocolVersion    = 1
	webAppRequestLimit       = 256 << 10
	webAppResponseLimit      = 1 << 20
	webAppRequestTimeout     = 30 * time.Second
	webAppRuntimeUnavailable = "runtime_unavailable"
)

// handleWebAppProtocol is called only after Runtime has authenticated and
// validated the capability URL. The path is relative to the _kandev root.
func (s *Service) handleWebAppProtocol(w http.ResponseWriter, r *http.Request, _ string, binding webapp.CapabilityBinding, path string) {
	webapp.SetProtocolHeaders(w, r.Header.Get("Origin"))
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, OPTIONS, PATCH, POST, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, If-Match, Last-Event-ID")
		w.Header().Set("Access-Control-Max-Age", "600")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	parts, ok := splitWebAppProtocolPath(path)
	if !ok || len(parts) < 2 || parts[0] != "v1" {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}

	ctx, cancel := context.WithTimeout(webAppRequestContext(r.Context(), binding), webAppRequestTimeout)
	defer cancel()
	switch parts[1] {
	case "context":
		s.handleWebAppContext(w, r, binding, parts)
	case "data":
		s.handleWebAppData(ctx, w, r, s.webAppHost(binding), binding, parts)
	case "state":
		s.handleWebAppState(ctx, w, r, binding, parts)
	case "events":
		s.handleWebAppEvents(w, r, binding, parts)
	case "actions":
		s.handleWebAppAction(w, r, binding, parts)
	default:
		writeWebAppError(w, http.StatusNotFound, "not_found")
	}
}

func (s *Service) handleWebAppEvents(w http.ResponseWriter, r *http.Request, binding webapp.CapabilityBinding, parts []string) {
	if r.Method != http.MethodGet || len(parts) != 2 {
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	hub := s.WebAppEventHub()
	if hub == nil {
		writeWebAppError(w, http.StatusServiceUnavailable, webAppRuntimeUnavailable)
		return
	}
	hub.Serve(w, r, webapp.EventSubscriptionRequest{
		InstanceID: binding.InstanceID,
		UserID:     binding.UserID,
		Filter:     s.webAppEventFilter(binding),
	})
}

func splitWebAppProtocolPath(path string) ([]string, bool) {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return nil, false
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return nil, false
		}
	}
	return parts, true
}

func webAppRequestContext(ctx context.Context, binding webapp.CapabilityBinding) context.Context {
	return authn.WithIdentity(ctx, authn.Identity{
		UserID:    binding.UserID,
		Role:      authn.RoleMember,
		Synthetic: binding.UserID == userstore.DefaultUserID,
	})
}

func (s *Service) webAppHost(binding webapp.CapabilityBinding) *pluginHost {
	var host *pluginHost
	if s.registry != nil {
		host, _ = s.hostForPlugin(binding.PluginID).(*pluginHost)
	}
	if host == nil {
		host = &pluginHost{
			pluginID:      binding.PluginID,
			taskData:      s.taskData,
			workflows:     s.workflows,
			workflowSteps: s.workflowSteps,
			messageData:   s.messageData,
			taskWriter:    s.taskWriter,
			taskPRsDep:    s.taskPRSourceDep,
			writeDeps:     s.writeDependencies,
		}
	}
	host.pluginID = binding.PluginID
	host.capabilities = webAppCapabilities(binding.Permissions)
	return host
}

func webAppCapabilities(permissions []string) manifest.Capabilities {
	capabilities := manifest.Capabilities{}
	seenRead := make(map[string]struct{})
	seenWrite := make(map[string]struct{})
	for _, permission := range permissions {
		permission = strings.TrimSpace(permission)
		switch {
		case permission == "state":
			capabilities.State = true
		case strings.HasPrefix(permission, "api_read:"):
			resource := strings.TrimPrefix(permission, "api_read:")
			if resource != "" {
				if _, ok := seenRead[resource]; !ok {
					capabilities.APIRead = append(capabilities.APIRead, resource)
					seenRead[resource] = struct{}{}
				}
			}
		case strings.HasPrefix(permission, "api_write:"):
			resource := strings.TrimPrefix(permission, "api_write:")
			if resource != "" {
				if _, ok := seenWrite[resource]; !ok {
					capabilities.APIWrite = append(capabilities.APIWrite, resource)
					seenWrite[resource] = struct{}{}
				}
			}
		}
	}
	return capabilities
}

type webAppContext struct {
	ProtocolVersion int      `json:"protocol_version"`
	InstanceID      string   `json:"instance_id"`
	PluginID        string   `json:"plugin_id"`
	ReleaseID       string   `json:"release_id"`
	WebAppKey       string   `json:"web_app_key"`
	Placement       string   `json:"placement"`
	ScopeKind       string   `json:"scope_kind"`
	WorkspaceID     string   `json:"workspace_id,omitempty"`
	TaskID          string   `json:"task_id,omitempty"`
	SessionID       string   `json:"session_id,omitempty"`
	RepositoryID    string   `json:"repository_id,omitempty"`
	Capabilities    []string `json:"capabilities"`
}

func (s *Service) handleWebAppContext(w http.ResponseWriter, r *http.Request, binding webapp.CapabilityBinding, parts []string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if len(parts) != 2 {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppContext{
		ProtocolVersion: webAppProtocolVersion,
		InstanceID:      binding.InstanceID,
		PluginID:        binding.PluginID,
		ReleaseID:       binding.ReleaseID,
		WebAppKey:       binding.WebAppKey,
		Placement:       binding.Placement,
		ScopeKind:       binding.ScopeKind,
		WorkspaceID:     binding.WorkspaceID,
		TaskID:          binding.TaskID,
		SessionID:       binding.SessionID,
		RepositoryID:    binding.RepositoryID,
		Capabilities:    append([]string(nil), binding.Permissions...),
	})
}

func (s *Service) handleWebAppData(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, parts []string) {
	if len(parts) < 3 {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	switch parts[2] {
	case "tasks":
		s.handleWebAppTasks(ctx, w, r, host, binding, parts[3:])
	case "workflows":
		s.handleWebAppWorkflows(ctx, w, r, host, binding, parts[3:])
	default:
		writeWebAppError(w, http.StatusNotFound, "not_found")
	}
}

func (s *Service) handleWebAppTasks(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, parts []string) {
	switch {
	case len(parts) == 0 && r.Method == http.MethodGet:
		s.listWebAppTasks(ctx, w, r, host, binding)
	case len(parts) == 1 && r.Method == http.MethodGet:
		s.getWebAppTask(ctx, w, r, host, binding, parts[0])
	case len(parts) == 1 && r.Method == http.MethodPatch:
		s.updateWebAppTask(ctx, w, r, host, binding, parts[0])
	case len(parts) == 2 && parts[1] == "messages" && r.Method == http.MethodPost:
		s.sendWebAppMessage(ctx, w, r, host, binding, parts[0])
	default:
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (s *Service) handleWebAppWorkflows(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, parts []string) {
	switch {
	case len(parts) == 0 && r.Method == http.MethodGet:
		s.listWebAppWorkflows(ctx, w, r, host, binding)
	case len(parts) == 2 && parts[1] == "steps" && r.Method == http.MethodGet:
		s.listWebAppWorkflowSteps(ctx, w, r, host, binding, parts[0])
	default:
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
	}
}

func (s *Service) handleWebAppAction(w http.ResponseWriter, r *http.Request, binding webapp.CapabilityBinding, parts []string) {
	if r.Method != http.MethodPost || len(parts) != 2 || !validWebAppKey(parts[1]) {
		writeWebAppError(w, http.StatusMethodNotAllowed, "method_not_allowed")
		return
	}
	if !webAppHasActionPermission(binding.Permissions, parts[1]) {
		writeWebAppError(w, http.StatusForbidden, "plugin_permission_denied")
		return
	}
	if _, err := readWebAppBody(r); err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	writeWebAppError(w, http.StatusNotImplemented, "plugin_action_unavailable")
}

func webAppHasActionPermission(permissions []string, key string) bool {
	for _, permission := range permissions {
		if permission == "action:"+key || permission == "actions:"+key {
			return true
		}
	}
	return false
}

func validWebAppKey(value string) bool {
	if value == "" || len(value) > 128 {
		return false
	}
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func webAppRequestPage(r *http.Request) (pluginsdk.Page, error) {
	query := r.URL.Query()
	page := pluginsdk.Page{Cursor: query.Get("cursor")}
	if raw := query.Get("limit"); raw != "" {
		limit, err := strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > maxPageLimit {
			return pluginsdk.Page{}, errors.New("invalid limit")
		}
		page.Limit = int32(limit)
	}
	return page, nil
}

func webAppQueryList(queryValues []string) []string {
	var values []string
	for _, value := range queryValues {
		for _, item := range strings.Split(value, ",") {
			if item = strings.TrimSpace(item); item != "" {
				values = append(values, item)
			}
		}
	}
	return values
}

func webAppBoolQuery(r *http.Request, name string) (bool, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return false, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, errors.New("invalid boolean")
	}
	return parsed, nil
}

func webAppParentQuery(r *http.Request) *string {
	value := strings.TrimSpace(r.URL.Query().Get("parent_id"))
	if value == "" {
		return nil
	}
	return &value
}

func webAppProtocolStatus(err error) int {
	if errors.Is(err, webapp.ErrRuntimeTokenStale) {
		return http.StatusUnauthorized
	}
	if errors.Is(err, instances.ErrNotFound) || status.Code(err) == codes.NotFound {
		return http.StatusNotFound
	}
	if errors.Is(err, state.ErrConflict) {
		return http.StatusConflict
	}
	switch status.Code(err) {
	case codes.PermissionDenied:
		return http.StatusForbidden
	case codes.InvalidArgument:
		return http.StatusBadRequest
	case codes.FailedPrecondition:
		return http.StatusConflict
	case codes.Unimplemented:
		return http.StatusNotImplemented
	default:
		if errors.Is(err, context.DeadlineExceeded) {
			return http.StatusGatewayTimeout
		}
		return http.StatusInternalServerError
	}
}

// validateWebAppBinding rechecks every durable authority component on each
// asset and protocol request. The token therefore cannot survive release,
// scope, grant, or instance removal changes.
func (s *Service) validateWebAppBinding(ctx context.Context, binding webapp.CapabilityBinding) error {
	store := s.Instances()
	if store == nil {
		return webapp.ErrRuntimeTokenStale
	}
	instance, err := store.Get(ctx, binding.InstanceID)
	if err != nil || instance.Status != instances.StatusActive || instance.PluginID != binding.PluginID ||
		instance.ActiveReleaseID != binding.ReleaseID || instance.GrantGeneration != binding.GrantGeneration {
		return webapp.ErrRuntimeTokenStale
	}
	release, err := store.GetRelease(ctx, binding.ReleaseID)
	if err != nil || release.InstanceID != binding.InstanceID || release.PluginID != binding.PluginID ||
		release.ValidationStatus != instances.ValidationValid || release.PackageDigest != binding.Artifact.Digest ||
		release.ArtifactPath != binding.Artifact.RelativePath {
		return webapp.ErrRuntimeTokenStale
	}
	if err := validateWebAppManifestBinding(release.ManifestJSON, binding); err != nil {
		return webapp.ErrRuntimeTokenStale
	}
	if err := s.validateWebAppPermissions(ctx, binding, release.ManifestJSON); err != nil {
		return webapp.ErrRuntimeTokenStale
	}
	return s.validateWebAppScope(webAppRequestContext(ctx, binding), binding)
}

func (s *Service) validateWebAppPermissions(ctx context.Context, binding webapp.CapabilityBinding, raw json.RawMessage) error {
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	store := s.Instances()
	if store == nil {
		return errors.New("web app instance store is unavailable")
	}
	grants, err := store.ListGrants(ctx, binding.InstanceID)
	if err != nil {
		return err
	}
	for _, permission := range webAppDeclaredPermissions(m, binding.WebAppKey) {
		if !webAppGrantCovers(permission, binding.ScopeKind, grants) {
			return errors.New("web app permission grant is stale")
		}
	}
	for _, origin := range binding.NetworkOrigins {
		if !webAppOriginGranted(origin, binding.ScopeKind, grants) {
			return errors.New("web app network grant is stale")
		}
	}
	return nil
}

func webAppDeclaredPermissions(m manifest.Manifest, webAppKey string) []string {
	permissions := make([]string, 0, len(m.Capabilities.APIRead)+len(m.Capabilities.APIWrite)+len(m.Capabilities.Events)+1)
	for _, resource := range m.Capabilities.APIRead {
		permissions = append(permissions, "api_read:"+resource)
	}
	for _, resource := range m.Capabilities.APIWrite {
		permissions = append(permissions, "api_write:"+resource)
	}
	for _, subject := range m.Capabilities.Events {
		permissions = append(permissions, "events:"+subject)
	}
	if m.Capabilities.State {
		permissions = append(permissions, "state")
	}
	for _, app := range m.UI.WebApps {
		if app.Key != webAppKey {
			continue
		}
		origins, err := webapp.NormalizeNetworkOrigins(app.NetworkOrigins)
		if err != nil {
			break
		}
		for _, origin := range origins {
			permissions = append(permissions, "network:"+origin)
		}
		break
	}
	return permissions
}

func webAppGrantCovers(permission, scope string, grants []instances.Grant) bool {
	parts := strings.SplitN(permission, ":", 2)
	kind, resource := parts[0], ""
	if len(parts) == 2 {
		resource = parts[1]
	}
	for _, grant := range grants {
		if kind == "network" {
			if grant.PermissionKind == kind && grant.NetworkOrigin == resource && webAppGrantScopeCovers(grant.ScopeCeiling, scope) {
				return true
			}
			continue
		}
		if grant.PermissionKind == kind && grant.Resource == resource && webAppGrantScopeCovers(grant.ScopeCeiling, scope) {
			return true
		}
	}
	return false
}

func webAppOriginGranted(origin, scope string, grants []instances.Grant) bool {
	for _, grant := range grants {
		if grant.NetworkOrigin == origin && webAppGrantScopeCovers(grant.ScopeCeiling, scope) {
			return true
		}
	}
	return false
}

func webAppGrantScopeCovers(ceiling, scope string) bool {
	return ceiling == instances.ScopeInstance || ceiling == scope
}

func validateWebAppManifestBinding(raw json.RawMessage, binding webapp.CapabilityBinding) error {
	var m manifest.Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return err
	}
	for _, app := range m.UI.WebApps {
		if app.Key == binding.WebAppKey && app.Entry == binding.Entry && webAppContainsString(app.Placements, binding.Placement) {
			return nil
		}
	}
	return errors.New("web app binding is not declared by the release")
}

func (s *Service) validateWebAppScope(ctx context.Context, binding webapp.CapabilityBinding) error {
	if binding.ScopeKind == instances.ScopeInstance || s.taskData == nil {
		return nil
	}
	if binding.WorkspaceID == "" {
		return webapp.ErrRuntimeTokenStale
	}
	workspaces, err := s.taskData.ListWorkspaces(ctx)
	if err != nil {
		return webapp.ErrRuntimeTokenStale
	}
	visible := false
	for _, workspace := range workspaces {
		if workspace != nil && workspace.ID == binding.WorkspaceID {
			visible = true
			break
		}
	}
	if !visible {
		return webapp.ErrRuntimeTokenStale
	}
	if binding.ScopeKind == instances.ScopeTask && binding.TaskID != "" {
		task, err := s.taskData.GetTask(ctx, binding.TaskID)
		if err != nil || task == nil || task.WorkspaceID != binding.WorkspaceID {
			return webapp.ErrRuntimeTokenStale
		}
	}
	return nil
}

func webAppContainsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

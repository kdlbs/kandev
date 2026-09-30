// Package managed provides the narrow MCP callback transport used by remote
// managed-agent runtimes. Its bearer grants never create an application user
// identity; each request resolves the task owner and session from local rows.
package managed

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/cursorcloud"
	"github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/task/models"
)

const (
	// ManagedCallbackPath is the route prefix to append to the configured
	// callback base URL. The opaque grant ID is the final path segment.
	ManagedCallbackPath = cursorcloud.ManagedCallbackPath + "/"
	MaxGrantLifetime    = 24 * time.Hour
	maxBufferedResponse = 4 << 20
)

type GrantWriter interface {
	CreateManagedAgentToolGrant(context.Context, *models.ManagedAgentToolGrant) error
}

type Repository interface {
	GrantWriter
	GetManagedAgentToolGrantByHash(context.Context, string) (*models.ManagedAgentToolGrant, error)
	GetManagedAgentBinding(context.Context, string) (*models.ManagedAgentBinding, error)
	GetManagedAgentOperation(context.Context, string) (*models.ManagedAgentOperation, error)
}

type Authority interface {
	GetTask(context.Context, string) (*models.Task, error)
	GetTaskSession(context.Context, string) (*models.TaskSession, error)
}

type ScopeResolver interface {
	ScopeOverridingIdentity(context.Context, string) (context.Context, error)
	ScopePrincipal(context.Context, string, string) (context.Context, error)
}

type HandlerFactory func(context.Context, *models.ManagedAgentBinding, profile.Context, string) (http.Handler, error)

type Config struct {
	Repository     Repository
	Authority      Authority
	Scope          ScopeResolver
	Enabled        func() bool
	Now            func() time.Time
	HandlerFactory HandlerFactory
}

type Transport struct {
	repository Repository
	authority  Authority
	scope      ScopeResolver
	enabled    func() bool
	now        func() time.Time
	factory    HandlerFactory
}

type IssuedGrant struct {
	Grant models.ManagedAgentToolGrant
	Token string
	URL   string
}

func IssueGrant(
	ctx context.Context,
	repository GrantWriter,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	toolProfile profile.Context,
	now time.Time,
	lifetime time.Duration,
) (IssuedGrant, error) {
	if _, err := validateGrantIssuance(repository, binding, operation, toolProfile); err != nil {
		return IssuedGrant{}, err
	}
	now, lifetime = normalizeGrantTime(now, lifetime)
	token, tokenHash, err := newGrantToken()
	if err != nil {
		return IssuedGrant{}, err
	}
	scopeSnapshot, err := json.Marshal(toolProfile)
	if err != nil {
		return IssuedGrant{}, fmt.Errorf("encode managed MCP tool profile: %w", err)
	}
	grant := models.ManagedAgentToolGrant{
		ID: uuid.NewString(), BindingID: binding.ID, OperationID: operation.ID,
		TokenHash: tokenHash, Scope: string(scopeSnapshot),
		Generation: operation.DispatchGeneration, ExpiresAt: now.Add(lifetime), CreatedAt: now,
	}
	callbackURL, err := cursorcloud.ManagedCallbackURL(binding.Launch.CallbackURL, grant.ID)
	if err != nil {
		return IssuedGrant{}, fmt.Errorf("managed MCP callback URL is invalid")
	}
	if err := repository.CreateManagedAgentToolGrant(ctx, &grant); err != nil {
		return IssuedGrant{}, fmt.Errorf("persist managed MCP grant: %w", err)
	}
	return IssuedGrant{
		Grant: grant, Token: token,
		URL: callbackURL,
	}, nil
}

func validateGrantIssuance(
	repository GrantWriter,
	binding *models.ManagedAgentBinding,
	operation *models.ManagedAgentOperation,
	toolProfile profile.Context,
) (string, error) {
	if repository == nil || binding == nil || operation == nil || !managedBindingActive(binding) ||
		!managedOperationActive(operation) || operation.BindingID != binding.ID ||
		operation.DispatchGeneration < 1 || operation.DispatchGeneration != binding.DispatchGeneration {
		return "", fmt.Errorf("managed MCP grant authority is incomplete")
	}
	if err := validateToolProfile(toolProfile); err != nil {
		return "", err
	}
	callbackURL, err := cursorcloud.ManagedCallbackURL(binding.Launch.CallbackURL, "grant")
	if err != nil {
		return "", fmt.Errorf("managed MCP callback URL is invalid")
	}
	return callbackURL, nil
}

func normalizeGrantTime(now time.Time, lifetime time.Duration) (time.Time, time.Duration) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if lifetime <= 0 || lifetime > MaxGrantLifetime {
		lifetime = MaxGrantLifetime
	}
	return now, lifetime
}

func newGrantToken() (string, string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", "", fmt.Errorf("generate managed MCP bearer: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	hash := sha256.Sum256([]byte(token))
	return token, hex.EncodeToString(hash[:]), nil
}

func NewTransport(config Config) (*Transport, error) {
	if config.Repository == nil || config.Authority == nil || config.Scope == nil || config.HandlerFactory == nil {
		return nil, fmt.Errorf("managed MCP transport dependencies are incomplete")
	}
	if config.Enabled == nil {
		config.Enabled = func() bool { return true }
	}
	if config.Now == nil {
		config.Now = time.Now
	}
	return &Transport{
		repository: config.Repository, authority: config.Authority, scope: config.Scope,
		enabled: config.Enabled, now: config.Now, factory: config.HandlerFactory,
	}, nil
}

func (t *Transport) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if t == nil || t.enabled == nil || !t.enabled() {
		http.NotFound(writer, request)
		return
	}
	authority, status := t.authorize(request)
	if status != 0 {
		writeUnavailable(writer, status)
		return
	}
	handler, err := t.factory(authority.context, authority.binding, authority.toolProfile, authority.endpointPath)
	if err != nil || handler == nil {
		writeUnavailable(writer, http.StatusServiceUnavailable)
		return
	}
	buffer := newBufferedResponse()
	handler.ServeHTTP(buffer, request.WithContext(authority.context))
	if !t.enabled() {
		writeUnavailable(writer, http.StatusUnauthorized)
		return
	}
	if _, status := t.authorize(request); status != 0 {
		writeUnavailable(writer, http.StatusUnauthorized)
		return
	}
	buffer.flush(writer)
}

type grantAuthority struct {
	context      context.Context
	binding      *models.ManagedAgentBinding
	toolProfile  profile.Context
	endpointPath string
}

func (t *Transport) authorize(request *http.Request) (grantAuthority, int) {
	grantID, ok := grantIDFromPath(request.URL.Path)
	if !ok {
		return grantAuthority{}, http.StatusUnauthorized
	}
	token, ok := bearerToken(request.Header.Get("Authorization"))
	if !ok {
		return grantAuthority{}, http.StatusUnauthorized
	}
	grant, err := t.repository.GetManagedAgentToolGrantByHash(request.Context(), hashBearer(token))
	if err != nil || !validGrant(grant, grantID, t.now()) {
		return grantAuthority{}, http.StatusUnauthorized
	}
	binding, _, status := t.loadGrantAuthority(request.Context(), grant)
	if status != 0 {
		return grantAuthority{}, status
	}
	toolProfile, err := decodeToolProfile(grant.Scope)
	if err != nil {
		return grantAuthority{}, http.StatusUnauthorized
	}
	ctx, status := t.resolveRequestContext(request.Context(), binding)
	if status != 0 {
		return grantAuthority{}, status
	}
	return grantAuthority{
		context: ctx, binding: binding, toolProfile: toolProfile,
		endpointPath: ManagedCallbackPath + grant.ID,
	}, 0
}

func (t *Transport) loadGrantAuthority(
	ctx context.Context,
	grant *models.ManagedAgentToolGrant,
) (*models.ManagedAgentBinding, *models.ManagedAgentOperation, int) {
	if grant == nil || grant.ID == "" || grant.BindingID == "" || grant.OperationID == "" {
		return nil, nil, http.StatusUnauthorized
	}
	binding, err := t.repository.GetManagedAgentBinding(ctx, grant.BindingID)
	if err != nil || !managedBindingActive(binding) || binding.DispatchGeneration != grant.Generation {
		return nil, nil, http.StatusUnauthorized
	}
	operation, err := t.repository.GetManagedAgentOperation(ctx, grant.OperationID)
	if err != nil || operation.BindingID != binding.ID || operation.DispatchGeneration != grant.Generation ||
		!managedOperationActive(operation) {
		return nil, nil, http.StatusUnauthorized
	}
	return binding, operation, 0
}

func (t *Transport) resolveRequestContext(ctx context.Context, binding *models.ManagedAgentBinding) (context.Context, int) {
	if err := t.validateBoundTask(ctx, binding); err != nil {
		return nil, http.StatusUnauthorized
	}
	if err := t.validateBoundSession(ctx, binding); err != nil {
		return nil, http.StatusUnauthorized
	}
	ctx, err := t.scope.ScopeOverridingIdentity(ctx, binding.TaskID)
	if err != nil || !identityMatchesBinding(ctx, binding) {
		return nil, http.StatusUnauthorized
	}
	ctx, err = t.scope.ScopePrincipal(ctx, binding.TaskID, binding.SessionID)
	if err != nil || !principalMatchesBinding(ctx, binding) {
		return nil, http.StatusUnauthorized
	}
	return ctx, 0
}

func (t *Transport) validateBoundTask(ctx context.Context, binding *models.ManagedAgentBinding) error {
	task, err := t.authority.GetTask(ctx, binding.TaskID)
	if err != nil || task == nil || task.ID != binding.TaskID || task.WorkspaceID != binding.WorkspaceID ||
		task.ArchivedAt != nil || task.IsFromOffice || task.Autopilot || task.IsEphemeral {
		return fmt.Errorf("managed MCP task authority is unavailable")
	}
	return nil
}

func (t *Transport) validateBoundSession(ctx context.Context, binding *models.ManagedAgentBinding) error {
	session, err := t.authority.GetTaskSession(ctx, binding.SessionID)
	if err != nil || !sessionMatchesBinding(session, binding) {
		return fmt.Errorf("managed MCP session authority is unavailable")
	}
	return nil
}

func identityMatchesBinding(ctx context.Context, binding *models.ManagedAgentBinding) bool {
	identity, hasIdentity := authn.IdentityFromContext(ctx)
	if hasIdentity && identity.UserID != binding.UserID {
		return false
	}
	return true
}

func principalMatchesBinding(ctx context.Context, binding *models.ManagedAgentBinding) bool {
	principal, ok := scope.PrincipalFromContext(ctx)
	return ok && principal.CallerTaskID == binding.TaskID && principal.CallerSessionID == binding.SessionID &&
		principal.WorkspaceID == binding.WorkspaceID && principal.Surface == profile.SurfaceKanbanTask
}

func sessionMatchesBinding(session *models.TaskSession, binding *models.ManagedAgentBinding) bool {
	if session == nil || session.ID != binding.SessionID || session.TaskID != binding.TaskID ||
		session.AgentExecutionID != binding.ExecutionID || session.ExecutorID != binding.ExecutorID ||
		session.ExecutorProfileID != binding.ExecutorProfileID {
		return false
	}
	switch session.State {
	case models.TaskSessionStateCreated, models.TaskSessionStateStarting,
		models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput:
		return true
	default:
		return false
	}
}

func validGrant(grant *models.ManagedAgentToolGrant, grantID string, now time.Time) bool {
	return grant != nil && grant.ID != "" && subtle.ConstantTimeCompare([]byte(grant.ID), []byte(grantID)) == 1 &&
		grant.RevokedAt == nil && grant.Generation > 0 && grant.ExpiresAt.After(now)
}

func managedBindingActive(binding *models.ManagedAgentBinding) bool {
	return binding != nil && (binding.Lifecycle == models.ManagedAgentBindingCreating || binding.Lifecycle == models.ManagedAgentBindingReady)
}

func managedOperationActive(operation *models.ManagedAgentOperation) bool {
	if operation == nil || operation.SettledAt != nil {
		return false
	}
	switch operation.State {
	case models.ManagedAgentSubmissionReserved, models.ManagedAgentSubmissionSubmitting,
		models.ManagedAgentSubmissionAccepted, models.ManagedAgentSubmissionUnknown:
		return true
	default:
		return false
	}
}

func validateToolProfile(toolProfile profile.Context) error {
	if toolProfile.Surface != profile.SurfaceManagedTask || len(toolProfile.Providers) != 0 {
		return fmt.Errorf("managed MCP tool profile is unsupported")
	}
	for _, capability := range toolProfile.Capabilities {
		switch capability {
		case profile.CapabilityUserQuestion, profile.CapabilityTaskTitle:
		default:
			return fmt.Errorf("managed MCP capability is unsupported")
		}
	}
	return nil
}

func decodeToolProfile(encoded string) (profile.Context, error) {
	var toolProfile profile.Context
	if encoded == "" || json.Unmarshal([]byte(encoded), &toolProfile) != nil {
		return profile.Context{}, errors.New("managed MCP profile is unavailable")
	}
	if err := validateToolProfile(toolProfile); err != nil {
		return profile.Context{}, err
	}
	return profile.New(toolProfile.Surface, toolProfile.Capabilities, nil), nil
}

func grantIDFromPath(path string) (string, bool) {
	if !strings.HasPrefix(path, ManagedCallbackPath) {
		return "", false
	}
	id := strings.TrimPrefix(path, ManagedCallbackPath)
	return id, id != "" && !strings.Contains(id, "/")
}

func bearerToken(header string) (string, bool) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", false
	}
	return parts[1], true
}

func hashBearer(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func writeUnavailable(writer http.ResponseWriter, status int) {
	writer.Header().Set("Cache-Control", "no-store")
	http.Error(writer, "managed MCP callback unavailable", status)
}

type bufferedResponse struct {
	header   http.Header
	status   int
	body     []byte
	overflow bool
}

func newBufferedResponse() *bufferedResponse {
	return &bufferedResponse{header: make(http.Header)}
}

func (b *bufferedResponse) Header() http.Header { return b.header }

func (b *bufferedResponse) WriteHeader(status int) {
	if b.status == 0 {
		b.status = status
	}
}

func (b *bufferedResponse) Write(body []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	room := maxBufferedResponse - len(b.body)
	if room < len(body) {
		b.overflow = true
		if room > 0 {
			b.body = append(b.body, body[:room]...)
		}
		return 0, fmt.Errorf("managed MCP response exceeds the buffer limit")
	}
	b.body = append(b.body, body...)
	return len(body), nil
}

func (b *bufferedResponse) Flush() {}

func (b *bufferedResponse) flush(writer http.ResponseWriter) {
	if b.overflow {
		writeUnavailable(writer, http.StatusBadGateway)
		return
	}
	for key, values := range b.header {
		for _, value := range values {
			writer.Header().Add(key, value)
		}
	}
	writer.Header().Set("Cache-Control", "no-store")
	if b.status == 0 {
		b.status = http.StatusOK
	}
	writer.WriteHeader(b.status)
	_, _ = writer.Write(b.body)
}

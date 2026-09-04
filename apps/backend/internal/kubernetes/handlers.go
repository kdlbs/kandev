// Package kubernetes exposes Kubernetes executor diagnostics and active-session status.
package kubernetes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	authorizationv1 "k8s.io/api/authorization/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeclient "k8s.io/client-go/kubernetes"

	agentkubernetes "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	ws "github.com/kandev/kandev/pkg/websocket"
)

var (
	errAuthenticationRequired = errors.New("authentication required")
	errAdminRequired          = errors.New("administrator identity required")
	errExecutorIDRequired     = errors.New("executor id required")
	errTaskIDRequired         = errors.New("task id is required when session id is provided")
	errExecutorNotKubernetes  = errors.New("executor is not a Kubernetes executor")
	errSessionStatusNotWired  = errors.New("kubernetes session status is unavailable")
	errStreamingProbeCleanup  = errors.New("streaming probe cleanup failed")
)

const (
	errorJSONKey             = "error"
	verbGet                  = "get"
	verbCreate               = "create"
	resourcePods             = "pods"
	resourcePersistentClaims = "persistentvolumeclaims"
	connectionTestIdentity   = "connection-test"
	metadataNamespace        = "kubernetes_namespace"
	metadataPodName          = "kubernetes_pod_name"
	metadataPodUID           = "kubernetes_pod_uid"
	metadataMainContainer    = "kubernetes_main_container"
	metadataWorkspaceMode    = "kubernetes_workspace_mode"
	metadataResourceExecutor = agentkubernetes.MetadataKeyResourceExecutorID
	metadataResourceProfile  = agentkubernetes.MetadataKeyResourceProfileID
	metadataResourceInstance = agentkubernetes.MetadataKeyResourceInstanceID
	metadataResourceTask     = agentkubernetes.MetadataKeyResourceTaskID
	metadataResourceSession  = agentkubernetes.MetadataKeyResourceSessionID
	metadataResourceEnv      = agentkubernetes.MetadataKeyResourceEnvironmentID
)

type ResourceRepository interface {
	ListExecutorsRunning(ctx context.Context) ([]*models.ExecutorRunning, error)
	GetTaskSession(ctx context.Context, id string) (*models.TaskSession, error)
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

type AccessChecker interface {
	AuthorizeTaskAccess(ctx context.Context, taskID string) error
}

type ClientFactory func(config agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error)

type StreamingProbe func(
	ctx context.Context,
	client *agentkubernetes.Client,
	namespace string,
	podName string,
	mainContainer string,
) error

type Handler struct {
	repo           ResourceRepository
	access         AccessChecker
	clients        ClientFactory
	probeStreaming StreamingProbe
}

type TestRequest struct {
	Config        map[string]string `json:"config"`
	ProfileConfig map[string]string `json:"profile_config,omitempty"`
}

type TestStep struct {
	Key        string `json:"key"`
	Success    bool   `json:"success"`
	DurationMS int64  `json:"duration_ms"`
	Detail     string `json:"detail"`
	Error      string `json:"error,omitempty"`
}

type TestResult struct {
	Success       bool                      `json:"success"`
	ServerVersion string                    `json:"server_version,omitempty"`
	Namespace     string                    `json:"namespace,omitempty"`
	Steps         []TestStep                `json:"steps"`
	Warnings      []agentkubernetes.Warning `json:"warnings"`
	Error         string                    `json:"error,omitempty"`
}

type requiredPermission struct {
	verb        string
	resource    string
	subresource string
}

func NewHandler(repo ResourceRepository, access AccessChecker, clients ClientFactory) *Handler {
	if clients == nil {
		clients = func(config agentkubernetes.ExecutorConfig) (*agentkubernetes.Client, error) {
			return agentkubernetes.NewClient(
				config,
				agentkubernetes.DefaultConfigLoader(),
				nil,
			)
		}
	}
	return &Handler{
		repo: repo, access: access, clients: clients,
		probeStreaming: probeStreamingTransport,
	}
}

// RegisterRoutes wires Kubernetes executor diagnostics.
func RegisterRoutes(
	router *gin.Engine,
	dispatcher *ws.Dispatcher,
	repo ResourceRepository,
	access AccessChecker,
	_ *logger.Logger,
) {
	handler := NewHandler(repo, access, nil)
	handler.registerHTTP(router)
	handler.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1/kubernetes")
	api.POST("/test", h.httpTest)
	api.GET("/executors/:id/sessions", h.httpListSessions)
	api.GET("/executors/:id/session-impact", h.httpSessionImpact)
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc("kubernetes.test", h.wsTest)
	dispatcher.RegisterFunc("kubernetes.sessions.list", h.wsListSessions)
	dispatcher.RegisterFunc("kubernetes.sessions.impact", h.wsSessionImpact)
}

func (h *Handler) httpTest(c *gin.Context) {
	if err := requireAdmin(c.Request.Context()); err != nil {
		writeAdminError(c, err)
		return
	}
	var request TestRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{errorJSONKey: "invalid request body"})
		return
	}
	c.JSON(http.StatusOK, h.testConnection(c.Request.Context(), request))
}

func (h *Handler) wsTest(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if err := requireAdmin(ctx); err != nil {
		return adminWSError(msg, err)
	}
	var request TestRequest
	if err := msg.ParsePayload(&request); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, h.testConnection(ctx, request))
}

func requireAdmin(ctx context.Context) error {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok {
		return errAuthenticationRequired
	}
	if !identity.IsAdmin() {
		return errAdminRequired
	}
	return nil
}

func writeAdminError(c *gin.Context, err error) {
	if errors.Is(err, errAuthenticationRequired) {
		c.JSON(http.StatusUnauthorized, gin.H{errorJSONKey: err.Error()})
		return
	}
	c.JSON(http.StatusForbidden, gin.H{errorJSONKey: err.Error()})
}

func adminWSError(msg *ws.Message, err error) (*ws.Message, error) {
	code := ws.ErrorCodeForbidden
	if errors.Is(err, errAuthenticationRequired) {
		code = ws.ErrorCodeUnauthorized
	}
	return ws.NewError(msg.ID, msg.Action, code, err.Error(), nil)
}

func (h *Handler) httpListSessions(c *gin.Context) {
	executorID := strings.TrimSpace(c.Param("id"))
	filter := SessionFilter{
		TaskID:    strings.TrimSpace(c.Query("task_id")),
		SessionID: strings.TrimSpace(c.Query("session_id")),
	}
	rows, err := h.listSessions(c.Request.Context(), executorID, filter)
	if err != nil {
		status, message := sessionHTTPError(err)
		c.JSON(status, gin.H{errorJSONKey: message})
		return
	}
	c.JSON(http.StatusOK, rows)
}

func (h *Handler) wsListSessions(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		ExecutorID string `json:"executor_id"`
		TaskID     string `json:"task_id,omitempty"`
		SessionID  string `json:"session_id,omitempty"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	rows, err := h.listSessions(ctx, strings.TrimSpace(payload.ExecutorID), SessionFilter{
		TaskID:    strings.TrimSpace(payload.TaskID),
		SessionID: strings.TrimSpace(payload.SessionID),
	})
	if err != nil {
		code, message := sessionWSError(err)
		return ws.NewError(msg.ID, msg.Action, code, message, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, rows)
}

func (h *Handler) httpSessionImpact(c *gin.Context) {
	if err := requireAdmin(c.Request.Context()); err != nil {
		writeAdminError(c, err)
		return
	}
	impact, err := h.sessionImpact(c.Request.Context(), strings.TrimSpace(c.Param("id")))
	if err != nil {
		status, message := sessionHTTPError(err)
		c.JSON(status, gin.H{errorJSONKey: message})
		return
	}
	c.JSON(http.StatusOK, impact)
}

func (h *Handler) wsSessionImpact(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	if err := requireAdmin(ctx); err != nil {
		return adminWSError(msg, err)
	}
	var payload struct {
		ExecutorID string `json:"executor_id"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	impact, err := h.sessionImpact(ctx, strings.TrimSpace(payload.ExecutorID))
	if err != nil {
		code, message := sessionWSError(err)
		return ws.NewError(msg.ID, msg.Action, code, message, nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, impact)
}

func sessionHTTPError(err error) (int, string) {
	switch {
	case errors.Is(err, errExecutorIDRequired), errors.Is(err, errTaskIDRequired),
		errors.Is(err, errExecutorNotKubernetes):
		return http.StatusBadRequest, err.Error()
	case errors.Is(err, models.ErrExecutorNotFound):
		return http.StatusNotFound, "Kubernetes executor not found"
	default:
		return http.StatusInternalServerError, "Kubernetes session status is unavailable"
	}
}

func sessionWSError(err error) (string, string) {
	switch {
	case errors.Is(err, errExecutorIDRequired), errors.Is(err, errTaskIDRequired),
		errors.Is(err, errExecutorNotKubernetes):
		return ws.ErrorCodeBadRequest, err.Error()
	case errors.Is(err, models.ErrExecutorNotFound):
		return ws.ErrorCodeNotFound, "Kubernetes executor not found"
	default:
		return ws.ErrorCodeInternalError, "Kubernetes session status is unavailable"
	}
}

func (h *Handler) testConnection(ctx context.Context, request TestRequest) *TestResult {
	result := &TestResult{Steps: make([]TestStep, 0, 6), Warnings: []agentkubernetes.Warning{}}
	config, err := agentkubernetes.ParseExecutorConfig(request.Config)
	if err != nil {
		result.addFailedStep("configuration", time.Now(), "Executor configuration is invalid", err)
		return result
	}
	profile, err := parseOptionalProfile(request.ProfileConfig)
	if err != nil {
		result.addFailedStep("configuration", time.Now(), "Profile configuration is invalid", err)
		return result
	}
	result.Namespace = config.Namespace
	client, err := h.clients(config)
	if err != nil || client == nil || client.Clientset == nil {
		if err == nil {
			err = errors.New("kubernetes client is unavailable")
		}
		result.addFailedStep("discovery", time.Now(), "Kubernetes API discovery failed", err)
		return result
	}
	testCtx, cancel := context.WithTimeout(ctx, time.Duration(config.RequestTimeoutSeconds)*time.Second)
	defer cancel()
	if !runDiscoveryStep(testCtx, client.Clientset, result) {
		return result
	}
	runNamespaceStep(config.Namespace, result)
	basePermissions := []requiredPermission{
		{verb: verbGet, resource: resourcePods},
		{verb: verbCreate, resource: resourcePods},
		{verb: "delete", resource: resourcePods},
		{verb: "watch", resource: resourcePods},
	}
	basePermissions = append(basePermissions, workspacePermissions(profile)...)
	rbacStep := runPermissionStep(
		testCtx, client.Clientset, config.Namespace, "rbac", basePermissions,
	)
	result.Steps = append(result.Steps, rbacStep)
	admissionSuccess := true
	if profile != nil {
		admissionSuccess = false
		if rbacStep.Success {
			admissionSuccess = runAdmissionSteps(
				testCtx, client.Clientset, config.Namespace, *profile, result,
			)
		}
	}
	streamingPermissions := []requiredPermission{
		{verb: verbGet, resource: resourcePods, subresource: "exec"},
		{verb: verbCreate, resource: resourcePods, subresource: "exec"},
		{verb: verbGet, resource: resourcePods, subresource: "portforward"},
		{verb: verbCreate, resource: resourcePods, subresource: "portforward"},
	}
	streamingStep := h.runStreamingStep(
		testCtx, client, config.Namespace, streamingPermissions,
		profile, rbacStep.Success && admissionSuccess,
	)
	result.Steps = append(result.Steps, streamingStep)
	result.finalize()
	return result
}

func parseOptionalProfile(values map[string]string) (*agentkubernetes.ProfileConfig, error) {
	if len(values) == 0 {
		return nil, nil
	}
	profile, err := agentkubernetes.ParseProfileConfig(values)
	if err != nil {
		return nil, err
	}
	return &profile, nil
}

func workspacePermissions(profile *agentkubernetes.ProfileConfig) []requiredPermission {
	if profile == nil {
		return nil
	}
	switch profile.Workspace.Mode {
	case agentkubernetes.WorkspaceModeManagedPVC:
		return []requiredPermission{
			{verb: verbGet, resource: resourcePersistentClaims},
			{verb: verbCreate, resource: resourcePersistentClaims},
			{verb: "delete", resource: resourcePersistentClaims},
		}
	case agentkubernetes.WorkspaceModeExistingClaim:
		return []requiredPermission{{verb: verbGet, resource: resourcePersistentClaims}}
	default:
		return nil
	}
}

func runAdmissionSteps(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	profile agentkubernetes.ProfileConfig,
	result *TestResult,
) bool {
	firstStep := len(result.Steps)
	resourceName := "kandev-admission-probe-" + uuid.NewString()[:8]
	identity := agentkubernetes.ResourceIdentity{
		ExecutorID: connectionTestIdentity, ProfileID: connectionTestIdentity, InstanceID: connectionTestIdentity,
		TaskID: connectionTestIdentity, SessionID: connectionTestIdentity, EnvironmentID: connectionTestIdentity,
	}
	switch profile.Workspace.Mode {
	case agentkubernetes.WorkspaceModeManagedPVC:
		runPVCDryRun(ctx, client, namespace, resourceName, identity, profile, result)
	case agentkubernetes.WorkspaceModeExistingClaim:
		runExistingClaimCheck(ctx, client, namespace, profile.Workspace.ClaimName, result)
	}
	runPodDryRun(ctx, client, namespace, resourceName, identity, profile, result)
	for _, step := range result.Steps[firstStep:] {
		if !step.Success {
			return false
		}
	}
	return true
}

func runExistingClaimCheck(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	claimName string,
	result *TestResult,
) {
	started := time.Now()
	claim, err := client.CoreV1().PersistentVolumeClaims(namespace).Get(
		ctx, claimName, metav1.GetOptions{},
	)
	if err == nil && (claim == nil || claim.Namespace != namespace || claim.Name != claimName || claim.UID == "") {
		err = errors.New("existing workspace claim identity is incomplete or mismatched")
	}
	if err != nil {
		result.Steps = append(result.Steps, failedStep(
			"storage.existing_claim", started, "Existing workspace claim lookup failed", err,
		))
		return
	}
	result.Steps = append(result.Steps, TestStep{
		Key: "storage.existing_claim", Success: true,
		DurationMS: time.Since(started).Milliseconds(),
		Detail:     "Existing workspace claim is available",
	})
}

func runPVCDryRun(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	name string,
	identity agentkubernetes.ResourceIdentity,
	profile agentkubernetes.ProfileConfig,
	result *TestResult,
) {
	started := time.Now()
	claim, err := agentkubernetes.BuildPersistentVolumeClaim(profile.Workspace, agentkubernetes.PVCOptions{
		Name: name, Namespace: namespace, Identity: identity,
	})
	if err == nil {
		var admitted *corev1.PersistentVolumeClaim
		admitted, err = client.CoreV1().PersistentVolumeClaims(namespace).Create(
			ctx, claim, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}},
		)
		if err == nil {
			err = agentkubernetes.ValidateAdmittedPVC(admitted, claim)
		}
		if err == nil && admitted.UID == "" {
			err = errors.New("admitted PVC dry-run UID is unavailable")
		}
		if err == nil && admitted.Spec.VolumeName != claim.Spec.VolumeName {
			err = errors.New("admitted PVC dry-run changed the volume name")
		}
	}
	appendAdmissionStep(result, "admission.pvc", started, "PVC dry-run admission succeeded", err)
}

func runPodDryRun(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	name string,
	identity agentkubernetes.ResourceIdentity,
	profile agentkubernetes.ProfileConfig,
	result *TestResult,
) {
	started := time.Now()
	template, err := agentkubernetes.ParsePodTemplate(profile.PodTemplateYAML)
	var pod *corev1.Pod
	var warnings []agentkubernetes.Warning
	if err == nil {
		pod, warnings, err = agentkubernetes.ComposePod(template, profile, agentkubernetes.PodOptions{
			Name: name, Namespace: namespace, Identity: identity,
			Command: []string{"sh"}, Args: []string{"-c", "sleep 1"},
			WorkingDir: "/workspace", AgentctlPort: agentkubernetes.DefaultAgentctlPort,
			ManagedPVCName: name,
		})
	}
	if err == nil {
		result.Warnings = append(result.Warnings, warnings...)
		var admitted *corev1.Pod
		admitted, err = client.CoreV1().Pods(namespace).Create(
			ctx, pod, metav1.CreateOptions{DryRun: []string{metav1.DryRunAll}},
		)
		if err == nil {
			err = agentkubernetes.ValidateAdmittedPod(admitted, pod, profile.MainContainer)
		}
		if err == nil && admitted.UID == "" {
			err = errors.New("admitted Pod dry-run UID is unavailable")
		}
	}
	appendAdmissionStep(result, "admission.pod", started, "Pod dry-run admission succeeded", err)
}

func appendAdmissionStep(result *TestResult, key string, started time.Time, detail string, err error) {
	if err != nil {
		result.Steps = append(result.Steps, failedStep(key, started, "Dry-run admission failed", err))
		return
	}
	result.Steps = append(result.Steps, TestStep{
		Key: key, Success: true, DurationMS: time.Since(started).Milliseconds(), Detail: detail,
	})
}

func runDiscoveryStep(ctx context.Context, client kubeclient.Interface, result *TestResult) bool {
	started := time.Now()
	serverVersion, err := client.Discovery().ServerVersion()
	if err != nil {
		result.addFailedStep("discovery", started, "Kubernetes API discovery failed", err)
		return false
	}
	result.ServerVersion = serverVersion.GitVersion
	result.Steps = append(result.Steps, TestStep{
		Key: "discovery", Success: true, DurationMS: time.Since(started).Milliseconds(),
		Detail: "Kubernetes API discovery succeeded",
	})
	return true
}

func runNamespaceStep(namespace string, result *TestResult) {
	started := time.Now()
	result.Steps = append(result.Steps, TestStep{
		Key: "namespace", Success: true, DurationMS: time.Since(started).Milliseconds(),
		Detail: "Using configured namespace " + namespace,
	})
}

func runPermissionStep(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	key string,
	permissions []requiredPermission,
) TestStep {
	started := time.Now()
	denied := make([]string, 0)
	for _, permission := range permissions {
		allowed, err := checkPermission(ctx, client, namespace, permission)
		if err != nil {
			return failedStep(key, started, "Permission check failed", err)
		}
		if !allowed {
			denied = append(denied, permission.String())
		}
	}
	if len(denied) > 0 {
		return TestStep{
			Key: key, Success: false, DurationMS: time.Since(started).Milliseconds(),
			Detail: "Denied permissions: " + strings.Join(denied, ", "),
			Error:  "required Kubernetes permissions are denied",
		}
	}
	return TestStep{
		Key: key, Success: true, DurationMS: time.Since(started).Milliseconds(),
		Detail: fmt.Sprintf("All %d required permissions are allowed", len(permissions)),
	}
}

func checkPermission(
	ctx context.Context,
	client kubeclient.Interface,
	namespace string,
	permission requiredPermission,
) (bool, error) {
	review := &authorizationv1.SelfSubjectAccessReview{Spec: authorizationv1.SelfSubjectAccessReviewSpec{
		ResourceAttributes: &authorizationv1.ResourceAttributes{
			Namespace: namespace, Verb: permission.verb,
			Resource: permission.resource, Subresource: permission.subresource,
		},
	}}
	created, err := client.AuthorizationV1().SelfSubjectAccessReviews().Create(ctx, review, metav1.CreateOptions{})
	if err != nil {
		return false, err
	}
	return created.Status.Allowed, nil
}

func (p requiredPermission) String() string {
	resource := p.resource
	if p.subresource != "" {
		resource += "/" + p.subresource
	}
	return p.verb + " " + resource
}

func (r *TestResult) addFailedStep(key string, started time.Time, detail string, err error) {
	step := failedStep(key, started, detail, err)
	r.Steps = append(r.Steps, step)
	r.Error = step.Error
}

func failedStep(key string, started time.Time, detail string, err error) TestStep {
	return TestStep{
		Key: key, Success: false, DurationMS: time.Since(started).Milliseconds(),
		Detail: detail, Error: sanitizeError(err),
	}
}

func (r *TestResult) finalize() {
	r.Success = true
	for _, step := range r.Steps {
		if !step.Success {
			r.Success = false
			if r.Error == "" {
				r.Error = step.Error
			}
		}
	}
}

func sanitizeError(err error) string {
	if err == nil {
		return "Kubernetes request failed"
	}
	var fieldErr *agentkubernetes.FieldError
	if errors.As(err, &fieldErr) {
		return fieldErr.Error()
	}
	return routingerr.SanitizeError(err).Error()
}

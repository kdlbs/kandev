package sprites

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	ws "github.com/kandev/kandev/pkg/websocket"
)

const (
	spritesPrefix   = "kandev-"
	requestTimeout  = 30 * time.Second
	testStepTimeout = 60 * time.Second
)

// Handler provides HTTP and WebSocket handlers for Sprites management.
type Handler struct {
	secretStore secrets.SecretStore
	logger      *logger.Logger
}

// NewHandler creates a new sprites handler.
func NewHandler(secretStore secrets.SecretStore, log *logger.Logger) *Handler {
	return &Handler{
		secretStore: secretStore,
		logger:      log.WithFields(zap.String("component", "sprites-handler")),
	}
}

// RegisterRoutes registers both HTTP and WS handlers.
func RegisterRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, secretStore secrets.SecretStore, log *logger.Logger) {
	h := NewHandler(secretStore, log)
	h.registerHTTP(router)
	h.registerWS(dispatcher)
}

func (h *Handler) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1/sprites")
	api.GET("/status", h.httpStatus)
	api.GET("/instances", h.httpListInstances)
	api.DELETE("/instances/:name", h.httpDestroyInstance)
	api.DELETE("/instances", h.httpDestroyAll)
	api.POST("/test", h.httpTest)
	api.GET("/network-policies", h.httpGetNetworkPolicy)
	api.PUT("/network-policies", h.httpUpdateNetworkPolicy)
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionSpritesStatus, h.wsStatus)
	dispatcher.RegisterFunc(ws.ActionSpritesInstancesList, h.wsListInstances)
	dispatcher.RegisterFunc(ws.ActionSpritesInstancesDestroy, h.wsDestroyInstance)
	dispatcher.RegisterFunc(ws.ActionSpritesTest, h.wsTest)
	dispatcher.RegisterFunc(ws.ActionSpritesNetworkPolicyGet, h.wsGetNetworkPolicy)
	dispatcher.RegisterFunc(ws.ActionSpritesNetworkPolicyUpdate, h.wsUpdateNetworkPolicy)
}

// --- Response types ---

// SpritesStatus is the status response.
type SpritesStatus struct {
	Connected       bool   `json:"connected"`
	TokenConfigured bool   `json:"token_configured"`
	InstanceCount   int    `json:"instance_count"`
	Error           string `json:"error,omitempty"`
}

// SpritesInstance represents a running sprite.
type SpritesInstance struct {
	Name          string `json:"name"`
	HealthStatus  string `json:"health_status"`
	CreatedAt     string `json:"created_at"`
	UptimeSeconds int64  `json:"uptime_seconds"`
}

// SpritesTestResult is the test connection result.
type SpritesTestResult struct {
	Success         bool              `json:"success"`
	Steps           []SpritesTestStep `json:"steps"`
	TotalDurationMs int64             `json:"total_duration_ms"`
	SpriteName      string            `json:"sprite_name"`
	Error           string            `json:"error,omitempty"`
}

// SpritesTestStep is a single step in the test.
type SpritesTestStep struct {
	Name       string `json:"name"`
	DurationMs int64  `json:"duration_ms"`
	Success    bool   `json:"success"`
	Error      string `json:"error,omitempty"`
}

// --- HTTP handlers ---

func (h *Handler) httpStatus(c *gin.Context) {
	secretID := c.Query("secret_id")
	status := h.getStatus(c.Request.Context(), secretID)
	c.JSON(http.StatusOK, status)
}

func (h *Handler) httpListInstances(c *gin.Context) {
	secretID := c.Query("secret_id")
	instances, err := h.listInstances(c.Request.Context(), secretID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, instances)
}

func (h *Handler) httpDestroyInstance(c *gin.Context) {
	secretID := c.Query("secret_id")
	name := c.Param("name")
	if err := h.destroyInstance(c.Request.Context(), secretID, name); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (h *Handler) httpDestroyAll(c *gin.Context) {
	secretID := c.Query("secret_id")
	count, err := h.destroyAll(c.Request.Context(), secretID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "destroyed": count})
}

func (h *Handler) httpTest(c *gin.Context) {
	secretID := c.Query("secret_id")
	result := h.testConnection(c.Request.Context(), secretID)
	c.JSON(http.StatusOK, result)
}

func (h *Handler) httpGetNetworkPolicy(c *gin.Context) {
	secretID := c.Query("secret_id")
	spriteName := c.Query("sprite_name")
	if spriteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sprite_name is required"})
		return
	}
	policy, err := h.getNetworkPolicy(c.Request.Context(), secretID, spriteName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, policy)
}

func (h *Handler) httpUpdateNetworkPolicy(c *gin.Context) {
	secretID := c.Query("secret_id")
	spriteName := c.Query("sprite_name")
	if spriteName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sprite_name is required"})
		return
	}

	var policy sprites.NetworkPolicy
	if err := c.ShouldBindJSON(&policy); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	if err := h.updateNetworkPolicy(c.Request.Context(), secretID, spriteName, &policy); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// --- WS handlers ---

func (h *Handler) wsStatus(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID string `json:"secret_id"`
	}
	_ = msg.ParsePayload(&payload)
	return ws.NewResponse(msg.ID, msg.Action, h.getStatus(ctx, payload.SecretID))
}

func (h *Handler) wsListInstances(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID string `json:"secret_id"`
	}
	_ = msg.ParsePayload(&payload)
	instances, err := h.listInstances(ctx, payload.SecretID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, instances)
}

func (h *Handler) wsDestroyInstance(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID string `json:"secret_id"`
		Name     string `json:"name"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	if err := h.destroyInstance(ctx, payload.SecretID, payload.Name); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

func (h *Handler) wsTest(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID string `json:"secret_id"`
	}
	_ = msg.ParsePayload(&payload)
	return ws.NewResponse(msg.ID, msg.Action, h.testConnection(ctx, payload.SecretID))
}

func (h *Handler) wsGetNetworkPolicy(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID   string `json:"secret_id"`
		SpriteName string `json:"sprite_name"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	if payload.SpriteName == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "sprite_name is required", nil)
	}
	policy, err := h.getNetworkPolicy(ctx, payload.SecretID, payload.SpriteName)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, policy)
}

func (h *Handler) wsUpdateNetworkPolicy(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var payload struct {
		SecretID   string                `json:"secret_id"`
		SpriteName string                `json:"sprite_name"`
		Policy     sprites.NetworkPolicy `json:"policy"`
	}
	if err := msg.ParsePayload(&payload); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload", nil)
	}
	if payload.SpriteName == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "sprite_name is required", nil)
	}
	if err := h.updateNetworkPolicy(ctx, payload.SecretID, payload.SpriteName, &payload.Policy); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}

// --- Business logic ---

func (h *Handler) getToken(ctx context.Context, secretID string) (string, error) {
	if h.secretStore == nil {
		return "", fmt.Errorf("secret store not available")
	}
	if secretID == "" {
		return "", fmt.Errorf("secret_id is required")
	}
	return h.secretStore.Reveal(ctx, secretID)
}

func (h *Handler) getStatus(ctx context.Context, secretID string) *SpritesStatus {
	token, err := h.getToken(ctx, secretID)
	if err != nil {
		return &SpritesStatus{TokenConfigured: false}
	}
	if token == "" {
		return &SpritesStatus{TokenConfigured: false}
	}

	instances, err := h.listInstances(ctx, secretID)
	if err != nil {
		return &SpritesStatus{
			TokenConfigured: true,
			Connected:       false,
			Error:           err.Error(),
		}
	}

	return &SpritesStatus{
		TokenConfigured: true,
		Connected:       true,
		InstanceCount:   len(instances),
	}
}

func (h *Handler) listInstances(ctx context.Context, secretID string) ([]*SpritesInstance, error) {
	client, err := h.createClient(ctx, secretID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	list, err := client.ListSprites(reqCtx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to list sprites: %w", err)
	}

	var result []*SpritesInstance
	for _, s := range list.Sprites {
		if !strings.HasPrefix(s.Name, spritesPrefix) {
			continue
		}
		createdAt := s.CreatedAt.Format(time.RFC3339)
		result = append(result, &SpritesInstance{
			Name:          s.Name,
			HealthStatus:  s.Status,
			CreatedAt:     createdAt,
			UptimeSeconds: computeUptime(createdAt),
		})
	}
	return result, nil
}

func (h *Handler) destroyInstance(ctx context.Context, secretID, name string) error {
	token, err := h.getToken(ctx, secretID)
	if err != nil {
		return fmt.Errorf("API token not configured: %w", err)
	}

	client := sprites.New(token)
	sprite := client.Sprite(name)
	if err := sprite.Destroy(); err != nil {
		return fmt.Errorf("failed to destroy sprite %q: %w", name, err)
	}
	h.logger.Info("destroyed sprite", zap.String("name", name))
	return nil
}

func (h *Handler) destroyAll(ctx context.Context, secretID string) (int, error) {
	instances, err := h.listInstances(ctx, secretID)
	if err != nil {
		return 0, err
	}

	token, err := h.getToken(ctx, secretID)
	if err != nil {
		return 0, err
	}

	destroyed := 0
	client := sprites.New(token)
	for _, inst := range instances {
		sprite := client.Sprite(inst.Name)
		if err := sprite.Destroy(); err != nil {
			h.logger.Warn("failed to destroy sprite", zap.String("name", inst.Name), zap.Error(err))
			continue
		}
		destroyed++
	}
	h.logger.Info("destroyed all kandev sprites", zap.Int("count", destroyed))
	return destroyed, nil
}

func (h *Handler) testConnection(ctx context.Context, secretID string) *SpritesTestResult {
	start := time.Now()
	result := &SpritesTestResult{}

	// Step 1: Get token
	tokenStep := h.runTestStep("Get API token", func() error {
		_, err := h.getToken(ctx, secretID)
		return err
	})
	result.Steps = append(result.Steps, tokenStep)
	if !tokenStep.Success {
		result.Error = tokenStep.Error
		result.TotalDurationMs = time.Since(start).Milliseconds()
		return result
	}

	// Step 2: List sprites (verifies token + API connectivity)
	var instanceCount int
	listStep := h.runTestStep("List sprites", func() error {
		instances, err := h.listInstances(ctx, secretID)
		if err != nil {
			return err
		}
		instanceCount = len(instances)
		return nil
	})
	result.Steps = append(result.Steps, listStep)
	if listStep.Success {
		result.SpriteName = fmt.Sprintf("%d active sprite(s)", instanceCount)
	}

	result.Success = tokenStep.Success && listStep.Success
	if !result.Success {
		for _, s := range result.Steps {
			if s.Error != "" {
				result.Error = s.Error
				break
			}
		}
	}
	result.TotalDurationMs = time.Since(start).Milliseconds()
	return result
}

func (h *Handler) runTestStep(name string, fn func() error) SpritesTestStep {
	start := time.Now()
	err := fn()
	step := SpritesTestStep{
		Name:       name,
		DurationMs: time.Since(start).Milliseconds(),
		Success:    err == nil,
	}
	if err != nil {
		step.Error = err.Error()
	}
	return step
}

func (h *Handler) createClient(ctx context.Context, secretID string) (*sprites.Client, error) {
	token, err := h.getToken(ctx, secretID)
	if err != nil {
		return nil, fmt.Errorf("API token not configured: %w", err)
	}
	return sprites.New(token, sprites.WithDisableControl()), nil
}

func (h *Handler) getNetworkPolicy(ctx context.Context, secretID, spriteName string) (*sprites.NetworkPolicy, error) {
	client, err := h.createClient(ctx, secretID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = client.Close() }()

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	policy, err := client.GetNetworkPolicy(reqCtx, spriteName)
	if err != nil {
		return nil, fmt.Errorf("failed to get network policy for %q: %w", spriteName, err)
	}
	return policy, nil
}

func (h *Handler) updateNetworkPolicy(ctx context.Context, secretID, spriteName string, policy *sprites.NetworkPolicy) error {
	client, err := h.createClient(ctx, secretID)
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	if err := client.UpdateNetworkPolicy(reqCtx, spriteName, policy); err != nil {
		return fmt.Errorf("failed to update network policy for %q: %w", spriteName, err)
	}
	h.logger.Info("updated network policy",
		zap.String("sprite_name", spriteName),
		zap.Int("rule_count", len(policy.Rules)))
	return nil
}

func computeUptime(createdAt string) int64 {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return 0
	}
	return int64(time.Since(t).Seconds())
}

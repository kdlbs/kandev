package sprites

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	spritesAPIBase  = "https://api.sprites.dev/v1"
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
}

func (h *Handler) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionSpritesStatus, h.wsStatus)
	dispatcher.RegisterFunc(ws.ActionSpritesInstancesList, h.wsListInstances)
	dispatcher.RegisterFunc(ws.ActionSpritesInstancesDestroy, h.wsDestroyInstance)
	dispatcher.RegisterFunc(ws.ActionSpritesTest, h.wsTest)
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
	Success        bool            `json:"success"`
	Steps          []SpritesTestStep `json:"steps"`
	TotalDurationMs int64          `json:"total_duration_ms"`
	SpriteName     string          `json:"sprite_name"`
	Error          string          `json:"error,omitempty"`
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
	token, err := h.getToken(ctx, secretID)
	if err != nil {
		return nil, fmt.Errorf("API token not configured: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, spritesAPIBase+"/sprites", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var apiSprites []struct {
		Name      string `json:"name"`
		CreatedAt string `json:"created_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiSprites); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	var result []*SpritesInstance
	for _, s := range apiSprites {
		if !strings.HasPrefix(s.Name, spritesPrefix) {
			continue
		}
		uptime := computeUptime(s.CreatedAt)
		result = append(result, &SpritesInstance{
			Name:          s.Name,
			HealthStatus:  "unknown",
			CreatedAt:     s.CreatedAt,
			UptimeSeconds: uptime,
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
	spriteName := fmt.Sprintf("kandev-test-%d", time.Now().UnixMilli())
	result := &SpritesTestResult{SpriteName: spriteName}

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

	token, _ := h.getToken(ctx, secretID)
	client := sprites.New(token)
	sprite := client.Sprite(spriteName)

	// Step 2: Create sprite (lazy, via first command)
	createStep := h.runTestStep("Create sprite", func() error {
		stepCtx, cancel := context.WithTimeout(ctx, testStepTimeout)
		defer cancel()
		out, err := sprite.CommandContext(stepCtx, "echo", "hello-kandev").Output()
		if err != nil {
			return err
		}
		if !strings.Contains(string(out), "hello-kandev") {
			return fmt.Errorf("unexpected output: %s", string(out))
		}
		return nil
	})
	result.Steps = append(result.Steps, createStep)

	// Step 3: Destroy
	destroyStep := h.runTestStep("Destroy sprite", func() error {
		return sprite.Destroy()
	})
	result.Steps = append(result.Steps, destroyStep)

	result.Success = tokenStep.Success && createStep.Success && destroyStep.Success
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

func computeUptime(createdAt string) int64 {
	t, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		return 0
	}
	return int64(time.Since(t).Seconds())
}

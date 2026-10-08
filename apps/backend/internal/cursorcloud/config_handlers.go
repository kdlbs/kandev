package cursorcloud

import (
	"context"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/secrets"
)

type ConfigClient interface {
	ListModels(context.Context) (ModelCatalog, error)
	ListRepositories(context.Context) (RepositoryCatalog, error)
}

type ConfigClientFactory func(string) (ConfigClient, error)

type ConfigHandlers struct {
	store     secrets.SecretStore
	enabled   bool
	newClient ConfigClientFactory
}

func NewConfigHandlers(store secrets.SecretStore, enabled bool, factory ConfigClientFactory) *ConfigHandlers {
	if factory == nil {
		factory = func(apiKey string) (ConfigClient, error) {
			return NewRuntimeClient(apiKey, enabled)
		}
	}
	return &ConfigHandlers{store: store, enabled: enabled, newClient: factory}
}

func RegisterConfigRoutes(router *gin.Engine, store secrets.SecretStore, enabled bool, factory ConfigClientFactory) {
	if !enabled {
		return
	}
	h := NewConfigHandlers(store, enabled, factory)
	api := router.Group("/api/v1/cursor-cloud")
	api.POST("/test", h.httpTestConnection)
	api.POST("/catalog", h.httpCatalog)
}

type configRequest struct {
	SecretID    string `json:"secret_id"`
	CallbackURL string `json:"callback_url"`
}

type configResponse struct {
	Connected          bool         `json:"connected"`
	CallbackRoute      string       `json:"callback_route,omitempty"`
	CursorReachability string       `json:"cursor_reachability,omitempty"`
	Models             []Model      `json:"models,omitempty"`
	Repositories       []Repository `json:"repositories,omitempty"`
}

func (h *ConfigHandlers) httpTestConnection(c *gin.Context) {
	request, ok := h.decodeRequest(c)
	if !ok {
		return
	}
	client, ok := h.authorizedClient(c, request)
	if !ok {
		return
	}
	if _, err := client.ListModels(c.Request.Context()); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cursor Cloud connection failed"})
		return
	}
	c.JSON(http.StatusOK, configResponse{
		Connected: true, CallbackRoute: "configured", CursorReachability: "not_verified",
	})
}

func (h *ConfigHandlers) httpCatalog(c *gin.Context) {
	request, ok := h.decodeRequest(c)
	if !ok {
		return
	}
	client, ok := h.authorizedClient(c, request)
	if !ok {
		return
	}
	models, err := client.ListModels(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cursor Cloud catalog is unavailable"})
		return
	}
	repositories, err := client.ListRepositories(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Cursor Cloud catalog is unavailable"})
		return
	}
	c.JSON(http.StatusOK, configResponse{
		Connected: true, CallbackRoute: "configured", CursorReachability: "not_verified",
		Models: models.Items, Repositories: repositories.Items,
	})
}

func (h *ConfigHandlers) decodeRequest(c *gin.Context) (configRequest, bool) {
	var request configRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Cursor Cloud configuration request"})
		return configRequest{}, false
	}
	request.SecretID = strings.TrimSpace(request.SecretID)
	request.CallbackURL = strings.TrimSpace(request.CallbackURL)
	if request.SecretID == "" || ValidateCallbackURL(request.CallbackURL) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "A global API key secret and valid callback URL are required"})
		return configRequest{}, false
	}
	return request, true
}

func (h *ConfigHandlers) authorizedClient(c *gin.Context, request configRequest) (ConfigClient, bool) {
	if !h.enabled {
		c.JSON(http.StatusNotFound, gin.H{"error": "Cursor Cloud is disabled"})
		return nil, false
	}
	if err := secrets.ValidateGlobalReference(c.Request.Context(), h.store, request.SecretID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cursor Cloud API key secret is unavailable"})
		return nil, false
	}
	apiKey, err := h.store.Reveal(c.Request.Context(), request.SecretID)
	if err != nil || strings.TrimSpace(apiKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Cursor Cloud API key secret is unavailable"})
		return nil, false
	}
	client, err := h.newClient(apiKey)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Cursor Cloud is unavailable"})
		return nil, false
	}
	return client, true
}

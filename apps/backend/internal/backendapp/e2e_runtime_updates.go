package backendapp

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/agents"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type e2eRuntimeUpdateNotifier interface {
	HandleAgentRuntimeUpdate(context.Context, agents.RuntimeUpdateNotice)
}

type e2eRuntimeUpdateHooks struct {
	latestVersion string
	ready         chan struct{}
	once          sync.Once
}

func newE2ERuntimeUpdateHooks() *e2eRuntimeUpdateHooks {
	if os.Getenv("KANDEV_E2E_MOCK") != "true" || os.Getenv("KANDEV_MOCK_AGENT") != "true" {
		return nil
	}
	latestVersion := strings.TrimSpace(os.Getenv("KANDEV_E2E_RUNTIME_UPDATE_LATEST_VERSION"))
	if latestVersion == "" {
		return nil
	}
	return &e2eRuntimeUpdateHooks{latestVersion: latestVersion, ready: make(chan struct{})}
}

func (h *e2eRuntimeUpdateHooks) resolveLatestVersion(ctx context.Context, _ string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", ctx.Err()
	}
	return h.latestVersion, nil
}

func (h *e2eRuntimeUpdateHooks) startupReadiness(ctx context.Context, hostUtilityReady <-chan struct{}) <-chan struct{} {
	ready := make(chan struct{})
	go func() {
		select {
		case <-hostUtilityReady:
		case <-ctx.Done():
			return
		}
		select {
		case <-h.ready:
			close(ready)
		case <-ctx.Done():
		}
	}()
	return ready
}

func (h *e2eRuntimeUpdateHooks) release() {
	h.once.Do(func() { close(h.ready) })
}

type e2eRuntimeUpdateStartupRequest struct {
	AgentName       string `json:"agent_name"`
	RuntimeID       string `json:"runtime_id"`
	DisplayName     string `json:"display_name"`
	PreviousVersion string `json:"previous_version"`
	TargetVersion   string `json:"target_version"`
}

func registerE2ERuntimeUpdateRoutes(
	router *gin.Engine,
	controller *agentsettingscontroller.Controller,
	notifier e2eRuntimeUpdateNotifier,
	hooks *e2eRuntimeUpdateHooks,
	log *logger.Logger,
) {
	if hooks == nil || controller == nil || notifier == nil {
		return
	}
	api := router.Group("/api/v1/e2e/runtime-updates")
	api.POST("/startup", func(c *gin.Context) {
		var request e2eRuntimeUpdateStartupRequest
		if err := c.ShouldBindJSON(&request); err != nil || request.AgentName == "" || request.RuntimeID == "" || request.DisplayName == "" || request.TargetVersion == "" {
			c.JSON(http.StatusBadRequest, gin.H{errKey: "runtime update outcome fields are required"})
			return
		}

		hooks.release()
		if err := controller.ReplayRuntimeUpdateNotices(c.Request.Context()); err != nil {
			log.Warn("E2E runtime update startup replay failed", zap.Error(err))
			c.JSON(http.StatusServiceUnavailable, gin.H{errKey: "runtime update discovery failed"})
			return
		}
		notifier.HandleAgentRuntimeUpdate(c.Request.Context(), agents.RuntimeUpdateNotice{
			AgentID: request.AgentName, RuntimeID: request.RuntimeID, DisplayName: request.DisplayName,
			PreviousVersion: request.PreviousVersion, Version: request.TargetVersion, Status: "failed",
			URL:          "/settings/agents#runtime-update-" + url.PathEscape(request.AgentName),
			OccurrenceID: fmt.Sprintf("e2e-runtime-update:%d", time.Now().UnixNano()),
		})
		c.JSON(http.StatusAccepted, gin.H{"status": "started"})
	})
}

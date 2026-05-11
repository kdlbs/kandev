package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/agent/discovery"
	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// buildCommandString builds a display-friendly command string with proper quoting.
func buildCommandString(cmd []string) string {
	var parts []string
	for _, arg := range cmd {
		if strings.ContainsAny(arg, " \t\n\"'`$\\") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			parts = append(parts, "\""+escaped+"\"")
		} else {
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

var (
	ErrAgentNotFound        = errors.New("agent not found")
	ErrAgentAlreadyExists   = errors.New("agent already exists")
	ErrAgentProfileNotFound = errors.New("agent profile not found")
	ErrAgentMcpUnsupported  = errors.New("mcp not supported by agent")
	ErrModelRequired        = errors.New("model is required for agent profiles")
	ErrLogoNotAvailable     = errors.New("logo not available for agent")
	ErrInvalidSlug          = errors.New("display name must produce a valid slug")
	ErrCommandRequired      = errors.New("command is required")
)

type Controller struct {
	repo           store.Repository
	discovery      *discovery.Registry
	agentRegistry  *registry.Registry
	sessionChecker SessionChecker
	mcpService     *mcpconfig.Service
	modelCache     *modelfetcher.Cache
	hostUtility    *hostutility.Manager
	jobStore       *JobStore
	hub            JobBroadcaster
	logger         *logger.Logger
}

// ErrProfileInUseDetail is returned when a profile cannot be deleted because active sessions exist.
type ErrProfileInUseDetail struct {
	ActiveSessions []agentdto.ActiveTaskInfo
}

func (e *ErrProfileInUseDetail) Error() string {
	return fmt.Sprintf("agent profile is used by %d active session(s)", len(e.ActiveSessions))
}

type SessionChecker interface {
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
	DeleteEphemeralTasksByAgentProfile(ctx context.Context, agentProfileID string) (int64, error)
	GetActiveTaskInfoByAgentProfile(ctx context.Context, agentProfileID string) ([]agentdto.ActiveTaskInfo, error)
}

func NewController(repo store.Repository, discoveryRegistry *discovery.Registry, agentRegistry *registry.Registry, sessionChecker SessionChecker, log *logger.Logger,
) *Controller {
	return &Controller{
		repo:           repo,
		discovery:      discoveryRegistry,
		agentRegistry:  agentRegistry,
		sessionChecker: sessionChecker,
		mcpService:     mcpconfig.NewService(repo),
		modelCache:     modelfetcher.NewCache(),
		logger:         log.WithFields(zap.String("component", "agent-settings-controller")),
	}
}

// SetHostUtility wires the host utility manager into the controller so that
// endpoints like /agent-models can read the cached capability data. Called
// once at startup after the host utility manager is constructed; leaving this
// unset simply causes the model endpoints to report "not_configured".
func (c *Controller) SetHostUtility(h *hostutility.Manager) {
	c.hostUtility = h
}

// SetJobBroadcaster initializes the install job store with a WS broadcaster
// for streaming install progress. Called once during handler registration.
// If unset (hub == nil), the streaming install API returns
// ErrJobStoreUnavailable — without this guard a nil hub would silently
// degrade to a non-broadcasting store and the UI would never see progress.
func (c *Controller) SetJobBroadcaster(hub JobBroadcaster) {
	c.hub = hub
	if hub == nil {
		c.jobStore = nil
		return
	}
	c.jobStore = NewJobStore(hub, c.logger.Zap(), func(agentName string) {
		c.InvalidateDiscoveryCache()
		// Kick a fresh capability probe immediately so the UI doesn't sit on
		// stale "not_installed" until the next periodic poll. When the probe
		// finishes, re-broadcast the updated availability so any open profile
		// page transitions out of "Probing…" without a manual refresh.
		if c.hostUtility != nil {
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				if _, err := c.hostUtility.Refresh(ctx, agentName); err != nil {
					c.logger.Debug("post-install capability refresh failed",
						zap.String("agent", agentName), zap.Error(err))
				}
				c.BroadcastAvailableAgents()
			}()
		}
		c.logger.Info("install succeeded", zap.String("agent", agentName))
	})
}

// BroadcastAvailableAgents fetches the current available-agents snapshot and
// pushes it over WS as `agent.available.updated`. Used after install + probe
// so the UI flips from "probing" to the resolved status without a refresh.
func (c *Controller) BroadcastAvailableAgents() {
	if c.hub == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	resp, err := c.ListAvailableAgents(ctx)
	if err != nil {
		c.logger.Debug("broadcast available agents: list failed", zap.Error(err))
		return
	}
	msg, err := ws.NewNotification(ws.ActionAgentAvailableUpdated, map[string]any{
		"agents": resp.Agents,
		"tools":  resp.Tools,
	})
	if err != nil {
		return
	}
	c.hub.Broadcast(msg)
}

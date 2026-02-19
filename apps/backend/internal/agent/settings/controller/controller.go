package controller

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
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
	ErrAgentProfileInUse    = errors.New("agent profile is used by an active agent session")
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
	logger         *logger.Logger
}

type SessionChecker interface {
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/agent/discovery"
	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/hostutility"
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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/agent/discovery"
	agentdto "github.com/kandev/kandev/internal/agent/dto"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/modelfetcher"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// buildCommandString builds a display-friendly command string with proper quoting.
func buildCommandString(cmd []string) string {
	var parts []string
	for _, arg := range cmd {
		switch {
		case arg == "":
			parts = append(parts, `""`)
		case strings.ContainsAny(arg, " \t\n\"'`$\\"):
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			parts = append(parts, "\""+escaped+"\"")
		default:
			parts = append(parts, arg)
		}
	}
	return strings.Join(parts, " ")
}

var (
	ErrAgentNotFound                        = errors.New("agent not found")
	ErrAgentAlreadyExists                   = errors.New("agent already exists")
	ErrAgentProfileNotFound                 = errors.New("agent profile not found")
	ErrAgentMcpUnsupported                  = errors.New("mcp not supported by agent")
	ErrModelRequired                        = errors.New("model is required for agent profiles")
	ErrLogoNotAvailable                     = errors.New("logo not available for agent")
	ErrInvalidSlug                          = errors.New("display name must produce a valid slug")
	ErrCommandRequired                      = errors.New("command is required")
	ErrInvalidProfileEnvVars                = errors.New("invalid profile env vars")
	ErrInvalidCommandPrefix                 = errors.New("invalid command prefix")
	ErrUnknownMCPStrategy                   = errors.New("unknown MCP strategy")
	ErrNotCustomTUIAgent                    = errors.New("agent is not a custom TUI agent")
	ErrDynamicAgentRoutingDisabled          = errors.New("dynamic agent routing is disabled")
	ErrDynamicProfileCandidatesRequired     = errors.New("dynamic profile candidates are required")
	ErrDynamicProfilePositions              = errors.New("dynamic profile candidate positions must be contiguous")
	ErrDynamicProfileRule                   = errors.New("unsupported dynamic profile rule")
	ErrDynamicProfileCandidate              = errors.New("invalid dynamic profile candidate")
	ErrDynamicProfileVersionConflict        = store.ErrDynamicProfileVersionConflict
	ErrDynamicProfileDuplicationUnsupported = errors.New("dynamic profile duplication is not supported")
)

type Controller struct {
	repo                        store.Repository
	discovery                   *discovery.Registry
	agentRegistry               *registry.Registry
	sessionChecker              SessionChecker
	watcherDeps                 WatcherDependencyChecker
	routingTierDeps             RoutingTierDependencyChecker
	automationDeps              AutomationDependencyChecker
	utilityDeps                 UtilityDependencyChecker
	mcpService                  *mcpconfig.Service
	modelCache                  *modelfetcher.Cache
	hostUtility                 hostUtilityProvider
	jobStore                    *JobStore
	updateJobStore              *AgentUpdateJobStore
	runtimeUpdater              RuntimeUpdater
	managedRuntimeSelections    managedruntime.SelectionStore
	maintenance                 *maintenanceCoordinator
	hub                         JobBroadcaster
	logger                      *logger.Logger
	secretStore                 secrets.SecretStore
	runtimeUpdateStatusMu       sync.Mutex
	runtimeUpdateStatusCache    map[string]runtimeUpdateStatusCacheEntry
	runtimeUpdateStatusNow      func() time.Time
	runtimeUpdateStatusResolver RuntimeUpdateStatusResolver
	runtimeUpdateStatusLookup   chan struct{}
	dynamicAgentRoutingEnabled  bool
}

// SetDynamicAgentRoutingEnabled applies the authoritative runtime flag to the
// settings boundary. Dynamic configuration remains readable when disabled, but
// all dynamic writes fail before they create or alter rows.
func (c *Controller) SetDynamicAgentRoutingEnabled(enabled bool) {
	c.dynamicAgentRoutingEnabled = enabled
}

// SetSecretStore wires the metadata-only validator used by shared agent
// profiles. Workspace-scoped references are intentionally rejected here.
func (c *Controller) SetSecretStore(secretStore secrets.SecretStore) {
	c.secretStore = secretStore
}

// SetWatcherDependencyChecker wires in the watcher dependency enumerator so
// DeleteProfile can include referencing watchers in ErrProfileInUseDetail.
// Optional; when unset the delete path keeps its pre-watcher behaviour.
func (c *Controller) SetWatcherDependencyChecker(w WatcherDependencyChecker) {
	c.watcherDeps = w
}

// SetRoutingTierDependencyChecker wires in workspace routing tier lookups so
// DeleteProfile can reject profiles selected as a workspace tier source.
func (c *Controller) SetRoutingTierDependencyChecker(r RoutingTierDependencyChecker) {
	c.routingTierDeps = r
}

// SetAutomationDependencyChecker wires in the automation enumerator so
// DeleteProfile can name the automations that would be left pointing at a
// deleted profile. Optional; when unset the delete path keeps its
// pre-automation behaviour.
func (c *Controller) SetAutomationDependencyChecker(a AutomationDependencyChecker) {
	c.automationDeps = a
}

// SetUtilityDependencyChecker wires utility-agent binding lookups used by
// disable and delete confirmation flows.
func (c *Controller) SetUtilityDependencyChecker(u UtilityDependencyChecker) {
	c.utilityDeps = u
}

// ErrProfileInUseDetail is returned when a profile cannot be deleted because
// active sessions or external integration watchers reference it. The UI uses
// the breakdown to render a "this will also disable N watchers — continue?"
// confirmation dialog before re-issuing the request with force=true.
type ErrProfileInUseDetail struct {
	ActiveSessions  []agentdto.ActiveTaskInfo
	Watchers        []WatcherReference
	RoutingTiers    []RoutingTierReference
	Automations     []AutomationReference
	UtilityAgents   []UtilityAgentReference
	DynamicProfiles []DynamicProfileReference
}

func (e *ErrProfileInUseDetail) Error() string {
	return fmt.Sprintf(
		"agent profile is used by %d active session(s), %d watcher(s), %d routing tier(s), %d automation(s), %d utility agent(s), and %d dynamic profile(s)",
		len(e.ActiveSessions), len(e.Watchers), len(e.RoutingTiers), len(e.Automations), len(e.UtilityAgents), len(e.DynamicProfiles))
}

type UtilityAgentReference struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// DynamicProfileReference identifies a dynamic profile that contains the
// concrete profile as a candidate. The route remains stored after a forced
// disable or delete so the router can skip it and the user can repair it.
type DynamicProfileReference struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

type UtilityDependencyChecker interface {
	ListUtilityAgentsByAgentProfile(context.Context, string) ([]UtilityAgentReference, error)
	ClearUtilityAgentProfileBindings(context.Context, string) error
}

// WatcherReference points at one issue/PR watcher row that uses the profile
// being deleted. Kind is the integration name ("linear", "jira",
// "github_issue", "github_review"). Label is a short human-friendly string
// (the filter, repo list, or JQL clipped to a UI-safe length by the producer).
type WatcherReference struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

// RoutingTierReference points at a workspace provider-routing tier that was
// seeded from the profile being deleted.
type RoutingTierReference struct {
	WorkspaceID string `json:"workspace_id"`
	ProviderID  string `json:"provider_id"`
	Tier        string `json:"tier"`
}

// WatcherDependencyChecker enumerates watcher rows that reference an agent
// profile and disables them on force-delete. Implementations live in
// cmd/kandev (one per integration store); the controller stays decoupled
// from linear/jira/github packages.
//
// ListWatchersByAgentProfile feeds the confirmation dialog; the user sees
// the list and confirms. DisableWatchersByAgentProfile fires on force-delete
// so the watcher row reflects the deletion immediately — without it, the
// watcher stays enabled-but-orphaned until its next external trigger fires
// the lazy preflight, which never happens for filters that match nothing
// new after the profile is deleted.
// AutomationReference points at one enabled automation that would be left
// referencing a deleted agent profile. An automation is configuration rather
// than a session: nothing is running, so it does not show up in the active-task
// list, but its next firing would launch against a profile that no longer
// exists.
type AutomationReference struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	WorkspaceID string `json:"workspace_id"`
}

// AutomationDependencyChecker enumerates enabled automations bound to an agent
// profile and disables them when that profile is deleted.
//
// ListEnabledAutomationsByAgentProfile feeds the confirmation dialog.
// DisableAutomationsByAgentProfile runs on the delete path *before* the profile
// row is removed and its error aborts the delete — unlike the watcher
// equivalent, which runs after and is best-effort. The asymmetry is deliberate:
// a watcher left enabled against a deleted profile is repaired by the dispatch
// coordinator's preflight on the next poll, and an automation has no such
// backstop, so the only safe moment to disable it is while the delete can still
// be called off.
type AutomationDependencyChecker interface {
	ListEnabledAutomationsByAgentProfile(ctx context.Context, agentProfileID string) ([]AutomationReference, error)
	DisableAutomationsByAgentProfile(ctx context.Context, agentProfileID string) ([]AutomationReference, error)
}

type WatcherDependencyChecker interface {
	ListWatchersByAgentProfile(ctx context.Context, agentProfileID string) ([]WatcherReference, error)
	DisableWatchersByAgentProfile(ctx context.Context, agentProfileID, cause string) ([]WatcherReference, error)
}

type SessionChecker interface {
	HasActiveTaskSessionsByAgentProfile(ctx context.Context, agentProfileID string) (bool, error)
	DeleteEphemeralTasksByAgentProfile(ctx context.Context, agentProfileID string) (int64, error)
	GetActiveTaskInfoByAgentProfile(ctx context.Context, agentProfileID string) ([]agentdto.ActiveTaskInfo, error)
}

type RoutingTierDependencyChecker interface {
	ListRoutingTierReferencesByAgentProfile(ctx context.Context, profileID string) ([]RoutingTierReference, error)
}

type hostUtilityProvider interface {
	Get(agentType string) (hostutility.AgentCapabilities, bool)
	Refresh(ctx context.Context, agentType string) (hostutility.AgentCapabilities, error)
	ResolveModelConfig(
		ctx context.Context,
		agentType string,
		req hostutility.ModelConfigResolutionRequest,
	) (hostutility.ModelConfigResolution, error)
}

func NewController(repo store.Repository, discoveryRegistry *discovery.Registry, agentRegistry *registry.Registry, sessionChecker SessionChecker, log *logger.Logger,
) *Controller {
	return &Controller{
		repo:                      repo,
		discovery:                 discoveryRegistry,
		agentRegistry:             agentRegistry,
		sessionChecker:            sessionChecker,
		mcpService:                mcpconfig.NewService(repo),
		modelCache:                modelfetcher.NewCache(),
		logger:                    log.WithFields(zap.String("component", "agent-settings-controller")),
		runtimeUpdateStatusCache:  make(map[string]runtimeUpdateStatusCacheEntry),
		runtimeUpdateStatusNow:    time.Now,
		runtimeUpdateStatusLookup: make(chan struct{}, runtimeUpdateStatusMaxConcurrent),
	}
}

// SetHostUtility wires the host utility manager into the controller so that
// endpoints like /agent-models can read the cached capability data. Called
// once at startup after the host utility manager is constructed; leaving this
// unset simply causes the model endpoints to report "not_configured".
func (c *Controller) SetHostUtility(h *hostutility.Manager) {
	c.hostUtility = h
	if h == nil {
		c.runtimeUpdater = nil
		c.updateJobStore = nil
		return
	}
	c.SetRuntimeUpdater(&hostRuntimeUpdater{
		host:     h,
		executor: execDirectCommandExecutor{},
	})
}

// SetRuntimeUpdater replaces the process/probe boundary used by update jobs.
// Production wires the host utility implementation; embedders may provide a
// deterministic implementation without invoking npm.
func (c *Controller) SetRuntimeUpdater(updater RuntimeUpdater) {
	c.runtimeUpdater = updater
	c.initializeUpdateJobStore()
}

// SetManagedRuntimeSelectionStore wires the install-wide active-version
// persistence used by previews, jobs, and the available-agent catalogue.
func (c *Controller) SetManagedRuntimeSelectionStore(store managedruntime.SelectionStore) {
	c.managedRuntimeSelections = store
	c.initializeUpdateJobStore()
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
		c.updateJobStore = nil
		c.maintenance = nil
		return
	}
	c.maintenance = newMaintenanceCoordinator()
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
	}, c.maintenance)
	c.initializeUpdateJobStore()
}

func (c *Controller) initializeUpdateJobStore() {
	if c.hub == nil || c.runtimeUpdater == nil {
		c.updateJobStore = nil
		return
	}
	if c.maintenance == nil {
		c.maintenance = newMaintenanceCoordinator()
	}
	c.updateJobStore = NewAgentUpdateJobStore(
		c.hub,
		c.logger.Zap(),
		c.runtimeUpdater,
		c.maintenance,
		c.BroadcastAvailableAgents,
		c.managedRuntimeSelections,
	)
	c.updateJobStore.SetStatusInvalidator(c.InvalidateRuntimeUpdateStatus)
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

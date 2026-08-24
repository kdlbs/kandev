package plugins

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"

	"go.uber.org/zap"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/mcp/plugintools"
	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

type userStateCleanupStore interface {
	DeleteAllForPlugin(context.Context, string) error
}

// Service is the core plugin service: install/uninstall, the in-memory
// Registry, the lifecycle state machine, and the runtime.Manager wiring
// that spawns/supervises each plugin's subprocess.
//
// # Extension points
//
// Event delivery (internal/plugins/delivery) is wired in by backendapp
// after Provide, following the same post-construction "SetX" pattern
// internal/jira/service.go uses for SetTaskDeleter / SetRepositoryLookup
// (avoids an import cycle between this package and its siblings):
//
//   - SetDeliverer(d Deliverer) attaches the event-delivery subsystem.
//     Install, Uninstall, Enable, Disable, and any successful SetStatus
//     call notify the attached Deliverer via Refresh() so it can
//     re-subscribe to the event bus based on current registry state.
//   - StateStore() exposes the already-constructed *state.Store so the
//     HTTP layer doesn't need a second NewStore(pool) call.
//   - Registry() and EventBus() are exposed for any other read-only wiring
//     (e.g. proxies checking a plugin's manifest/capabilities without
//     going through Service's error-wrapping Get).
type Service struct {
	mu sync.Mutex
	// ownershipMu makes cross-plugin provider/reference ownership checks and
	// transitions into active one atomic reservation. Per-plugin lifecycle
	// locks cannot protect two different IDs claiming the same identity.
	ownershipMu sync.Mutex

	// syncMu serializes Sync/bootScan calls (service_sync.go) so concurrent
	// operator clicks — or a boot scan racing an operator-triggered sync —
	// cannot double-install the same dropped tarball or dir sideload.
	syncMu sync.Mutex

	// lifecycleLocks serializes Enable/Disable/Install/InstallFromURL/
	// Uninstall/UpdateConfig per plugin id, so two near-simultaneous
	// lifecycle requests for the same id (e.g. two Enable clicks) cannot
	// both pass an idempotency check built on a stale read and race each
	// other's status-machine transition. Different ids stay fully
	// concurrent. Never taken by handleStatusChange (the runtime.Manager
	// supervision-goroutine callback) — that path only touches s.mu — so
	// holding a lifecycleLocks entry while calling into PluginRuntime
	// cannot deadlock against it.
	lifecycleLocks *keyedMutex
	// dispatchLocks keep lifecycle replacement/disable boundaries from racing
	// authenticated actions and reference RPCs. Dispatch holds a read lease for
	// the full RPC; lifecycle mutation holds the write side.
	dispatchLocks *keyedRWMutex

	// agentToolInstallMu makes exposed-name collision validation and registry
	// insertion one atomic catalog mutation across different plugin IDs.
	agentToolInstallMu sync.Mutex

	// extractingMu guards extractingPaths and, crucially, is held across the
	// pkgtar extraction that registers into it: a version directory must never
	// be observable on disk before it is marked in flight, or a concurrent
	// prune could delete it in that gap. See extractPackage.
	extractingMu sync.Mutex
	// extractingPaths counts, per version directory, the installs that have
	// extracted it but have not yet finished. Install extracts before it can
	// know the plugin id (and therefore before it can take that id's lifecycle
	// lock), so this is what tells a prune running under the lock that a
	// directory belongs to an install still waiting for it.
	extractingPaths map[string]int

	pluginsDir       string
	store            store.Store
	registry         *Registry
	state            *state.Store
	userState        *state.UserStore
	userStateCleanup userStateCleanupStore
	eventBus         bus.EventBus
	log              *logger.Logger

	deliverer                Deliverer
	agentToolCatalogListener AgentToolCatalogListener
	agentToolGeneration      string
	agentToolRevision        uint64
	agentToolSnapshot        plugintools.Snapshot
	agentToolSnapshotReady   bool
	runtime                  PluginRuntime
	secrets                  SecretVault

	// revokeGitCredentialProvider invalidates leases for a repository provider
	// when its owning plugin is no longer active. It is wired by backendapp to
	// the provider-neutral broker after both subsystems are constructed.
	revokeGitCredentialProvider func(string)

	// Host data API (ADR 0043) service-layer dependencies, wired via
	// SetDataSources and handed to every pluginHost hostForPlugin builds.
	// nil until backendapp calls SetDataSources (see its doc comment); a
	// pluginHost built before that falls back to Unimplemented for these
	// accessors regardless of declared capabilities (see host_data.go's
	// accessor nil-checks).
	taskData         taskDataSource
	workflows        workflowLister
	workflowSteps    workflowStepLister
	agentProfiles    agentProfileDataSource
	sessionCodeStats sessionCodeStatsSource
	messageData      messageDataSource
	taskWriter       taskWriter

	// Utility agent invocation (ADR 0048), wired via SetUtilityAgent.
	utilityAgents utilityAgentSource
	utilityRunner utilityRunner

	// Host data API write dependencies wired late via SetWriteDeps (ADR
	// 0043): the task-message delivery path and the orchestrator task-starter,
	// both constructed after boot-active plugins spawn (see writeDeps on
	// pluginHost). Mutex-guarded against the concurrent hostForPlugin reads.
	messenger   taskMessenger
	taskStarter taskStarter

	// authLogin establishes an authenticated browser session for an external
	// identity an auth-capable plugin asserts via its webhook response
	// (OIDC/SAML SSO), wired via SetAuthLoginBridge. nil until backendapp
	// wires it (only when the auth service exists); a nil bridge means the SSO
	// login directive is rejected.
	authLogin AuthLoginBridge

	// kandevVersion is the currently running kandev build version, used to
	// enforce a package's manifest.min_kandev_version at Install (see
	// SetKandevVersion / checkMinKandevVersion). Empty (the default, e.g. in
	// tests that construct a Service directly) or the "dev" sentinel means no
	// enforcement; backendapp wires the real ldflags-injected version.
	kandevVersion string

	httpClient *http.Client

	// marketplace is the plugin-discovery catalog service (nil until
	// SetMarketplace is called by Provide). See marketplace.go.
	marketplace *marketplace.Service

	// settings persists instance-wide plugin preferences (the auto-update
	// default). nil until SetSettings is called by Provide; the auto-update
	// accessors treat a nil store as "default off, no overrides possible".
	settings *settingsStore

	reservedReferenceSources       map[string]struct{}
	reservedReferenceProviderKinds map[string]struct{}
}

// ReferenceIdentity reserves a host-owned composer source and its canonical
// provider/kind pair so a plugin cannot shadow a built-in integration.
type ReferenceIdentity struct {
	Source   string
	Provider string
	Kind     string
}

// NewService wires a Service from its already-constructed dependencies.
// Provide is the usual entry point in production; NewService is exposed
// directly for tests that want a fake store.Store/PluginRuntime.
func NewService(pluginStore store.Store, registry *Registry, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:               pluginStore,
		registry:            registry,
		eventBus:            eventBus,
		log:                 log,
		httpClient:          &http.Client{},
		lifecycleLocks:      newKeyedMutex(),
		dispatchLocks:       newKeyedRWMutex(),
		agentToolGeneration: uuid.NewString(),
	}
}

// SetGitCredentialLeaseRevoker wires immediate provider-lease revocation for
// plugin lifecycle changes. The callback receives manifest-declared provider
// IDs, never a plugin ID, because broker leases are scoped by provider.
func (s *Service) SetGitCredentialLeaseRevoker(revoker func(string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokeGitCredentialProvider = revoker
}

// SetReservedReferenceIdentities installs the host-owned mention vocabulary.
// It runs during backend composition before active plugin processes start.
// Persisted active records that now collide are demoted to error so the
// dynamic mention bridge cannot make backend startup fail.
func (s *Service) SetReservedReferenceIdentities(identities []ReferenceIdentity) {
	sources := make(map[string]struct{}, len(identities))
	providerKinds := make(map[string]struct{}, len(identities))
	for _, identity := range identities {
		source := strings.TrimSpace(identity.Source)
		provider := strings.TrimSpace(identity.Provider)
		kind := strings.TrimSpace(identity.Kind)
		if source == "" || provider == "" || kind == "" {
			continue
		}
		sources[source] = struct{}{}
		providerKinds[provider+"\x00"+kind] = struct{}{}
	}
	s.mu.Lock()
	s.reservedReferenceSources = sources
	s.reservedReferenceProviderKinds = providerKinds
	s.mu.Unlock()

	for _, record := range s.List() {
		if record.Status != StatusActive || !s.collidesWithReservedReference(record.ReferenceSources) {
			continue
		}
		if err := s.setStatus(record.ID, StatusError); err != nil {
			s.log.Warn("plugins: could not revoke host-owned reference collision",
				zap.String("plugin_id", record.ID), zap.Error(err))
		}
	}
}

func (s *Service) collidesWithReservedReference(sources []manifest.ReferenceSource) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, source := range sources {
		if _, reserved := s.reservedReferenceSources[source.Source]; reserved {
			return true
		}
		if _, reserved := s.reservedReferenceProviderKinds[source.Provider+"\x00"+source.Kind]; reserved {
			return true
		}
	}
	return false
}

func (s *Service) revokeGitCredentialProviderLeases(providers []string) {
	s.mu.Lock()
	revoker := s.revokeGitCredentialProvider
	s.mu.Unlock()
	if revoker == nil {
		return
	}
	for _, provider := range providers {
		revoker(provider)
	}
}

// keyedMutex hands out a *sync.Mutex per key, creating it on first use and
// keeping it around for the process lifetime (the plugin id keyspace is
// small and long-lived, so there is nothing to garbage-collect). Mirrors the
// parentMutex pattern in internal/task/service/handoff_service.go.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

type keyedRWMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.RWMutex
}

func newKeyedRWMutex() *keyedRWMutex {
	return &keyedRWMutex{locks: make(map[string]*sync.RWMutex)}
}

func (k *keyedRWMutex) lockFor(key string) *sync.RWMutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	lock, ok := k.locks[key]
	if !ok {
		lock = &sync.RWMutex{}
		k.locks[key] = lock
	}
	return lock
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*sync.Mutex)}
}

// lockFor returns the mutex for key, creating it if this is the first call
// for that key. Callers must Lock/Unlock the returned mutex themselves.
func (k *keyedMutex) lockFor(key string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, ok := k.locks[key]
	if !ok {
		m = &sync.Mutex{}
		k.locks[key] = m
	}
	return m
}

// SetDeliverer attaches the event-delivery subsystem. See the "Extension
// points" doc comment on Service. Safe to call at most once during startup
// wiring; not safe to call concurrently with Install/SetStatus/Uninstall.
func (s *Service) SetDeliverer(d Deliverer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliverer = d
}

// SetAgentToolCatalogListener attaches the dynamic MCP registry bridge.
func (s *Service) SetAgentToolCatalogListener(listener AgentToolCatalogListener) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.agentToolCatalogListener = listener
}

func (s *Service) notifyAgentToolCatalogChanged() {
	s.mu.Lock()
	listener := s.agentToolCatalogListener
	s.mu.Unlock()
	if listener != nil {
		listener.NotifyAgentToolCatalogChanged()
	}
}

// Deliverer returns the currently attached event-delivery subsystem, or nil
// if none has been attached yet.
func (s *Service) Deliverer() Deliverer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliverer
}

// SetState wires the already-constructed plugin_state store. Provide calls
// this; also exposed for tests (in this package and others, e.g.
// internal/backendapp) that build a Service without going through Provide.
func (s *Service) SetState(st *state.Store) {
	s.state = st
}

// StateStore returns the plugin_state store Provide constructed, for the
// Host RPC implementation (host.go) and any HTTP wiring that needs it
// without re-initializing the schema.
func (s *Service) StateStore() *state.Store {
	return s.state
}

// SetUserState wires the already-constructed plugin_user_state store. Provide
// calls this; also exposed for tests that build a Service without going
// through Provide. See UserState's doc comment for how it differs from
// StateStore.
func (s *Service) SetUserState(st *state.UserStore) {
	s.userState = st
	s.userStateCleanup = st
}

// setUserStateCleanupStore replaces the narrow uninstall-cleanup seam. The
// concrete UserStore remains the handler-facing store; this seam lets tests
// exercise fail-closed uninstall behavior without weakening that accessor.
func (s *Service) setUserStateCleanupStore(cleanup userStateCleanupStore) {
	s.userStateCleanup = cleanup
}

// UserState returns the plugin_user_state store Provide constructed, for the
// authenticated per-user storage HTTP routes (user_state_handlers.go).
// Unlike StateStore (plugin_state, written only by a plugin's own
// gRPC-connected backend via the Host RPC), this store is reachable directly
// from an authenticated browser request — every read/write is scoped to the
// calling user (Approach D1, docs/decisions/2026-08-01-per-user-plugin-storage.md).
func (s *Service) UserState() *state.UserStore {
	return s.userState
}

// SetSecrets wires the secret vault Provide was constructed with.
func (s *Service) SetSecrets(v SecretVault) {
	s.secrets = v
}

// SetDataSources wires the Host data API's (ADR 0043) service-layer
// dependencies, following the same post-construction "SetX" pattern as
// SetDeliverer/SetSecrets (see the "Extension points" doc comment on
// Service). backendapp calls this once, passing its already-constructed
// task, workflow, agent-settings, and analytics services directly — each
// argument's interface is a narrow slice of one of those services
// (host_data.go's taskDataSource/workflowLister/workflowStepLister/
// agentProfileDataSource/sessionCodeStatsSource), satisfied structurally, so
// no adapter type is needed. Not called by Provide itself: the plugins
// package cannot import internal/task/service, internal/workflow/service,
// etc. without an import cycle, mirroring why event delivery is wired the
// same way. Every pluginHost hostForPlugin builds after this call gets
// these dependencies; one built before (e.g. very early boot) falls back to
// Unimplemented for the Host data API regardless of declared capabilities.
func (s *Service) SetDataSources(
	tasks taskDataSource,
	workflows workflowLister,
	workflowSteps workflowStepLister,
	agentProfiles agentProfileDataSource,
	sessionCodeStats sessionCodeStatsSource,
	messages messageDataSource,
	taskWrites taskWriter,
) {
	s.taskData = tasks
	s.workflows = workflows
	s.workflowSteps = workflowSteps
	s.agentProfiles = agentProfiles
	s.sessionCodeStats = sessionCodeStats
	s.messageData = messages
	s.taskWriter = taskWrites
}

// SetWriteDeps wires the Host data API's late write dependencies (ADR 0043
// phase 2): the task-message delivery path behind the SendMessage RPC
// (api_write:messages) and the orchestrator task-starter behind CreateTask's
// start_agent flag. Wired LATE (not in SetDataSources) because the orchestrator
// is constructed after StartActivePlugins has spawned boot-active plugins, so
// hosts read these live via writeDependencies rather than snapshotting them —
// the write here is mutex-guarded against those concurrent reads. Either
// argument may be nil (feature-gated off), in which case the corresponding
// write path returns Unimplemented / is a best-effort no-op.
func (s *Service) SetWriteDeps(messenger taskMessenger, starter taskStarter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messenger = messenger
	s.taskStarter = starter
}

// writeDependencies returns the currently-wired task messenger and task
// starter. Read live (not snapshotted at hostForPlugin time) so a plugin
// spawned before SetWriteDeps still resolves them once it is called. Guarded by
// s.mu against the SetWriteDeps write.
func (s *Service) writeDependencies() (taskMessenger, taskStarter) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.messenger, s.taskStarter
}

// SetUtilityAgent wires the dependencies behind Host.InvokeUtilityAgent
// (ADR 0048): the service that resolves the utility agent selected in plugin
// configuration, and the sessionless runner that executes a one-shot
// completion. Wired by backendapp (not Provide) for the same import-cycle
// reason as SetDataSources. Unlike the data sources, this is wired LATE in boot
// (hostUtilityMgr is only available after agentctl control is healthy, by which
// point StartActivePlugins has already spawned boot-active plugins), so hosts
// read these live via utilityAgentDeps rather than snapshotting them — the
// write here is mutex-guarded against those concurrent reads.
func (s *Service) SetUtilityAgent(agents utilityAgentSource, runner utilityRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.utilityAgents = agents
	s.utilityRunner = runner
}

// utilityAgentDeps returns the currently-wired utility-agent dependencies. Read
// live (not snapshotted at hostForPlugin time) so a plugin spawned before
// SetUtilityAgent still resolves them once it is called. Guarded by s.mu against
// the SetUtilityAgent write.
func (s *Service) utilityAgentDeps() (utilityAgentSource, utilityRunner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.utilityAgents, s.utilityRunner
}

// SetAuthLoginBridge wires the SSO login bridge auth-capable plugins use to
// establish a browser session from a validated external identity (see
// auth_login.go). Called once during startup wiring, only when the auth
// service exists.
func (s *Service) SetAuthLoginBridge(b AuthLoginBridge) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.authLogin = b
}

// authLoginBridge returns the wired SSO login bridge, or nil when auth is not
// available. Read under s.mu against the SetAuthLoginBridge write.
func (s *Service) authLoginBridge() AuthLoginBridge {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.authLogin
}

// sessionCookieName returns the name of Kandev's own session cookie via the
// wired auth bridge, or "" when no bridge is wired (auth disabled entirely,
// so no session cookie is ever minted). Used by the webhook relay to strip
// that cookie before forwarding headers to a plugin subprocess.
func (s *Service) sessionCookieName() string {
	bridge := s.authLoginBridge()
	if bridge == nil {
		return ""
	}
	return bridge.SessionCookieName()
}

// SetKandevVersion wires the currently running kandev build version,
// enabling Install to enforce a package's manifest.min_kandev_version
// (checkMinKandevVersion): a package requiring a newer kandev is rejected
// rather than installed and left to fail confusingly at spawn time.
// Called once during startup wiring from internal/backendapp, which owns the
// ldflags-injected Version. Passing DevKandevVersion (an un-stamped local
// build) leaves the check a no-op — see checkMinKandevVersion.
func (s *Service) SetKandevVersion(v string) {
	s.kandevVersion = v
}

// KandevVersion returns the running build version wired via
// SetKandevVersion, or "" when startup wiring never supplied one (in which
// case min_kandev_version is not enforced).
func (s *Service) KandevVersion() string {
	return s.kandevVersion
}

// SetRuntime wires the runtime.Manager Provide constructed.
func (s *Service) SetRuntime(rt PluginRuntime) {
	s.runtime = rt
}

// Runtime returns the runtime manager Service spawns/supervises plugin
// processes through, for boot-time wiring (spawning every active plugin)
// and the HTTP layer (webhook/tool invocation).
func (s *Service) Runtime() PluginRuntime {
	return s.runtime
}

// Shutdown stops every currently-running plugin process. Callers (e.g.
// backendapp's startPluginsSubsystems) register this with addCleanup for
// graceful backend shutdown.
func (s *Service) Shutdown() {
	if s.runtime != nil {
		s.runtime.StopAll()
	}
}

// SetPluginsDir wires the root directory pkgtar.Install/pkgtar.Remove
// operate under (the same directory store.FSStore persists records in).
func (s *Service) SetPluginsDir(dir string) {
	s.pluginsDir = dir
}

// RevealSecret resolves the cleartext value of the secret reference ref via
// the shared secret vault. Returns an error if no vault was wired (e.g. a
// test Service constructed via NewService directly) or if ref does not
// resolve.
func (s *Service) RevealSecret(ctx context.Context, ref string) (string, error) {
	if s.secrets == nil {
		return "", errors.New("plugins: secret vault not configured")
	}
	return s.secrets.Reveal(ctx, ref)
}

// ActiveUIPlugins returns every StatusActive plugin record that declares a
// native UI bundle (ui.bundle), used to populate the boot payload's Plugins
// list.
func (s *Service) ActiveUIPlugins() []store.Record {
	var out []store.Record
	for _, rec := range s.List() {
		if rec.Status == StatusActive && rec.UI.Bundle != "" {
			out = append(out, *rec)
		}
	}
	return out
}

// Registry returns the underlying in-memory Registry.
func (s *Service) Registry() *Registry {
	return s.registry
}

// EventBus returns the event bus Service was constructed with (may be nil
// in tests).
func (s *Service) EventBus() bus.EventBus {
	return s.eventBus
}

// hostForPlugin builds the Host implementation bound to pluginID, gated by
// that plugin's currently-registered capabilities. Passed to
// PluginRuntime.Start as the hostFactory; the runtime manager calls it
// again on every restart, so a config/capability change takes effect on
// the plugin's next spawn.
func (s *Service) hostForPlugin(pluginID string) pluginsdk.Host {
	rec, err := s.Get(pluginID)
	if err != nil {
		rec = &store.Record{} // every capability check below denies; should not happen in practice
	}
	return &pluginHost{
		pluginID:            pluginID,
		capabilities:        rec.Capabilities,
		repositoryProviders: rec.RepositoryProviders,
		configSchema:        rec.ConfigSchema,
		state:               s.state,
		secrets:             s.secrets,
		bus:                 s.eventBus,
		configs:             s.store,
		taskData:            s.taskData,
		workflows:           s.workflows,
		workflowSteps:       s.workflowSteps,
		agentProfiles:       s.agentProfiles,
		sessionCodeStats:    s.sessionCodeStats,
		messageData:         s.messageData,
		taskWriter:          s.taskWriter,
		utilityDeps:         s.utilityAgentDeps,
		writeDeps:           s.writeDependencies,
	}
}

// notifyDeliverer calls Refresh on the attached Deliverer, if any. Must be
// called without s.mu held (Deliverer implementations may call back into
// Service).
func (s *Service) notifyDeliverer() {
	s.mu.Lock()
	d := s.deliverer
	s.mu.Unlock()
	if d != nil {
		d.Refresh()
	}
}

// List returns every installed plugin, sorted by id.
func (s *Service) List() []*store.Record {
	return s.registry.List()
}

// Get returns the record for id, or store.ErrNotFound.
func (s *Service) Get(id string) (*store.Record, error) {
	rec, ok := s.registry.Get(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	return rec, nil
}

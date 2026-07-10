package controller

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

const (
	// Orphan cleanup is startup hygiene, not a user-confirmed bulk action.
	// Keep automatic batches small so registry bugs fail closed.
	maxOrphanCleanupAgentTypesPerRun = 20
	maxOrphanCleanupProfilesPerRun   = 50
)

// CapabilityReader is the minimum surface of the host utility manager that
// the reconciler needs. Declared as an interface so tests can inject a fake.
type CapabilityReader interface {
	Get(agentType string) (hostutility.AgentCapabilities, bool)
}

// ProfileReconciler reconciles persisted agent profiles against the host
// utility capability cache. On boot it seeds default profiles for newly
// probed agents, validates existing profile models/modes against the cache
// (auto-healing stale values), and soft-deletes profiles whose agent_id is
// no longer registered (e.g. after removing the non-ACP agent variants).
//
// The reconciler is idempotent and safe to run on every boot. It runs in a
// goroutine after hostUtility.Start so probe results are available, and
// never blocks task startup — profiles used before reconciliation simply get
// validated on the next boot.
type ProfileReconciler struct {
	hostUtility CapabilityReader
	registry    *registry.Registry
	store       store.Repository
	log         *logger.Logger
}

type orphanCleanupCandidate struct {
	agent   *models.Agent
	profile *models.AgentProfile
}

type orphanCleanupSummary struct {
	enabledAgentCount        int
	dbAgentCount             int
	orphanAgentCount         int
	profilesCandidateCount   int
	profilesCandidatePartial bool
	profilesDeletedCount     int
	profileListFailureCount  int
	skipped                  bool
	skipReason               string
	enabledAgentIDs          []string
	orphanAgentNames         []string
	maxAgentTypesPerRun      int
	maxProfilesPerRun        int
}

// NewProfileReconciler constructs a reconciler.
func NewProfileReconciler(
	h CapabilityReader,
	reg *registry.Registry,
	st store.Repository,
	log *logger.Logger,
) *ProfileReconciler {
	return &ProfileReconciler{
		hostUtility: h,
		registry:    reg,
		store:       st,
		log:         log.WithFields(zap.String("component", "profile-reconciler")),
	}
}

// Run executes one reconciliation pass: orphan cleanup, default seeding, and
// stale model/mode healing. Errors are logged and the pass continues — the
// goal is best-effort convergence, not atomicity.
func (r *ProfileReconciler) Run(ctx context.Context) error {
	if r == nil || r.hostUtility == nil || r.registry == nil || r.store == nil {
		return fmt.Errorf("reconciler not fully configured")
	}

	// Orphan cleanup first: removed agents can't come back, regardless of
	// probe state. The cleanup itself fails closed when the registry is empty
	// so a transient registry/bootstrap issue cannot mass-delete profiles.
	r.cleanupOrphans(ctx)

	// Walk enabled inference agents and reconcile each one.
	for _, ia := range r.registry.ListInferenceAgents() {
		ag, ok := ia.(agents.Agent)
		if !ok {
			continue
		}
		r.reconcileAgent(ctx, ag)
	}
	return nil
}

// cleanupOrphans soft-deletes profiles whose DB agent row references an
// agent type that is no longer registered or enabled (e.g. profiles left
// behind by the removed streamjson-based variants). The settings store keys
// each DB agent by a UUID in `id`, with the registry-facing identifier stored
// in `name` — we match against the registry on `name`.
func (r *ProfileReconciler) cleanupOrphans(ctx context.Context) {
	summary := orphanCleanupSummary{
		maxAgentTypesPerRun: maxOrphanCleanupAgentTypesPerRun,
		maxProfilesPerRun:   maxOrphanCleanupProfilesPerRun,
	}
	defer func() {
		r.logOrphanCleanupSummary(summary)
	}()

	if !r.registry.IsLoaded() {
		summary.skipped = true
		summary.skipReason = "registry_not_loaded"
		r.log.Warn("orphan cleanup skipped: registry is not loaded")
		return
	}
	enabledAgents := r.registry.ListEnabled()
	summary.enabledAgentCount = len(enabledAgents)
	if len(enabledAgents) == 0 {
		summary.skipped = true
		summary.skipReason = "enabled_registry_empty"
		r.log.Warn("orphan cleanup skipped: enabled agent registry is empty")
		return
	}
	enabled := make(map[string]struct{}, len(enabledAgents))
	for _, ag := range enabledAgents {
		id := ag.ID()
		enabled[id] = struct{}{}
		summary.enabledAgentIDs = append(summary.enabledAgentIDs, id)
	}

	dbAgents, err := r.store.ListAgents(ctx)
	if err != nil {
		summary.skipped = true
		summary.skipReason = "list_agents_failed"
		r.log.Warn("orphan cleanup: list agents failed", zap.Error(err))
		return
	}
	summary.dbAgentCount = len(dbAgents)
	candidates := r.collectOrphanCleanupCandidates(ctx, dbAgents, enabled, &summary)
	if summary.profileListFailureCount > 0 {
		summary.skipped = true
		summary.skipReason = "profile_list_failed"
		r.log.Warn("orphan cleanup skipped: profile list failed for one or more orphan agents",
			zap.Int("profile_list_failure_count", summary.profileListFailureCount))
		return
	}
	if exceedsOrphanCleanupLimit(summary.orphanAgentCount, len(candidates)) {
		summary.skipped = true
		summary.skipReason = "safety_limit_exceeded"
		r.log.Warn("orphan cleanup skipped: candidate batch exceeds safety limit",
			zap.Int("orphan_agent_count", summary.orphanAgentCount),
			zap.Int("profiles_candidate_count", len(candidates)),
			zap.Int("max_agent_types_per_run", maxOrphanCleanupAgentTypesPerRun),
			zap.Int("max_profiles_per_run", maxOrphanCleanupProfilesPerRun),
			zap.Strings("orphan_agents", summary.orphanAgentNames))
		return
	}
	r.deleteOrphanCleanupCandidates(ctx, candidates, &summary)
}

func (r *ProfileReconciler) collectOrphanCleanupCandidates(
	ctx context.Context,
	dbAgents []*models.Agent,
	enabled map[string]struct{},
	summary *orphanCleanupSummary,
) []orphanCleanupCandidate {
	var candidates []orphanCleanupCandidate
	for _, dbAgent := range dbAgents {
		if _, ok := enabled[dbAgent.Name]; ok {
			continue
		}
		summary.orphanAgentCount++
		summary.orphanAgentNames = append(summary.orphanAgentNames, dbAgent.Name)
		profiles, err := r.store.ListAgentProfiles(ctx, dbAgent.ID)
		if err != nil {
			summary.profileListFailureCount++
			summary.profilesCandidatePartial = true
			r.log.Warn("orphan cleanup: list profiles failed",
				zap.String("agent_id", dbAgent.ID),
				zap.String("agent_name", dbAgent.Name),
				zap.Error(err))
			continue
		}
		for _, p := range profiles {
			candidates = append(candidates, orphanCleanupCandidate{
				agent:   dbAgent,
				profile: p,
			})
		}
	}
	if summary.profileListFailureCount == 0 {
		summary.profilesCandidateCount = len(candidates)
	}
	return candidates
}

func exceedsOrphanCleanupLimit(orphanAgentCount, profileCount int) bool {
	return orphanAgentCount > maxOrphanCleanupAgentTypesPerRun ||
		profileCount > maxOrphanCleanupProfilesPerRun
}

func (r *ProfileReconciler) deleteOrphanCleanupCandidates(
	ctx context.Context,
	candidates []orphanCleanupCandidate,
	summary *orphanCleanupSummary,
) {
	for _, candidate := range candidates {
		r.log.Info("soft-deleting orphan profile",
			zap.String("profile_id", candidate.profile.ID),
			zap.String("agent_id", candidate.profile.AgentID),
			zap.String("agent_name", candidate.agent.Name))
		if err := r.store.DeleteAgentProfile(ctx, candidate.profile.ID); err != nil {
			r.log.Warn("orphan cleanup: delete failed",
				zap.String("profile_id", candidate.profile.ID), zap.Error(err))
			continue
		}
		summary.profilesDeletedCount++
	}
}

func (r *ProfileReconciler) logOrphanCleanupSummary(summary orphanCleanupSummary) {
	fields := []zap.Field{
		zap.Int("enabled_agent_count", summary.enabledAgentCount),
		zap.Int("db_agent_count", summary.dbAgentCount),
		zap.Int("orphan_agent_count", summary.orphanAgentCount),
		zap.Int("profiles_candidate_count", summary.profilesCandidateCount),
		zap.Bool("profiles_candidate_partial", summary.profilesCandidatePartial),
		zap.Int("profiles_deleted_count", summary.profilesDeletedCount),
		zap.Int("profile_list_failure_count", summary.profileListFailureCount),
		zap.Bool("skipped", summary.skipped),
		zap.String("skip_reason", summary.skipReason),
		zap.Int("max_agent_types_per_run", summary.maxAgentTypesPerRun),
		zap.Int("max_profiles_per_run", summary.maxProfilesPerRun),
		zap.Strings("enabled_agents", summary.enabledAgentIDs),
		zap.Strings("orphan_agents", summary.orphanAgentNames),
	}
	r.log.Info("orphan cleanup summary", fields...)
}

// reconcileAgent validates or seeds profiles for a single inference agent.
// When the agent's probe is not "ok", existing profiles are left untouched —
// the UI surfaces the probe error and the user fixes it before we retry.
func (r *ProfileReconciler) reconcileAgent(ctx context.Context, ag agents.Agent) {
	agentType := ag.ID()
	caps, ok := r.hostUtility.Get(agentType)
	if !ok || caps.Status != hostutility.StatusOK {
		r.log.Debug("skipping reconciliation: probe not ok",
			zap.String("agent_id", agentType),
			zap.String("status", string(caps.Status)))
		return
	}

	dbAgent, err := r.ensureDBAgent(ctx, ag)
	if err != nil {
		r.log.Warn("reconcile: ensure db agent failed",
			zap.String("agent_id", agentType), zap.Error(err))
		return
	}

	profiles, err := r.store.ListAgentProfiles(ctx, dbAgent.ID)
	if err != nil {
		r.log.Warn("reconcile: list profiles failed",
			zap.String("agent_id", agentType), zap.Error(err))
		return
	}

	if len(profiles) == 0 {
		// Only seed for an agent that has never been provisioned. Soft-deleted
		// rows mean the profile(s) were deliberately removed, so re-seeding
		// would resurrect them on every boot (the bug this guards).
		//
		// A soft-deleted row here implies a *user* deletion, not system orphan
		// cleanup: cleanupOrphans is the only system path that soft-deletes
		// profiles, and it acts solely on agents absent from ListEnabled(),
		// whereas reconcileAgent runs only for ListInferenceAgents(). Both gate
		// on Enabled(), so the sets are disjoint — an enabled, reconciled agent
		// is never orphan-cleaned. (If Enabled() ever becomes dynamic, revisit:
		// a re-enabled agent reuses its DB id via ensureDBAgent and would then
		// see its orphan-cleaned rows here.)
		hadProfiles, err := r.store.HasDeletedAgentProfiles(ctx, dbAgent.ID)
		if err != nil {
			r.log.Warn("reconcile: check deleted profiles failed",
				zap.String("agent_id", agentType), zap.Error(err))
			return
		}
		if hadProfiles {
			r.log.Debug("skipping seed: agent has user-deleted profiles",
				zap.String("agent_id", agentType))
			return
		}
		r.seedDefaultProfile(ctx, ag, dbAgent, caps)
		return
	}

	for _, p := range profiles {
		r.healProfile(ctx, p, caps)
	}
}

// ensureDBAgent looks up the agent row in the store or creates one if missing.
// The store's agent row uses an auto-generated UUID for `ID`; `Name` holds
// the registry-facing identifier like "claude-acp".
//
// Callers must distinguish "not found" from transient DB errors to avoid
// duplicate rows on lock/timeout.
func (r *ProfileReconciler) ensureDBAgent(ctx context.Context, ag agents.Agent) (*models.Agent, error) {
	dbAgent, err := r.store.GetAgentByName(ctx, ag.ID())
	if err == nil && dbAgent != nil {
		return dbAgent, nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Transient DB error — don't attempt to create, caller will retry.
		return nil, fmt.Errorf("get agent %q: %w", ag.ID(), err)
	}
	// Not found: create a new row. The store assigns ID on CreateAgent.
	newAgent := &models.Agent{
		Name:        ag.ID(),
		SupportsMCP: true,
	}
	if createErr := r.store.CreateAgent(ctx, newAgent); createErr != nil {
		return nil, createErr
	}
	return newAgent, nil
}

// seedDefaultProfile creates a single default profile for an agent that has no
// existing profiles, using the probed CurrentModelID and CurrentModeID.
func (r *ProfileReconciler) seedDefaultProfile(
	ctx context.Context,
	ag agents.Agent,
	dbAgent *models.Agent,
	caps hostutility.AgentCapabilities,
) {
	profile := &models.AgentProfile{
		AgentID:          dbAgent.ID,
		Name:             profileNameFromCaps(ag, caps),
		AgentDisplayName: ag.DisplayName(),
		Model:            caps.CurrentModelID,
		Mode:             caps.CurrentModeID,
		AllowIndexing:    ag.ID() == "auggie",
		CLIPassthrough:   false,
		UserModified:     false,
	}
	if err := r.store.CreateAgentProfile(ctx, profile); err != nil {
		r.log.Warn("seed default profile failed",
			zap.String("agent_id", dbAgent.ID), zap.Error(err))
		return
	}
	r.log.Info("seeded default profile from probe",
		zap.String("profile_id", profile.ID),
		zap.String("agent_id", dbAgent.ID),
		zap.String("model", profile.Model),
		zap.String("mode", profile.Mode))
}

// healProfile validates the profile's model and mode against the cache and
// auto-heals values that no longer exist. User-modified profiles are still
// healed — we always keep profiles in a usable state; the "user_modified"
// flag survives the write to retain user intent for other fields.
func (r *ProfileReconciler) healProfile(
	ctx context.Context,
	p *models.AgentProfile,
	caps hostutility.AgentCapabilities,
) {
	changed := healProfileName(p, caps)

	if p.Model != "" && !modelExists(p.Model, caps.Models) {
		r.log.Info("profile model no longer available, auto-healing",
			zap.String("profile_id", p.ID),
			zap.String("old_model", p.Model),
			zap.String("new_model", caps.CurrentModelID))
		p.Model = caps.CurrentModelID
		changed = true
	}
	if p.Model == "" && caps.CurrentModelID != "" {
		p.Model = caps.CurrentModelID
		changed = true
	}

	if p.Mode != "" && !modeExists(p.Mode, caps.Modes) {
		r.log.Info("profile mode no longer available, clearing",
			zap.String("profile_id", p.ID),
			zap.String("old_mode", p.Mode))
		p.Mode = ""
		changed = true
	}
	if p.Mode == "" && caps.CurrentModeID != "" {
		p.Mode = caps.CurrentModeID
		changed = true
	}

	if !changed {
		return
	}
	if err := r.store.UpdateAgentProfile(ctx, p); err != nil {
		r.log.Warn("profile heal update failed",
			zap.String("profile_id", p.ID), zap.Error(err))
	}
}

// healProfileName updates the profile name when it still matches the agent
// display name (stale seed from before we started naming profiles after the
// current model). User-modified profiles are skipped.
func healProfileName(p *models.AgentProfile, caps hostutility.AgentCapabilities) bool {
	if p.UserModified || p.Name != p.AgentDisplayName || caps.CurrentModelID == "" {
		return false
	}
	for _, m := range caps.Models {
		if m.ID == caps.CurrentModelID && m.Name != "" && m.Name != p.Name {
			p.Name = m.Name
			return true
		}
	}
	return false
}

// profileNameFromCaps picks a user-facing default profile name from the
// probed model list: prefer the Name of the agent's currentModelId, fall
// back to the model id, then the agent display name. Keeps the profile
// label meaningful ("Claude Sonnet 4.6") instead of the agent name echoed.
func profileNameFromCaps(ag agents.Agent, caps hostutility.AgentCapabilities) string {
	if caps.CurrentModelID != "" {
		for _, m := range caps.Models {
			if m.ID == caps.CurrentModelID && m.Name != "" {
				return m.Name
			}
		}
		return caps.CurrentModelID
	}
	return ag.DisplayName()
}

func modelExists(id string, models []hostutility.Model) bool {
	for _, m := range models {
		if m.ID == id {
			return true
		}
	}
	return false
}

func modeExists(id string, modes []hostutility.Mode) bool {
	for _, m := range modes {
		if m.ID == id {
			return true
		}
	}
	return false
}

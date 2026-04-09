package controller

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/hostutility"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
)

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
	hostUtility *hostutility.Manager
	registry    *registry.Registry
	store       store.Repository
	log         *logger.Logger
}

// NewProfileReconciler constructs a reconciler.
func NewProfileReconciler(
	h *hostutility.Manager,
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
	// probe state, so we can clean these unconditionally.
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
	enabled := make(map[string]struct{})
	for _, ag := range r.registry.ListEnabled() {
		enabled[ag.ID()] = struct{}{}
	}

	dbAgents, err := r.store.ListAgents(ctx)
	if err != nil {
		r.log.Warn("orphan cleanup: list agents failed", zap.Error(err))
		return
	}
	for _, dbAgent := range dbAgents {
		if _, ok := enabled[dbAgent.Name]; ok {
			continue
		}
		profiles, err := r.store.ListAgentProfiles(ctx, dbAgent.ID)
		if err != nil {
			r.log.Warn("orphan cleanup: list profiles failed",
				zap.String("agent_id", dbAgent.ID),
				zap.String("agent_name", dbAgent.Name),
				zap.Error(err))
			continue
		}
		for _, p := range profiles {
			r.log.Info("soft-deleting orphan profile",
				zap.String("profile_id", p.ID),
				zap.String("agent_id", p.AgentID),
				zap.String("agent_name", dbAgent.Name))
			if err := r.store.DeleteAgentProfile(ctx, p.ID); err != nil {
				r.log.Warn("orphan cleanup: delete failed",
					zap.String("profile_id", p.ID), zap.Error(err))
			}
		}
	}
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
		r.seedDefaultProfile(ctx, ag, dbAgent, caps)
		return
	}

	for _, p := range profiles {
		r.healProfile(ctx, p, caps)
	}
}

// ensureDBAgent looks up the agent row in the store or creates one if missing.
// The store's agent row is the parent of profiles; it is keyed by the
// registry ID.
func (r *ProfileReconciler) ensureDBAgent(ctx context.Context, ag agents.Agent) (*models.Agent, error) {
	dbAgent, err := r.store.GetAgentByName(ctx, ag.ID())
	if err == nil && dbAgent != nil {
		return dbAgent, nil
	}
	newAgent := &models.Agent{
		ID:          ag.ID(),
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
		Name:             ag.DisplayName(),
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
	changed := false

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

	if !changed {
		return
	}
	if err := r.store.UpdateAgentProfile(ctx, p); err != nil {
		r.log.Warn("profile heal update failed",
			zap.String("profile_id", p.ID), zap.Error(err))
	}
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

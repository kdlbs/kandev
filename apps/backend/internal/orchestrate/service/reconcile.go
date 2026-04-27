package service

import (
	"context"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestrate/models"

	"go.uber.org/zap"
)

// Reconciler synchronises DB operational state with the filesystem config.
// It is safe to call ReconcileAll multiple times; each sub-step is best-effort
// and logs warnings instead of failing the caller.
type Reconciler struct {
	svc *Service
}

// NewReconciler creates a reconciler backed by the given orchestrate service.
func NewReconciler(svc *Service) *Reconciler {
	return &Reconciler{svc: svc}
}

// ReconcileAll runs every reconciliation step. Errors are logged, not returned,
// so that startup is never blocked by a single reconciliation failure.
func (r *Reconciler) ReconcileAll(ctx context.Context) {
	if err := r.reconcileAgentRuntime(ctx); err != nil {
		r.svc.logger.Warn("reconcile agent runtime", zap.Error(err))
	}
	if err := r.reconcileRoutineTriggers(ctx); err != nil {
		r.svc.logger.Warn("reconcile routine triggers", zap.Error(err))
	}
	if err := r.reconcileBudgetPolicies(ctx); err != nil {
		r.svc.logger.Warn("reconcile budget policies", zap.Error(err))
	}
	if err := r.reconcileChannels(ctx); err != nil {
		r.svc.logger.Warn("reconcile channels", zap.Error(err))
	}
}

// reconcileAgentRuntime ensures every agent in the config has a runtime row
// and removes rows for agents no longer in config.
func (r *Reconciler) reconcileAgentRuntime(ctx context.Context) error {
	if r.svc.cfgLoader == nil {
		return nil
	}
	agents := r.svc.cfgLoader.GetAgents(defaultWorkspaceName)
	runtimes, err := r.svc.repo.ListAgentRuntimes(ctx)
	if err != nil {
		return err
	}

	// Build a set of config agent IDs for fast lookup.
	configIDs := make(map[string]struct{}, len(agents))
	for _, a := range agents {
		configIDs[a.ID] = struct{}{}
	}

	// Create runtime rows for new agents; merge status for existing ones.
	for _, a := range agents {
		rt, exists := runtimes[a.ID]
		if !exists {
			if upsertErr := r.svc.repo.UpsertAgentRuntime(
				ctx, a.ID, string(models.AgentStatusIdle), "",
			); upsertErr != nil {
				r.svc.logger.Warn("create runtime row",
					zap.String("agent", a.Name), zap.Error(upsertErr))
			}
			continue
		}
		// Merge persisted runtime status into in-memory cache.
		a.Status = models.AgentStatus(rt.Status)
		a.PauseReason = rt.PauseReason
		a.LastWakeupFinishedAt = rt.LastWakeupFinishedAt
	}

	// Delete rows for agents removed from config.
	for id := range runtimes {
		if _, ok := configIDs[id]; ok {
			continue
		}
		if delErr := r.svc.repo.DeleteAgentRuntime(ctx, id); delErr != nil {
			r.svc.logger.Warn("delete orphan runtime row",
				zap.String("agent_id", id), zap.Error(delErr))
		}
	}
	return nil
}

// reconcileRoutineTriggers creates triggers for new routines, updates changed
// triggers, and removes triggers/runs for deleted routines.
func (r *Reconciler) reconcileRoutineTriggers(ctx context.Context) error {
	if r.svc.cfgLoader == nil {
		return nil
	}
	fsRoutines := r.svc.cfgLoader.GetRoutines(defaultWorkspaceName)
	dbRoutineIDs, err := r.svc.repo.ListDistinctTriggerRoutineIDs(ctx)
	if err != nil {
		return err
	}

	fsIDSet := make(map[string]*models.Routine, len(fsRoutines))
	for _, rt := range fsRoutines {
		fsIDSet[rt.ID] = rt
	}
	dbIDSet := make(map[string]struct{}, len(dbRoutineIDs))
	for _, id := range dbRoutineIDs {
		dbIDSet[id] = struct{}{}
	}

	r.createTriggersForNewRoutines(ctx, fsRoutines, dbIDSet)
	r.updateChangedTriggers(ctx, fsRoutines, dbIDSet)
	r.deleteOrphanRoutineData(ctx, dbRoutineIDs, fsIDSet)
	return nil
}

func (r *Reconciler) createTriggersForNewRoutines(
	ctx context.Context, routines []*models.Routine, dbIDs map[string]struct{},
) {
	for _, rt := range routines {
		if _, exists := dbIDs[rt.ID]; exists {
			continue
		}
		// The routine YAML doesn't store trigger config as parsed fields on
		// the Routine model today. We create a placeholder cron trigger row
		// so the routine is tracked; the user can configure it via the UI.
		trigger := &models.RoutineTrigger{
			ID:        uuid.New().String(),
			RoutineID: rt.ID,
			Kind:      "manual",
			Enabled:   true,
		}
		if createErr := r.svc.repo.CreateRoutineTrigger(ctx, trigger); createErr != nil {
			r.svc.logger.Warn("create trigger for new routine",
				zap.String("routine", rt.Name), zap.Error(createErr))
		}
	}
}

func (r *Reconciler) updateChangedTriggers(
	ctx context.Context, routines []*models.Routine, dbIDs map[string]struct{},
) {
	// For routines that exist both in config and DB, compare trigger config.
	// Currently routineYAML triggers are not round-tripped into models.Routine
	// (the Triggers field is YAML-only), so this is a no-op placeholder that
	// will be populated when trigger config is added to the model.
	_ = ctx
	_ = routines
	_ = dbIDs
}

func (r *Reconciler) deleteOrphanRoutineData(
	ctx context.Context, dbIDs []string, fsIDs map[string]*models.Routine,
) {
	for _, id := range dbIDs {
		if _, ok := fsIDs[id]; ok {
			continue
		}
		if err := r.svc.repo.DeleteTriggersByRoutineID(ctx, id); err != nil {
			r.svc.logger.Warn("delete triggers for removed routine",
				zap.String("routine_id", id), zap.Error(err))
		}
		if err := r.svc.repo.DeleteRunsByRoutineID(ctx, id); err != nil {
			r.svc.logger.Warn("delete runs for removed routine",
				zap.String("routine_id", id), zap.Error(err))
		}
	}
}

// reconcileBudgetPolicies removes budget policies that reference agents or
// projects no longer present in the filesystem config.
func (r *Reconciler) reconcileBudgetPolicies(ctx context.Context) error {
	if r.svc.cfgLoader == nil {
		return nil
	}
	agentIDs := collectAgentIDs(r.svc.cfgLoader.GetAgents(defaultWorkspaceName))
	projectIDs := collectProjectIDs(r.svc.cfgLoader.GetProjects(defaultWorkspaceName))

	if _, err := r.svc.repo.DeleteBudgetPoliciesForRemovedScopes(
		ctx, "agent", agentIDs,
	); err != nil {
		return err
	}
	if _, err := r.svc.repo.DeleteBudgetPoliciesForRemovedScopes(
		ctx, "project", projectIDs,
	); err != nil {
		return err
	}
	return nil
}

// reconcileChannels removes channels that reference agent instances no longer
// present in the filesystem config.
func (r *Reconciler) reconcileChannels(ctx context.Context) error {
	if r.svc.cfgLoader == nil {
		return nil
	}
	agentIDs := collectAgentIDs(r.svc.cfgLoader.GetAgents(defaultWorkspaceName))
	_, err := r.svc.repo.DeleteChannelsForRemovedAgents(ctx, agentIDs)
	return err
}

func collectAgentIDs(agents []*models.AgentInstance) []string {
	ids := make([]string, len(agents))
	for i, a := range agents {
		ids[i] = a.ID
	}
	return ids
}

func collectProjectIDs(projects []*models.Project) []string {
	ids := make([]string, len(projects))
	for i, p := range projects {
		ids[i] = p.ID
	}
	return ids
}

package infra

import (
	"context"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"

	"go.uber.org/zap"
)

// Reconciler synchronises derived DB state with the canonical config tables.
// It creates default routine triggers, drops budget policies and channels for
// removed agents/projects, and reconciles legacy agent runtime rows. It is
// safe to call ReconcileAll multiple times; each sub-step is best-effort and
// logs warnings instead of failing the caller.
type Reconciler struct {
	repo   *sqlite.Repository
	logger *logger.Logger
}

// NewReconciler creates a Reconciler backed by the given repository.
func NewReconciler(repo *sqlite.Repository, log *logger.Logger) *Reconciler {
	return &Reconciler{
		repo:   repo,
		logger: log.WithFields(zap.String("component", "office-reconciler")),
	}
}

// ReconcileAll runs every reconciliation step. Errors are logged, not returned,
// so that startup is never blocked by a single reconciliation failure.
func (r *Reconciler) ReconcileAll(ctx context.Context) {
	if err := r.reconcileAgentRuntime(ctx); err != nil {
		r.logger.Warn("reconcile agent runtime", zap.Error(err))
	}
	if err := r.reconcileRoutineTriggers(ctx); err != nil {
		r.logger.Warn("reconcile routine triggers", zap.Error(err))
	}
	if err := r.reconcileBudgetPolicies(ctx); err != nil {
		r.logger.Warn("reconcile budget policies", zap.Error(err))
	}
	if err := r.reconcileChannels(ctx); err != nil {
		r.logger.Warn("reconcile channels", zap.Error(err))
	}
}

// reconcileAgentRuntime drops legacy runtime rows for agents that no longer
// exist in the canonical agent table.
func (r *Reconciler) reconcileAgentRuntime(ctx context.Context) error {
	agents, err := r.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return err
	}
	runtimes, err := r.repo.ListAgentRuntimes(ctx)
	if err != nil {
		return err
	}
	agentIDs := make(map[string]struct{}, len(agents))
	for _, a := range agents {
		agentIDs[a.ID] = struct{}{}
	}
	for id := range runtimes {
		if _, ok := agentIDs[id]; ok {
			continue
		}
		if delErr := r.repo.DeleteAgentRuntime(ctx, id); delErr != nil {
			r.logger.Warn("delete orphan runtime row",
				zap.String("agent_id", id), zap.Error(delErr))
		}
	}
	return nil
}

// reconcileRoutineTriggers creates triggers for new routines, updates changed
// triggers, and removes triggers/runs for deleted routines.
func (r *Reconciler) reconcileRoutineTriggers(ctx context.Context) error {
	routines, err := r.repo.ListRoutines(ctx, "")
	if err != nil {
		return err
	}
	dbRoutineIDs, err := r.repo.ListDistinctTriggerRoutineIDs(ctx)
	if err != nil {
		return err
	}

	idSet := make(map[string]*models.Routine, len(routines))
	for _, rt := range routines {
		idSet[rt.ID] = rt
	}
	triggerIDSet := make(map[string]struct{}, len(dbRoutineIDs))
	for _, id := range dbRoutineIDs {
		triggerIDSet[id] = struct{}{}
	}

	r.createTriggersForNewRoutines(ctx, routines, triggerIDSet)
	r.deleteOrphanRoutineData(ctx, dbRoutineIDs, idSet)
	return nil
}

func (r *Reconciler) createTriggersForNewRoutines(
	ctx context.Context, routines []*models.Routine, dbIDs map[string]struct{},
) {
	for _, rt := range routines {
		if _, exists := dbIDs[rt.ID]; exists {
			continue
		}
		trigger := &models.RoutineTrigger{
			ID:        uuid.New().String(),
			RoutineID: rt.ID,
			Kind:      "manual",
			Enabled:   true,
		}
		if createErr := r.repo.CreateRoutineTrigger(ctx, trigger); createErr != nil {
			r.logger.Warn("create trigger for new routine",
				zap.String("routine", rt.Name), zap.Error(createErr))
		}
	}
}

func (r *Reconciler) deleteOrphanRoutineData(
	ctx context.Context, dbIDs []string, fsIDs map[string]*models.Routine,
) {
	for _, id := range dbIDs {
		if _, ok := fsIDs[id]; ok {
			continue
		}
		if err := r.repo.DeleteTriggersByRoutineID(ctx, id); err != nil {
			r.logger.Warn("delete triggers for removed routine",
				zap.String("routine_id", id), zap.Error(err))
		}
		if err := r.repo.DeleteRunsByRoutineID(ctx, id); err != nil {
			r.logger.Warn("delete runs for removed routine",
				zap.String("routine_id", id), zap.Error(err))
		}
	}
}

// reconcileBudgetPolicies removes budget policies that reference agents or
// projects no longer present in the canonical config tables.
func (r *Reconciler) reconcileBudgetPolicies(ctx context.Context) error {
	agents, err := r.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return err
	}
	projects, err := r.repo.ListProjects(ctx, "")
	if err != nil {
		return err
	}
	if _, err := r.repo.DeleteBudgetPoliciesForRemovedScopes(
		ctx, "agent", collectAgentIDs(agents),
	); err != nil {
		return err
	}
	if _, err := r.repo.DeleteBudgetPoliciesForRemovedScopes(
		ctx, "project", collectProjectIDs(projects),
	); err != nil {
		return err
	}
	return nil
}

// reconcileChannels removes channels that reference agent instances no longer
// present in the canonical agent table.
func (r *Reconciler) reconcileChannels(ctx context.Context) error {
	agents, err := r.repo.ListAgentInstances(ctx, "")
	if err != nil {
		return err
	}
	_, err = r.repo.DeleteChannelsForRemovedAgents(ctx, collectAgentIDs(agents))
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

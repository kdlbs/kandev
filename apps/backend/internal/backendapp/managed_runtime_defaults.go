package backendapp

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
)

func managedRuntimeDefaultGenerations(agentRegistry *registry.Registry) []managedruntime.DefaultGeneration {
	if agentRegistry == nil {
		return nil
	}

	generations := make([]managedruntime.DefaultGeneration, 0)
	for _, agent := range agentRegistry.List() {
		managed, ok := agent.(agents.ManagedNPMRuntimeAgent)
		if !ok {
			continue
		}
		spec := managed.ManagedNPMRuntime()
		generations = append(generations, managedruntime.DefaultGeneration{
			AgentID: agent.ID(),
			Package: spec.Package,
			Version: spec.DefaultVersionOrPinned(),
		})
	}
	return generations
}

func reconcileManagedRuntimeDefaults(
	ctx context.Context,
	store *managedruntime.Store,
	agentRegistry *registry.Registry,
	log *logger.Logger,
) error {
	if store == nil {
		return fmt.Errorf("managed runtime selection store is unavailable")
	}
	store.SetDefaultGenerationChangeHandler(func(change managedruntime.DefaultGenerationChange) {
		if log == nil {
			return
		}
		log.Info("managed runtime default generation changed",
			zap.String("agent_id", change.Current.AgentID),
			zap.String("previous_package", change.Previous.Package),
			zap.String("previous_version", change.Previous.Version),
			zap.String("package", change.Current.Package),
			zap.String("version", change.Current.Version),
		)
	})
	if err := store.ReconcileDefaults(ctx, managedRuntimeDefaultGenerations(agentRegistry)); err != nil {
		if log != nil {
			log.Error("managed runtime default reconciliation failed", zap.Error(err))
		}
		return err
	}
	return nil
}

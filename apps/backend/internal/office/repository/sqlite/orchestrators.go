package sqlite

import (
	"context"
	orchestrationmodels "github.com/kandev/kandev/internal/orchestration/models"
	orchestrationstore "github.com/kandev/kandev/internal/orchestration/repository/sqlite"
)

// OrchestrationStore exposes the feature-owned store to composition code.
// Compatibility methods below preserve Office callers without owning its schema.
func (r *Repository) OrchestrationStore() *orchestrationstore.Repository {
	return orchestrationstore.New(r.db, r.ro)
}
func (r *Repository) createOrchestrationTables() error { return r.OrchestrationStore().Migrate() }
func (r *Repository) ListOrchestratorRoles(ctx context.Context) ([]orchestrationmodels.OrchestratorRole, error) {
	return r.OrchestrationStore().ListOrchestratorRoles(ctx)
}
func (r *Repository) GetOrchestratorRole(ctx context.Context, id string) (*orchestrationmodels.OrchestratorRole, error) {
	return r.OrchestrationStore().GetOrchestratorRole(ctx, id)
}
func (r *Repository) SaveOrchestratorRole(ctx context.Context, role *orchestrationmodels.OrchestratorRole) error {
	return r.OrchestrationStore().SaveOrchestratorRole(ctx, role)
}
func (r *Repository) DeleteOrchestratorRole(ctx context.Context, id string) error {
	return r.OrchestrationStore().DeleteOrchestratorRole(ctx, id)
}
func (r *Repository) RegisterOrchestrator(ctx context.Context, agentID, workspaceID, roleID string) error {
	return r.OrchestrationStore().RegisterOrchestrator(ctx, agentID, workspaceID, roleID)
}
func (r *Repository) ListOrchestratorIDs(ctx context.Context, workspaceID string) ([]string, error) {
	return r.OrchestrationStore().ListOrchestratorIDs(ctx, workspaceID)
}
func (r *Repository) OrchestratorRoleID(ctx context.Context, agentID string) (string, error) {
	return r.OrchestrationStore().OrchestratorRoleID(ctx, agentID)
}
func (r *Repository) UnregisterOrchestrator(ctx context.Context, id string) error {
	return r.OrchestrationStore().UnregisterOrchestrator(ctx, id)
}

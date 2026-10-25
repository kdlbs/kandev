package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// SetHostRepositoryCloner shares the authenticated attachment clone boundary.
func (b *branchMaterializer) SetHostRepositoryCloner(cloner orchestrator.RepositoryHostCloner) {
	if b != nil {
		b.hostCloner = cloner
	}
}

func (b *branchMaterializer) ensureHostRepositoryPath(ctx context.Context, taskID, sessionID string, repository *models.Repository) error {
	if repository.SourceType == hostSourceLocal || (repository.LocalPath != "" && repository.SourceType != hostSourceProvider) {
		return nil
	}
	if b.hostCloner == nil {
		return fmt.Errorf("managed attachment host cloner is unavailable")
	}
	path, err := b.hostCloner.EnsureRepositoryClonedForSession(ctx, taskID, sessionID, repository)
	if err != nil {
		return err
	}
	if path == "" {
		return fmt.Errorf("managed attachment clone produced no path")
	}
	repository.LocalPath = path
	return nil
}

func (b *branchMaterializer) verifyBranchCloneTarget(ctx context.Context, taskID string, session *models.TaskSession, environment *models.TaskEnvironment, expected *taskservice.BranchMaterializationTarget) error {
	if expected != nil && expected.TaskEnvironmentID != "" && (expected.TaskEnvironmentID != environment.ID || expected.SessionID != session.ID) {
		return fmt.Errorf("%w: branch materialization target changed", models.ErrWorkspaceReuseUnsafe)
	}
	current, err := b.pickActiveSession(ctx, taskID)
	if err != nil {
		return err
	}
	if current == nil || current.ID != session.ID || current.TaskEnvironmentID != session.TaskEnvironmentID {
		return fmt.Errorf("%w: branch materialization session changed", models.ErrWorkspaceReuseUnsafe)
	}
	resolved, err := b.resolveMaterializationEnvironment(ctx, taskID, current)
	if err != nil {
		return err
	}
	if !sameHostCloneEnvironment(resolved, environment) {
		return fmt.Errorf("%w: branch materialization environment changed", models.ErrWorkspaceReuseUnsafe)
	}
	return nil
}

func hasHostProviderIdentity(repository *models.Repository) bool {
	return (repository.ProviderOwner != "" && repository.ProviderName != "") || (repository.ProviderScope != "" && repository.ProviderRepoID != "")
}

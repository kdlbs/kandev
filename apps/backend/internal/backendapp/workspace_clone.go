package backendapp

import (
	"context"
	"errors"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowrepo "github.com/kandev/kandev/internal/workflow/repository"
)

type workspaceIntegrationCloner interface {
	CopyWorkspaceConfigurationTx(context.Context, *sqlx.Tx, string, string) (bool, error)
}

type workspaceClonePersistence struct {
	writer          *sqlx.DB
	tasks           *taskrepo.Repository
	workflows       *workflowrepo.Repository
	github          workspaceIntegrationCloner
	secrets         secrets.CredentialCopier
	validateProfile func(context.Context, string) error
}

func (c *workspaceClonePersistence) CloneWorkspace(ctx context.Context, source, target *models.Workspace) ([]*models.Workflow, error) {
	tx, err := c.writer.BeginTxx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	current, err := c.tasks.GetWorkspaceForCloneTx(ctx, tx, source.ID)
	if err != nil {
		return nil, err
	}
	if !current.UpdatedAt.Equal(source.UpdatedAt) || current.OwnerID != source.OwnerID || current.OrgID != source.OrgID || current.UnitID != source.UnitID {
		return nil, repoerrors.ErrTaskVersionConflict
	}
	if current.OfficeWorkflowID != "" || current.IsImproveKandev() {
		return nil, repoerrors.ErrWorkspaceCloneConfiguration
	}
	if err := c.tasks.CreateWorkspaceCloneTx(ctx, tx, target); err != nil {
		return nil, err
	}
	workflows, err := c.copyGraph(ctx, tx, source.ID, target.ID)
	if err != nil {
		return nil, err
	}
	if err := c.copyGitHub(ctx, tx, source.ID, target.ID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return workflows, nil
}

func (c *workspaceClonePersistence) copyGitHub(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) error {
	needsPAT, err := c.github.CopyWorkspaceConfigurationTx(ctx, tx, sourceID, targetID)
	if err != nil {
		return err
	}
	if needsPAT {
		if c.secrets == nil {
			return repoerrors.ErrWorkspaceCloneConfiguration
		}
		if err := c.secrets.CopyCredentialTx(ctx, tx, github.WorkspacePATSecretKey(sourceID), github.WorkspacePATSecretKey(targetID)); err != nil {
			if errors.Is(err, secrets.ErrNotFound) {
				return repoerrors.ErrWorkspaceCloneConfiguration
			}
			return err
		}
	}
	return nil
}

func (c *workspaceClonePersistence) copyGraph(ctx context.Context, tx *sqlx.Tx, sourceID, targetID string) ([]*models.Workflow, error) {
	mapped, err := c.tasks.CopyWorkspaceConfigurationTx(ctx, tx, sourceID, targetID)
	if err != nil {
		return nil, err
	}
	if len(mapped) == 0 {
		workflow, err := c.tasks.BootstrapWorkspaceCloneTx(ctx, tx, targetID)
		if err != nil {
			return nil, err
		}
		return []*models.Workflow{workflow}, nil
	}
	steps, err := c.workflows.CopyWorkspaceStepsTx(ctx, tx, mapped)
	if err != nil {
		return nil, err
	}
	// Read through the transaction, so the creation events use the committed
	// definitions without another writer checkout.
	workflows, err := c.tasks.ListWorkspaceCloneWorkflowsTx(ctx, tx, targetID)
	if err != nil {
		return nil, err
	}
	if err := c.validateGraphProfiles(ctx, workflows, steps); err != nil {
		return nil, err
	}
	return workflows, nil
}

func (c *workspaceClonePersistence) validateGraphProfiles(ctx context.Context, workflows []*models.Workflow, steps []*workflowmodels.WorkflowStep) error {
	profiles := make(map[string]struct{})
	for _, workflow := range workflows {
		if workflow.AgentProfileID != "" {
			profiles[workflow.AgentProfileID] = struct{}{}
		}
	}
	for _, step := range steps {
		if step.AgentProfileID != "" {
			profiles[step.AgentProfileID] = struct{}{}
		}
		for _, action := range step.Events.OnEnter {
			if action.Type == workflowmodels.OnEnterRunCodeReview {
				if id, ok := action.Config[workflowmodels.ReviewAgentProfileConfigKey].(string); ok && id != "" {
					profiles[id] = struct{}{}
				}
			}
		}
	}
	for id := range profiles {
		if c.validateProfile == nil {
			return repoerrors.ErrWorkspaceCloneConfiguration
		}
		if err := c.validateProfile(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/office/routing"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

// exclusionSetupEvaluator is the routing.Provider subset the import advisor
// needs.
type exclusionSetupEvaluator interface {
	ExclusionSetup(ctx context.Context, workspaceID string, decls []routing.ExclusionDeclaration) ([]routing.SetupWarning, error)
}

// importExclusionAdvisorAdapter satisfies workflowservice.ImportExclusionAdvisor
// over Office routing, so workflow import and sync can warn about
// exclude_author_provider setups without the workflow system importing Office.
type importExclusionAdvisorAdapter struct {
	setup exclusionSetupEvaluator
}

// AdviseExclusionSetup implements workflowservice.ImportExclusionAdvisor.
func (a *importExclusionAdvisorAdapter) AdviseExclusionSetup(
	ctx context.Context, workspaceID string, decls []workflowmodels.AuthorExclusionDeclaration,
) ([]workflowservice.ImportWarning, error) {
	routingDecls := make([]routing.ExclusionDeclaration, 0, len(decls))
	for _, d := range decls {
		routingDecls = append(routingDecls, routing.ExclusionDeclaration{
			WorkflowName: d.WorkflowName, StepName: d.StepName, Role: d.Role,
		})
	}
	warnings, err := a.setup.ExclusionSetup(ctx, workspaceID, routingDecls)
	if err != nil {
		return nil, err
	}
	out := make([]workflowservice.ImportWarning, 0, len(warnings))
	for _, w := range warnings {
		out = append(out, workflowservice.ImportWarning{
			Code:         string(w.Code),
			WorkflowName: w.Declaration.WorkflowName,
			StepName:     w.Declaration.StepName,
			Role:         w.Declaration.Role,
			ProviderID:   string(w.ProviderID),
			AgentName:    w.AgentName,
		})
	}
	return out, nil
}

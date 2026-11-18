package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/workflow/models"
)

// ImportWarningCheckFailed is the warning code used when the exclusion setup
// could not be evaluated. The import continues.
const ImportWarningCheckFailed = "check_failed"

// ImportWarning is a non-blocking notice about an imported or synced
// workflow. Code is a closed set the client localizes:
// routing_disabled, single_provider, agent_single_provider, check_failed
// (REQ-OFFICE-REVIEW-AUTHOR-EXCL-005).
type ImportWarning struct {
	Code         string `json:"code"`
	WorkflowName string `json:"workflow_name,omitempty"`
	StepName     string `json:"step_name,omitempty"`
	Role         string `json:"role,omitempty"`
	ProviderID   string `json:"provider_id,omitempty"`
	AgentName    string `json:"agent_name,omitempty"`
}

// ImportExclusionAdvisor predicts problems with exclude_author_provider
// declarations under a workspace's current setup. It is implemented outside
// the workflow system (Office routing) and wired by backendapp, so the
// workflow system never imports Office.
type ImportExclusionAdvisor interface {
	AdviseExclusionSetup(ctx context.Context, workspaceID string, decls []models.AuthorExclusionDeclaration) ([]ImportWarning, error)
}

// SetImportExclusionAdvisor wires the optional advisor. Unset means no
// exclusion warnings are produced.
func (s *Service) SetImportExclusionAdvisor(advisor ImportExclusionAdvisor) {
	s.exclusionAdvisor = advisor
}

// exclusionWarnings advises on the declarations in workflows. It never fails
// the caller: an advisor error becomes one check_failed warning.
func (s *Service) exclusionWarnings(
	ctx context.Context, workspaceID string, workflows []models.WorkflowPortable,
) []ImportWarning {
	if s.exclusionAdvisor == nil {
		return nil
	}
	decls := models.AuthorExclusionDeclarationsFor(workflows)
	if len(decls) == 0 {
		return nil
	}
	warnings, err := s.exclusionAdvisor.AdviseExclusionSetup(ctx, workspaceID, decls)
	if err != nil {
		s.logger.Warn("author exclusion setup check failed",
			zap.String("workspace_id", workspaceID), zap.Error(err))
		return []ImportWarning{{Code: ImportWarningCheckFailed}}
	}
	return warnings
}

func selectedWorkflows(export *models.WorkflowExport, indexes []int) []models.WorkflowPortable {
	out := make([]models.WorkflowPortable, 0, len(indexes))
	for _, i := range indexes {
		out = append(out, export.Workflows[i])
	}
	return out
}

// syncWarningText renders an exclusion warning in the workflow-sync warning
// format, which the sync status banner shows verbatim.
func syncWarningText(w ImportWarning) string {
	const rule = "Review on a different provider"
	subject := fmt.Sprintf("%s workflow, %s step (%s): ", w.WorkflowName, w.StepName, w.Role)
	switch w.Code {
	case "routing_disabled":
		return subject + "Automatic provider fallback is off, so " + rule + " is ignored." +
			" Reviews may run on the provider that did the work, so they are not an independent check." +
			" Turn on Automatic provider fallback in Office > Routing, or turn off " + rule + " for this step there."
	case "single_provider":
		return subject + w.ProviderID + " is the only provider, so the reviewer can never use a different one from the work." +
			" Reviews will wait until you act." +
			" Add a second provider in Office > Routing, or turn off " + rule + " for this step there."
	case "agent_single_provider":
		return subject + fmt.Sprintf("Agent %q can only use %s, so its reviews wait whenever the work ran on %s."+
			" Give %q another provider in its routing settings, or turn off "+rule+" for this step in Office > Routing.",
			w.AgentName, w.ProviderID, w.ProviderID, w.AgentName)
	default:
		return rule + " could not be checked. Open Office > Routing to check it."
	}
}

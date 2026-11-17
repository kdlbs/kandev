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
	subject := fmt.Sprintf("%s/%s: %s author-provider exclusion", w.WorkflowName, w.StepName, w.Role)
	switch w.Code {
	case "routing_disabled":
		return subject + " is not enforced because provider routing is off"
	case "single_provider":
		return fmt.Sprintf("%s can hold reviews: only %s is configured, so review runs wait whenever the work ran on %s",
			subject, w.ProviderID, w.ProviderID)
	case "agent_single_provider":
		return fmt.Sprintf("%s can hold reviews: agent %q can only use %s, so its review runs wait whenever the work ran on %s",
			subject, w.AgentName, w.ProviderID, w.ProviderID)
	default:
		return "author-provider exclusion setup could not be checked"
	}
}

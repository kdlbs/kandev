package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/workflow/models"
)

// ImportWarningCheckFailed is the warning code used when the exclusion setup
// could not be evaluated. The import continues.
const ImportWarningCheckFailed = "check_failed"

// ImportWarning is a non-blocking notice about an imported or synced
// workflow. Code is a closed set the client localizes:
// routing_disabled, single_provider, agent_single_provider,
// agent_bound_profile, check_failed.
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

// syncWarningTexts renders exclusion warnings in the workflow-sync warning
// format, which the sync status banner shows verbatim. Warnings that share a
// code, provider, and agent become one string: the problem, the affected
// steps, then one fix.
func syncWarningTexts(warnings []ImportWarning) []string {
	type group struct {
		first ImportWarning
		steps []string
	}
	var order []string
	groups := map[string]*group{}
	for _, w := range warnings {
		key := w.Code + "\x00" + w.ProviderID + "\x00" + w.AgentName
		g, ok := groups[key]
		if !ok {
			g = &group{first: w}
			groups[key] = g
			order = append(order, key)
		}
		if w.WorkflowName != "" {
			g.steps = append(g.steps, fmt.Sprintf("%s workflow, %s step (%s)", w.WorkflowName, w.StepName, w.Role))
		}
	}
	out := make([]string, 0, len(order))
	for _, key := range order {
		g := groups[key]
		out = append(out, syncWarningGroupText(g.first, g.steps))
	}
	return out
}

func syncWarningGroupText(w ImportWarning, steps []string) string {
	const rule = "Review on a different provider"
	target, affected := "this step", ""
	if len(steps) == 1 {
		affected = " Affected step: " + steps[0] + "."
	} else if len(steps) > 1 {
		target = "these steps"
		affected = " Affected steps: " + strings.Join(steps, "; ") + "."
	}
	switch w.Code {
	case "routing_disabled":
		return "Automatic provider fallback is off, so " + rule + " is ignored." +
			" Reviews may run on the provider that did the work, so they are not an independent check." + affected +
			" Turn on Automatic provider fallback in Office > Routing, or turn off " + rule + " for " + target + " there."
	case "single_provider":
		return w.ProviderID + " is the only provider, so reviews can never run on a different one from the work." +
			" Reviews will wait until you act." + affected +
			" Add a second provider in Office > Routing, or turn off " + rule + " for " + target + " there."
	case "agent_single_provider":
		return fmt.Sprintf("Agent %q can only use %s, so its reviews wait whenever the work ran on %s.",
			w.AgentName, w.ProviderID, w.ProviderID) + affected +
			fmt.Sprintf(" Give %q another provider in its routing settings, or turn off %s for %s in Office > Routing.",
				w.AgentName, rule, target)
	case "agent_bound_profile":
		return fmt.Sprintf("Agent %q uses its own execution profile, which %s does not cover,"+
			" so its reviews can run on the provider that did the work.", w.AgentName, rule) + affected +
			fmt.Sprintf(" Clear the Execution profile in %q's configuration so it follows Office provider routing.",
				w.AgentName)
	default:
		return rule + " could not be checked. Open Office > Routing to check it."
	}
}

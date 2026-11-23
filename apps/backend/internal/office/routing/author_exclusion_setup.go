package routing

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/office/models"
)

// ExclusionDeclaration names one workflow step whose ensure_participant_seat
// action sets exclude_author_provider for a participant role.
type ExclusionDeclaration struct {
	WorkflowID   string
	WorkflowName string
	StepID       string
	StepName     string
	Role         string
}

// SetupWarningCode is the closed set of author-exclusion setup predictions.
type SetupWarningCode string

const (
	// SetupWarningRoutingDisabled: routing is off or not configured, so the
	// exclusion is not enforced.
	SetupWarningRoutingDisabled SetupWarningCode = "routing_disabled"
	// SetupWarningSingleProvider: the workspace order has exactly one
	// provider, so reviews wait whenever the work ran on it.
	SetupWarningSingleProvider SetupWarningCode = "single_provider"
	// SetupWarningAgentSingleProvider: a seat-pool agent's override pins it
	// to exactly one provider, so its reviews wait whenever the work ran on it.
	SetupWarningAgentSingleProvider SetupWarningCode = "agent_single_provider"
	// SetupWarningAgentBoundProfile: a seat-pool agent runs through its own
	// bound execution profile, which workspace provider routing does not
	// govern, so its reviews are not kept off the author's provider.
	SetupWarningAgentBoundProfile SetupWarningCode = "agent_bound_profile"
	// SetupWarningCheckFailed: the prediction could not be computed.
	SetupWarningCheckFailed SetupWarningCode = "check_failed"
)

// SetupWarning is one predicted problem for one declaration. ProviderID is set
// for the single-provider codes; AgentID and AgentName only for the agent
// codes.
type SetupWarning struct {
	Code        SetupWarningCode
	Declaration ExclusionDeclaration
	ProviderID  ProviderID
	AgentID     string
	AgentName   string
}

// EvaluateExclusionSetup predicts, from the current routing config and Office
// agents, which declarations will not be enforced or can park their reviews.
// The halts are possible, not certain: a run waits only when the work ran on
// the one remaining provider, which is unknown until the work runs.
func EvaluateExclusionSetup(
	cfg *WorkspaceConfig, agents []*models.AgentInstance, decls []ExclusionDeclaration,
) []SetupWarning {
	var out []SetupWarning
	for _, decl := range decls {
		out = append(out, evaluateDeclaration(cfg, agents, decl)...)
	}
	sortSetupWarnings(out)
	return out
}

func evaluateDeclaration(
	cfg *WorkspaceConfig, agents []*models.AgentInstance, decl ExclusionDeclaration,
) []SetupWarning {
	if cfg == nil || !cfg.Enabled {
		return []SetupWarning{{Code: SetupWarningRoutingDisabled, Declaration: decl}}
	}
	var out []SetupWarning
	poolSize, inheriting := 0, 0
	for _, agent := range agents {
		if !models.IsSeatPoolEligible(decl.Role, agent) {
			continue
		}
		poolSize++
		if agent.ExecutionAgentProfileID != "" {
			out = append(out, SetupWarning{
				Code: SetupWarningAgentBoundProfile, Declaration: decl,
				AgentID: agent.ID, AgentName: agent.Name,
			})
			continue
		}
		ov, err := ReadAgentOverrides(agent.Settings)
		if err != nil {
			continue
		}
		if ov.ProviderOrderSource != ProviderOrderSourceOverride {
			inheriting++
			continue
		}
		if len(ov.ProviderOrder) == 1 {
			out = append(out, SetupWarning{
				Code: SetupWarningAgentSingleProvider, Declaration: decl,
				ProviderID: ov.ProviderOrder[0], AgentID: agent.ID, AgentName: agent.Name,
			})
		}
	}
	// An empty pool falls back to seating the runner, whose order is unknown,
	// so the workspace order is the only evidence.
	if len(cfg.ProviderOrder) == 1 && (poolSize == 0 || inheriting > 0) {
		out = append(out, SetupWarning{
			Code: SetupWarningSingleProvider, Declaration: decl, ProviderID: cfg.ProviderOrder[0],
		})
	}
	return out
}

func sortSetupWarnings(ws []SetupWarning) {
	sort.SliceStable(ws, func(i, j int) bool {
		a, b := ws[i], ws[j]
		for _, cmp := range [][2]string{
			{a.Declaration.WorkflowName, b.Declaration.WorkflowName},
			{a.Declaration.StepName, b.Declaration.StepName},
			{a.Declaration.Role, b.Declaration.Role},
			{string(a.Code), string(b.Code)},
			{a.AgentName, b.AgentName},
		} {
			if cmp[0] != cmp[1] {
				return cmp[0] < cmp[1]
			}
		}
		return false
	})
}

// ExclusionSetup evaluates decls against the workspace's current routing
// config and Office agents. No declarations means nothing to read.
func (p *Provider) ExclusionSetup(
	ctx context.Context, workspaceID string, decls []ExclusionDeclaration,
) ([]SetupWarning, error) {
	if len(decls) == 0 {
		return nil, nil
	}
	cfg, err := p.repo.GetWorkspaceRouting(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("load routing config: %w", err)
	}
	agents, err := p.repo.ListAgentInstances(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	return EvaluateExclusionSetup(cfg, agents, decls), nil
}

// StoredExclusionSetup evaluates the declarations stored in the workspace's
// workflows.
func (p *Provider) StoredExclusionSetup(ctx context.Context, workspaceID string) ([]SetupWarning, error) {
	decls, err := p.repo.ListAuthorExclusionDeclarations(ctx, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list exclusion declarations: %w", err)
	}
	return p.ExclusionSetup(ctx, workspaceID, decls)
}

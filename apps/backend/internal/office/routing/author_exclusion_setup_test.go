package routing_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

func setupAgent(t *testing.T, id string, role models.AgentRole, status models.AgentStatus, pin []routing.ProviderID) *models.AgentInstance {
	t.Helper()
	settings := "{}"
	if pin != nil {
		blob, err := json.Marshal(map[string]interface{}{"routing": routing.AgentOverrides{
			ProviderOrderSource: routing.ProviderOrderSourceOverride,
			ProviderOrder:       pin,
		}})
		if err != nil {
			t.Fatalf("marshal overrides: %v", err)
		}
		settings = string(blob)
	}
	return &models.AgentInstance{ID: id, Name: "Agent " + id, Role: role, Status: status, Settings: settings}
}

var reviewDecl = routing.ExclusionDeclaration{
	WorkflowID: "wf-1", WorkflowName: "Office Review", StepID: "s-review", StepName: "Review", Role: "reviewer",
}

func TestEvaluateExclusionSetup(t *testing.T) {
	singleCfg := twoProviderCfg()
	singleCfg.ProviderOrder = []routing.ProviderID{"claude-acp"}
	disabledCfg := twoProviderCfg()
	disabledCfg.Enabled = false

	cases := []struct {
		name   string
		cfg    *routing.WorkspaceConfig
		agents []*models.AgentInstance
		decls  []routing.ExclusionDeclaration
		want   []routing.SetupWarning
	}{
		{
			name: "no declarations", cfg: singleCfg,
			agents: []*models.AgentInstance{setupAgent(t, "a", models.AgentRoleCEO, "", nil)},
		},
		{
			name: "routing not configured", cfg: nil, decls: []routing.ExclusionDeclaration{reviewDecl},
			want: []routing.SetupWarning{{Code: routing.SetupWarningRoutingDisabled, Declaration: reviewDecl}},
		},
		{
			name: "routing disabled", cfg: disabledCfg, decls: []routing.ExclusionDeclaration{reviewDecl},
			want: []routing.SetupWarning{{Code: routing.SetupWarningRoutingDisabled, Declaration: reviewDecl}},
		},
		{
			name: "two providers, inheriting agents", cfg: twoProviderCfg(), decls: []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{setupAgent(t, "a", models.AgentRoleCEO, "", nil)},
		},
		{
			name: "single workspace provider", cfg: singleCfg, decls: []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{setupAgent(t, "a", models.AgentRoleCEO, "", nil)},
			want:   []routing.SetupWarning{{Code: routing.SetupWarningSingleProvider, Declaration: reviewDecl, ProviderID: "claude-acp"}},
		},
		{
			name: "single workspace provider, empty pool falls back to runner", cfg: singleCfg,
			decls:  []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{setupAgent(t, "w", models.AgentRoleWorker, "", nil)},
			want:   []routing.SetupWarning{{Code: routing.SetupWarningSingleProvider, Declaration: reviewDecl, ProviderID: "claude-acp"}},
		},
		{
			name: "single workspace provider, every pool agent overrides with two", cfg: singleCfg,
			decls: []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{
				setupAgent(t, "a", models.AgentRoleCEO, "", []routing.ProviderID{"claude-acp", "codex-acp"}),
			},
		},
		{
			name: "pinned pool agent", cfg: twoProviderCfg(), decls: []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{
				setupAgent(t, "q", models.AgentRoleSpecialist, "", []routing.ProviderID{"codex-acp"}),
				setupAgent(t, "a", models.AgentRoleCEO, "", nil),
			},
			want: []routing.SetupWarning{{
				Code: routing.SetupWarningAgentSingleProvider, Declaration: reviewDecl,
				ProviderID: "codex-acp", AgentID: "q", AgentName: "Agent q",
			}},
		},
		{
			name: "pinned agents outside pool or inactive are ignored", cfg: twoProviderCfg(),
			decls: []routing.ExclusionDeclaration{reviewDecl},
			agents: []*models.AgentInstance{
				setupAgent(t, "w", models.AgentRoleWorker, "", []routing.ProviderID{"codex-acp"}),
				setupAgent(t, "s", models.AgentRoleCEO, models.AgentStatusStopped, []routing.ProviderID{"codex-acp"}),
				setupAgent(t, "p", models.AgentRoleCEO, models.AgentStatusPendingApproval, []routing.ProviderID{"codex-acp"}),
			},
		},
		{
			name: "approver pool excludes specialists", cfg: twoProviderCfg(),
			decls: []routing.ExclusionDeclaration{{WorkflowName: "W", StepName: "Approval", Role: "approver"}},
			agents: []*models.AgentInstance{
				setupAgent(t, "q", models.AgentRoleSpecialist, "", []routing.ProviderID{"codex-acp"}),
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := routing.EvaluateExclusionSetup(tc.cfg, tc.agents, tc.decls)
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("warnings = %+v\nwant       %+v", got, tc.want)
			}
		})
	}
}

func TestEvaluateExclusionSetupSortsDeterministically(t *testing.T) {
	singleCfg := twoProviderCfg()
	singleCfg.ProviderOrder = []routing.ProviderID{"claude-acp"}
	b := routing.ExclusionDeclaration{WorkflowName: "B", StepName: "Review", Role: "reviewer"}
	a := routing.ExclusionDeclaration{WorkflowName: "A", StepName: "Review", Role: "reviewer"}
	got := routing.EvaluateExclusionSetup(singleCfg, nil, []routing.ExclusionDeclaration{b, a})
	if len(got) != 2 || got[0].Declaration.WorkflowName != "A" || got[1].Declaration.WorkflowName != "B" {
		t.Fatalf("warnings = %+v, want A then B", got)
	}
}

package routing_test

import (
	"reflect"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

func boundAgent(t *testing.T, id string, pin []routing.ProviderID) *models.AgentInstance {
	t.Helper()
	agent := setupAgent(t, id, models.AgentRoleCEO, "", pin)
	agent.ExecutionAgentProfileID = "profile-" + id
	return agent
}

func TestEvaluateExclusionSetupFlagsBoundProfileAgents(t *testing.T) {
	singleCfg := twoProviderCfg()
	singleCfg.ProviderOrder = []routing.ProviderID{"claude-acp"}
	disabledCfg := twoProviderCfg()
	disabledCfg.Enabled = false
	bound := func(id string) routing.SetupWarning {
		return routing.SetupWarning{
			Code: routing.SetupWarningAgentBoundProfile, Declaration: reviewDecl, AgentID: id, AgentName: "Agent " + id,
		}
	}

	cases := []struct {
		name   string
		cfg    *routing.WorkspaceConfig
		agents []*models.AgentInstance
		want   []routing.SetupWarning
	}{
		{
			name:   "bound pool agent",
			cfg:    twoProviderCfg(),
			agents: []*models.AgentInstance{boundAgent(t, "b", nil), setupAgent(t, "a", models.AgentRoleCEO, "", nil)},
			want:   []routing.SetupWarning{bound("b")},
		},
		{
			name:   "a bound agent's routing override is irrelevant",
			cfg:    twoProviderCfg(),
			agents: []*models.AgentInstance{boundAgent(t, "b", []routing.ProviderID{"codex-acp"})},
			want:   []routing.SetupWarning{bound("b")},
		},
		{
			name:   "a bound agent does not count as inheriting the single workspace provider",
			cfg:    singleCfg,
			agents: []*models.AgentInstance{boundAgent(t, "b", nil)},
			want:   []routing.SetupWarning{bound("b")},
		},
		{
			name:   "routing disabled already says the exclusion is not enforced",
			cfg:    disabledCfg,
			agents: []*models.AgentInstance{boundAgent(t, "b", nil)},
			want:   []routing.SetupWarning{{Code: routing.SetupWarningRoutingDisabled, Declaration: reviewDecl}},
		},
		{
			name: "bound agents outside the pool are ignored",
			cfg:  twoProviderCfg(),
			agents: []*models.AgentInstance{func() *models.AgentInstance {
				a := setupAgent(t, "w", models.AgentRoleWorker, "", nil)
				a.ExecutionAgentProfileID = "profile-w"
				return a
			}()},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := routing.EvaluateExclusionSetup(tc.cfg, tc.agents, []routing.ExclusionDeclaration{reviewDecl})
			if len(got) == 0 && len(tc.want) == 0 {
				return
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("warnings = %+v\nwant       %+v", got, tc.want)
			}
		})
	}
}

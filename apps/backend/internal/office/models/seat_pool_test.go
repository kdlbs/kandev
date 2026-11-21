package models

import "testing"

func TestIsSeatPoolEligible(t *testing.T) {
	agent := func(role AgentRole, status AgentStatus) *AgentInstance {
		return &AgentInstance{Role: role, Status: status}
	}
	cases := []struct {
		name  string
		role  string
		agent *AgentInstance
		want  bool
	}{
		{"reviewer takes ceo", "reviewer", agent(AgentRoleCEO, ""), true},
		{"reviewer takes specialist", "reviewer", agent(AgentRoleSpecialist, ""), true},
		{"reviewer rejects worker", "reviewer", agent(AgentRoleWorker, ""), false},
		{"approver takes ceo", "approver", agent(AgentRoleCEO, ""), true},
		{"approver rejects specialist", "approver", agent(AgentRoleSpecialist, ""), false},
		{"other role is ceo only", "watcher", agent(AgentRoleSpecialist, ""), false},
		{"stopped excluded", "reviewer", agent(AgentRoleCEO, AgentStatusStopped), false},
		{"pending approval excluded", "reviewer", agent(AgentRoleCEO, AgentStatusPendingApproval), false},
		{"nil agent", "reviewer", nil, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsSeatPoolEligible(tc.role, tc.agent); got != tc.want {
				t.Fatalf("IsSeatPoolEligible(%q) = %v, want %v", tc.role, got, tc.want)
			}
		})
	}
}

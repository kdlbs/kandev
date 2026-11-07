package coordinator

import (
	"context"
	"encoding/json"
	"testing"
)

func (f *resumeFixture) tuneDailyLimit(t *testing.T, limit int) {
	t.Helper()
	stored, err := f.store.GetCoordinatorByID(context.Background(), f.c.ID)
	if err != nil {
		t.Fatal(err)
	}
	p := f.svc.policyFor(stored)
	e := p.EffectiveEligibility()
	e.DailyLimit = limit
	p.Eligibility = &e
	raw, _ := json.Marshal(p)
	mustExec(t, f.store, `UPDATE coordinators SET policy_json = ? WHERE id = ?`, string(raw), f.c.ID)
}

func (f *resumeFixture) proposeStartingMove(t *testing.T) *Proposal {
	t.Helper()
	target := f.targetID
	p := &Proposal{CoordinatorID: f.c.ID, WorkspaceID: f.c.WorkspaceID, Kind: ProposalKindMove, TargetTaskID: &target, StartsAgent: true, RawSpec: `{"task_id":"` + target + `"}`}
	if err := f.store.InsertProposal(context.Background(), p, true); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOverride_RejectingAStartsAgentMoveLowersStartAgent(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionStartAgent, testRaiser)
	got := f.reject(t, f.proposeStartingMove(t))
	if got.LoweredClass != "start_agent" || f.setting(t, ActionStartAgent) != SettingRequiresApproval {
		t.Fatalf("lowered = %q start_agent = %v", got.LoweredClass, f.setting(t, ActionStartAgent))
	}
}

func TestOverride_RejectingAPlainMoveLowersNothing(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionStartAgent, testRaiser)
	p := f.proposeStartingMove(t)
	mustExec(t, f.store, `UPDATE coordinator_proposals SET starts_agent = FALSE WHERE id = ?`, p.ID)
	if got := f.reject(t, p); got.LoweredClass != "" || f.setting(t, ActionStartAgent) != SettingAutomatic {
		t.Fatalf("a plain move reject lowered %q", got.LoweredClass)
	}
}

func TestAutomatic_AStartsAgentMoveNeverGoesAutomatic(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionStartAgent, testRaiser)
	p := f.proposeStartingMove(t)
	if res := f.try(t, p); res != nil {
		t.Fatalf("res = %+v, want the phase 1 result for a non-raisable kind", res)
	}
	if got := reload(t, f.store, f.c, p.ID); got.Status != ProposalStatusPending || f.resumer.calls != 0 {
		t.Fatalf("status = %s calls = %d", got.Status, f.resumer.calls)
	}
}

func TestAutomaticResume_DailyLimitIsTunableAndSharedAcrossClasses(t *testing.T) {
	f := newResumeFixture(t)
	f.raise(t, ActionResume, testRaiser)
	f.raise(t, ActionCreateTask, testRaiser)
	f.raise(t, ActionStartAgent, testRaiser)
	f.tuneDailyLimit(t, 2)
	if res := f.try(t, f.propose(t)); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("first resume = %+v", res)
	}
	if res := f.try(t, f.proposeStarting(t)); res == nil || res.Status != ProposalStatusApproved {
		t.Fatalf("agent-starting create = %+v", res)
	}
	third := f.propose(t)
	res := f.try(t, third)
	if res == nil || res.Status != ProposalStatusPending || res.Note != limitReachedNote {
		t.Fatalf("third automatic action = %+v, want held by the limit of 2", res)
	}
	if f.setting(t, ActionResume) != SettingAutomatic {
		t.Fatal("the limit lowered the class")
	}
}

func TestAutomatic_MultipleRaisers(t *testing.T) {
	cases := []struct {
		name         string
		createBy     string
		startBy      string
		wantPending  bool
		wantCreate   Setting
		wantStart    Setting
		wantDecideBy string
	}{
		{"both invalid lower their own class", "gone", "ghost", true, SettingRequiresApproval, SettingRequiresApproval, ""},
		{"one invalid lowers only its class", testRaiser, "gone", true, SettingAutomatic, SettingRequiresApproval, ""},
		{"all valid acts as the newest raiser", testRaiser, "mgr-2", false, SettingAutomatic, SettingAutomatic, "mgr-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newResumeFixture(t)
			f.az.allowed["mgr-2"] = true
			f.svc.SetAutomaticIdentities(fakeIdentities{known: map[string]bool{testRaiser: true, "mgr-2": true}})
			f.raise(t, ActionCreateTask, tc.createBy)
			f.raise(t, ActionStartAgent, tc.startBy)
			p := f.proposeStarting(t)
			res := f.try(t, p)
			if tc.wantPending {
				if res == nil || res.Status != ProposalStatusPending || res.Note != unavailableNote {
					t.Fatalf("res = %+v", res)
				}
			} else if res == nil || res.Status != ProposalStatusApproved {
				t.Fatalf("res = %+v", res)
			}
			if f.setting(t, ActionCreateTask) != tc.wantCreate || f.setting(t, ActionStartAgent) != tc.wantStart {
				t.Fatalf("create_task = %v start_agent = %v", f.setting(t, ActionCreateTask), f.setting(t, ActionStartAgent))
			}
			if tc.wantDecideBy != "" {
				got := reload(t, f.store, f.c, p.ID)
				if got.DecidedBy == nil || *got.DecidedBy != tc.wantDecideBy {
					t.Fatalf("decided by %v, want %s", got.DecidedBy, tc.wantDecideBy)
				}
			}
		})
	}
}

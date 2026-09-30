package coordinator

import (
	"errors"
	"testing"
)

func allDenied() map[Action]Setting {
	m := map[Action]Setting{}
	for _, a := range AllActions {
		m[a] = SettingDenied
	}
	return m
}

func TestParsePolicy_ResultTable(t *testing.T) {
	phaseOne := PhaseOnePolicy().Actions
	cases := []struct {
		name    string
		raw     *string
		want    map[Action]Setting
		wantErr bool
	}{
		{"NULL is phase-1", nil, phaseOne, false},
		{"empty", strPtr(""), allDenied(), true},
		{"whitespace", strPtr("  \n"), allDenied(), true},
		{"invalid json", strPtr("{"), allDenied(), true},
		{"non-object top level", strPtr(`[1]`), allDenied(), true},
		{"null top level", strPtr(`null`), allDenied(), true},
		{"actions null", strPtr(`{"version":1,"actions":null}`), allDenied(), true},
		{"actions missing", strPtr(`{"version":1}`), allDenied(), true},
		{"actions not object", strPtr(`{"version":1,"actions":[]}`), allDenied(), true},
		{"version 2", strPtr(`{"version":2,"actions":{"create_task":"requires_approval"}}`), allDenied(), true},
		{"version missing", strPtr(`{"actions":{"create_task":"requires_approval"}}`), allDenied(), true},
		{"absent action denied", strPtr(`{"version":1,"actions":{"create_task":"requires_approval"}}`), withSetting(allDenied(), ActionCreateTask, SettingRequiresApproval), false},
		{"unknown key dropped", strPtr(`{"version":1,"actions":{"nuke":"automatic","move":"requires_approval"}}`), withSetting(allDenied(), ActionMove, SettingRequiresApproval), false},
		{"null setting denied", strPtr(`{"version":1,"actions":{"move":null}}`), allDenied(), false},
		{"unknown setting denied", strPtr(`{"version":1,"actions":{"move":"always"}}`), allDenied(), false},
		{"automatic parses", strPtr(`{"version":1,"actions":{"move":"automatic"}}`), withSetting(allDenied(), ActionMove, SettingAutomatic), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ParsePolicy(tc.raw)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if tc.wantErr && !errors.Is(err, ErrPolicyUnreadable) {
				t.Fatalf("err %v is not ErrPolicyUnreadable", err)
			}
			for _, a := range AllActions {
				if got.Actions[a] != tc.want[a] {
					t.Errorf("%s = %q, want %q", a, got.Actions[a], tc.want[a])
				}
			}
			if len(got.Actions) != len(AllActions) {
				t.Errorf("actions has %d keys, want %d", len(got.Actions), len(AllActions))
			}
		})
	}
}

func withSetting(m map[Action]Setting, a Action, s Setting) map[Action]Setting {
	m[a] = s
	return m
}

func TestPhaseOnePolicy(t *testing.T) {
	p := PhaseOnePolicy()
	if p.Version != 1 {
		t.Fatalf("version = %d", p.Version)
	}
	for _, a := range AllActions {
		want := SettingDenied
		if a == ActionCreateTask {
			want = SettingRequiresApproval
		}
		if p.Actions[a] != want {
			t.Errorf("%s = %q, want %q", a, p.Actions[a], want)
		}
	}
}

func TestPolicyAllows(t *testing.T) {
	p := PhaseOnePolicy()
	if !p.Allows(ActionCreateTask) || p.Allows(ActionMove) {
		t.Fatal("phase-1 policy allows only create_task")
	}
	for _, a := range []Action{ActionUnknown, Action("bogus"), Action("")} {
		if p.Allows(a) {
			t.Errorf("Allows(%q) = true", a)
		}
	}
	if (Policy{}).Allows(ActionCreateTask) {
		t.Error("zero policy must allow nothing")
	}
}

func TestAllActionsFixedOrderExcludesUnknown(t *testing.T) {
	want := []Action{ActionCreateTask, ActionStartAgent, ActionMessage, ActionMove, ActionResume, ActionStop}
	if len(AllActions) != len(want) {
		t.Fatalf("AllActions = %v", AllActions)
	}
	for i, a := range want {
		if AllActions[i] != a {
			t.Errorf("AllActions[%d] = %q, want %q", i, AllActions[i], a)
		}
	}
}

func TestValidatePolicy(t *testing.T) {
	ok := PhaseOnePolicy()
	if err := Validate(ok); err != nil {
		t.Fatalf("phase-1 policy invalid: %v", err)
	}
	all := allDenied()
	all[ActionMove] = SettingRequiresApproval
	all[ActionResume] = SettingRequiresApproval
	if err := Validate(Policy{Version: 1, Actions: all}); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	cases := []struct {
		name      string
		mutate    func(m map[Action]Setting)
		wantField string
		wantCode  string
	}{
		{"stop not denied", func(m map[Action]Setting) { m[ActionStop] = SettingRequiresApproval }, "stop", ""},
		{"automatic", func(m map[Action]Setting) { m[ActionMove] = SettingAutomatic }, "move", "automatic_not_available"},
		{"bad value", func(m map[Action]Setting) { m[ActionMessage] = Setting("maybe") }, "message", ""},
		{"unknown action", func(m map[Action]Setting) { m[Action("nuke")] = SettingDenied }, "nuke", ""},
		{"first failing wins", func(m map[Action]Setting) {
			m[ActionStartAgent] = SettingAutomatic
			m[ActionStop] = SettingAutomatic
		}, "start_agent", "automatic_not_available"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := allDenied()
			tc.mutate(m)
			err := Validate(Policy{Version: 1, Actions: m})
			var fe *PolicyFieldError
			if !errors.As(err, &fe) {
				t.Fatalf("err = %v, want PolicyFieldError", err)
			}
			if fe.Field != tc.wantField || fe.Code != tc.wantCode {
				t.Fatalf("got field %q code %q, want %q %q", fe.Field, fe.Code, tc.wantField, tc.wantCode)
			}
		})
	}
}

func TestActionForTool(t *testing.T) {
	cases := map[string]Action{
		"propose_task_kandev":    ActionCreateTask,
		"propose_message_kandev": ActionMessage,
		"propose_move_kandev":    ActionMove,
		"propose_resume_kandev":  ActionResume,
		"list_tasks_kandev":      ActionUnknown,
		"":                       ActionUnknown,
		"unheard_of":             ActionUnknown,
	}
	for name, want := range cases {
		if got := ActionForTool(name); got != want {
			t.Errorf("ActionForTool(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestToolNames(t *testing.T) {
	phase1 := ToolNames(PhaseOnePolicy(), false, false)
	if len(phase1) != 7 {
		t.Fatalf("phase-1 tools = %v", phase1)
	}
	if phase1[len(phase1)-1] != "propose_task_kandev" {
		t.Fatalf("phase-1 last tool = %q", phase1[len(phase1)-1])
	}
	p := allDenied()
	p[ActionMove] = SettingRequiresApproval
	p[ActionCreateTask] = SettingRequiresApproval
	p[ActionStartAgent] = SettingRequiresApproval
	got := ToolNames(Policy{Version: 1, Actions: p}, true, false)
	want := append(append([]string{}, readTools...), "list_coordinator_activity_kandev", "propose_task_kandev", "propose_move_kandev")
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	// start_agent and stop map to no tool.
	none := ToolNames(Policy{Version: 1, Actions: allDenied()}, true, false)
	if len(none) != len(readTools)+1 {
		t.Fatalf("all denied tools = %v", none)
	}
}

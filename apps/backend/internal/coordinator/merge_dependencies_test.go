package coordinator

import "testing"

const (
	kA = "github|github.com|r1|1"
	kB = "github|github.com|r1|2"
	kC = "github|github.com|r1|10"
)

func graph() depGraph {
	return depGraph{
		deps:    map[string][]DepRef{},
		changes: map[string][]string{"ta": {kA}, "tb": {kB}, "tc": {kC}, "multi": {kA, kB}, "none": nil},
		linked:  map[string]bool{kA: true, kB: true, kC: true},
	}
}

func TestDependencyValidation(t *testing.T) {
	g := graph()
	cases := []struct {
		name string
		refs []DepRef
		code string
	}{
		{"empty ref", []DepRef{{}}, codeDependencyInvalid},
		{"both set", []DepRef{{Task: "ta", Change: kA}}, codeDependencyInvalid},
		{"self task", []DepRef{{Task: "tc"}}, codeDependencySelf},
		{"own change", []DepRef{{Change: kC}}, codeDependencySelf},
		{"duplicate", []DepRef{{Task: "ta"}, {Task: "ta"}}, codeDependencyDuplicate},
		{"task and change name one change", []DepRef{{Task: "ta"}, {Change: kA}}, codeDependencyDuplicate},
		{"malformed change", []DepRef{{Change: "x"}}, codeDependencyInvalid},
		{"unlinked change", []DepRef{{Change: "github|github.com|r9|5"}}, codeDependencyUnknown},
		{"task with two changes", []DepRef{{Task: "multi"}}, codeDependencyUnknown},
		{"unknown task", []DepRef{{Task: "ghost"}}, codeDependencyUnknown},
	}
	for _, tc := range cases {
		err := validateDepRefs("tc", tc.refs, g)
		if err == nil || err.Code != tc.code {
			t.Fatalf("%s: err = %v want %s", tc.name, err, tc.code)
		}
	}
	if err := validateDepRefs("tc", []DepRef{{Task: "ta"}, {Change: kB}}, g); err != nil {
		t.Fatalf("valid set refused: %v", err)
	}
	if err := validateDepRefs("tc", nil, g); err != nil {
		t.Fatalf("empty set refused: %v", err)
	}
}

func TestDependencyCycleBlocksEveryMember(t *testing.T) {
	g := graph()
	g.deps["ta"] = []DepRef{{Task: "tb"}}
	g.deps["tb"] = []DepRef{{Task: "tc"}}
	g.deps["tc"] = []DepRef{{Task: "ta"}}
	for _, card := range []string{"ta", "tb", "tc"} {
		if _, blocked := g.Predecessors(card); !blocked {
			t.Fatalf("%s in a cycle was not blocked", card)
		}
	}
	g.deps["tc"] = nil
	if _, blocked := g.Predecessors("ta"); blocked {
		t.Fatal("acyclic chain blocked")
	}
}

func TestUnlinkedChangeRefBlocks(t *testing.T) {
	g := graph()
	g.deps["tc"] = []DepRef{{Change: kA}}
	g.linked[kA] = false
	if _, blocked := g.Predecessors("tc"); !blocked {
		t.Fatal("a change no live task links must block")
	}
	delete(g.changes, "tb")
	g.deps["tc"] = []DepRef{{Task: "tb"}}
	if _, blocked := g.Predecessors("tc"); !blocked {
		t.Fatal("a deleted or archived task must block")
	}
	g.deps["tc"] = []DepRef{{Task: "multi"}}
	if _, blocked := g.Predecessors("tc"); !blocked {
		t.Fatal("a task with several changes must block")
	}
}

func TestPredecessorsSortedCanonically(t *testing.T) {
	g := graph()
	g.deps["tc"] = []DepRef{{Change: kC}, {Task: "tb"}, {Task: "ta"}}
	delete(g.deps, "tc")
	g.deps["x"] = []DepRef{{Task: "tc"}, {Task: "tb"}, {Task: "ta"}}
	keys, blocked := g.Predecessors("x")
	if blocked || len(keys) != 3 || keys[0].Number != 1 || keys[1].Number != 2 || keys[2].Number != 10 {
		t.Fatalf("keys = %+v blocked = %v", keys, blocked)
	}
}

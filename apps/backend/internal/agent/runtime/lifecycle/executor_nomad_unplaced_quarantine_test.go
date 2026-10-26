package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func TestSoleAllocationNode(t *testing.T) {
	alloc := func(node string) nomad.AllocationRecord { return nomad.AllocationRecord{NodeID: node} }
	cases := []struct {
		name   string
		allocs []nomad.AllocationRecord
		want   string
	}{
		{"none", nil, ""},
		{"one", []nomad.AllocationRecord{alloc("n1")}, "n1"},
		{"same node twice", []nomad.AllocationRecord{alloc("n1"), alloc("n1")}, "n1"},
		{"two nodes", []nomad.AllocationRecord{alloc("n1"), alloc("n2")}, ""},
		{"unplaced allocation", []nomad.AllocationRecord{alloc("")}, ""},
	}
	for _, tc := range cases {
		if got := soleAllocationNode(tc.allocs); got != tc.want {
			t.Errorf("%s: node = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestCheckQuarantineTargetForPoolNodes(t *testing.T) {
	profile := nomad.ProfileConfig{Mode: nomad.ModePool, NodePool: "kandev", NodeID: "n1"}
	if err := checkQuarantineTarget(profile, &nomad.NodeRecord{ID: "n1", NodePool: "kandev"}); err != nil {
		t.Fatalf("pool member refused: %v", err)
	}
	for name, node := range map[string]*nomad.NodeRecord{"other pool": {ID: "n1", NodePool: "other"}, "other node": {ID: "n2", NodePool: "kandev"}, "nil": nil} {
		if err := checkQuarantineTarget(profile, node); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
	if err := checkQuarantineTarget(nomad.ProfileConfig{Mode: nomad.ModePool, NodePool: "kandev"}, &nomad.NodeRecord{ID: "n1", NodePool: "kandev"}); err == nil {
		t.Error("unnamed node accepted")
	}
}

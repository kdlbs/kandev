package models

import "testing"

func TestWorkspaceOrphaned(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]interface{}
		want     bool
	}{
		{"nil metadata", nil, false},
		{"no workspace key", map[string]interface{}{}, false},
		{"workspace not an object", map[string]interface{}{"workspace": "oops"}, false},
		{
			"orphaned absent",
			map[string]interface{}{"workspace": map[string]interface{}{"mode": "inherit_parent"}},
			false,
		},
		{
			"orphaned false",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": false, "mode": "inherit_parent",
			}},
			false,
		},
		{
			"orphaned null",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": nil, "mode": "inherit_parent",
			}},
			false,
		},
		{
			"orphaned string true",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": "true", "mode": "inherit_parent",
			}},
			false,
		},
		{
			"orphaned number 1",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": float64(1), "mode": "inherit_parent",
			}},
			false,
		},
		{
			"orphaned true, mode inherit_parent",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": true, "mode": "inherit_parent",
			}},
			true,
		},
		{
			"orphaned true, mode shared_group (moved off inherit_parent)",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": true, "mode": "shared_group",
			}},
			false,
		},
		{
			"orphaned true, mode new_workspace",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": true, "mode": "new_workspace",
			}},
			false,
		},
		{
			"orphaned true, mode absent",
			map[string]interface{}{"workspace": map[string]interface{}{
				"orphaned": true,
			}},
			false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := WorkspaceOrphaned(tc.metadata); got != tc.want {
				t.Errorf("WorkspaceOrphaned(%#v) = %v, want %v", tc.metadata, got, tc.want)
			}
		})
	}
}

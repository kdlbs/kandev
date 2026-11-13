package managedruntime

import "testing"

// @covers AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.18
func TestManagedRuntimeOwnerPolicy(t *testing.T) {
	cases := []struct {
		name  string
		input RepairOwnerInput
		want  string
	}{
		{
			name: "real actor wins over workspace owner",
			input: RepairOwnerInput{
				ActorUserID: "actor-a", WorkspaceOwnerUserID: "workspace-owner",
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "actor-a",
		},
		{
			name: "workspace owner wins over permitted fallback",
			input: RepairOwnerInput{
				WorkspaceOwnerUserID:     "workspace-owner",
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "workspace-owner",
		},
		{
			name: "disabled mode unowned background uses fallback",
			input: RepairOwnerInput{
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "system",
		},
		{
			name: "enabled mode unowned background has no owner",
			input: RepairOwnerInput{
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: false,
			},
			want: "",
		},
		{
			name:  "unavailable service has no fallback",
			input: RepairOwnerInput{SingleUserFallbackUserID: "system"},
			want:  "",
		},
		{
			name: "synthetic actor yields to an owned workspace",
			input: RepairOwnerInput{
				ActorUserID: "system", ActorSynthetic: true, WorkspaceOwnerUserID: "workspace-owner",
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "workspace-owner",
		},
		{
			name: "synthetic actor on an unowned workspace uses the disabled fallback",
			input: RepairOwnerInput{
				ActorUserID: "system", ActorSynthetic: true,
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "system",
		},
		{
			name: "synthetic actor on an unowned workspace cannot use an enabled fallback",
			input: RepairOwnerInput{
				ActorUserID: "system", ActorSynthetic: true,
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: false,
			},
			want: "",
		},
		{
			name: "distinct actors remain distinct",
			input: RepairOwnerInput{
				ActorUserID: "actor-b", WorkspaceOwnerUserID: "actor-a",
				SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true,
			},
			want: "actor-b",
		},
		{
			name:  "blank identities stay empty",
			input: RepairOwnerInput{ActorUserID: " ", WorkspaceOwnerUserID: " ", SingleUserFallbackUserID: "system", SingleUserFallbackAllowed: true},
			want:  "system",
		},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveRepairOwner(tt.input); got != tt.want {
				t.Fatalf("ResolveRepairOwner(%+v) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

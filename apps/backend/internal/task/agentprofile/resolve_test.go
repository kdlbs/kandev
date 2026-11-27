package agentprofile

import "testing"

func TestResolve(t *testing.T) {
	cases := []struct {
		name   string
		in     Input
		wantID string
		want   Source
	}{
		{"empty", Input{}, "", SourceNone},
		{"step replacement wins", Input{HasStep: true, StepReplacement: "r", StepProfile: "s", WorkflowDefault: "w", TaskMetadata: "m"}, "r", SourceStep},
		{"step profile", Input{HasStep: true, StepProfile: "s", WorkflowDefault: "w", TaskMetadata: "m"}, "s", SourceStep},
		{"workflow default beats task metadata", Input{HasStep: true, WorkflowDefault: "w", TaskMetadata: "m"}, "w", SourceWorkflow},
		{"workflow default needs a step", Input{WorkflowDefault: "w", TaskMetadata: "m"}, "m", SourceTaskMetadata},
		{"session target skips step and workflow", Input{HasStep: true, StepSessionTarget: true, StepReplacement: "r", StepProfile: "s", WorkflowDefault: "w"}, "", SourceNone},
		{"session target falls to metadata", Input{HasStep: true, StepSessionTarget: true, StepProfile: "s", WorkflowDefault: "w", TaskMetadata: "m"}, "m", SourceTaskMetadata},
		{"assignee", Input{Assignee: "a", WorkspaceDefault: "d"}, "a", SourceAssignee},
		{"metadata beats assignee", Input{TaskMetadata: "m", Assignee: "a"}, "m", SourceTaskMetadata},
		{"workspace default last", Input{HasStep: true, WorkspaceDefault: "d"}, "d", SourceWorkspace},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			id, src := Resolve(tc.in)
			if id != tc.wantID || src != tc.want {
				t.Fatalf("Resolve() = (%q, %q), want (%q, %q)", id, src, tc.wantID, tc.want)
			}
		})
	}
}

package contract

import "testing"

// @covers AC-RELEASE-COMPACT-RUNTIME-001.3
func TestParsePlanWriteMode(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    PlanWriteMode
		wantErr string
	}{
		{name: "empty defaults to replace", raw: "", want: PlanWriteModeReplace},
		{name: "replace", raw: "replace", want: PlanWriteModeReplace},
		{name: "append", raw: "append", want: PlanWriteModeAppend},
		{name: "case variant", raw: "Append", wantErr: `mode must be "replace" or "append"`},
		{name: "unknown mode", raw: "merge", wantErr: `mode must be "replace" or "append"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlanWriteMode(tt.raw)
			if tt.wantErr != "" {
				if err == nil || err.Error() != tt.wantErr {
					t.Fatalf("ParsePlanWriteMode(%q) error = %v, want %q", tt.raw, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParsePlanWriteMode(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParsePlanWriteMode(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestTaskLimits(t *testing.T) {
	if TaskTitleMaxLength != 60 {
		t.Errorf("TaskTitleMaxLength = %d, want 60", TaskTitleMaxLength)
	}
	if MaxPlanRevisionPageLimit != 100 {
		t.Errorf("MaxPlanRevisionPageLimit = %d, want 100", MaxPlanRevisionPageLimit)
	}
}

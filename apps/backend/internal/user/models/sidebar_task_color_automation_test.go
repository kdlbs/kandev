package models

import (
	"strings"
	"testing"
)

func TestValidateSidebarTaskColorAutomationAcceptsCompleteAndIncompleteRules(t *testing.T) {
	settings := SidebarTaskColorAutomation{
		Enabled: true,
		Rules: []SidebarTaskColorRule{
			{
				ID:      "blocked",
				Enabled: true,
				Condition: SidebarTaskColorCondition{
					Dimension: SidebarTaskColorDimensionTaskState,
					Value:     "BLOCKED",
					Label:     "Blocked",
				},
				Output: SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "red"},
			},
			{
				ID:      "unfinished",
				Enabled: false,
				Condition: SidebarTaskColorCondition{
					Dimension: SidebarTaskColorDimensionRepository,
					Value:     nil,
				},
				Output: SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "blue"},
			},
		},
	}

	if err := ValidateSidebarTaskColorAutomation(settings); err != nil {
		t.Fatalf("validate automatic colors: %v", err)
	}
}

func TestValidateSidebarTaskColorAutomationRejectsInvalidRules(t *testing.T) {
	tests := []struct {
		name  string
		value SidebarTaskColorAutomation
		want  string
	}{
		{
			name: "duplicate ids",
			value: SidebarTaskColorAutomation{Rules: []SidebarTaskColorRule{
				{ID: "same", Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "TODO"}, Output: SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "red"}},
				{ID: "same", Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "REVIEW"}, Output: SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "blue"}},
			}},
			want: "duplicate rule id",
		},
		{
			name: "enabled incomplete rule",
			value: SidebarTaskColorAutomation{Rules: []SidebarTaskColorRule{{
				ID:        "missing-target",
				Enabled:   true,
				Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionWorkflow, Value: nil},
				Output:    SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "red"},
			}}},
			want: "enabled rule must have a target",
		},
		{
			name: "unknown color",
			value: SidebarTaskColorAutomation{Rules: []SidebarTaskColorRule{{
				ID:        "unknown-color",
				Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "TODO"},
				Output:    SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "chartreuse"},
			}}},
			want: "unsupported fixed color",
		},
		{
			name: "workflow output on another dimension",
			value: SidebarTaskColorAutomation{Rules: []SidebarTaskColorRule{{
				ID:        "invalid-output",
				Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "TODO"},
				Output:    SidebarTaskColorOutput{Kind: SidebarTaskColorOutputWorkflowStep},
			}}},
			want: "workflow-step output requires workflow_step",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidateSidebarTaskColorAutomation(tt.value); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want text %q", err, tt.want)
			}
		})
	}
}

func TestValidateSidebarTaskColorAutomationEnforcesBounds(t *testing.T) {
	tooMany := make([]SidebarTaskColorRule, MaxSidebarTaskColorAutomationRules+1)
	for i := range tooMany {
		tooMany[i] = SidebarTaskColorRule{
			ID:        string(rune('a'+i%26)) + "-" + string(rune('0'+i%10)),
			Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "TODO"},
			Output:    SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "red"},
		}
	}
	if err := ValidateSidebarTaskColorAutomation(SidebarTaskColorAutomation{Rules: tooMany}); err == nil {
		t.Fatal("expected rule-count validation error")
	}

	oversizedLabel := SidebarTaskColorAutomation{Rules: []SidebarTaskColorRule{{
		ID:        "label",
		Condition: SidebarTaskColorCondition{Dimension: SidebarTaskColorDimensionTaskState, Value: "TODO", Label: strings.Repeat("界", MaxSidebarTaskColorAutomationLabelCodePoints+1)},
		Output:    SidebarTaskColorOutput{Kind: SidebarTaskColorOutputFixed, Color: "red"},
	}}}
	if err := ValidateSidebarTaskColorAutomation(oversizedLabel); err == nil {
		t.Fatal("expected label-size validation error")
	}
}

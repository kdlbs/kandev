package agents

import (
	"testing"
)

func TestAuggieParseModels(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantLen  int
		wantErr  bool
		wantIDs  []string
		wantDef  string // expected default model ID
	}{
		{
			name: "normal output with descriptions",
			input: `Available models:
 - Haiku 4.5 [haiku4.5]
     Fast and efficient responses
 - Claude Opus 4.5 [opus4.5]
     Best for complex tasks
 - Sonnet 4.5 [sonnet4.5]
     Great for everyday tasks`,
			wantLen: 3,
			wantIDs: []string{"haiku4.5", "opus4.5", "sonnet4.5"},
			wantDef: "sonnet4.5",
		},
		{
			name: "models without descriptions",
			input: `Available models:
 - Haiku 4.5 [haiku4.5]
 - Sonnet 4.5 [sonnet4.5]`,
			wantLen: 2,
			wantIDs: []string{"haiku4.5", "sonnet4.5"},
			wantDef: "sonnet4.5",
		},
		{
			name:    "empty output returns empty slice",
			input:   "",
			wantLen: 0,
		},
		{
			name:    "header only returns error",
			input:   "Available models:\n",
			wantErr: true,
		},
		{
			name:    "non-empty garbage returns error",
			input:   "some unexpected output\nno models here",
			wantErr: true,
		},
		{
			name: "non-matching lines ignored when models exist",
			input: `Some random text
 - Valid Model [valid-id]
     A description
Not a model line
Another random line`,
			wantLen: 1,
			wantIDs: []string{"valid-id"},
		},
		{
			name: "default model gets IsDefault true",
			input: ` - Other [other-id]
     Some model
 - Sonnet 4.5 [sonnet4.5]
     Default model`,
			wantLen: 2,
			wantDef: "sonnet4.5",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := auggieParseModels(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatal("auggieParseModels() expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("auggieParseModels() error = %v", err)
			}
			if len(models) != tt.wantLen {
				t.Fatalf("auggieParseModels() returned %d models, want %d", len(models), tt.wantLen)
			}
			for i, wantID := range tt.wantIDs {
				if i >= len(models) {
					break
				}
				if models[i].ID != wantID {
					t.Errorf("model[%d].ID = %q, want %q", i, models[i].ID, wantID)
				}
				if models[i].Source != "dynamic" {
					t.Errorf("model[%d].Source = %q, want %q", i, models[i].Source, "dynamic")
				}
			}
			if tt.wantDef != "" {
				found := false
				for _, m := range models {
					if m.ID == tt.wantDef {
						if !m.IsDefault {
							t.Errorf("model %q should have IsDefault=true", tt.wantDef)
						}
						found = true
					} else if m.IsDefault {
						t.Errorf("model %q should not have IsDefault=true", m.ID)
					}
				}
				if !found {
					t.Errorf("default model %q not found in results", tt.wantDef)
				}
			}
		})
	}
}

func TestAuggieParseModels_Descriptions(t *testing.T) {
	input := ` - Haiku 4.5 [haiku4.5]
     Fast and efficient responses
 - Sonnet 4.5 [sonnet4.5]`

	models, err := auggieParseModels(input)
	if err != nil {
		t.Fatalf("auggieParseModels() error = %v", err)
	}
	if len(models) != 2 {
		t.Fatalf("got %d models, want 2", len(models))
	}
	if models[0].Description != "Fast and efficient responses" {
		t.Errorf("model[0].Description = %q, want %q", models[0].Description, "Fast and efficient responses")
	}
	if models[0].Name != "Haiku 4.5" {
		t.Errorf("model[0].Name = %q, want %q", models[0].Name, "Haiku 4.5")
	}
	if models[1].Description != "" {
		t.Errorf("model[1].Description = %q, want empty", models[1].Description)
	}
}

package modelfetcher

import (
	"testing"
)

func TestDefaultParser_Parse(t *testing.T) {
	parser := &DefaultParser{}

	tests := []struct {
		name         string
		output       string
		defaultModel string
		wantCount    int
		wantFirst    string
		wantErr      bool
	}{
		{
			name: "single model with provider",
			output: `anthropic/claude-sonnet-4-20250514`,
			defaultModel: "",
			wantCount:    1,
			wantFirst:    "anthropic/claude-sonnet-4-20250514",
			wantErr:      false,
		},
		{
			name: "multiple models with providers",
			output: `anthropic/claude-sonnet-4-20250514
openai/gpt-4o
google/gemini-2.5-pro`,
			defaultModel: "",
			wantCount:    3,
			wantFirst:    "anthropic/claude-sonnet-4-20250514",
			wantErr:      false,
		},
		{
			name: "model without provider",
			output: `gpt-4o`,
			defaultModel: "",
			wantCount:    1,
			wantFirst:    "gpt-4o",
			wantErr:      false,
		},
		{
			name: "empty lines ignored",
			output: `anthropic/claude-sonnet

openai/gpt-4o

`,
			defaultModel: "",
			wantCount:    2,
			wantFirst:    "anthropic/claude-sonnet",
			wantErr:      false,
		},
		{
			name:         "empty output",
			output:       "",
			defaultModel: "",
			wantCount:    0,
			wantErr:      false,
		},
		{
			name:         "whitespace only",
			output:       "   \n\t\n   ",
			defaultModel: "",
			wantCount:    0,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			models, err := parser.Parse(tt.output, tt.defaultModel)

			if (err != nil) != tt.wantErr {
				t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(models) != tt.wantCount {
				t.Errorf("Parse() returned %d models, want %d", len(models), tt.wantCount)
				return
			}

			if tt.wantCount > 0 && models[0].ID != tt.wantFirst {
				t.Errorf("Parse() first model ID = %q, want %q", models[0].ID, tt.wantFirst)
			}
		})
	}
}

func TestDefaultParser_ParseLine(t *testing.T) {
	parser := &DefaultParser{}

	tests := []struct {
		name         string
		line         string
		defaultModel string
		wantID       string
		wantName     string
		wantProvider string
		wantDefault  bool
		wantSource   string
	}{
		{
			name:         "provider/model format",
			line:         "anthropic/claude-sonnet-4-20250514",
			defaultModel: "",
			wantID:       "anthropic/claude-sonnet-4-20250514",
			wantName:     "claude-sonnet-4-20250514",
			wantProvider: "anthropic",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
		{
			name:         "model only format",
			line:         "gpt-4o",
			defaultModel: "",
			wantID:       "gpt-4o",
			wantName:     "gpt-4o",
			wantProvider: "",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
		{
			name:         "is default model",
			line:         "anthropic/claude-sonnet-4",
			defaultModel: "anthropic/claude-sonnet-4",
			wantID:       "anthropic/claude-sonnet-4",
			wantName:     "claude-sonnet-4",
			wantProvider: "anthropic",
			wantDefault:  true,
			wantSource:   "dynamic",
		},
		{
			name:         "simple provider/model",
			line:         "openai/gpt-4",
			defaultModel: "",
			wantID:       "openai/gpt-4",
			wantName:     "gpt-4",
			wantProvider: "openai",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
		{
			name:         "nested provider path - split on last slash",
			line:         "openrouter/qwen/qwen3-30b-a3b-thinking",
			defaultModel: "",
			wantID:       "openrouter/qwen/qwen3-30b-a3b-thinking",
			wantName:     "qwen3-30b-a3b-thinking",
			wantProvider: "openrouter/qwen",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
		{
			name:         "github-copilot provider",
			line:         "github-copilot/gemini-3-flash-preview",
			defaultModel: "",
			wantID:       "github-copilot/gemini-3-flash-preview",
			wantName:     "gemini-3-flash-preview",
			wantProvider: "github-copilot",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
		{
			name:         "opencode provider",
			line:         "opencode/big-pickle",
			defaultModel: "",
			wantID:       "opencode/big-pickle",
			wantName:     "big-pickle",
			wantProvider: "opencode",
			wantDefault:  false,
			wantSource:   "dynamic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := parser.parseLine(tt.line, tt.defaultModel)

			if entry.ID != tt.wantID {
				t.Errorf("parseLine() ID = %q, want %q", entry.ID, tt.wantID)
			}
			if entry.Name != tt.wantName {
				t.Errorf("parseLine() Name = %q, want %q", entry.Name, tt.wantName)
			}
			if entry.Provider != tt.wantProvider {
				t.Errorf("parseLine() Provider = %q, want %q", entry.Provider, tt.wantProvider)
			}
			if entry.IsDefault != tt.wantDefault {
				t.Errorf("parseLine() IsDefault = %v, want %v", entry.IsDefault, tt.wantDefault)
			}
			if entry.Source != tt.wantSource {
				t.Errorf("parseLine() Source = %q, want %q", entry.Source, tt.wantSource)
			}
		})
	}
}

func TestGetParser(t *testing.T) {
	tests := []struct {
		name     string
		agentID  string
		wantType string
	}{
		{
			name:     "opencode agent",
			agentID:  "opencode",
			wantType: "*modelfetcher.OpenCodeParser",
		},
		{
			name:     "unknown agent uses default",
			agentID:  "unknown-agent",
			wantType: "*modelfetcher.DefaultParser",
		},
		{
			name:     "claude-code uses default",
			agentID:  "claude-code",
			wantType: "*modelfetcher.DefaultParser",
		},
		{
			name:     "empty string uses default",
			agentID:  "",
			wantType: "*modelfetcher.DefaultParser",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := GetParser(tt.agentID)

			// Check type by attempting type assertions
			switch tt.wantType {
			case "*modelfetcher.OpenCodeParser":
				if _, ok := parser.(*OpenCodeParser); !ok {
					t.Errorf("GetParser(%q) returned %T, want *OpenCodeParser", tt.agentID, parser)
				}
			case "*modelfetcher.DefaultParser":
				if _, ok := parser.(*DefaultParser); !ok {
					t.Errorf("GetParser(%q) returned %T, want *DefaultParser", tt.agentID, parser)
				}
			}
		})
	}
}

func TestOpenCodeParser_Parse(t *testing.T) {
	parser := NewOpenCodeParser()

	// OpenCode outputs models in provider/model format
	output := `anthropic/claude-sonnet-4-20250514
anthropic/claude-opus-4-20250514
openai/gpt-4.1
google/gemini-2.5-pro`

	models, err := parser.Parse(output, "anthropic/claude-sonnet-4-20250514")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if len(models) != 4 {
		t.Fatalf("Parse() returned %d models, want 4", len(models))
	}

	// Check first model is marked as default
	if !models[0].IsDefault {
		t.Error("first model should be marked as default")
	}

	// Check other models are not default
	for i := 1; i < len(models); i++ {
		if models[i].IsDefault {
			t.Errorf("model %d should not be marked as default", i)
		}
	}

	// Verify all are marked as dynamic
	for i, m := range models {
		if m.Source != "dynamic" {
			t.Errorf("model %d Source = %q, want 'dynamic'", i, m.Source)
		}
	}
}

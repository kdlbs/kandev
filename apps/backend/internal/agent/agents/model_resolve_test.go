package agents

import "testing"

func TestACPModelID(t *testing.T) {
	tests := []struct {
		name  string
		model Model
		want  string
	}{
		{
			name:  "returns ACPID when set",
			model: Model{ID: "claude-opus-4-6", ACPID: "default"},
			want:  "default",
		},
		{
			name:  "falls back to ID when ACPID empty",
			model: Model{ID: "gpt-5-1"},
			want:  "gpt-5-1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.model.ACPModelID(); got != tt.want {
				t.Errorf("ACPModelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveACPModelID(t *testing.T) {
	models := []Model{
		{ID: "claude-sonnet-4-6", ACPID: "sonnet"},
		{ID: "claude-opus-4-6", ACPID: "default"},
		{ID: "claude-haiku-4-5", ACPID: "haiku"},
		{ID: "gpt-5-1"}, // no ACPID
	}

	tests := []struct {
		name         string
		profileModel string
		want         string
	}{
		{
			name:         "resolves to ACPID when mapping exists",
			profileModel: "claude-opus-4-6",
			want:         "default",
		},
		{
			name:         "resolves sonnet mapping",
			profileModel: "claude-sonnet-4-6",
			want:         "sonnet",
		},
		{
			name:         "returns original ID when no ACPID",
			profileModel: "gpt-5-1",
			want:         "gpt-5-1",
		},
		{
			name:         "returns original ID when model not in list",
			profileModel: "unknown-model",
			want:         "unknown-model",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ResolveACPModelID(models, tt.profileModel); got != tt.want {
				t.Errorf("ResolveACPModelID() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveACPModelID_EmptyList(t *testing.T) {
	got := ResolveACPModelID(nil, "claude-opus-4-6")
	if got != "claude-opus-4-6" {
		t.Errorf("ResolveACPModelID(nil) = %q, want %q", got, "claude-opus-4-6")
	}
}

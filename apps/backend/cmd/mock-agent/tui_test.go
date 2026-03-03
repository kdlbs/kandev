package main

import "testing"

func TestParsePromptFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "no flag returns empty",
			args: []string{"mock-agent"},
			want: "",
		},
		{
			name: "separate flag and value",
			args: []string{"mock-agent", "--prompt", "fix the bug"},
			want: "fix the bug",
		},
		{
			name: "equals syntax",
			args: []string{"mock-agent", "--prompt=fix the bug"},
			want: "fix the bug",
		},
		{
			name: "prompt as last arg without value",
			args: []string{"mock-agent", "--prompt"},
			want: "",
		},
		{
			name: "prompt with other flags before",
			args: []string{"mock-agent", "--model", "mock-fast", "--prompt", "hello world"},
			want: "hello world",
		},
		{
			name: "prompt with other flags after",
			args: []string{"mock-agent", "--prompt", "hello world", "--tui"},
			want: "hello world",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parsePromptFromArgs(tt.args)
			if got != tt.want {
				t.Errorf("parsePromptFromArgs(%v) = %q, want %q", tt.args, got, tt.want)
			}
		})
	}
}

func TestTuiDelay(t *testing.T) {
	tests := []struct {
		model string
		want  int
	}{
		{"mock-fast", 100},
		{"mock-slow", 2000},
		{"mock-default", 500},
		{"unknown", 500},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			got := tuiDelay(tt.model)
			if got != tt.want {
				t.Errorf("tuiDelay(%q) = %d, want %d", tt.model, got, tt.want)
			}
		})
	}
}

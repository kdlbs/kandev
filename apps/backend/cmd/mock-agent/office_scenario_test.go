package main

import "testing"

func TestParseOfficeScenarioDirective(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
		ok     bool
	}{
		{
			name:   "directive line inside an assembled Office prompt",
			prompt: "You are the CEO.\n\n## Instructions\ne2e:office_scenario(permission-flow)\n\nYou have been woken.",
			want:   "permission-flow",
			ok:     true,
		},
		{name: "unknown scenario is ignored", prompt: "e2e:office_scenario(no-such-scenario)"},
		{name: "directive must start its line", prompt: "say e2e:office_scenario(permission-flow) please"},
		{name: "no directive", prompt: "You have been woken for reason: heartbeat."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseOfficeScenarioDirective(tt.prompt)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("parseOfficeScenarioDirective() = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

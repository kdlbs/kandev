package constants

import (
	"math"
	"testing"
	"time"
)

func TestDefaultSetupScriptTimeoutIsTenMinutes(t *testing.T) {
	if got := resolveSetupScriptTimeout(""); got != 10*time.Minute {
		t.Fatalf("resolveSetupScriptTimeout(\"\") = %s, want 10m", got)
	}
}

func TestResolveSetupScriptTimeout(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "missing", want: 10 * time.Minute},
		{name: "valid", value: "15m", want: 15 * time.Minute},
		{name: "malformed", value: "fifteen minutes", want: 10 * time.Minute},
		{name: "zero", value: "0s", want: 10 * time.Minute},
		{name: "negative", value: "-1m", want: 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSetupScriptTimeout(tt.value); got != tt.want {
				t.Fatalf("resolveSetupScriptTimeout(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestAgentLaunchTimeoutIncludesSetupAllowance(t *testing.T) {
	want := SetupScriptTimeout + 5*time.Minute
	if AgentLaunchTimeout != want {
		t.Fatalf("AgentLaunchTimeout = %s, want %s", AgentLaunchTimeout, want)
	}
}

func TestResolveSetupScriptTimeoutRejectsLaunchBudgetOverflow(t *testing.T) {
	maxSafe := time.Duration(math.MaxInt64) - 5*time.Minute
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{name: "maximum safe value", value: maxSafe.String(), want: maxSafe},
		{name: "launch budget overflow", value: (maxSafe + time.Nanosecond).String(), want: 10 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveSetupScriptTimeout(tt.value); got != tt.want {
				t.Fatalf("resolveSetupScriptTimeout(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestApplyPreparationTimeoutUsesTypedStartupValue(t *testing.T) {
	previousSetup := SetupScriptTimeout
	previousLaunch := AgentLaunchTimeout
	t.Cleanup(func() {
		SetupScriptTimeout = previousSetup
		AgentLaunchTimeout = previousLaunch
	})

	ApplyPreparationTimeout(17 * time.Minute)
	if SetupScriptTimeout != 17*time.Minute {
		t.Fatalf("SetupScriptTimeout = %s, want 17m", SetupScriptTimeout)
	}
	if AgentLaunchTimeout != 22*time.Minute {
		t.Fatalf("AgentLaunchTimeout = %s, want 22m", AgentLaunchTimeout)
	}
}

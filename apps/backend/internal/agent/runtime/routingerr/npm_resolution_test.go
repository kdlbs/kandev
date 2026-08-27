package routingerr

import (
	"strings"
	"testing"
)

func TestManagedRuntimeNpmResolutionRequiresBoundedETargetEvidence(t *testing.T) {
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{
			name:  "new npm prefix",
			input: "npm error code ETARGET\nnpm error notarget No matching version found for @kandev/agent@1.2.3.",
			match: true,
		},
		{
			name:  "old npm prefix",
			input: "npm ERR! code ETARGET\nnpm ERR! notarget No matching version found for @kandev/agent@1.2.3.",
			match: true,
		},
		{
			name:  "generic disconnect",
			input: "peer disconnected while starting the agent",
		},
		{
			name:  "unrelated npm failure",
			input: "npm error code EAI_AGAIN\nnpm error network request failed",
		},
		{
			name:  "missing matching version line",
			input: "npm error code ETARGET\nnpm error notarget No matching version found for another-package.",
		},
		{
			name: "evidence beyond bounded sample",
			input: "npm error code ETARGET\n" + strings.Repeat("filler\n", MaxRawExcerptBytes) +
				"\nnpm error notarget No matching version found for @kandev/agent@1.2.3.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := matchRuntimeEnvironmentRules(Sanitize(tt.input))
			managedMatch := ok && got.Code == CodeManagedRuntimeNpmResolution
			if managedMatch != tt.match {
				t.Fatalf("managed match = %v, want %v; error = %#v", managedMatch, tt.match, got)
			}
			if tt.match && got.Code != CodeManagedRuntimeNpmResolution {
				t.Fatalf("code = %q, want %q", got.Code, CodeManagedRuntimeNpmResolution)
			}
		})
	}
}

func TestManagedRuntimeNpmResolutionSanitizesDetails(t *testing.T) {
	input := "npm error code ETARGET\n" +
		"npm error notarget No matching version found for @kandev/agent@1.2.3.\n" +
		"token=super-secret-token-value"

	classified := Classify(Input{Phase: PhaseSessionInit, Stderr: input})
	if classified.Code != CodeManagedRuntimeNpmResolution {
		t.Fatalf("code = %q, want %q", classified.Code, CodeManagedRuntimeNpmResolution)
	}
	if len(classified.RawExcerpt) > MaxRawExcerptBytes {
		t.Fatalf("excerpt length = %d, want <= %d", len(classified.RawExcerpt), MaxRawExcerptBytes)
	}
	if strings.Contains(classified.RawExcerpt, "super-secret-token-value") {
		t.Fatalf("raw excerpt contains an unsanitized secret: %q", classified.RawExcerpt)
	}
}

func TestManagedRuntimeNpmResolutionMatchesExactPackage(t *testing.T) {
	const packageSpec = "@kandev/agent@1.2.3"
	tests := []struct {
		name  string
		input string
		match bool
	}{
		{
			name: "exact selected package",
			input: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for @kandev/agent@1.2.3.",
			match: true,
		},
		{
			name: "transitive dependency",
			input: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for dependency@9.9.9.",
		},
		{
			name: "nearby version is not exact",
			input: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for @kandev/agent@1.2.30.",
		},
		{
			name:  "diagnostic without ETARGET code",
			input: "npm error notarget No matching version found for @kandev/agent@1.2.3.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ManagedRuntimeNpmResolutionMatchesPackage(tt.input, packageSpec); got != tt.match {
				t.Fatalf("ManagedRuntimeNpmResolutionMatchesPackage() = %v, want %v", got, tt.match)
			}
		})
	}
}

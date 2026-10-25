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

func TestManagedRuntimeReleaseAgePolicyHasDistinctClassification(t *testing.T) {
	const packageSpec = "@agentclientprotocol/claude-agent-acp@0.81.0"
	input := "npm error code ETARGET\n" +
		"npm error notarget No matching version found for " + packageSpec + " with a date before 9/22/2026, 12:28:47 PM."
	classified := Classify(Input{
		Phase:                     PhaseSessionInit,
		Stderr:                    input,
		ManagedRuntimePackageSpec: packageSpec,
	})
	if got, want := classified.Code, Code("managed_runtime_npm_policy"); got != want {
		t.Fatalf("code = %q, want distinct release-age policy code %q", got, want)
	}
	if !strings.Contains(classified.RawExcerpt, "<release-date>") || strings.Contains(classified.RawExcerpt, "9/22/2026") || strings.Contains(classified.RawExcerpt, packageSpec) {
		t.Fatalf("excerpt = %q, want bounded canonical details without raw date or package", classified.RawExcerpt)
	}
}

func TestManagedRuntimeReleaseAgePolicyRequiresExactTrustedPackage(t *testing.T) {
	input := "npm error code ETARGET\n" +
		"npm error notarget No matching version found for dependency@0.81.0 with a date before 9/22/2026, 12:28:47 PM."
	classified := Classify(Input{
		Phase:                     PhaseSessionInit,
		Stderr:                    input,
		ManagedRuntimePackageSpec: "@agentclientprotocol/claude-agent-acp@0.81.0",
	})
	if classified.Code == CodeManagedRuntimeNpmPolicy {
		t.Fatalf("unrelated package was classified as a managed runtime policy failure: %#v", classified)
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

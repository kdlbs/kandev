package npmresolution

import "testing"

func TestMatchesReleaseAgePolicyForExactTopLevelPackage(t *testing.T) {
	const packageSpec = "@agentclientprotocol/claude-agent-acp@0.81.0"
	tests := []struct {
		name   string
		stderr string
		match  bool
	}{
		{
			name: "RFC3339 date-qualified diagnostic",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec +
				" with a date before 2026-09-24T11:00:00.000Z.",
			match: true,
		},
		{
			name: "npm 11.16 locale-formatted diagnostic from issue 3902",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for @agentclientprotocol/claude-agent-acp@0.81.0 with a date before 9/22/2026, 12:28:47 PM.",
			match: true,
		},
		{
			name: "canonical agentctl marker",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec +
				" with a date before " + ReleaseDateMarker,
			match: true,
		},
		{
			name: "legacy npm prefix",
			stderr: "npm ERR! code ETARGET\n" +
				"npm ERR! notarget No matching version found for " + packageSpec +
				" with a date before 2026-09-24T11:00:00Z.",
			match: true,
		},
		{
			name: "ordinary exact-package ETARGET",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec + ".",
		},
		{
			name: "transitive package",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for dependency@9.9.9 with a date before 2026-09-24T11:00:00Z.",
		},
		{
			name: "missing ETARGET code",
			stderr: "npm error notarget No matching version found for " + packageSpec +
				" with a date before 2026-09-24T11:00:00Z.",
		},
		{
			name: "invalid date",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec +
				" with a date before 2026-02-30T11:00:00Z.",
		},
		{
			name: "malformed date text",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec + " with a date before yesterday.",
		},
		{
			name: "invalid locale-formatted calendar date",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec + " with a date before 9/31/2026, 12:28:47 PM.",
		},
		{
			name: "malformed locale-formatted time",
			stderr: "npm error code ETARGET\n" +
				"npm error notarget No matching version found for " + packageSpec + " with a date before 9/22/2026, 12:88:47 PM.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchesReleaseAgePolicy(tt.stderr, packageSpec); got != tt.match {
				t.Fatalf("MatchesReleaseAgePolicy() = %v, want %v", got, tt.match)
			}
		})
	}
}

func TestReleaseAgePolicyRemainsDistinctFromOrdinaryExactETarget(t *testing.T) {
	const packageSpec = "managed-acp@1.2.3"
	policy := "npm error code ETARGET\n" +
		"npm error notarget No matching version found for " + packageSpec +
		" with a date before 2026-09-24T11:00:00Z."
	if MatchesExactPackage(policy, packageSpec) {
		t.Fatal("date-qualified ETARGET matched ordinary stale-metadata recovery")
	}
	if !ContainsReleaseAgePolicy(policy) {
		t.Fatal("date-qualified ETARGET was not detected")
	}
}

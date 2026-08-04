package models

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRemoteContributionRoundTripsAndPreservesUnrelatedMetadata(t *testing.T) {
	binding := RemoteContribution{
		Version:      1,
		Provider:     RemoteContributionProviderGitHub,
		Kind:         RemoteContributionKindPullRequest,
		CanonicalURL: "https://github.com/acme/widget/pull/123",
		Number:       123,
		State:        RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "fix/widget",
		HeadSHA:      "0123456789abcdef0123456789abcdef01234567",
		SourceRepository: RemoteContributionRepository{
			Host:       "github.com",
			Path:       "contributor/widget",
			ProviderID: "42",
			RemoteURL:  "https://github.com/contributor/widget.git",
		},
		CollaborationAllowed: true,
	}
	metadata := map[string]interface{}{"unrelated": "value"}
	if err := PutRemoteContribution(metadata, &binding); err != nil {
		t.Fatalf("PutRemoteContribution: %v", err)
	}

	got, ok, err := LoadRemoteContribution(metadata)
	if err != nil {
		t.Fatalf("LoadRemoteContribution: %v", err)
	}
	if !ok {
		t.Fatal("LoadRemoteContribution reported no binding")
	}
	if got != binding {
		t.Fatalf("round trip mismatch: %#v != %#v", got, binding)
	}
	if metadata["unrelated"] != "value" {
		t.Fatalf("unrelated metadata changed: %#v", metadata)
	}
}

func TestRemoteContributionRejectsUnsafeOrCredentialBearingValues(t *testing.T) {
	base := RemoteContribution{
		Version:      1,
		Provider:     RemoteContributionProviderGitLab,
		Kind:         RemoteContributionKindMergeRequest,
		CanonicalURL: "https://gitlab.example.test/group/widget/-/merge_requests/7",
		Number:       7,
		State:        RemoteContributionStateOpen,
		BaseBranch:   "main",
		HeadBranch:   "feature/fix",
		HeadSHA:      "0123456789abcdef0123456789abcdef01234567",
		SourceRepository: RemoteContributionRepository{
			Host:      "gitlab.example.test",
			Path:      "group/widget",
			RemoteURL: "https://gitlab.example.test/group/widget.git",
		},
		CollaborationAllowed: true,
	}
	cases := []struct {
		name   string
		mutate func(*RemoteContribution)
		want   string
	}{
		{name: "unsafe head ref", mutate: func(v *RemoteContribution) { v.HeadBranch = "feature/../escape" }, want: "head_branch"},
		{name: "unsafe base ref", mutate: func(v *RemoteContribution) { v.BaseBranch = "-main" }, want: "base_branch"},
		{name: "credential in source URL", mutate: func(v *RemoteContribution) {
			v.SourceRepository.RemoteURL = "https://user:secret@gitlab.example.test/group/widget.git"
		}, want: "credentials"},
		{name: "unknown version", mutate: func(v *RemoteContribution) { v.Version = 99 }, want: "version"},
		{name: "canonical host mismatch", mutate: func(v *RemoteContribution) {
			v.CanonicalURL = "https://other.example.test/group/widget/-/merge_requests/7"
		}, want: "host"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			candidate := base
			tc.mutate(&candidate)
			if err := candidate.Validate(); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Validate() error = %v, want substring %q", err, tc.want)
			}
		})
	}
}

func TestLoadRemoteContributionRejectsMalformedJSON(t *testing.T) {
	metadata := map[string]interface{}{RemoteContributionMetadataKey: json.RawMessage(`{"version":1}`)}
	if _, ok, err := LoadRemoteContribution(metadata); err == nil || ok {
		t.Fatalf("LoadRemoteContribution() = (%v, %v, %v), want error and no binding", ok, err, ok)
	}
}

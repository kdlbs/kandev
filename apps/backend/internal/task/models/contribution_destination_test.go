package models

import (
	"strings"
	"testing"
)

func testContributionDestination() ContributionDestination {
	return ContributionDestination{
		Version:  ContributionDestinationVersion,
		Provider: ContributionDestinationProviderGitHub,
		SourceRepository: RemoteContributionRepository{
			Host:       "github.com",
			Path:       "kdlbs/kandev",
			ProviderID: "100",
			RemoteURL:  "https://github.com/kdlbs/kandev.git",
		},
		TargetRepository: RemoteContributionRepository{
			Host:       "github.com",
			Path:       "alice/kandev",
			ProviderID: "200",
			RemoteURL:  "https://github.com/alice/kandev.git",
		},
	}
}

func TestContributionDestinationRoundTripsThroughMetadata(t *testing.T) {
	destination := testContributionDestination()
	metadata := map[string]interface{}{"unrelated": "value"}
	if err := PutContributionDestination(metadata, &destination); err != nil {
		t.Fatalf("PutContributionDestination: %v", err)
	}

	got, ok, err := LoadContributionDestination(metadata)
	if err != nil {
		t.Fatalf("LoadContributionDestination: %v", err)
	}
	if !ok {
		t.Fatal("LoadContributionDestination reported no destination")
	}
	if got != destination {
		t.Fatalf("round trip mismatch: %#v != %#v", got, destination)
	}
	if metadata["unrelated"] != "value" {
		t.Fatalf("unrelated metadata changed: %#v", metadata)
	}
}

func TestContributionDestinationCredentialBindingValidation(t *testing.T) {
	cases := []struct {
		name    string
		binding ContributionDestinationCredentialBinding
		valid   bool
	}{
		{
			name: "pat",
			binding: ContributionDestinationCredentialBinding{
				Source: "pat", Login: "alice", CredentialGeneration: 3,
			},
			valid: true,
		},
		{
			name: "app installation",
			binding: ContributionDestinationCredentialBinding{
				Source: "github_app_installation", InstallationID: 42, AppRegistrationID: "app-1",
				CredentialGeneration: 3, AppCredentialGeneration: 7,
			},
			valid: true,
		},
		{
			name: "unknown source",
			binding: ContributionDestinationCredentialBinding{
				Source: "ambient", Login: "alice", CredentialGeneration: 3,
			},
		},
		{
			name:    "missing generation",
			binding: ContributionDestinationCredentialBinding{Source: "pat", Login: "alice"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.binding.Validate()
			if tc.valid && err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if !tc.valid && err == nil {
				t.Fatal("Validate() accepted an invalid binding")
			}
		})
	}
}

func TestContributionDestinationRejectsUnsafeOrAmbiguousValues(t *testing.T) {
	base := testContributionDestination()
	cases := []struct {
		name   string
		mutate func(*ContributionDestination)
		want   string
	}{
		{
			name: "credential in target URL",
			mutate: func(value *ContributionDestination) {
				value.TargetRepository.RemoteURL = "https://alice:secret@github.com/alice/kandev.git"
			},
			want: "credentials",
		},
		{
			name: "target path does not match target URL",
			mutate: func(value *ContributionDestination) {
				value.TargetRepository.Path = "other/kandev"
			},
			want: "path",
		},
		{
			name: "target is not a GitHub repository",
			mutate: func(value *ContributionDestination) {
				value.TargetRepository.Path = "alice/team/kandev"
				value.TargetRepository.RemoteURL = "https://github.com/alice/team/kandev.git"
			},
			want: "repository identity",
		},
		{
			name: "source and target alias",
			mutate: func(value *ContributionDestination) {
				value.TargetRepository.Path = value.SourceRepository.Path
				value.TargetRepository.RemoteURL = value.SourceRepository.RemoteURL
			},
			want: "different repositories",
		},
		{
			name: "unknown version",
			mutate: func(value *ContributionDestination) {
				value.Version = 99
			},
			want: "version",
		},
		{
			name: "unsupported provider",
			mutate: func(value *ContributionDestination) {
				value.Provider = "gitlab"
			},
			want: "unsupported",
		},
		{
			name: "missing provider identity",
			mutate: func(value *ContributionDestination) {
				value.TargetRepository.ProviderID = ""
			},
			want: "provider_id",
		},
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

func TestContributionDestinationRemoteNameIsStableAndTargetSpecific(t *testing.T) {
	first := testContributionDestination()
	second := first
	second.TargetRepository.Path = "bob/kandev"
	second.TargetRepository.RemoteURL = "https://github.com/bob/kandev.git"
	second.TargetRepository.ProviderID = "300"

	if first.ContributionDestinationRemoteName() != first.ContributionRemoteName() {
		t.Fatal("destination remote name aliases are inconsistent")
	}
	if first.ContributionRemoteName() == second.ContributionRemoteName() {
		t.Fatal("different target repositories share a contribution remote name")
	}
}

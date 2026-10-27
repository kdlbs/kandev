package models

import (
	"strings"
	"testing"
)

func TestEnterpriseContributionDestinations(t *testing.T) {
	for _, host := range []string{"git.example.com:8443", "tenant.ghe.com"} {
		destination := testContributionDestination()
		destination.SourceRepository.Host = host
		destination.TargetRepository.Host = host
		destination.SourceRepository.RemoteURL = strings.ReplaceAll(destination.SourceRepository.RemoteURL, "github.com", host)
		destination.TargetRepository.RemoteURL = strings.ReplaceAll(destination.TargetRepository.RemoteURL, "github.com", host)
		if err := destination.Validate(); err != nil {
			t.Fatal(err)
		}
	}
}
func TestEnterpriseRemoteContribution(t *testing.T) {
	binding := RemoteContribution{
		Version: 1, Provider: RemoteContributionProviderGitHub, Kind: RemoteContributionKindPullRequest,
		CanonicalURL: "https://git.example.com/acme/widget/pull/7", Number: 7, State: RemoteContributionStateOpen,
		BaseBranch: "main", HeadBranch: "feature", HeadSHA: strings.Repeat("a", 40), CollaborationAllowed: true,
		SourceRepository: RemoteContributionRepository{Host: "git.example.com", Path: "fork/widget", RemoteURL: "https://git.example.com/fork/widget.git"},
	}
	if err := binding.Validate(); err != nil {
		t.Fatal(err)
	}
}
func TestEnterpriseComparisonTarget(t *testing.T) {
	target := testComparisonTarget()
	target.HeadRepository.Host = "git.example.com"
	target.TargetRepository.Host = "git.example.com"
	target.HeadRepository.RemoteURL = strings.ReplaceAll(target.HeadRepository.RemoteURL, "github.com", "git.example.com")
	target.TargetRepository.RemoteURL = strings.ReplaceAll(target.TargetRepository.RemoteURL, "github.com", "git.example.com")
	if err := target.Validate(); err != nil {
		t.Fatal(err)
	}
}

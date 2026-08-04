package github

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseGitHubContributionURLRejectsAmbiguousURLs(t *testing.T) {
	for _, raw := range []string{
		"https://github.com/acme/widget/pull/7?diff=1",
		"https://github.com/acme/widget/pull/7#discussion",
		"https://github.com/acme/widget/pull/0",
		"https://github.com/acme/widget/issues/7",
		"https://user:pass@github.com/acme/widget/pull/7",
	} {
		if _, _, _, err := parseGitHubContributionURL(raw); err == nil {
			t.Errorf("parseGitHubContributionURL(%q) returned nil error", raw)
		}
	}
}

func TestBuildGitHubRemoteContributionUsesValidatedForkIdentity(t *testing.T) {
	pr := &PR{
		Number:              7,
		State:               "open",
		RepoOwner:           "acme",
		RepoName:            "widget",
		HeadRepoOwner:       "contributor",
		HeadRepoName:        "widget-fork",
		HeadRepoID:          42,
		BaseRepoID:          99,
		BaseRepoOwner:       "acme",
		BaseRepoName:        "widget",
		BaseDefaultBranch:   "main",
		HeadBranch:          "feature/remote",
		HeadSHA:             strings.Repeat("a", 40),
		BaseBranch:          "main",
		MaintainerCanModify: true,
		Title:               "untrusted title must not persist",
		Body:                "untrusted body must not persist",
	}
	resolution, err := buildGitHubRemoteContribution("https://github.com/acme/widget/pull/7", "acme", "widget", 7, pr)
	if err != nil {
		t.Fatalf("buildGitHubRemoteContribution() error = %v", err)
	}
	if got := resolution.Binding.SourceRepository.Path; got != "contributor/widget-fork" {
		t.Fatalf("source path = %q, want contributor/widget-fork", got)
	}
	if resolution.TargetDefaultBranch != "main" || resolution.TargetProviderID != "99" {
		t.Fatalf("target identity = (%q, %q), want (main, 99)", resolution.TargetDefaultBranch, resolution.TargetProviderID)
	}
	data, err := json.Marshal(resolution.Binding)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "untrusted title") || strings.Contains(string(data), "untrusted body") {
		t.Fatalf("provider-authored content leaked into binding: %s", data)
	}
}

func TestBuildGitHubRemoteContributionRejectsNonEditableFork(t *testing.T) {
	pr := &PR{
		Number:        7,
		State:         "open",
		RepoOwner:     "acme",
		RepoName:      "widget",
		HeadRepoOwner: "contributor",
		HeadRepoName:  "widget-fork",
		BaseRepoOwner: "acme",
		BaseRepoName:  "widget",
		HeadBranch:    "feature/remote",
		HeadSHA:       strings.Repeat("a", 40),
		BaseBranch:    "main",
	}
	if _, err := buildGitHubRemoteContribution("https://github.com/acme/widget/pull/7", "acme", "widget", 7, pr); err == nil {
		t.Fatal("buildGitHubRemoteContribution() accepted a non-editable fork")
	}
}

func TestBuildGitHubRemoteContributionRejectsMissingRepositoryIdentity(t *testing.T) {
	pr := &PR{
		Number:              7,
		State:               "open",
		RepoOwner:           "acme",
		RepoName:            "widget",
		HeadBranch:          "feature/remote",
		HeadSHA:             strings.Repeat("a", 40),
		BaseBranch:          "main",
		MaintainerCanModify: true,
	}
	if _, err := buildGitHubRemoteContribution("https://github.com/acme/widget/pull/7", "acme", "widget", 7, pr); err == nil {
		t.Fatal("buildGitHubRemoteContribution() accepted a response without live repository identity")
	}
}

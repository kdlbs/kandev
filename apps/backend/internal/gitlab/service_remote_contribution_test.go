package gitlab

import (
	"strings"
	"testing"
)

func TestParseGitLabContributionURLRejectsQueryAndWrongHost(t *testing.T) {
	for _, raw := range []string{
		"https://gitlab.example.com/group/project/-/merge_requests/4?view=diff",
		"https://gitlab.example.com/group/project/-/merge_requests/4#note",
		"https://other.example.com/group/project/-/merge_requests/4",
	} {
		if _, _, err := parseGitLabContributionURL(raw, "https://gitlab.example.com"); err == nil {
			t.Errorf("parseGitLabContributionURL(%q) returned nil error", raw)
		}
	}
}

func TestBuildGitLabRemoteContributionUsesForkIdentity(t *testing.T) {
	mr := &MR{
		IID:                 4,
		ProjectID:           99,
		TargetProjectID:     99,
		ProjectPath:         "group/project",
		TargetProjectPath:   "group/project",
		SourceProjectID:     42,
		SourceProjectPath:   "contributor/project",
		TargetDefaultBranch: "main",
		State:               "open",
		HeadBranch:          "feature/remote",
		HeadSHA:             strings.Repeat("b", 40),
		BaseBranch:          "main",
		AllowCollaboration:  true,
		Title:               "untrusted title must not persist",
		Body:                "untrusted body must not persist",
	}
	resolution, err := buildGitLabRemoteContribution("https://gitlab.example.com/group/project/-/merge_requests/4", "https://gitlab.example.com", "group/project", 4, mr)
	if err != nil {
		t.Fatalf("buildGitLabRemoteContribution() error = %v", err)
	}
	if resolution.Binding.SourceRepository.Path != "contributor/project" || resolution.TargetProviderID != "99" {
		t.Fatalf("resolved identity = (%q, %q), want (contributor/project, 99)", resolution.Binding.SourceRepository.Path, resolution.TargetProviderID)
	}
	if strings.Contains(resolution.Binding.CanonicalURL, "untrusted") {
		t.Fatal("provider-authored content leaked into canonical URL")
	}
}

func TestBuildGitLabRemoteContributionAcceptsCustomPortHost(t *testing.T) {
	mr := &MR{
		IID:                 4,
		ProjectID:           99,
		TargetProjectID:     99,
		ProjectPath:         "group/project",
		TargetProjectPath:   "group/project",
		SourceProjectID:     99,
		SourceProjectPath:   "group/project",
		TargetDefaultBranch: "main",
		State:               "open",
		HeadBranch:          "feature/remote",
		HeadSHA:             strings.Repeat("b", 40),
		BaseBranch:          "main",
	}
	resolution, err := buildGitLabRemoteContribution(
		"https://gitlab.example.com:8443/group/project/-/merge_requests/4",
		"https://gitlab.example.com:8443",
		"group/project",
		4,
		mr,
	)
	if err != nil {
		t.Fatalf("buildGitLabRemoteContribution() error = %v", err)
	}
	if got := resolution.Binding.SourceRepository.Host; got != "gitlab.example.com:8443" {
		t.Fatalf("source host = %q, want custom-port authority", got)
	}
	if got := resolution.Binding.SourceRepository.RemoteURL; got != "https://gitlab.example.com:8443/group/project.git" {
		t.Fatalf("source URL = %q, want custom-port URL", got)
	}
}

func TestBuildGitLabRemoteContributionRejectsNonEditableFork(t *testing.T) {
	mr := &MR{
		IID:               4,
		ProjectPath:       "group/project",
		TargetProjectPath: "group/project",
		SourceProjectPath: "contributor/project",
		State:             "open",
		HeadBranch:        "feature/remote",
		HeadSHA:           strings.Repeat("b", 40),
		BaseBranch:        "main",
	}
	if _, err := buildGitLabRemoteContribution("https://gitlab.example.com/group/project/-/merge_requests/4", "https://gitlab.example.com", "group/project", 4, mr); err == nil {
		t.Fatal("buildGitLabRemoteContribution() accepted a non-editable fork")
	}
}

func TestBuildGitLabRemoteContributionRejectsForkWithoutHydratedSourceIdentity(t *testing.T) {
	mr := &MR{
		IID:                4,
		ProjectID:          99,
		TargetProjectID:    99,
		ProjectPath:        "group/project",
		TargetProjectPath:  "group/project",
		SourceProjectID:    42,
		State:              "open",
		HeadBranch:         "feature/remote",
		HeadSHA:            strings.Repeat("b", 40),
		BaseBranch:         "main",
		AllowCollaboration: true,
	}
	if _, err := buildGitLabRemoteContribution("https://gitlab.example.com/group/project/-/merge_requests/4", "https://gitlab.example.com", "group/project", 4, mr); err == nil {
		t.Fatal("buildGitLabRemoteContribution() treated a fork with only IDs as same-project")
	}
}

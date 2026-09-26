package gitlab

import "testing"

func TestGitLabComparisonTargetCandidateUsesExplicitForkAndBaseProjects(t *testing.T) {
	candidate, ok := gitLabComparisonTargetCandidate("https://gitlab.example.com", &MR{
		IID:               23,
		HeadBranch:        "feature/fork",
		BaseBranch:        "main",
		SourceProjectID:   42,
		SourceProjectPath: "contributor/widget-fork",
		TargetProjectID:   99,
		TargetProjectPath: "upstream/widget",
	})
	if !ok {
		t.Fatal("gitLabComparisonTargetCandidate() rejected a complete MR identity")
	}
	if got := candidate.HeadRepository.Path; got != "contributor/widget-fork" {
		t.Fatalf("head repository = %q, want contributor/widget-fork", got)
	}
	if got := candidate.TargetRepository.Path; got != "upstream/widget" {
		t.Fatalf("target repository = %q, want upstream/widget", got)
	}
	if candidate.HeadRepository.ProviderID != "42" || candidate.TargetRepository.ProviderID != "99" {
		t.Fatalf("project IDs = (%q, %q), want (42, 99)", candidate.HeadRepository.ProviderID, candidate.TargetRepository.ProviderID)
	}
}

func TestGitLabComparisonTargetCandidateRejectsMissingSourceProjectPath(t *testing.T) {
	if _, ok := gitLabComparisonTargetCandidate("https://gitlab.example.com", &MR{
		IID:               23,
		HeadBranch:        "feature/fork",
		BaseBranch:        "main",
		SourceProjectID:   42,
		TargetProjectID:   99,
		TargetProjectPath: "upstream/widget",
	}); ok {
		t.Fatal("gitLabComparisonTargetCandidate() accepted an MR without explicit source identity")
	}
}

package handlers

import "testing"

func TestValidateTaskChangeLinkRequiresProviderRepositoryAndNumber(t *testing.T) {
	valid, err := validateTaskChangeLink("GitLab", "repo-1", 42)
	if err != nil || valid.Provider != "gitlab" {
		t.Fatalf("validateTaskChangeLink() = %#v, %v", valid, err)
	}
	for _, input := range []TaskChangeLink{
		{Provider: "", RepositoryID: "repo-1", Number: 1},
		{Provider: "github", RepositoryID: "", Number: 1},
		{Provider: "github", RepositoryID: "repo-1", Number: 0},
		{Provider: "other", RepositoryID: "repo-1", Number: 1},
	} {
		if _, err := validateTaskChangeLink(input.Provider, input.RepositoryID, input.Number); err == nil {
			t.Fatalf("validateTaskChangeLink(%#v) succeeded", input)
		}
	}
}

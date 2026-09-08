package backendapp

import "testing"

func TestTaskChangeURLsRequireCanonicalRepositoryIdentity(t *testing.T) {
	githubURL, err := githubChangeURL("https://github.com", "acme", "api", 7)
	if err != nil || githubURL != "https://github.com/acme/api/pull/7" {
		t.Fatalf("githubChangeURL() = %q, %v", githubURL, err)
	}
	gitlabURL, err := gitlabChangeURL("https://gitlab.example.test/", "group", "api", 9)
	if err != nil || gitlabURL != "https://gitlab.example.test/group/api/-/merge_requests/9" {
		t.Fatalf("gitlabChangeURL() = %q, %v", gitlabURL, err)
	}
	if _, err := githubChangeURL("https://gitlab.example.test", "acme", "api", 7); err == nil {
		t.Fatal("githubChangeURL accepted a non-GitHub host")
	}
	if _, err := gitlabChangeURL("", "group", "api", 9); err == nil {
		t.Fatal("gitlabChangeURL accepted an incomplete repository identity")
	}
}

package github

import (
	"context"
	"testing"
)

// @covers AC-OFFICE-AUTOMATIONS-PR-FOLLOW-UP-002.1
func TestListOpenPRsPaginatesAndCopiesFilterFields(t *testing.T) {
	pages := []string{
		`[{"number":1,"title":"one","state":"open","draft":false,
			"head":{"ref":"feat","sha":"aaa"},"base":{"ref":"main","sha":"b1"},
			"user":{"login":"alice"},"labels":[{"name":"needs-review"}],
			"requested_reviewers":[{"login":"Kandev-Bot"}],
			"requested_teams":[{"slug":"reviewers","name":"Reviewers"}]}]`,
		`[{"number":2,"title":"two","state":"open","draft":true,
			"head":{"ref":"fix","sha":"bbb"},"base":{"ref":"release/1","sha":"b2"},
			"user":{"login":"bob"},"labels":[]}]`,
	}
	client, requests := newLinkPaginatedPATServer(t, "/repos/acme/widget/pulls", pages)

	prs, complete, err := client.ListOpenPRs(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListOpenPRs: %v", err)
	}
	if !complete {
		t.Fatal("complete = false, want true after reading every page")
	}
	if len(*requests) != 2 {
		t.Fatalf("requests = %d, want 2 pages", len(*requests))
	}
	query := parseQueryValues(t, (*requests)[0].Query)
	if query.Get("state") != "open" || query.Get("per_page") != "100" {
		t.Fatalf("first page query = %q", (*requests)[0].Query)
	}
	if len(prs) != 2 {
		t.Fatalf("prs = %d, want 2", len(prs))
	}
	first := prs[0]
	if first.HeadSHA != "aaa" || first.BaseBranch != "main" || first.AuthorLogin != "alice" {
		t.Fatalf("first PR = %+v", first)
	}
	if len(first.Labels) != 1 || first.Labels[0] != "needs-review" {
		t.Fatalf("labels = %v", first.Labels)
	}
	if len(first.RequestedReviewers) != 2 ||
		first.RequestedReviewers[0] != (RequestedReviewer{Login: "Kandev-Bot", Type: reviewerTypeUser}) ||
		first.RequestedReviewers[1] != (RequestedReviewer{Login: "reviewers", Type: reviewerTypeTeam}) {
		t.Fatalf("requested reviewers = %+v", first.RequestedReviewers)
	}
	if first.RepoOwner != "acme" || first.RepoName != "widget" {
		t.Fatalf("repo = %s/%s", first.RepoOwner, first.RepoName)
	}
	if !prs[1].Draft || prs[1].BaseBranch != "release/1" || prs[1].HeadSHA != "bbb" {
		t.Fatalf("second PR = %+v", prs[1])
	}
}

func TestListOpenPRsReportsPartialListing(t *testing.T) {
	pages := []string{
		`[{"number":1,"state":"open","head":{"sha":"aaa"},"base":{"ref":"main"},"user":{"login":"a"}}]`,
	}
	// The served page advertises a next link only when more pages exist, so a
	// two-page listing whose second page is missing fails after page one.
	client, _ := newLinkPaginatedPATServer(t, "/repos/acme/widget/pulls", append(pages, "not json"))

	prs, complete, err := client.ListOpenPRs(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("partial listing must return what was read without an error: %v", err)
	}
	if complete {
		t.Fatal("complete = true after a failed page")
	}
	if len(prs) != 1 || prs[0].Number != 1 {
		t.Fatalf("prs = %+v, want the first page", prs)
	}
}

func TestListOpenPRsFirstPageErrorFails(t *testing.T) {
	client, _ := newLinkPaginatedPATServer(t, "/repos/acme/other/pulls", nil)

	if _, _, err := client.ListOpenPRs(context.Background(), "acme", "widget"); err == nil {
		t.Fatal("expected an error when the first page fails")
	}
}

func TestGHClientListOpenPRsCopiesFilterFields(t *testing.T) {
	calls := newFakeGH(t, ghResponse{
		Prefix: "pr list",
		Stdout: `[{"number":3,"title":"t","url":"https://github.com/acme/widget/pull/3","state":"OPEN",
			"headRefName":"feat","headRefOid":"ccc","baseRefName":"main","author":{"login":"carol"},
			"isDraft":false,"labels":[{"name":"needs-review"}],
			"reviewRequests":[{"__typename":"User","login":"kandev-bot"}]}]`,
	})

	prs, complete, err := NewGHClient().ListOpenPRs(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("ListOpenPRs: %v", err)
	}
	assertGHArgv(t, calls(t), 0, []string{
		"pr", "list", "--repo", "acme/widget", "--state", "open", "--limit", "1000",
		"--json", "number,title,url,state,body,headRefName,headRefOid,baseRefName,author,isDraft,labels,reviewRequests,createdAt,updatedAt",
	})
	if !complete || len(prs) != 1 {
		t.Fatalf("complete=%v prs=%d", complete, len(prs))
	}
	pr := prs[0]
	if pr.HeadSHA != "ccc" || pr.BaseBranch != "main" || len(pr.Labels) != 1 || pr.Labels[0] != "needs-review" {
		t.Fatalf("pr = %+v", pr)
	}
	if len(pr.RequestedReviewers) != 1 || pr.RequestedReviewers[0].Login != "kandev-bot" {
		t.Fatalf("requested reviewers = %+v", pr.RequestedReviewers)
	}
}

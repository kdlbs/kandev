package gitlab

import (
	"math"
	"testing"
)

func seedMilestoneIssues(t *testing.T, c *MockClient) {
	t.Helper()
	c.SeedIssue("acme/beta", &Issue{IID: 1, ProjectPath: "acme/beta", Title: "beta-1", Milestone: "Next"})
	c.SeedIssue("acme/alpha", &Issue{IID: 2, ProjectPath: "acme/alpha", Title: "alpha-2", Milestone: "Old"})
	c.SeedIssue("acme/alpha", &Issue{IID: 1, ProjectPath: "acme/alpha", Title: "alpha-1", Milestone: "Next"})
	c.SeedIssue("acme/alpha", &Issue{IID: 3, ProjectPath: "acme/alpha", Title: "alpha-3", Milestone: ""})
}

func TestMockClient_ListIssues_EmptyMilestoneReturnsEverything(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "", "", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
}

func TestMockClient_ListIssues_ParameterMilestoneFiltersExact(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "", "", "Next")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2: %#v", len(got), got)
	}
	// Deterministic order: ProjectPath asc, then IID asc.
	if got[0].ProjectPath != "acme/alpha" || got[0].IID != 1 {
		t.Errorf("got[0] = %+v, want acme/alpha#1", got[0])
	}
	if got[1].ProjectPath != "acme/beta" || got[1].IID != 1 {
		t.Errorf("got[1] = %+v, want acme/beta#1", got[1])
	}
}

func TestMockClient_ListIssues_DeterministicOrderAcrossAllIssues(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "", "", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	want := []struct {
		Project string
		IID     int
	}{
		{"acme/alpha", 1}, {"acme/alpha", 2}, {"acme/alpha", 3}, {"acme/beta", 1},
	}
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].ProjectPath != w.Project || got[i].IID != w.IID {
			t.Errorf("got[%d] = %s#%d, want %s#%d", i, got[i].ProjectPath, got[i].IID, w.Project, w.IID)
		}
	}
}

func TestMockClient_ListIssues_CustomQueryMilestoneWinsOverParameter(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "", "milestone=Old", "Next")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 1 || got[0].Milestone != "Old" {
		t.Fatalf("got = %#v, want exactly one Old-milestone issue", got)
	}
}

func TestMockClient_ListIssues_CustomQueryEmptyMilestoneKeySuppressesParameterFallback(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	// Rule 1: an empty milestone= key in customQuery wins outright (does not
	// fall through to the milestone parameter), matching the live-path fold.
	got, err := c.ListIssues(t.Context(), "", "milestone=", "Next")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (empty milestone key means unfiltered)", len(got))
	}
}

func TestMockClient_ListIssues_HandwrittenCustomQueryUsesFirstMilestoneValue(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "", "milestone=Old&milestone=Next", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 1 || got[0].Milestone != "Old" {
		t.Fatalf("got = %#v, want exactly one Old-milestone issue (first value wins)", got)
	}
}

func TestMockClient_ListIssues_FilterMilestoneFallbackWhenNonEmpty(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "milestone=Old", "", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 1 || got[0].Milestone != "Old" {
		t.Fatalf("got = %#v, want exactly one Old-milestone issue", got)
	}
}

func TestMockClient_ListIssues_FilterMilestoneEmptyValueIsIgnored(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	got, err := c.ListIssues(t.Context(), "milestone=", "", "")
	if err != nil {
		t.Fatalf("ListIssues() error = %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4 (empty filter milestone value is not adopted)", len(got))
	}
}

func TestMockClient_ListIssuesPaged_PagesOverFilteredSortedResult(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	page, err := c.ListIssuesPaged(t.Context(), "", "", "", 1, 2)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", page.TotalCount)
	}
	if len(page.Issues) != 2 || page.Issues[0].IID != 1 || page.Issues[1].IID != 2 {
		t.Fatalf("page 1 issues = %#v, want alpha#1, alpha#2", page.Issues)
	}

	page2, err := c.ListIssuesPaged(t.Context(), "", "", "", 2, 2)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page2.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", page2.TotalCount)
	}
	if len(page2.Issues) != 2 || page2.Issues[0].IID != 3 || page2.Issues[1].ProjectPath != "acme/beta" {
		t.Fatalf("page 2 issues = %#v, want alpha#3, beta#1", page2.Issues)
	}
}

func TestMockClient_ListIssuesPaged_PageBeyondEndYieldsEmptyWithSameTotal(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	page, err := c.ListIssuesPaged(t.Context(), "", "", "", 5, 2)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", page.TotalCount)
	}
	if len(page.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0", len(page.Issues))
	}
	if page.Page != 5 || page.PerPage != 2 {
		t.Errorf("Page/PerPage = %d/%d, want 5/2", page.Page, page.PerPage)
	}
}

// Scenario 16: 30 seeded issues, 26 in milestone "Next", per_page=25 pages
// the filtered set (not the unfiltered 30), with TotalCount staying at the
// filtered count on every page including the beyond-end one.
func TestMockClient_ListIssuesPaged_PagesOverFilteredMilestoneSetAcrossMultiplePages(t *testing.T) {
	c := NewMockClient(DefaultHost)
	for i := 1; i <= 26; i++ {
		c.SeedIssue("acme/repo", &Issue{IID: i, ProjectPath: "acme/repo", Milestone: "Next"})
	}
	for i := 27; i <= 30; i++ {
		c.SeedIssue("acme/repo", &Issue{IID: i, ProjectPath: "acme/repo", Milestone: "Old"})
	}

	page1, err := c.ListIssuesPaged(t.Context(), "", "", "Next", 1, 25)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page1.TotalCount != 26 {
		t.Errorf("page1 TotalCount = %d, want 26", page1.TotalCount)
	}
	if len(page1.Issues) != 25 {
		t.Errorf("page1 len(Issues) = %d, want 25", len(page1.Issues))
	}

	page2, err := c.ListIssuesPaged(t.Context(), "", "", "Next", 2, 25)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page2.TotalCount != 26 {
		t.Errorf("page2 TotalCount = %d, want 26", page2.TotalCount)
	}
	if len(page2.Issues) != 1 {
		t.Errorf("page2 len(Issues) = %d, want 1", len(page2.Issues))
	}

	page3, err := c.ListIssuesPaged(t.Context(), "", "", "Next", 3, 25)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page3.TotalCount != 26 {
		t.Errorf("page3 TotalCount = %d, want 26", page3.TotalCount)
	}
	if len(page3.Issues) != 0 {
		t.Errorf("page3 len(Issues) = %d, want 0", len(page3.Issues))
	}
}

func TestMockClient_ListIssuesPaged_NegativePageNormalisedToOne(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	page, err := c.ListIssuesPaged(t.Context(), "", "", "", -3, 2)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page.Page != 1 {
		t.Errorf("Page = %d, want normalised to 1", page.Page)
	}
	if len(page.Issues) != 2 || page.Issues[0].IID != 1 {
		t.Fatalf("issues = %#v, want the first page's worth", page.Issues)
	}
}

func TestMockClient_ListIssuesPaged_NonPositivePerPageReturnsNoIssues(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	page, err := c.ListIssuesPaged(t.Context(), "", "", "", 1, 0)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if len(page.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0", len(page.Issues))
	}
	if page.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4 (full filtered count, even with no results)", page.TotalCount)
	}
	if page.PerPage != 0 {
		t.Errorf("PerPage = %d, want normalised to 0", page.PerPage)
	}

	negative, err := c.ListIssuesPaged(t.Context(), "", "", "", 1, -5)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if len(negative.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0", len(negative.Issues))
	}
	if negative.PerPage != 0 {
		t.Errorf("PerPage = %d, want normalised to 0", negative.PerPage)
	}
}

// The HTTP layer's paginationFromQuery only floors page/perPage, it never
// caps the upper bound (unlike PATClient's clampSearchPage), so an
// out-of-range per_page can reach the mock directly. ListIssuesPaged must
// not overflow its start/end arithmetic and panic on the resulting slice.
// Spec (spec.md:736-747) authorizes only two normalisations — page < 1 and
// perPage < 1 — so an extreme-but-positive perPage is echoed back as given,
// not clamped to a page size the spec never mentions.
func TestMockClient_ListIssuesPaged_ExtremePerPageDoesNotOverflowOrPanic(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	first, err := c.ListIssuesPaged(t.Context(), "", "", "", 1, math.MaxInt)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if len(first.Issues) != 4 {
		t.Errorf("len(Issues) = %d, want all 4 on page 1", len(first.Issues))
	}
	if first.PerPage != math.MaxInt {
		t.Errorf("PerPage = %d, want echoed back as math.MaxInt, not clamped", first.PerPage)
	}
	if first.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", first.TotalCount)
	}

	page, err := c.ListIssuesPaged(t.Context(), "", "", "", 2, math.MaxInt)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page.TotalCount != 4 {
		t.Errorf("TotalCount = %d, want 4", page.TotalCount)
	}
	if len(page.Issues) != 0 {
		t.Errorf("len(Issues) = %d, want 0 (page 2 is beyond the end of the unfiltered set)", len(page.Issues))
	}
	if page.PerPage != math.MaxInt {
		t.Errorf("PerPage = %d, want echoed back as math.MaxInt, not clamped", page.PerPage)
	}
}

func TestMockClient_ListIssuesPaged_ForwardsMilestoneWithoutResolvingTwice(t *testing.T) {
	c := NewMockClient(DefaultHost)
	seedMilestoneIssues(t, c)

	page, err := c.ListIssuesPaged(t.Context(), "", "", "Next", 1, 10)
	if err != nil {
		t.Fatalf("ListIssuesPaged() error = %v", err)
	}
	if page.TotalCount != 2 {
		t.Errorf("TotalCount = %d, want 2", page.TotalCount)
	}
	for _, issue := range page.Issues {
		if issue.Milestone != "Next" {
			t.Errorf("issue %+v has milestone %q, want Next", issue, issue.Milestone)
		}
	}
}

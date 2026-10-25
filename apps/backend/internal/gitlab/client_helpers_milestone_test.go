package gitlab

import (
	"net/url"
	"testing"
)

func TestBuildIssueSearchQuery_PresetPathComposesMilestoneAfterFilter(t *testing.T) {
	got := buildIssueSearchQuery("scope=assigned_to_me", "", "Next")
	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", got, err)
	}
	if got := values.Get("scope"); got != "assigned_to_me" {
		t.Errorf("scope = %q, want assigned_to_me (milestone must not clobber the preset)", got)
	}
	if got := values.Get("state"); got != "opened" {
		t.Errorf("state = %q, want opened", got)
	}
	if got := values.Get("milestone"); got != "Next" {
		t.Errorf("milestone = %q, want Next", got)
	}
}

func TestBuildIssueSearchQuery_EmptyMilestoneIsByteIdenticalToBeforeFeature(t *testing.T) {
	got := buildIssueSearchQuery("scope=assigned_to_me", "", "")
	want := buildIssueSearchQueryWithoutMilestoneForTest("scope=assigned_to_me", "")
	if got != want {
		t.Errorf("query = %q, want %q (empty milestone must add nothing)", got, want)
	}
}

// buildIssueSearchQueryWithoutMilestoneForTest reproduces the pre-feature
// query builder so the byte-identical claim can be checked without a second
// production code path.
func buildIssueSearchQueryWithoutMilestoneForTest(filter, customQuery string) string {
	if customQuery != "" {
		return customQuery
	}
	values := url.Values{}
	values.Set("state", gitlabStateOpened)
	values.Set("scope", "all")
	if filter != "" {
		appendFilter(values, filter)
	}
	return values.Encode()
}

func TestBuildIssueSearchQuery_CustomQueryWithMilestoneWins(t *testing.T) {
	got := buildIssueSearchQuery("", "state=closed&milestone=Old", "Next")
	if got != "state=closed&milestone=Old" {
		t.Errorf("query = %q, want custom query unchanged (custom milestone wins)", got)
	}
}

// Scenario 31, live-path clause: a custom query carrying a repeated
// milestone key is forwarded upstream unchanged, both pairs included — the
// live path never resolves which one wins, only the mock does (Get,
// first-value).
func TestBuildIssueSearchQuery_CustomQueryWithRepeatedMilestoneKeyForwardedUnchanged(t *testing.T) {
	got := buildIssueSearchQuery("", "milestone=Old&milestone=Next", "Next")
	want := "milestone=Old&milestone=Next"
	if got != want {
		t.Errorf("query = %q, want %q (both pairs forwarded verbatim)", got, want)
	}
}

func TestBuildIssueSearchQuery_CustomQueryWithoutMilestoneIsExtended(t *testing.T) {
	got := buildIssueSearchQuery("", "state=closed", "Next")
	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", got, err)
	}
	if got := values.Get("state"); got != "closed" {
		t.Errorf("state = %q, want closed", got)
	}
	if got := values.Get("milestone"); got != "Next" {
		t.Errorf("milestone = %q, want Next", got)
	}
	if values.Has("scope") {
		t.Errorf("scope present in %q, want absent (custom query defaults are not reintroduced)", got)
	}
}

func TestBuildIssueSearchQuery_CustomQueryFragmentKeepsMilestoneInQuery(t *testing.T) {
	got := buildIssueSearchQuery("", "state=closed#client-fragment", "Next")
	want := "state=closed&milestone=Next#client-fragment"
	if got != want {
		t.Errorf("query = %q, want %q (milestone must precede the URL fragment)", got, want)
	}
}

// The HTTP controller rejects malformed custom queries before this helper is
// called. Keep this builder-level case to pin its defensive append behavior
// for other callers that construct the query directly.
func TestBuildIssueSearchQuery_MalformedCustomQueryAppendsMilestone(t *testing.T) {
	got := buildIssueSearchQuery("", "%zz", "Next")
	want := "%zz&milestone=Next"
	if got != want {
		t.Errorf("query = %q, want %q", got, want)
	}
}

func TestBuildIssueSearchQuery_EmptyMilestoneKeyInCustomQueryStillWins(t *testing.T) {
	got := buildIssueSearchQuery("", "state=closed&milestone=", "Next")
	if got != "state=closed&milestone=" {
		t.Errorf("query = %q, want custom query unchanged (empty milestone key still wins)", got)
	}
}

func TestBuildIssueSearchQuery_MalformedFilterCannotTakeMilestoneDownWithIt(t *testing.T) {
	got := buildIssueSearchQuery("%zz", "", "Next")
	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", got, err)
	}
	if got := values.Get("milestone"); got != "Next" {
		t.Errorf("milestone = %q, want Next (unparseable filter must not drop it)", got)
	}
	if got := values.Get("state"); got != "opened" {
		t.Errorf("state = %q, want opened", got)
	}
	if got := values.Get("scope"); got != "all" {
		t.Errorf("scope = %q, want all", got)
	}
}

func TestBuildIssueSearchQuery_MilestoneEscapedExactlyOnce(t *testing.T) {
	// Preset path: values.Encode() does the single escape.
	got := buildIssueSearchQuery("", "", "Q3 & Q4=x")
	values, err := url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", got, err)
	}
	if got := values.Get("milestone"); got != "Q3 & Q4=x" {
		t.Errorf("milestone = %q, want %q", got, "Q3 & Q4=x")
	}
	if len(values) != 3 {
		t.Errorf("values = %v, want exactly state/scope/milestone (no injected key)", values)
	}

	// Escape-hatch path: url.QueryEscape does the single escape.
	got = buildIssueSearchQuery("", "state=closed", "Q3 & Q4=x")
	values, err = url.ParseQuery(got)
	if err != nil {
		t.Fatalf("ParseQuery(%q): %v", got, err)
	}
	if got := values.Get("milestone"); got != "Q3 & Q4=x" {
		t.Errorf("milestone = %q, want %q", got, "Q3 & Q4=x")
	}
	if len(values) != 2 {
		t.Errorf("values = %v, want exactly state/milestone (no injected key)", values)
	}
}

// Scenario 22: GitLab's predefined milestone values (None, Any, Upcoming)
// are forwarded verbatim on "milestone" — kandev applies no special
// handling and never translates any of them onto "milestone_id".
func TestBuildIssueSearchQuery_PredefinedMilestoneValuesPassThroughUntranslated(t *testing.T) {
	for _, value := range []string{"None", "Any", "Upcoming"} {
		got := buildIssueSearchQuery("", "", value)
		values, err := url.ParseQuery(got)
		if err != nil {
			t.Fatalf("ParseQuery(%q): %v", got, err)
		}
		if got := values.Get("milestone"); got != value {
			t.Errorf("milestone = %q, want %q", got, value)
		}
		if values.Has("milestone_id") {
			t.Errorf("query %q carries milestone_id, want it absent for value %q", got, value)
		}
	}
}

func TestConvertRawIssue_MilestoneTitle(t *testing.T) {
	raw := &rawIssue{Milestone: &rawMilestone{Title: "Next"}}
	issue := convertRawIssue(raw)
	if issue.Milestone != "Next" {
		t.Errorf("Milestone = %q, want Next", issue.Milestone)
	}
}

func TestConvertRawIssue_NullMilestoneDecodesToEmptyString(t *testing.T) {
	raw := &rawIssue{Milestone: nil}
	issue := convertRawIssue(raw)
	if issue.Milestone != "" {
		t.Errorf("Milestone = %q, want empty for a null milestone", issue.Milestone)
	}
}

// trimGitLabWhitespace is the Go-side normative trim helper: Unicode
// White_Space together with U+FEFF. strings.TrimSpace already covers the
// Unicode White_Space set, including U+0085, but it does not cover U+FEFF.
// Scenario 27.
func TestTrimGitLabWhitespace_MatchesNormativeSet(t *testing.T) {
	trimmed := []rune{
		0x0009, 0x000A, 0x000B, 0x000C, 0x000D, 0x0020, 0x0085, 0x00A0,
		0x1680, 0x2000, 0x2028, 0x2029, 0x202F, 0x205F, 0x3000, 0xFEFF,
	}
	for _, r := range trimmed {
		got := trimGitLabWhitespace(string(r))
		if got != "" {
			t.Errorf("trimGitLabWhitespace(%U) = %q, want empty", r, got)
		}
	}

	preserved := []rune{0x200B, 0x180E, 0x2060, 0x00B7}
	for _, r := range preserved {
		got := trimGitLabWhitespace(string(r))
		if got != string(r) {
			t.Errorf("trimGitLabWhitespace(%U) = %q, want unchanged", r, got)
		}
	}

	nelPrefixed := string(rune(0x0085)) + "Next" + string(rune(0xFEFF))
	if got := trimGitLabWhitespace(nelPrefixed); got != "Next" {
		t.Errorf("trimGitLabWhitespace(NEL+Next+BOM) = %q, want Next", got)
	}
	if got := trimGitLabWhitespace("  Q1 Planning  "); got != "Q1 Planning" {
		t.Errorf("trimGitLabWhitespace(padded) = %q, want interior space preserved", got)
	}

	// The helper adds the one normative rune that TrimSpace does not trim.
	bomOnly := string(rune(0xFEFF))
	if got := trimGitLabWhitespace(bomOnly); got != "" {
		t.Errorf("trimGitLabWhitespace(BOM alone) = %q, want empty (TrimSpace keeps U+FEFF)", got)
	}
}

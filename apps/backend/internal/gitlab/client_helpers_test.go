package gitlab

import (
	"net/url"
	"testing"
)

// User-supplied filter keys must override defaults seeded by the caller.
// The previous implementation used values.Add(), which kept the default
// alongside the user value (e.g. state=opened&state=closed) and GitLab
// silently honoured the first one.
func TestAppendFilter_UserKeysOverrideDefaults(t *testing.T) {
	values := url.Values{}
	values.Set("state", "opened")
	values.Set("scope", "all")
	appendFilter(values, "state=closed&labels=bug")
	if got := values.Get("state"); got != "closed" {
		t.Errorf("state = %q, want closed (user filter must override default)", got)
	}
	if got := values["state"]; len(got) != 1 {
		t.Errorf("state values = %v, want length 1 (defaults must be cleared before re-adding)", got)
	}
	if got := values.Get("labels"); got != "bug" {
		t.Errorf("labels = %q, want bug", got)
	}
	if got := values.Get("scope"); got != "all" {
		t.Errorf("scope = %q, want all (untouched keys keep their default)", got)
	}
}

func TestAppendFilter_MultiValueOverride(t *testing.T) {
	values := url.Values{}
	values.Set("labels", "default")
	appendFilter(values, "labels=a&labels=b")
	got := values["labels"]
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("labels = %v, want [a b] (default cleared then both user values appended)", got)
	}
}

func TestAppendFilter_UnparseableIgnored(t *testing.T) {
	values := url.Values{}
	values.Set("state", "opened")
	appendFilter(values, "%bad%")
	if got := values.Get("state"); got != "opened" {
		t.Errorf("state = %q, want opened (unparseable filter must not nuke defaults)", got)
	}
}

// translateUserSearchFilter is the seam that turns the /gitlab page's tab
// values (assigned_to_me / created_by_me / review_requested) into proper
// GitLab API filter strings. Before this helper existed, the controller
// passed the bare tab tokens straight to appendFilter, which parsed them as
// keys with empty values — the filter contributed nothing and the page
// served the global, unscoped listing.
func TestTranslateUserSearchFilter(t *testing.T) {
	cases := []struct {
		name     string
		token    string
		username string
		want     string
	}{
		{"assigned_to_me", "assigned_to_me", "", "scope=assigned_to_me"},
		{"created_by_me", "created_by_me", "", "scope=created_by_me"},
		{"review_requested_with_user", "review_requested", "alice", "reviewer_username=alice&scope=all"},
		// Empty username means the controller failed to resolve /user; the
		// translator must refuse so the caller can surface the error instead
		// of silently falling back to the global listing.
		{"review_requested_without_user", "review_requested", "", ""},
		// Already in key=value form — power-user passthrough must not be
		// rewritten. Verified end-to-end by TestAppendFilter_*.
		{"raw_key_value_passthrough", "labels=bug", "", ""},
		{"raw_key_value_with_amp", "labels=bug&state=closed", "", ""},
		// Unknown token — leave it to appendFilter to ignore.
		{"unknown_token", "mystery", "", ""},
		{"empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := translateUserSearchFilter(tc.token, tc.username)
			if got != tc.want {
				t.Errorf("translateUserSearchFilter(%q, %q) = %q, want %q",
					tc.token, tc.username, got, tc.want)
			}
		})
	}
}

// Usernames with characters that need URL escaping (rare on GitLab but
// possible on self-managed instances configured to allow them) must not
// land on the wire unescaped — the resulting query string would be
// ambiguous and could either break parsing or worse, accidentally inject
// extra params.
func TestTranslateUserSearchFilter_EscapesUsername(t *testing.T) {
	got := translateUserSearchFilter("review_requested", "a&b=c")
	want := "reviewer_username=a%26b%3Dc&scope=all"
	if got != want {
		t.Errorf("got = %q, want %q (username must be URL-escaped)", got, want)
	}
}

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

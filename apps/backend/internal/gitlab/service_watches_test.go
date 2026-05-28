package gitlab

import "testing"

func TestAppendLabelsToQuery_EmptyQuery(t *testing.T) {
	got := appendLabelsToQuery("", []string{"bug", "p1"})
	want := "labels=bug%2Cp1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAppendLabelsToQuery_ExistingQuery(t *testing.T) {
	got := appendLabelsToQuery("state=opened&scope=all", []string{"bug"})
	want := "state=opened&scope=all&labels=bug"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAppendLabelsToQuery_PreservesExistingLabelsClause(t *testing.T) {
	// If the user already specified labels in their customQuery we leave it
	// alone — silently double-appending would change query semantics.
	got := appendLabelsToQuery("labels=critical", []string{"bug"})
	want := "labels=critical"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestIsValidCleanupPolicy(t *testing.T) {
	cases := map[string]bool{
		"":        true,
		"auto":    true,
		"always":  true,
		"never":   true,
		"unknown": false,
		"AUTO":    false,
	}
	for in, want := range cases {
		if got := IsValidCleanupPolicy(in); got != want {
			t.Errorf("IsValidCleanupPolicy(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNormalizeCleanupPolicy(t *testing.T) {
	if got := NormalizeCleanupPolicy(""); got != CleanupPolicyAuto {
		t.Errorf("empty should normalize to auto, got %q", got)
	}
	if got := NormalizeCleanupPolicy("never"); got != "never" {
		t.Errorf("never should pass through, got %q", got)
	}
}

func TestNormalizeProjectFilters(t *testing.T) {
	in := []ProjectFilter{
		{Path: "  group/project  "},
		{Path: ""},
		{Path: "group/other"},
	}
	got := normalizeProjectFilters(in)
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got[0].Path != "group/project" || got[1].Path != "group/other" {
		t.Fatalf("trim/empty-drop broken: %+v", got)
	}
}

func TestNormalizeProjectFilters_NilInput(t *testing.T) {
	if got := normalizeProjectFilters(nil); got == nil || len(got) != 0 {
		t.Fatalf("nil input should return empty slice, got %+v", got)
	}
}

package automation

import (
	"strings"
	"testing"
)

// AC-25/26: deriveAutomationSlug lowercases, replaces every character outside
// [a-z0-9] with '-', collapses consecutive '-', trims leading/trailing '-',
// truncates to 64 characters followed by a further trailing-'-' trim, and
// falls back to "automation" when the result is empty.

func TestDeriveAutomationSlug_LowercasesAndReplacesNonAlnum(t *testing.T) {
	got := deriveAutomationSlug("Daily Sync!")
	want := "daily-sync"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveAutomationSlug_CollapsesConsecutiveDashes(t *testing.T) {
	got := deriveAutomationSlug("Daily   Sync -- Now")
	want := "daily-sync-now"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveAutomationSlug_TrimsLeadingAndTrailingDashes(t *testing.T) {
	got := deriveAutomationSlug("!!!Daily Sync!!!")
	want := "daily-sync"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestDeriveAutomationSlug_TruncatesTo64ThenTrimsTrailingDash(t *testing.T) {
	// 63 alnum chars, then a run of non-alnum that lands the 64th output
	// character on a '-': truncation must then trim that trailing dash.
	name := strings.Repeat("a", 63) + " " + "bbbb"
	got := deriveAutomationSlug(name)
	want := strings.Repeat("a", 63)
	if got != want {
		t.Errorf("got %q (len %d), want %q (len %d)", got, len(got), want, len(want))
	}
}

func TestDeriveAutomationSlug_TruncatesTo64ExactBoundary(t *testing.T) {
	name := strings.Repeat("a", 70)
	got := deriveAutomationSlug(name)
	if len(got) != 64 {
		t.Errorf("got len %d, want 64", len(got))
	}
	if got != strings.Repeat("a", 64) {
		t.Errorf("got %q, want 64 a's", got)
	}
}

func TestDeriveAutomationSlug_EmptyResultFallsBackToAutomation(t *testing.T) {
	// AC-26's worked example: a name with no [a-zA-Z0-9] characters at all.
	got := deriveAutomationSlug("日次レビュー")
	if got != automationSlugFallback {
		t.Errorf("got %q, want %q", got, automationSlugFallback)
	}
}

func TestDeriveAutomationSlug_EmptyName(t *testing.T) {
	got := deriveAutomationSlug("")
	if got != automationSlugFallback {
		t.Errorf("got %q, want %q", got, automationSlugFallback)
	}
}

func TestDeriveAutomationSlug_SpecWorkedExamples(t *testing.T) {
	cases := []struct {
		name string
		want string
	}{
		{"Daily Review — @kegmil/offline-first", "daily-review-kegmil-offline-first"},
		{"Daily km-mobile-app-v2 repo drift --all", "daily-km-mobile-app-v2-repo-drift-all"},
	}
	for _, c := range cases {
		if got := deriveAutomationSlug(c.name); got != c.want {
			t.Errorf("deriveAutomationSlug(%q) = %q, want %q", c.name, got, c.want)
		}
	}
}

// AC-27: automations sharing a base slug are suffixed with the first 8
// characters of their id; a still-colliding result widens the suffix by 8
// more characters of id, up to the full id, until every entry is unique.

func TestUniqueAutomationSlugs_NoCollision_KeepsBareSlug(t *testing.T) {
	got := uniqueAutomationSlugs([]automationSlugSource{
		{ID: "id-1", Name: "Daily Sync"},
		{ID: "id-2", Name: "Weekly Review"},
	})
	want := map[string]string{"id-1": "daily-sync", "id-2": "weekly-review"}
	for id, wantSlug := range want {
		if got[id] != wantSlug {
			t.Errorf("id %s: got %q, want %q", id, got[id], wantSlug)
		}
	}
}

func TestUniqueAutomationSlugs_TwoCollide_BothSuffixedWithFirst8OfID(t *testing.T) {
	got := uniqueAutomationSlugs([]automationSlugSource{
		{ID: "aaaaaaaa1111", Name: "Daily Sync"},
		{ID: "bbbbbbbb2222", Name: "Daily Sync"},
	})
	want := map[string]string{
		"aaaaaaaa1111": "daily-sync-aaaaaaaa",
		"bbbbbbbb2222": "daily-sync-bbbbbbbb",
	}
	for id, wantSlug := range want {
		if got[id] != wantSlug {
			t.Errorf("id %s: got %q, want %q", id, got[id], wantSlug)
		}
	}
	if got["aaaaaaaa1111"] == got["bbbbbbbb2222"] {
		t.Fatalf("collision not resolved: both got %q", got["aaaaaaaa1111"])
	}
}

func TestUniqueAutomationSlugs_ThreeCollide_AllSuffixedNoneKeepsBareSlug(t *testing.T) {
	got := uniqueAutomationSlugs([]automationSlugSource{
		{ID: "aaaaaaaa1111", Name: "Daily Sync"},
		{ID: "bbbbbbbb2222", Name: "Daily Sync"},
		{ID: "cccccccc3333", Name: "Daily Sync"},
	})
	seen := make(map[string]bool, 3)
	for _, id := range []string{"aaaaaaaa1111", "bbbbbbbb2222", "cccccccc3333"} {
		slug := got[id]
		if slug == "daily-sync" {
			t.Errorf("id %s kept the bare slug %q; every member of a colliding group must be suffixed", id, slug)
		}
		if seen[slug] {
			t.Fatalf("duplicate slug %q produced", slug)
		}
		seen[slug] = true
	}
}

func TestUniqueAutomationSlugs_SuffixStillCollides_WidensToFullID(t *testing.T) {
	// Both ids share their first 8 characters, so the level-8 suffix also
	// collides; the algorithm must widen to the full (12-character) id.
	got := uniqueAutomationSlugs([]automationSlugSource{
		{ID: "abcdefgh1111", Name: "Daily Sync"},
		{ID: "abcdefgh2222", Name: "Daily Sync"},
	})
	want := map[string]string{
		"abcdefgh1111": "daily-sync-abcdefgh1111",
		"abcdefgh2222": "daily-sync-abcdefgh2222",
	}
	for id, wantSlug := range want {
		if got[id] != wantSlug {
			t.Errorf("id %s: got %q, want %q", id, got[id], wantSlug)
		}
	}
}

func TestUniqueAutomationSlugs_UnsuffixedEntryCollidesWithSuffixedGroup(t *testing.T) {
	// D's own bare slug happens to equal what E and F's level-8 suffixed
	// names collide to. AC-27 requires resolving this even though D was
	// never part of E/F's originally-colliding base-slug group.
	got := uniqueAutomationSlugs([]automationSlugSource{
		{ID: "dddddddd1111", Name: "Foo Aaaaaaaa"},
		{ID: "aaaaaaaa1111", Name: "Foo"},
		{ID: "aaaaaaaa2222", Name: "Foo"},
	})
	names := map[string]string{
		"dddddddd1111": got["dddddddd1111"],
		"aaaaaaaa1111": got["aaaaaaaa1111"],
		"aaaaaaaa2222": got["aaaaaaaa2222"],
	}
	seen := make(map[string]bool, 3)
	for id, slug := range names {
		if slug == "" {
			t.Fatalf("id %s got no slug", id)
		}
		if seen[slug] {
			t.Fatalf("duplicate slug %q produced across %v", slug, names)
		}
		seen[slug] = true
	}
}

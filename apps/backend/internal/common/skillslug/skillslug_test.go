package skillslug

import "testing"

func TestWellFormed(t *testing.T) {
	cases := map[string]bool{
		"protocol":      true,
		"kandev-memory": true,
		"a1_b-2":        true,
		"":              false,
		"has space":     false,
		"has/slash":     false,
		"has.dot":       false,
		"has:colon":     false,
		"..":            false,
	}
	for slug, want := range cases {
		if got := WellFormed(slug); got != want {
			t.Errorf("WellFormed(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestWellFormedRejectsDots(t *testing.T) {
	if WellFormed("foo.bar") {
		t.Errorf("WellFormed(%q) = true, want false: dots are not in the allowed charset", "foo.bar")
	}
}

func TestCanonical(t *testing.T) {
	cases := map[string]bool{
		"kandev-protocol": true,
		"protocol":        false,
		"kandev-":         true,
		"":                false,
		"kandev":          false,
		"KANDEV-foo":      false, // prefix match is case-sensitive
	}
	for slug, want := range cases {
		if got := Canonical(slug); got != want {
			t.Errorf("Canonical(%q) = %v, want %v", slug, got, want)
		}
	}
}

func TestCanonicalImpliesWellFormed(t *testing.T) {
	slugs := []string{"kandev-protocol", "kandev-", "kandev-a1_b-2"}
	for _, slug := range slugs {
		if Canonical(slug) && !WellFormed(slug) {
			t.Errorf("slug %q is Canonical but not WellFormed", slug)
		}
	}
}

func TestNormalize(t *testing.T) {
	cases := map[string]string{
		"protocol":        "kandev-protocol",
		"kandev-protocol": "kandev-protocol",
		"kandev-":         "kandev-",
	}
	for slug, want := range cases {
		if got := Normalize(slug); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", slug, got, want)
		}
	}
}

func TestNormalizeIdempotent(t *testing.T) {
	slugs := []string{"protocol", "kandev-protocol", "a1_b-2"}
	for _, slug := range slugs {
		once := Normalize(slug)
		twice := Normalize(once)
		if once != twice {
			t.Errorf("Normalize not idempotent for %q: once=%q twice=%q", slug, once, twice)
		}
	}
}

func TestNormalizeProducesCanonical(t *testing.T) {
	slugs := []string{"protocol", "kandev-protocol", "a1_b-2"}
	for _, slug := range slugs {
		if got := Normalize(slug); !Canonical(got) {
			t.Errorf("Normalize(%q) = %q, want a canonical slug", slug, got)
		}
	}
}

// TestEmptyStringBoundary documents why validation must precede
// normalization: Normalize("") is well-formed and canonical, but "" itself
// is not well-formed. Callers must reject not-well-formed input before
// calling Normalize, never rely on Normalize to reject it for them.
func TestEmptyStringBoundary(t *testing.T) {
	if WellFormed("") {
		t.Fatal("WellFormed(\"\") = true, want false")
	}
	normalized := Normalize("")
	if normalized != Prefix {
		t.Errorf("Normalize(\"\") = %q, want %q", normalized, Prefix)
	}
	if !WellFormed(normalized) {
		t.Errorf("Normalize(\"\") produced %q which is not well-formed", normalized)
	}
}

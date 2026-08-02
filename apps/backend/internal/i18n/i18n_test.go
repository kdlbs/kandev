package i18n

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ in, want string }{
		{"en", "en"},
		{"pseudo", "pseudo"},
		{"", "en"},
		{"fr", "en"},
		{"EN", "en"},
	} {
		if got := Normalize(tc.in); got != tc.want {
			t.Fatalf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTranslatesAndFallsBack(t *testing.T) {
	t.Parallel()
	en := T("en", "webapp.shellUnavailable")
	if en == "webapp.shellUnavailable" || en == "" {
		t.Fatalf("en catalog did not resolve the key, got %q", en)
	}
	// The pseudo catalog must differ from en; that is what makes it usable as the
	// externalization oracle.
	if pseudo := T("pseudo", "webapp.shellUnavailable"); pseudo == en {
		t.Fatalf("pseudo message should differ from en, both %q", en)
	}
	// An unknown locale falls back to en rather than erroring or blanking.
	if got := T("klingon", "webapp.shellUnavailable"); got != en {
		t.Fatalf("unknown locale = %q, want en fallback %q", got, en)
	}
	// A missing key degrades to the key itself, never empty.
	if got := T("en", "does.not.exist"); got != "does.not.exist" {
		t.Fatalf("missing key = %q, want the key echoed back", got)
	}
}

func TestTf(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		locale string
		key    string
		vars   map[string]any
		want   string
	}{
		{"interpolates a placeholder", "en", "share.pageTitle",
			map[string]any{"title": "Fix the flake"}, "Fix the flake — kandev share"},
		{"count of one selects the singular", "en", "share.messageCount",
			map[string]any{"count": 1}, "1 message"},
		{"count of zero selects the plural", "en", "share.messageCount",
			map[string]any{"count": 0}, "0 messages"},
		{"count of many selects the plural", "en", "share.messageCount",
			map[string]any{"count": 12}, "12 messages"},
		// JSON-decoded numbers arrive as float64; they must still select a form.
		{"float count selects a form", "en", "share.messageCount",
			map[string]any{"count": float64(1)}, "1 message"},
		// Only an exact 1 is singular. Truncating to an int would pick the
		// singular here and render "1.5 message".
		{"fractional count is plural", "en", "share.messageCount",
			map[string]any{"count": 1.5}, "1.5 messages"},
		// Widths other than int reach Tf from proto fields and store aggregates;
		// falling through to the base key would render the plural for every value.
		{"int32 count selects the singular", "en", "share.messageCount",
			map[string]any{"count": int32(1)}, "1 message"},
		{"uint64 count selects the plural", "en", "share.messageCount",
			map[string]any{"count": uint64(4)}, "4 messages"},
		// A key with no _one/_other entries must not resolve to a missing
		// suffixed key and echo it back — it falls back to the base message.
		{"key without plural forms falls back", "en", "share.untitledTask",
			map[string]any{"count": 3}, "Untitled task"},
		// A placeholder with no value stays visible; a silent hole in a
		// sentence is much harder to notice in review than "{{title}}".
		{"missing var leaves the placeholder", "en", "share.pageTitle",
			nil, "{{title}} — kandev share"},
		{"unsupported locale falls back to en", "klingon", "share.messageCount",
			map[string]any{"count": 2}, "2 messages"},
		{"non-numeric count is ignored", "en", "share.untitledTask",
			map[string]any{"count": "many"}, "Untitled task"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := Tf(tc.locale, tc.key, tc.vars); got != tc.want {
				t.Fatalf("Tf(%q, %q, %v) = %q, want %q", tc.locale, tc.key, tc.vars, got, tc.want)
			}
		})
	}
}

func TestTfPseudoLocaleKeepsPlaceholderValues(t *testing.T) {
	t.Parallel()
	// The pseudo catalog must translate the message but leave the interpolated
	// value alone — a transliterated URL or filename would be a dead pointer.
	got := Tf("pseudo", "share.messageCount", map[string]any{"count": 7})
	if !strings.Contains(got, "7") {
		t.Fatalf("pseudo render %q dropped the interpolated count", got)
	}
	if got == Tf("en", "share.messageCount", map[string]any{"count": 7}) {
		t.Fatalf("pseudo render is identical to en: %q", got)
	}
}

func TestCatalogsHaveMatchingKeys(t *testing.T) {
	t.Parallel()
	// Every en key must exist in pseudo, otherwise the QA locale silently shows
	// English and stops proving that a string was externalized.
	for _, key := range Keys() {
		if T("pseudo", key) == key {
			t.Fatalf("pseudo catalog is missing key %q", key)
		}
	}
}

func TestFromRequest(t *testing.T) {
	t.Parallel()

	t.Run("cookie wins", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: LocaleCookie, Value: "pseudo"})
		r.Header.Set("Accept-Language", "en")
		if got := FromRequest(r); got != "pseudo" {
			t.Fatalf("got %q, want pseudo", got)
		}
	})

	t.Run("invalid cookie ignored", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.AddCookie(&http.Cookie{Name: LocaleCookie, Value: "klingon"})
		if got := FromRequest(r); got != DefaultLocale {
			t.Fatalf("got %q, want %q", got, DefaultLocale)
		}
	})

	t.Run("accept-language honored by q-value", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Language", "fr;q=0.9, pseudo;q=1.0")
		if got := FromRequest(r); got != "pseudo" {
			t.Fatalf("got %q, want pseudo", got)
		}
	})

	t.Run("region subtag matches base locale", func(t *testing.T) {
		t.Parallel()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set("Accept-Language", "en-GB")
		if got := FromRequest(r); got != "en" {
			t.Fatalf("got %q, want en", got)
		}
	})

	t.Run("nil request is safe", func(t *testing.T) {
		t.Parallel()
		if got := FromRequest(nil); got != DefaultLocale {
			t.Fatalf("got %q, want %q", got, DefaultLocale)
		}
	})
}

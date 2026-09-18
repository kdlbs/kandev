package i18n

import (
	"net/http/httptest"
	"testing"
)

func TestParseAcceptLanguageExcludesUnparseableQValue(t *testing.T) {
	t.Parallel()
	got := parseAcceptLanguage("fr;q=not-a-number, en;q=0.5")
	if len(got) != 1 || got[0] != "en" {
		t.Fatalf("parseAcceptLanguage = %v, want only [en]: an unparseable q must exclude the tag, not promote it to 1.0", got)
	}
}

func TestParseAcceptLanguageExcludesZeroQValue(t *testing.T) {
	t.Parallel()
	got := parseAcceptLanguage("fr;q=0, en;q=0.5")
	if len(got) != 1 || got[0] != "en" {
		t.Fatalf("parseAcceptLanguage = %v, want only [en]: q=0 means not acceptable per RFC 7231", got)
	}
}

func TestParseAcceptLanguageExcludesNegativeQValue(t *testing.T) {
	t.Parallel()
	got := parseAcceptLanguage("fr;q=-1, en;q=0.5")
	if len(got) != 1 || got[0] != "en" {
		t.Fatalf("parseAcceptLanguage = %v, want only [en]", got)
	}
}

func TestParseAcceptLanguageOrdersDescendingByQValue(t *testing.T) {
	t.Parallel()
	got := parseAcceptLanguage("en;q=0.3, zh-cn;q=0.9, pt-pt;q=0.6")
	want := []string{"zh-cn", "pt-pt", "en"}
	if len(got) != len(want) {
		t.Fatalf("parseAcceptLanguage = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("parseAcceptLanguage = %v, want %v", got, want)
		}
	}
}

func TestParseAcceptLanguageDefaultsMissingQTo1(t *testing.T) {
	t.Parallel()
	got := parseAcceptLanguage("en, zh-cn;q=0.5")
	want := []string{"en", "zh-cn"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("parseAcceptLanguage = %v, want %v", got, want)
	}
}

func TestFromRequestFallsThroughAMalformedQValueToTheNextTag(t *testing.T) {
	t.Parallel()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept-Language", "fr;q=bogus, zh-cn;q=0.8")
	if got := FromRequest(r); got != "zh-cn" {
		t.Fatalf("FromRequest = %q, want %q: a malformed q must not win priority", got, "zh-cn")
	}
}

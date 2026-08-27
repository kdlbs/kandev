package service

import (
	"errors"
	"strings"
	"testing"
)

// TestNormalizeExternalIDAcceptsValidValues covers the spec's "Everything
// surviving those rules is accepted verbatim" examples plus the trim and
// empty-after-trim rules.
func TestNormalizeExternalIDAcceptsValidValues(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"jira key", "jira:PROJ-1234", "jira:PROJ-1234"},
		{"github issue path", "gh-issue/kdlbs/kandev#2325", "gh-issue/kdlbs/kandev#2325"},
		{"bare uuid", "8f14e45f-ceea-467e-bd6b-9b1a3f0c1a2e", "8f14e45f-ceea-467e-bd6b-9b1a3f0c1a2e"},
		{"non-ascii letters", "客户-1234", "客户-1234"},
		{"leading and trailing spaces trimmed", "  ext-1  ", "ext-1"},
		{"whitespace-only normalizes to absent", "   ", ""},
		{"empty string is absent", "", ""},
		{"case is preserved, not folded", "EXT-1", "EXT-1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormalizeExternalID(tc.raw)
			if err != nil {
				t.Fatalf("NormalizeExternalID(%q) error = %v, want nil", tc.raw, err)
			}
			if got != tc.want {
				t.Fatalf("NormalizeExternalID(%q) = %q, want %q", tc.raw, got, tc.want)
			}
		})
	}
}

// TestNormalizeExternalIDRejectsControlCharactersBeforeTrim pins the
// normative ordering: control-character rejection runs on the RAW value,
// before trimming — so a trailing newline or a lone tab is an error, not
// something the trim silently erases into "absent".
func TestNormalizeExternalIDRejectsControlCharactersBeforeTrim(t *testing.T) {
	cases := []string{
		"ext-1\n",
		"\t",
		"ext\x001",
		"ext-1\x7f",
	}
	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := NormalizeExternalID(raw); !errors.Is(err, ErrExternalIDInvalid) {
				t.Fatalf("NormalizeExternalID(%q) error = %v, want ErrExternalIDInvalid", raw, err)
			}
		})
	}
}

// TestNormalizeExternalIDEnforcesByteLength covers the 255-UTF-8-byte cap,
// applied to the TRIMMED value.
func TestNormalizeExternalIDEnforcesByteLength(t *testing.T) {
	ok := strings.Repeat("a", ExternalIDMaxBytes)
	if got, err := NormalizeExternalID(ok); err != nil || got != ok {
		t.Fatalf("255-byte value: got=%q err=%v, want it accepted unchanged", got, err)
	}

	tooLong := strings.Repeat("a", ExternalIDMaxBytes+1)
	if _, err := NormalizeExternalID(tooLong); !errors.Is(err, ErrExternalIDInvalid) {
		t.Fatalf("256-byte value error = %v, want ErrExternalIDInvalid", err)
	}

	// Padding with trimmed whitespace must not count against the limit —
	// length is measured on the trimmed value.
	paddedOK := "  " + ok + "  "
	if got, err := NormalizeExternalID(paddedOK); err != nil || got != ok {
		t.Fatalf("255-byte value padded with whitespace: got=%q err=%v, want trimmed value accepted", got, err)
	}
}

package truncate_test

import (
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/office/truncate"
)

func TestUTF8(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		maxBytes int
		want     string
	}{
		{name: "shorter than cap is unchanged", in: "hello", maxBytes: 8, want: "hello"},
		{name: "exactly at cap is unchanged", in: "hello", maxBytes: 5, want: "hello"},
		{name: "empty string", in: "", maxBytes: 4, want: ""},
		{name: "ascii cuts at the byte cap", in: "abcdef", maxBytes: 3, want: "abc"},
		{name: "zero cap yields empty", in: "abc", maxBytes: 0, want: ""},

		// Multi-byte runes straddling the cut are dropped whole rather
		// than sliced into invalid bytes. "é" is C3 A9 (2 bytes).
		{name: "2-byte rune split on its lead byte", in: "aé", maxBytes: 1, want: "a"},
		{name: "2-byte rune split on its continuation byte", in: "aé", maxBytes: 2, want: "a"},
		{name: "2-byte rune fits exactly", in: "aé", maxBytes: 3, want: "aé"},

		// "€" is E2 82 AC (3 bytes).
		{name: "3-byte rune split on its lead byte", in: "a€", maxBytes: 1, want: "a"},
		{name: "3-byte rune split after one continuation byte", in: "a€", maxBytes: 2, want: "a"},
		{name: "3-byte rune split after two continuation bytes", in: "a€", maxBytes: 3, want: "a"},
		{name: "3-byte rune fits exactly", in: "a€", maxBytes: 4, want: "a€"},

		// "😀" is F0 9F 98 80 (4 bytes).
		{name: "4-byte rune split on its lead byte", in: "a😀", maxBytes: 1, want: "a"},
		{name: "4-byte rune split after one continuation byte", in: "a😀", maxBytes: 2, want: "a"},
		{name: "4-byte rune split after two continuation bytes", in: "a😀", maxBytes: 3, want: "a"},
		{name: "4-byte rune split after three continuation bytes", in: "a😀", maxBytes: 4, want: "a"},
		{name: "4-byte rune fits exactly", in: "a😀", maxBytes: 5, want: "a😀"},

		// Nothing survives when the only rune straddles the cut.
		{name: "sole multi-byte rune is dropped entirely", in: "😀", maxBytes: 3, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate.UTF8(tt.in, tt.maxBytes)
			if got != tt.want {
				t.Errorf("UTF8(%q, %d) = %q, want %q", tt.in, tt.maxBytes, got, tt.want)
			}
			if len(got) > tt.maxBytes {
				t.Errorf("UTF8(%q, %d) returned %d bytes, over the cap", tt.in, tt.maxBytes, len(got))
			}
			if !utf8.ValidString(got) {
				t.Errorf("UTF8(%q, %d) returned invalid UTF-8: %q", tt.in, tt.maxBytes, got)
			}
		})
	}
}

// TestUTF8KeepsResultValidAcrossEveryCut walks every cut position of a
// mixed-width string: no cap may ever produce invalid UTF-8 or exceed
// its own budget.
func TestUTF8KeepsResultValidAcrossEveryCut(t *testing.T) {
	const s = "aé€😀bñ漢字"
	for maxBytes := 0; maxBytes <= len(s)+2; maxBytes++ {
		got := truncate.UTF8(s, maxBytes)
		if len(got) > maxBytes {
			t.Fatalf("maxBytes=%d: got %d bytes", maxBytes, len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("maxBytes=%d: invalid UTF-8 %q", maxBytes, got)
		}
		if got != s[:len(got)] {
			t.Fatalf("maxBytes=%d: %q is not a prefix of the input", maxBytes, got)
		}
	}
}

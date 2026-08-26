package truncate_test

import (
	"testing"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/common/truncate"
)

func TestUTF8CutsOnlyAtRuneBoundaries(t *testing.T) {
	for _, test := range []struct {
		name string
		in   string
		cap  int
		want string
	}{
		{name: "ascii", in: "abcdef", cap: 3, want: "abc"},
		{name: "split rune", in: "a😀b", cap: 3, want: "a"},
		{name: "fits rune", in: "a😀b", cap: 5, want: "a😀"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := truncate.UTF8(test.in, test.cap)
			if got != test.want {
				t.Fatalf("UTF8(%q, %d) = %q, want %q", test.in, test.cap, got, test.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("UTF8(%q, %d) returned invalid UTF-8", test.in, test.cap)
			}
		})
	}
}

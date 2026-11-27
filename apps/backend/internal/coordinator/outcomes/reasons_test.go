package outcomes

import "testing"

func TestCode(t *testing.T) {
	cases := []struct {
		name, coded, text, want string
	}{
		{"coded wins over text", "duplicate", "something else", "duplicate"},
		{"every code in the set", "too_broad", "", "too_broad"},
		{"wrong target", "wrong_target", "", "wrong_target"},
		{"wrong timing", "wrong_timing", "", "wrong_timing"},
		{"not wanted", "not_wanted", "", "not_wanted"},
		{"explicit other", "other", "", "other"},
		{"unknown code falls back to text", "bogus", "free words", "other"},
		{"text without code is other", "", "free words", "other"},
		{"whitespace text is none", "", "   \n", "none"},
		{"nothing is none", "", "", "none"},
		{"unknown code and no text is none", "bogus", "", "none"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Code(c.coded, c.text); got != c.want {
				t.Fatalf("Code(%q, %q) = %q, want %q", c.coded, c.text, got, c.want)
			}
		})
	}
}

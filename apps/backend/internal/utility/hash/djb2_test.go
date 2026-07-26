package hash

import "testing"

// djb2Fixtures are the expected outputs of `djb2Hash` in
// `apps/web/lib/utils/hash.ts`. The same table is asserted from
// `apps/web/lib/utils/hash.test.ts`, so a change to either implementation that
// breaks parity fails on both sides.
var djb2Fixtures = []struct {
	name  string
	input string
	want  string
}{
	{"empty", "", "1505"},
	{"single ascii char", "a", "2b606"},
	{"short ascii", "abc", "b885c8b"},
	{
		"unified diff body",
		"diff --git a/main.go b/main.go\n@@ -1,3 +1,4 @@\n+added line\n",
		"8c797eb",
	},
	{"latin-1 supplement", "café naïve über", "e4f5cae6"},
	{"astral plane emoji", "🚀 rocket", "9be64c4a"},
	{"cjk", "你好世界", "a96ad5c4"},
	{"long ascii", "The quick brown fox jumps over the lazy dog", "34cc38de"},
}

func TestDJB2MatchesWebImplementation(t *testing.T) {
	for _, tc := range djb2Fixtures {
		t.Run(tc.name, func(t *testing.T) {
			if got := DJB2(tc.input); got != tc.want {
				t.Fatalf("DJB2(%q) = %q, want %q (parity with apps/web/lib/utils/hash.ts)", tc.input, got, tc.want)
			}
		})
	}
}

func TestDJB2IsDeterministic(t *testing.T) {
	const input = "diff --git a/x b/x\n-old\n+new\n"
	first := DJB2(input)
	second := DJB2(input)
	if first != second {
		t.Fatalf("DJB2 must be deterministic for the same input, got %q then %q", first, second)
	}
	// A diff that differs only in trailing whitespace is a different diff, and a
	// finding anchored to the old one must be reported stale.
	if first == DJB2(input+" ") {
		t.Fatal("DJB2 must distinguish inputs differing by trailing whitespace")
	}
}

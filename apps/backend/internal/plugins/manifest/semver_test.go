package manifest

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "equal versions", a: "1.0.0", b: "1.0.0", want: 0},
		{name: "simple ascending patch", a: "1.0.0", b: "1.0.1", want: -1},
		{name: "simple descending patch", a: "1.0.1", b: "1.0.0", want: 1},
		{
			name: "double-digit minor beats lexically-larger single-digit minor",
			a:    "9.0.0", b: "10.0.0", want: -1,
		},
		{
			name: "double-digit minor beats lexically-larger single-digit minor (reversed)",
			a:    "10.0.0", b: "9.0.0", want: 1,
		},
		{name: "shorter version with fewer segments is less", a: "1.0", b: "1.0.1", want: -1},
		{name: "trailing zero segment is equal to the shorter form", a: "1.0", b: "1.0.0", want: 0},
		{
			name: "prerelease is older than the matching stable version",
			a:    "1.0.0-beta", b: "1.0.0", want: -1,
		},
		{
			name: "numeric prerelease identifiers sort numerically",
			a:    "1.0.0-alpha.10", b: "1.0.0-alpha.2", want: 1,
		},
		{
			name: "build metadata does not affect precedence",
			a:    "1.0.0+build.1", b: "1.0.0+build.2", want: 0,
		},
		{
			name: "malformed prerelease falls back to lexical ordering",
			a:    "1.0.0-", b: "1.0.0", want: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CompareVersions(tt.a, tt.b); got != tt.want {
				t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestNormalizeReleaseVersion(t *testing.T) {
	tests := []struct {
		raw  string
		want string
		ok   bool
	}{
		{raw: "1.2.3", want: "1.2.3", ok: true},
		{raw: "v1.2.3", want: "1.2.3", ok: true},
		{raw: " V1.2 ", want: "1.2", ok: true},
		{raw: "dev", ok: false},
		{raw: "v1.2.3-4-gabcdef", ok: false},
		{raw: "1.2.3.4", ok: false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			got, ok := NormalizeReleaseVersion(tt.raw)
			if got != tt.want || ok != tt.ok {
				t.Fatalf("NormalizeReleaseVersion(%q) = %q, %t; want %q, %t", tt.raw, got, ok, tt.want, tt.ok)
			}
		})
	}
}

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
		{raw: "1.2.4-nightly.sha123456789abc", ok: false},
		{raw: "v1.2.4-nightly.sha123456789abc", ok: false},
		{raw: "1.2.4-beta.1", ok: false},
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

// @covers AC-PLUGINS-PLUGINS-001.14
func TestCheckMinimumKandevVersion(t *testing.T) {
	const nightly = "v1.2.4-nightly.sha123456789abc"
	for _, tt := range []struct {
		name    string
		running string
		minimum string
		reject  bool
	}{
		{name: "nightly future stable", running: nightly, minimum: "2.0.0", reject: true},
		{name: "nightly same core stable", running: nightly, minimum: "1.2.4", reject: true},
		{name: "nightly older stable", running: nightly, minimum: "1.2.3"},
		{name: "bare nightly", running: "1.2.4-nightly.sha123456789abc", minimum: "1.2.4", reject: true},
		{name: "whitespace nightly", running: " " + nightly + " ", minimum: " v1.2.4 ", reject: true},
		{name: "digits-only hash", running: "v1.2.4-nightly.sha000123456789", minimum: "1.2.4", reject: true},
		{name: "stable future", running: "v1.2.4", minimum: "2.0.0", reject: true},
		{name: "stable equal", running: "v1.2.4", minimum: "v1.2.4"},
		{name: "stable earlier", running: "1.2.4", minimum: "1.2.3"},
		{name: "short stable", running: " V1.2 ", minimum: "1"},
		{name: "short stable future", running: "1", minimum: "2", reject: true},
		{name: "empty minimum", running: nightly},
		{name: "whitespace minimum", running: nightly, minimum: " "},
		{name: "empty running", minimum: "2.0.0"},
		{name: "whitespace running", running: " ", minimum: "2.0.0"},
		{name: "dev", running: "dev", minimum: "2.0.0"},
		{name: "dev casing", running: " DeV ", minimum: "2.0.0"},
		{name: "git describe", running: "v1.2.3-4-gabcdef", minimum: "2.0.0"},
		{name: "unknown", running: "custom", minimum: "2.0.0"},
		{name: "malformed", running: "v1..4", minimum: "2.0.0"},
		{name: "short hash", running: "v1.2.4-nightly.sha123456789ab", minimum: "2.0.0"},
		{name: "long hash", running: "v1.2.4-nightly.sha123456789abcd", minimum: "2.0.0"},
		{name: "nonhex hash", running: "v1.2.4-nightly.sha123456789abg", minimum: "2.0.0"},
		{name: "uppercase hash", running: "v1.2.4-nightly.sha123456789abC", minimum: "2.0.0"},
		{name: "noncanonical core", running: "v01.2.4-nightly.sha123456789abc", minimum: "2.0.0"},
		{name: "missing core segment", running: "v1.4-nightly.sha123456789abc", minimum: "2.0.0"},
		{name: "generic prerelease", running: "v1.2.4-beta.1", minimum: "2.0.0"},
		{name: "nightly git describe", running: nightly + "-4-gabcdef", minimum: "2.0.0"},
		{name: "nightly build metadata", running: nightly + "+build.1", minimum: "2.0.0"},
		{name: "invalid stable minimum", running: "v1.2.4", minimum: "invalid", reject: true},
		{name: "invalid nightly minimum", running: nightly, minimum: "invalid", reject: true},
		{name: "prerelease stable minimum", running: "v1.2.4", minimum: "1.2.3-beta", reject: true},
		{name: "prerelease nightly minimum", running: nightly, minimum: "1.2.3-beta", reject: true},
		{name: "nightly minimum", running: nightly, minimum: "1.2.3-nightly.sha123456789abc", reject: true},
		{name: "dev invalid minimum remains exempt", running: "dev", minimum: "invalid"},
		{name: "unwired invalid minimum remains exempt", minimum: "invalid"},
		{name: "unknown invalid minimum remains exempt", running: "custom", minimum: "invalid"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckMinimumKandevVersion(tt.minimum, tt.running)
			if (err != nil) != tt.reject {
				t.Fatalf("CheckMinimumKandevVersion(%q, %q) = %v, want rejection %t", tt.minimum, tt.running, err, tt.reject)
			}
		})
	}
}

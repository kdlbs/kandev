package updates

import "testing"

func TestCompareSemver(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{"equal", "v1.0.0", "v1.0.0", 0},
		{"equal_no_prefix", "1.2.3", "v1.2.3", 0},
		{"patch_less", "v1.0.0", "v1.0.1", -1},
		{"minor_greater", "v1.2.0", "v1.1.9", 1},
		{"major_greater", "v2.0.0", "v1.9.9", 1},
		{"prerelease_below_release", "v1.0.0-rc.1", "v1.0.0", -1},
		{"release_above_prerelease", "v1.0.0", "v1.0.0-rc.1", 1},

		// git-describe "ahead" builds must sort ABOVE their base tag.
		{"ahead_above_tag", "v0.79.0-60-g8fae44fb1", "v0.79.0", 1},
		{"tag_below_ahead", "v0.79.0", "v0.79.0-60-g8fae44fb1", -1},
		{"ahead_dirty_above_tag", "v0.79.0-60-g8fae44fb1-dirty", "v0.79.0", 1},
		// An ahead build outranks a real pre-release of the same tag.
		{"ahead_above_prerelease", "v1.0.0-3-gdeadbee", "v1.0.0-rc.1", 1},
		// More commits ahead wins.
		{"more_commits_ahead", "v1.0.0-10-gabc1234", "v1.0.0-2-gdef5678", 1},
		{"fewer_commits_ahead", "v1.0.0-2-gdef5678", "v1.0.0-10-gabc1234", -1},
		// A newer minor release still beats an ahead build of an older tag.
		{"newer_minor_beats_ahead", "v0.80.0", "v0.79.0-60-g8fae44fb1", 1},
		{"ahead_below_newer_minor", "v0.79.0-60-g8fae44fb1", "v0.80.0", -1},

		// Invalid inputs sort below valid ones.
		{"invalid_below_valid", "not-a-version", "v1.0.0", -1},
		{"valid_above_invalid", "v1.0.0", "not-a-version", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareSemver(tc.a, tc.b); got != tc.want {
				t.Errorf("compareSemver(%q,%q)=%d want %d", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestGitDescribeCommits(t *testing.T) {
	cases := []struct {
		name   string
		pre    string
		wantN  int
		wantOK bool
	}{
		{"standard", "60-g8fae44fb1", 60, true},
		{"dirty", "60-g8fae44fb1-dirty", 60, true},
		{"min_length_hash", "1-gabc1234", 1, true},
		{"empty", "", 0, false},
		{"real_prerelease", "rc.1", 0, false},
		{"prerelease_with_dot", "alpha.2", 0, false},
		{"missing_g_prefix", "60-8fae44fb1", 0, false},
		{"non_hex_hash", "60-gzzzzzzz", 0, false},
		{"non_numeric_count", "x-gabc1234", 0, false},
		// Short hashes below git's default abbreviation are treated as ordinary
		// pre-releases, not git-describe builds.
		{"single_char_hash", "3-ga", 0, false},
		{"below_min_hash", "3-gabc12", 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			gotN, gotOK := gitDescribeCommits(tc.pre)
			if gotN != tc.wantN || gotOK != tc.wantOK {
				t.Errorf("gitDescribeCommits(%q)=(%d,%v) want (%d,%v)", tc.pre, gotN, gotOK, tc.wantN, tc.wantOK)
			}
		})
	}
}

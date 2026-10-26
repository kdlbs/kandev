package executor

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"

	"github.com/stretchr/testify/require"
)

// nomadRepoBranchSlugImplementations lists every implementation that must
// agree with launchRepoBranchIdentitySlug. Add another implementation with
// one entry; each takes the raw branch the launch spec carries.
var nomadRepoBranchSlugImplementations = map[string]func(branch string) string{
	"launchRepoBranchIdentitySlug": func(branch string) string {
		return launchRepoBranchIdentitySlug(RepoSpec{BranchSlug: branch})
	},
	"lifecycle.NomadRecoveryRepoBranchSlug": func(branch string) string {
		return lifecycle.NomadRecoveryRepoBranchSlug("", branch)
	},
}

var nomadRepoBranchSlugCases = []struct{ name, branch, want string }{
	{"empty", "", ""},
	{"plain", "main", "main"},
	{"slashes", "feature/topic", "feature-topic"},
	{"nested slashes", "user/jane/wip", "user-jane-wip"},
	{"spaces", "weird name here", "weird-name-here"},
	{"uppercase preserved", "Feature/TOPIC", "Feature-TOPIC"},
	{"dots underscores", "release/v1.2_3", "release-v1.2_3"},
	{"leading and trailing separators", "/-lead/trail-/", "lead-trail"},
	{"unicode", "feature/café-日本", "feature-caf"},
}

func TestNomadRepoBranchSlugParity(t *testing.T) {
	for name, impl := range nomadRepoBranchSlugImplementations {
		for _, tc := range nomadRepoBranchSlugCases {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				require.Equal(t, tc.want, impl(tc.branch))
			})
		}
		t.Run(name+"/long", func(t *testing.T) {
			long := ""
			for i := 0; i < 300; i++ {
				long += "a/"
			}
			require.Equal(t, launchRepoBranchIdentitySlug(RepoSpec{BranchSlug: long}), impl(long))
		})
		t.Run(name+"/identity slug wins", func(t *testing.T) {
			require.Equal(t, "identity-slug", launchRepoBranchIdentitySlug(RepoSpec{BranchIdentitySlug: "identity/slug", BranchSlug: "other"}))
		})
	}
}

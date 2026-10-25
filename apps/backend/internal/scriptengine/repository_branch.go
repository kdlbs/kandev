package scriptengine

import "strings"

// repositoryBranchName removes one ref prefix for the origin clone source.
// A remaining origin/ prefix can be part of the upstream branch's literal name.
func repositoryBranchName(branch string) string {
	for _, prefix := range []string{"refs/heads/", "refs/remotes/origin/", "origin/"} {
		if name, ok := strings.CutPrefix(branch, prefix); ok {
			return name
		}
	}
	return branch
}

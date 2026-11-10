package github

import "github.com/kandev/kandev/internal/common/githubhost"

func markUnsupportedQueueObservations(exec GraphQLExecutor, statuses map[string]*PRStatus) {
	provider, ok := exec.(interface{ Host() githubhost.Host })
	if !ok || provider.Host().Family() != githubhost.EnterpriseFamily {
		return
	}
	for _, status := range statuses {
		if status != nil {
			status.mergeQueuePopulated = false
			status.mergeQueueRecoveryPopulated = false
		}
	}
}

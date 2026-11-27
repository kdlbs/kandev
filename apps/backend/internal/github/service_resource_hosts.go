package github

// Search pages can share provider/cache entries across requests. Bind response
// identity on copies so that a caller cannot change the cached resource host.
func prsForHost(prs []*PR, host string) []*PR {
	result := make([]*PR, 0, len(prs))
	for _, pr := range prs {
		if pr != nil {
			copy := *pr
			copy.GitHubHost = host
			result = append(result, &copy)
		}
	}
	return result
}

func issuesForHost(issues []*Issue, host string) []*Issue {
	result := make([]*Issue, 0, len(issues))
	for _, issue := range issues {
		if issue != nil {
			copy := *issue
			copy.GitHubHost = host
			result = append(result, &copy)
		}
	}
	return result
}

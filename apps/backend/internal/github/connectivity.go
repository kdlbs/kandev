package github

import "strings"

// isConnectivityError returns true when err looks like a transient network
// failure from `gh api` (offline, DNS, unreachable). These are noisy to log at
// ERROR since the poller retries every few minutes.
func isConnectivityError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	needles := []string{
		"error connecting to",
		"dial tcp",
		"no such host",
		"network is unreachable",
		"connection refused",
		"i/o timeout",
	}
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

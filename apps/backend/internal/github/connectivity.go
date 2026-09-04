package github

import "strings"

// IsConnectivityError returns true when err looks like a transient network
// failure from `gh api` (offline, DNS, unreachable). These are noisy to log at
// ERROR since the poller retries every few minutes; config sync's fetch-error
// classifier also uses this to distinguish an unreachable provider from a
// residue error on the gh-CLI transport, which reports connectivity failures
// as a bare exec error rather than the *url.Error the PAT clients produce.
func IsConnectivityError(err error) bool {
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

package github

import (
	"errors"
	"testing"
)

func TestIsConnectivityError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"dial tcp", errors.New("dial tcp 1.2.3.4:443: i/o timeout"), true},
		{"no such host", errors.New("Get https://api.github.com: no such host"), true},
		{"network unreachable", errors.New("network is unreachable"), true},
		{"connection refused", errors.New("connection refused"), true},
		{"gh api error connecting", errors.New("gh api: exit status 1: error connecting to api.github.com"), true},
		{"auth error", errors.New("HTTP 401: Bad credentials"), false},
		{"rate limit", errors.New("HTTP 429: API rate limit exceeded"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isConnectivityError(tc.err); got != tc.want {
				t.Errorf("isConnectivityError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

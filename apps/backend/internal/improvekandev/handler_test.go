package improvekandev

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

// fakeGitHubInfo is a configurable GitHubInfo for unit tests. Each method
// returns the value of the corresponding field; if the err counterpart is
// non-nil, the value is ignored and the error is returned instead.
type fakeGitHubInfo struct {
	login         string
	loginErr      error
	hasWrite      bool
	hasWriteErr   error
	hasFork       bool
	hasForkErr    error
	calledHasFork bool
}

func (f *fakeGitHubInfo) GetAuthenticatedLogin(_ context.Context) (string, error) {
	return f.login, f.loginErr
}

func (f *fakeGitHubInfo) HasRepoWriteAccess(_ context.Context, _, _ string) (bool, error) {
	return f.hasWrite, f.hasWriteErr
}

func (f *fakeGitHubInfo) UserHasFork(_ context.Context, _, _ string) (bool, error) {
	f.calledHasFork = true
	return f.hasFork, f.hasForkErr
}

func newTestHandler(gh GitHubInfo) *Handler {
	return &Handler{gh: gh, log: logger.Default()}
}

func TestResolveGitHubAccess_Writable(t *testing.T) {
	gh := &fakeGitHubInfo{login: "alice", hasWrite: true}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.login != "alice" || !access.hasWrite {
		t.Fatalf("login/write mismatch: %+v", access)
	}
	if access.forkStatus != ForkStatusWritable {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusWritable)
	}
	if gh.calledHasFork {
		t.Errorf("writable users should short-circuit before the fork check")
	}
}

func TestResolveGitHubAccess_ForkAlreadyExists(t *testing.T) {
	gh := &fakeGitHubInfo{login: "bob_corp", hasWrite: false, hasFork: true}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.forkStatus != ForkStatusReady {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusReady)
	}
	if access.forkMessage != "" {
		t.Errorf("ready status must not include a fork_message even for EMU-shaped logins: %q", access.forkMessage)
	}
}

func TestResolveGitHubAccess_BlockedEMU(t *testing.T) {
	gh := &fakeGitHubInfo{login: "eve_corp", hasWrite: false, hasFork: false}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.forkStatus != ForkStatusBlockedEMU {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusBlockedEMU)
	}
	if access.forkMessage == "" {
		t.Errorf("blocked_emu must include a fork_message for the dialog")
	}
}

func TestResolveGitHubAccess_UnknownOnLoginError(t *testing.T) {
	gh := &fakeGitHubInfo{loginErr: errors.New("auth failed")}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.login != "" || access.hasWrite {
		t.Errorf("login error must yield empty access: %+v", access)
	}
	if access.forkStatus != ForkStatusUnknown {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusUnknown)
	}
}

func TestResolveGitHubAccess_UnknownOnForkLookupError(t *testing.T) {
	gh := &fakeGitHubInfo{login: "carol_corp", hasWrite: false, hasForkErr: errors.New("network")}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.forkStatus != ForkStatusUnknown {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusUnknown)
	}
	if access.forkMessage != "" {
		t.Errorf("fork lookup failures must not produce an EMU message even for underscore logins")
	}
}

func TestResolveGitHubAccess_NoForkNotEMU(t *testing.T) {
	gh := &fakeGitHubInfo{login: "frank", hasWrite: false, hasFork: false}
	access := newTestHandler(gh).resolveGitHubAccess(context.Background())
	if access.forkStatus != ForkStatusUnknown {
		t.Errorf("fork status = %q, want %q", access.forkStatus, ForkStatusUnknown)
	}
	if access.forkMessage != "" {
		t.Errorf("non-EMU users should not get a fork_message")
	}
}

func TestIsEMULogin(t *testing.T) {
	cases := []struct {
		login string
		want  bool
	}{
		{"alice", false},
		{"alice-bob", false},
		{"", false},
		{"alice_corp", true},
		{"Carlos-Florencio-ii3_nbcuni", true},
	}
	for _, tc := range cases {
		if got := isEMULogin(tc.login); got != tc.want {
			t.Errorf("isEMULogin(%q) = %v, want %v", tc.login, got, tc.want)
		}
	}
}

func TestIsGHNotFound(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"unrelated", errors.New("network unreachable"), false},
		{"http 404", errors.New("gh api: exit status 1: HTTP 404: Not Found"), true},
		{"http 403", errors.New("gh api: HTTP 403: Forbidden"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isGHNotFound(tc.err); got != tc.want {
				t.Errorf("isGHNotFound(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

package github

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestPickDefaultMergeMethod(t *testing.T) {
	tests := []struct {
		name    string
		methods RepoMergeMethods
		want    string
	}{
		{"squash preferred over merge", RepoMergeMethods{Merge: true, Squash: true, Rebase: true}, "squash"},
		{"squash-only repo", RepoMergeMethods{Squash: true}, "squash"},
		{"merge when squash disabled", RepoMergeMethods{Merge: true, Rebase: true}, "merge"},
		{"rebase-only repo", RepoMergeMethods{Rebase: true}, "rebase"},
		{"none allowed → empty string", RepoMergeMethods{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickDefaultMergeMethod(tt.methods); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestService_GetRepoMergeMethods_Cached(t *testing.T) {
	mc := NewMockClient()
	mc.SetRepoMergeMethods("acme", "widget", RepoMergeMethods{Squash: true})
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	svc := NewService(mc, AuthMethodPAT, nil, nil, nil, log)

	// Two consecutive calls — cache must serve the second one without re-hitting
	// the client. We can't directly count calls into MockClient, but seeding a
	// different value after the first call lets us prove the cache won.
	first, err := svc.GetRepoMergeMethods(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("first: %v", err)
	}
	if !first.Squash || first.Merge || first.Rebase {
		t.Errorf("first: got %+v, want squash-only", first)
	}

	mc.SetRepoMergeMethods("acme", "widget", RepoMergeMethods{Merge: true, Rebase: true})
	second, err := svc.GetRepoMergeMethods(context.Background(), "acme", "widget")
	if err != nil {
		t.Fatalf("second: %v", err)
	}
	if second != first {
		t.Errorf("expected cached result %+v, got %+v", first, second)
	}
}

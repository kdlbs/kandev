package repoclone

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCloneURL(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		owner    string
		repo     string
		protocol string
		want     string
		wantErr  bool
	}{
		{
			"github SSH",
			"github", "owner", "repo", ProtocolSSH,
			"git@github.com:owner/repo.git", false,
		},
		{
			"github HTTPS",
			"github", "owner", "repo", ProtocolHTTPS,
			"https://github.com/owner/repo.git", false,
		},
		{
			"empty provider defaults to github SSH",
			"", "owner", "repo", ProtocolSSH,
			"git@github.com:owner/repo.git", false,
		},
		{
			"gitlab SSH",
			"gitlab", "owner", "repo", ProtocolSSH,
			"git@gitlab.com:owner/repo.git", false,
		},
		{
			"unsupported provider",
			"unknown", "owner", "repo", ProtocolSSH,
			"", true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CloneURL(tt.provider, tt.owner, tt.repo, tt.protocol)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCloneURLWithHost(t *testing.T) {
	tests := []struct {
		name     string
		provider string
		host     string
		owner    string
		repo     string
		protocol string
		want     string
	}{
		{
			"gitlab self-managed SSH",
			"gitlab", "https://gitlab.acme.corp", "team", "service", ProtocolSSH,
			"git@gitlab.acme.corp:team/service.git",
		},
		{
			"gitlab self-managed HTTPS",
			"gitlab", "https://gitlab.acme.corp/", "team", "service", ProtocolHTTPS,
			"https://gitlab.acme.corp/team/service.git",
		},
		{
			"empty host falls back to provider default",
			"gitlab", "", "team", "service", ProtocolSSH,
			"git@gitlab.com:team/service.git",
		},
		{
			"http scheme honored",
			"gitlab", "http://gitlab.local", "team", "service", ProtocolHTTPS,
			"http://gitlab.local/team/service.git",
		},
		{
			// scp-style "git@host:path" can't carry a port; ssh:// URL
			// form is the only correct shape when one is present.
			"self-managed SSH with port falls back to ssh:// URL",
			"gitlab", "https://gitlab.acme.corp:2222", "team", "service", ProtocolSSH,
			"ssh://git@gitlab.acme.corp:2222/team/service.git",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CloneURLWithHost(tt.provider, tt.host, tt.owner, tt.repo, tt.protocol)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestProviderHost(t *testing.T) {
	tests := []struct {
		provider string
		want     string
		wantErr  bool
	}{
		{"github", "github.com", false},
		{"GitHub", "github.com", false},
		{"", "github.com", false},
		{"gitlab", "gitlab.com", false},
		{"bitbucket", "", true},
		{"unknown", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got, err := providerHost(tt.provider)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDetectGitProtocolPrefersHostConfiguration(t *testing.T) {
	ghPath := filepath.Join(t.TempDir(), "gh")
	script := "#!/bin/sh\n" +
		"if [ \"$3\" = \"-h\" ]; then printf 'ssh\\n'; else printf 'https\\n'; fi\n"
	if err := os.WriteFile(ghPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake gh: %v", err)
	}
	t.Setenv("PATH", filepath.Dir(ghPath))

	resolver := NewGitProtocolResolver()
	if got := resolver.ResolveGitProtocol(context.Background(), "https://github.com/"); got != ProtocolSSH {
		t.Fatalf("ResolveGitProtocol() = %q, want host-specific %q", got, ProtocolSSH)
	}
}

func TestDetectGitProtocolFallbacks(t *testing.T) {
	tests := []struct {
		name       string
		hostOutput string
		hostErr    error
		globalOut  string
		globalErr  error
		want       string
		wantCalls  [][]string
	}{
		{
			name:       "host-specific value wins",
			hostOutput: ProtocolSSH,
			globalOut:  ProtocolHTTPS,
			want:       ProtocolSSH,
			wantCalls:  [][]string{{"config", "get", "-h", "github.com", "git_protocol"}},
		},
		{
			name:      "global value is fallback",
			hostErr:   errors.New("host config unavailable"),
			globalOut: ProtocolHTTPS,
			want:      ProtocolHTTPS,
			wantCalls: [][]string{
				{"config", "get", "-h", "github.com", "git_protocol"},
				{"config", "get", "git_protocol"},
			},
		},
		{
			name:       "unsupported host value falls back",
			hostOutput: "ssh+git",
			globalOut:  ProtocolHTTPS,
			want:       ProtocolHTTPS,
			wantCalls: [][]string{
				{"config", "get", "-h", "github.com", "git_protocol"},
				{"config", "get", "git_protocol"},
			},
		},
		{
			name:       "unsupported values default to ssh",
			hostOutput: "git",
			globalOut:  "http",
			want:       ProtocolSSH,
			wantCalls: [][]string{
				{"config", "get", "-h", "github.com", "git_protocol"},
				{"config", "get", "git_protocol"},
			},
		},
		{
			name:      "command failures default to ssh",
			hostErr:   errors.New("host lookup failed"),
			globalErr: errors.New("global lookup failed"),
			want:      ProtocolSSH,
			wantCalls: [][]string{
				{"config", "get", "-h", "github.com", "git_protocol"},
				{"config", "get", "git_protocol"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls [][]string
			runner := func(_ context.Context, args ...string) ([]byte, error) {
				calls = append(calls, append([]string(nil), args...))
				if len(calls) == 1 {
					return []byte(tt.hostOutput), tt.hostErr
				}
				return []byte(tt.globalOut), tt.globalErr
			}
			resolver := newGitProtocolResolver(runner)

			got := resolver.ResolveGitProtocol(context.Background(), "https://github.com/")
			if got != tt.want {
				t.Fatalf("ResolveGitProtocol() = %q, want %q", got, tt.want)
			}
			if !reflect.DeepEqual(calls, tt.wantCalls) {
				t.Fatalf("gh calls = %v, want %v", calls, tt.wantCalls)
			}
			if len(calls) > 0 && strings.Contains(strings.Join(calls[0], " "), "https://") {
				t.Fatalf("host-specific gh lookup received URL instead of hostname: %v", calls[0])
			}
		})
	}
}

func TestDetectGitProtocolStopsFallbackAfterContextDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	calls := 0
	runner := func(ctx context.Context, _ ...string) ([]byte, error) {
		calls++
		<-ctx.Done()
		return nil, ctx.Err()
	}
	resolver := newGitProtocolResolver(runner)

	if got := resolver.ResolveGitProtocol(ctx, "github.com"); got != ProtocolSSH {
		t.Fatalf("ResolveGitProtocol() = %q, want SSH default", got)
	}
	if calls != 1 {
		t.Fatalf("gh lookup calls = %d, want one lookup after deadline", calls)
	}
}

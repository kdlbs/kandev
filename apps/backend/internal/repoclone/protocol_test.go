package repoclone

import "testing"

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
			"bitbucket HTTPS",
			"bitbucket", "owner", "repo", ProtocolHTTPS,
			"https://bitbucket.org/owner/repo.git", false,
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
		{"bitbucket", "bitbucket.org", false},
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

func TestDetectGitProtocol(t *testing.T) {
	got := DetectGitProtocol()
	if got != ProtocolSSH {
		t.Errorf("expected %q, got %q", ProtocolSSH, got)
	}
}

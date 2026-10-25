package configsync

import (
	"errors"
	"testing"
)

func boolPtr(b bool) *bool { return &b }

func TestSetConfigRequestNormalize_RejectsOmittedProvider(t *testing.T) {
	req := &SetConfigRequest{RepoOwner: "acme", RepoName: "kandev-config"}
	if err := req.Normalize(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
	}
}

func TestSetConfigRequestNormalize_FillsDefaultsWhenProviderGiven(t *testing.T) {
	req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "acme", RepoName: "kandev-config"}
	if err := req.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if req.Provider != ProviderGitHub {
		t.Errorf("Provider = %q, want %q", req.Provider, ProviderGitHub)
	}
	if req.Branch != DefaultBranch {
		t.Errorf("Branch = %q, want %q", req.Branch, DefaultBranch)
	}
	if req.Path == nil || *req.Path != "" {
		t.Errorf("Path = %v, want pointer to \"\" (repository root)", req.Path)
	}
	if req.IntervalSeconds != DefaultIntervalSeconds {
		t.Errorf("IntervalSeconds = %d, want %d", req.IntervalSeconds, DefaultIntervalSeconds)
	}
	if req.PollEnabled == nil || !*req.PollEnabled {
		t.Errorf("PollEnabled = %v, want true", req.PollEnabled)
	}
}

func TestSetConfigRequestNormalize_GitHubRequiresOwnerAndName(t *testing.T) {
	tests := []struct {
		name string
		req  SetConfigRequest
	}{
		{"missing owner", SetConfigRequest{Provider: ProviderGitHub, RepoName: "repo"}},
		{"missing name", SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "owner"}},
		{"owner with slash", SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "a/b", RepoName: "repo"}},
		{"name with space", SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "owner", RepoName: "a b"}},
		{"project_path set on github", SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", ProjectPath: "g/p"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Normalize()
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestSetConfigRequestNormalize_GitLabRequiresNamespacedProjectPath(t *testing.T) {
	tests := []struct {
		name    string
		req     SetConfigRequest
		wantErr bool
	}{
		{"valid nested path", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "group/subgroup/project"}, false},
		{"missing project_path", SetConfigRequest{Provider: ProviderGitLab}, true},
		{"unnamespaced", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "project"}, true},
		{"empty segment", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "group//project"}, true},
		{"traversal segment", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "group/../project"}, true},
		{"repo_owner set on gitlab", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "g/p", RepoOwner: "x"}, true},
		{"space in path", SetConfigRequest{Provider: ProviderGitLab, ProjectPath: "g/p roject"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.req.Normalize()
			if tt.wantErr && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Normalize() unexpected error = %v", err)
			}
		})
	}
}

func TestSetConfigRequestNormalize_UnknownProvider(t *testing.T) {
	req := &SetConfigRequest{Provider: "bitbucket", RepoOwner: "o", RepoName: "r"}
	if err := req.Normalize(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
	}
}

func TestSetConfigRequestNormalize_NilPathMeansRepositoryRoot(t *testing.T) {
	req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", Path: nil}
	if err := req.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if req.Path == nil || *req.Path != "" {
		t.Errorf("Path = %v, want pointer to \"\" (repository root)", req.Path)
	}
}

func TestSetConfigRequestNormalize_PathValidation(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"empty means repository root", "", false},
		{"plain subdirectory", "foo/bar", false},
		{"rejects leading slash", "/foo/bar", true},
		{"rejects trailing slash", "foo/bar/", true},
		{"rejects backslash", `foo\bar`, true},
		{"rejects repeated slash", "foo//bar", true},
		{"rejects traversal", "foo/../bar", true},
		{"rejects dot segment", "foo/./bar", true},
		{"rejects whitespace-only", "   ", true},
		{"rejects surrounding whitespace", " foo/bar ", true},
		{"rejects NUL byte", "foo\x00bar", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", Path: &tt.path}
			err := req.Normalize()
			if tt.wantErr && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Normalize() unexpected error = %v", err)
			}
		})
	}
}

func TestSetConfigRequestNormalize_IntervalBounds(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		wantErr  bool
	}{
		{"zero falls back to default", 0, false},
		{"below minimum", MinIntervalSeconds - 1, true},
		{"at minimum", MinIntervalSeconds, false},
		{"at maximum", MaxIntervalSeconds, false},
		{"above maximum", MaxIntervalSeconds + 1, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", IntervalSeconds: tt.interval}
			err := req.Normalize()
			if tt.wantErr && !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Normalize() unexpected error = %v", err)
			}
		})
	}
}

func TestSetConfigRequestNormalize_PollEnabledExplicitFalse(t *testing.T) {
	req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", PollEnabled: boolPtr(false)}
	if err := req.Normalize(); err != nil {
		t.Fatalf("Normalize() error = %v", err)
	}
	if req.PollEnabled == nil || *req.PollEnabled {
		t.Errorf("PollEnabled = %v, want false preserved", req.PollEnabled)
	}
}

func TestSetConfigRequestNormalize_InvalidBranchName(t *testing.T) {
	req := &SetConfigRequest{Provider: ProviderGitHub, RepoOwner: "o", RepoName: "r", Branch: "-flag-looking"}
	if err := req.Normalize(); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Normalize() error = %v, want ErrInvalidConfig", err)
	}
}

func TestNormalizePathFrame(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"/foo/bar/", "foo/bar"},
		{"foo/bar", "foo/bar"},
		{"  foo/bar ", "foo/bar"},
		{`foo\bar`, "foo/bar"},
		{"", ""},
		{"///", ""},
	}
	for _, tt := range tests {
		if got := normalizePathFrame(tt.input); got != tt.want {
			t.Errorf("normalizePathFrame(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

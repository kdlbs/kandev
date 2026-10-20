package service

import (
	"errors"
	"testing"
)

func TestNormalizeInitialWorkspaceLayout(t *testing.T) {
	tests := []struct {
		name      string
		requested string
		repos     int
		executor  string
		want      string
		wantErr   bool
	}{
		{name: "single defaults to repository", repos: 1, want: WorkspaceLayoutRepository},
		{name: "single accepts task root", requested: WorkspaceLayoutTaskRoot, repos: 1, want: WorkspaceLayoutTaskRoot},
		{name: "multiple resolves to task root", requested: WorkspaceLayoutRepository, repos: 2, want: WorkspaceLayoutTaskRoot},
		{name: "repositoryless stays empty", want: ""},
		{name: "repositoryless rejects task root", requested: WorkspaceLayoutTaskRoot, wantErr: true},
		{name: "unknown value rejects", requested: "workspace", repos: 1, wantErr: true},
		{name: "unsupported executor rejects explicit task root", requested: WorkspaceLayoutTaskRoot, repos: 1, executor: "ssh", wantErr: true},
		{name: "unsupported executor keeps multiple repositories repository rooted", repos: 2, executor: "ssh", want: WorkspaceLayoutRepository},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeInitialWorkspaceLayout(tt.requested, tt.repos, tt.executor)
			if tt.wantErr {
				if err == nil || !errors.Is(err, ErrInvalidInitialWorkspaceLayout) {
					t.Fatalf("expected invalid layout error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeInitialWorkspaceLayout() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeInitialWorkspaceLayout() = %q, want %q", got, tt.want)
			}
		})
	}
}

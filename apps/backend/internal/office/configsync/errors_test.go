package configsync

import (
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/gitlab"
)

func TestClassifyFetchErr_GitHub(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", http.StatusNotFound, ErrNotFound},
		{"401 is unavailable", http.StatusUnauthorized, ErrUnavailable},
		{"403 is unavailable", http.StatusForbidden, ErrUnavailable},
		{"429 is unavailable", http.StatusTooManyRequests, ErrUnavailable},
		{"500 is unavailable", http.StatusInternalServerError, ErrUnavailable},
		{"422 is unreadable residue", http.StatusUnprocessableEntity, ErrUnreadable},
		{"400 is unreadable residue", http.StatusBadRequest, ErrUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &github.GitHubAPIError{StatusCode: tt.status, Endpoint: "/repos/o/r/contents/x"}
			got := classifyFetchErr(src)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyFetchErr(%d) = %v, want class %v", tt.status, got, tt.want)
			}
			if !errors.Is(got, src) {
				t.Errorf("classifyFetchErr(%d) lost the underlying cause via Unwrap", tt.status)
			}
		})
	}
}

func TestClassifyFetchErr_GitLab(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   error
	}{
		{"404 is not found", http.StatusNotFound, ErrNotFound},
		{"403 is unavailable", http.StatusForbidden, ErrUnavailable},
		{"418 is unreadable residue", http.StatusTeapot, ErrUnreadable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := &gitlab.APIError{StatusCode: tt.status, Endpoint: "/projects/1/repository/tree"}
			got := classifyFetchErr(src)
			if !errors.Is(got, tt.want) {
				t.Fatalf("classifyFetchErr(%d) = %v, want class %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestClassifyFetchErr_TransportErrorIsResidue(t *testing.T) {
	src := errors.New("dial tcp: connection refused")
	got := classifyFetchErr(src)
	if !errors.Is(got, ErrUnreadable) {
		t.Fatalf("classifyFetchErr(transport) = %v, want ErrUnreadable", got)
	}
	if !errors.Is(got, src) {
		t.Errorf("classifyFetchErr(transport) lost the underlying cause via Unwrap")
	}
}

func TestClassifyFetchErr_Nil(t *testing.T) {
	if got := classifyFetchErr(nil); got != nil {
		t.Fatalf("classifyFetchErr(nil) = %v, want nil", got)
	}
}

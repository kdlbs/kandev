package worktree

import (
	"errors"
	"testing"
)

// TestCreateRequest_Validate_FallsBackToFallbackBaseBranch pins the
// defence-in-depth path added with the add_branch_to_task fix: when the
// caller forgot to set BaseBranch but supplied a FallbackBaseBranch
// (typically the repository's persisted default_branch), Validate adopts
// the fallback instead of returning ErrInvalidBaseBranch.
func TestCreateRequest_Validate_FallsBackToFallbackBaseBranch(t *testing.T) {
	r := &CreateRequest{
		TaskID:             "task-1",
		RepositoryPath:     "/tmp/repo",
		BaseBranch:         "",
		FallbackBaseBranch: "main",
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate with fallback should succeed: %v", err)
	}
	if r.BaseBranch != "main" {
		t.Errorf("expected BaseBranch promoted from fallback, got %q", r.BaseBranch)
	}
}

func TestCreateRequest_Validate_NoFallbackStillRejects(t *testing.T) {
	r := &CreateRequest{
		TaskID:         "task-1",
		RepositoryPath: "/tmp/repo",
	}
	if err := r.Validate(); !errors.Is(err, ErrInvalidBaseBranch) {
		t.Errorf("expected ErrInvalidBaseBranch, got %v", err)
	}
}

func TestCreateRequest_Validate_ExplicitBaseWinsOverFallback(t *testing.T) {
	r := &CreateRequest{
		TaskID:             "task-1",
		RepositoryPath:     "/tmp/repo",
		BaseBranch:         "develop",
		FallbackBaseBranch: "main",
	}
	if err := r.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if r.BaseBranch != "develop" {
		t.Errorf("expected explicit BaseBranch to be preserved, got %q", r.BaseBranch)
	}
}

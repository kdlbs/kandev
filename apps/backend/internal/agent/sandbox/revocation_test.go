package sandbox

import (
	"errors"
	"testing"
)

func TestSandboxPolicyRevokedSelectionDoesNotFallBack(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	ctx := operatorContext(t)
	if _, err := s.PutBinding(ctx, Binding{Scope: ScopeTask, TargetID: "task", RevisionID: read.ID}, 0); err != nil {
		t.Fatal(err)
	}
	if err := s.Revoke(ctx, read.ID, read.Digest); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(ctx, SelectionContext{OrgID: "org", ExecutorID: "executor", TaskID: "task"}); !errors.Is(err, ErrIneligible) {
		t.Fatalf("revoked task policy fell back: %v", err)
	}
	got, err := s.GetRevision(ctx, read.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Revoked || got.Digest != read.Digest {
		t.Fatal("revocation lost immutable content")
	}
}

func TestSandboxPolicyDisabledAfterSelection(t *testing.T) {
	s, _, _ := policySelectionFixture(t)
	disabled, err := NewService(s.db, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = disabled.Resolve(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "executor"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("disabled required policy: %v", err)
	}
	if got, err := disabled.Resolve(t.Context(), SelectionContext{OrgID: "org", ExecutorID: "ordinary"}); err != nil || got != nil {
		t.Fatalf("ordinary wrapper blocked: %v", err)
	}
}

func TestSandboxAuthorityRevocation(t *testing.T) {
	s, _, read := policySelectionFixture(t)
	if err := s.Revoke(t.Context(), read.ID, read.Digest); !errors.Is(err, ErrAuthority) {
		t.Fatalf("agent revoked policy: %v", err)
	}
	if err := s.Revoke(operatorContext(t), read.ID, "wrong-digest"); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale revocation: %v", err)
	}
}

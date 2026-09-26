package gitcredentials

import (
	"errors"
	"testing"
	"time"
)

func TestBrokerReissueRestoresLeaseAfterBrokerRestart(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	authorizer := &recordingAuthorizer{}
	signer, err := NewReissueCapabilitySigner("stable-test-key")
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	beforeRestart := NewBroker(provider, authorizer)
	beforeRestart.SetReissueCapabilitySigner(signer)
	lease, capability, err := beforeRestart.IssueWithReissueCapability(t.Context(), testScope())
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	if _, err := beforeRestart.Redeem(t.Context(), testRedemption(lease.Token)); err != nil {
		t.Fatalf("Redeem() before restart error = %v", err)
	}

	afterRestart := NewBroker(provider, authorizer)
	afterRestart.SetReissueCapabilitySigner(signer)
	reissued, err := afterRestart.Reissue(t.Context(), ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	})
	if err != nil {
		t.Fatalf("Reissue() after restart error = %v", err)
	}
	if reissued.Token == "" || reissued.Token == lease.Token {
		t.Fatalf("reissued lease = %#v, original = %#v", reissued, lease)
	}
	if _, err := afterRestart.Redeem(t.Context(), testRedemption(reissued.Token)); err != nil {
		t.Fatalf("Redeem() reissued lease error = %v", err)
	}
}

func TestBrokerReissueRecoversRotatedBinding(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	signer, err := NewReissueCapabilitySigner("stable-test-key")
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	broker.SetReissueCapabilitySigner(signer)
	lease, capability, err := broker.IssueWithReissueCapability(t.Context(), testScope())
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	provider.mu.Lock()
	provider.binding = "generation-2"
	provider.mu.Unlock()
	if _, err := broker.Redeem(t.Context(), testRedemption(lease.Token)); !errors.Is(err, ErrLeaseRevoked) {
		t.Fatalf("Redeem() after rotation error = %v, want revoked lease", err)
	}
	reissued, err := broker.Reissue(t.Context(), ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	})
	if err != nil {
		t.Fatalf("Reissue() after rotation error = %v", err)
	}
	if _, err := broker.Redeem(t.Context(), testRedemption(reissued.Token)); err != nil {
		t.Fatalf("Redeem() reissued lease error = %v", err)
	}
}

func TestBrokerReissueReplacesPreviousCapabilityLease(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	signer, err := NewReissueCapabilitySigner("stable-test-key")
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	broker.SetReissueCapabilitySigner(signer)
	_, capability, err := broker.IssueWithReissueCapability(t.Context(), testScope())
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	request := ReissueRequest{
		Capability: capability.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1",
		Host: "bitbucket.example", Path: "/scm/ENG/widgets.git",
	}
	first, err := broker.Reissue(t.Context(), request)
	if err != nil {
		t.Fatalf("first Reissue() error = %v", err)
	}
	second, err := broker.Reissue(t.Context(), request)
	if err != nil {
		t.Fatalf("second Reissue() error = %v", err)
	}
	if first.Token == second.Token {
		t.Fatal("two reissues returned the same lease token")
	}
	if got, want := broker.ActiveLeaseCount(), 1; got != want {
		t.Fatalf("active leases after repeated reissue = %d, want %d", got, want)
	}
	if _, err := broker.Redeem(t.Context(), testRedemption(first.Token)); !errors.Is(err, ErrLeaseInvalid) {
		t.Fatalf("Redeem(first replacement) error = %v, want invalid lease", err)
	}
	if _, err := broker.Redeem(t.Context(), testRedemption(second.Token)); err != nil {
		t.Fatalf("Redeem(second replacement) error = %v", err)
	}
}

func TestBrokerReissueRejectsForgedExpiredAndMismatchedCapabilities(t *testing.T) {
	provider := &rotatingProvider{binding: "generation-1", username: "user", secret: "secret"}
	broker := NewBroker(provider, &recordingAuthorizer{})
	signer, err := NewReissueCapabilitySigner("stable-test-key")
	if err != nil {
		t.Fatalf("NewReissueCapabilitySigner() error = %v", err)
	}
	broker.SetReissueCapabilitySigner(signer)
	_, capability, err := broker.IssueWithReissueCapability(t.Context(), testScope())
	if err != nil {
		t.Fatalf("IssueWithReissueCapability() error = %v", err)
	}
	request := ReissueRequest{Capability: capability.Token, TaskID: "task-1", SessionID: "session-1", RepositoryID: "repository-1", Host: "bitbucket.example", Path: "/scm/ENG/widgets.git"}

	request.Capability += "forged"
	if _, err := broker.Reissue(t.Context(), request); !errors.Is(err, ErrReissueCapabilityInvalid) {
		t.Fatalf("forged Reissue() error = %v, want invalid capability", err)
	}
	request.Capability = capability.Token
	request.RepositoryID = "other-repository"
	if _, err := broker.Reissue(t.Context(), request); !errors.Is(err, ErrScopeDenied) {
		t.Fatalf("mismatched Reissue() error = %v, want scope denied", err)
	}

	expired, err := signer.Issue(testScope(), time.Now().Add(-reissueCapabilityTTL-time.Second))
	if err != nil {
		t.Fatalf("Issue() expired capability error = %v", err)
	}
	request.Capability = expired.Token
	request.RepositoryID = "repository-1"
	if _, err := broker.Reissue(t.Context(), request); !errors.Is(err, ErrReissueCapabilityExpired) {
		t.Fatalf("expired Reissue() error = %v, want expired capability", err)
	}
}

package github

import (
	"context"
	"testing"
	"time"
)

type publicationBarrierSecretManager struct {
	*mockSecretManager
	persisted chan struct{}
	release   chan struct{}
}

func (m *publicationBarrierSecretManager) Update(ctx context.Context, id, value string) error {
	if err := m.mockSecretManager.Update(ctx, id, value); err != nil {
		return err
	}
	close(m.persisted)
	<-m.release
	return nil
}

// @covers AC-INTEGRATIONS-GITHUB-RATE-003.8
func TestLegacyFingerprintWaitsForCredentialPublication(t *testing.T) {
	svc, secrets := setupLegacyRevisionTest(t)
	ctx := context.Background()
	mgr := &publicationBarrierSecretManager{mockSecretManager: secrets, persisted: make(chan struct{}), release: make(chan struct{})}
	svc.secretManager = mgr
	if err := svc.ConfigureToken(ctx, "alice"); err != nil {
		t.Fatal(err)
	}
	request := ResolveCredentialRequest{WorkspaceID: "work", Purpose: CredentialPurposeAutomation}
	if _, err := svc.resolver.Resolve(ctx, request); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- svc.ConfigureToken(ctx, "bob") }()
	<-mgr.persisted
	fingerprint := make(chan string, 1)
	go func() {
		value, err := svc.WorkspaceConnectionFingerprint(ctx, "work")
		if err != nil {
			t.Error(err)
		}
		fingerprint <- value
	}()
	early := false
	select {
	case <-fingerprint:
		early = true
		t.Error("replacement fingerprint escaped before credential publication")
	case <-time.After(100 * time.Millisecond):
	}
	close(mgr.release)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if !early && <-fingerprint == "" {
		t.Error("published fingerprint is empty")
	}
	next, err := svc.resolver.Resolve(ctx, request)
	if err != nil {
		t.Fatal(err)
	}
	if next.Principal.Login != "bob" {
		t.Fatalf("published principal = %s", next.Principal.Login)
	}
}

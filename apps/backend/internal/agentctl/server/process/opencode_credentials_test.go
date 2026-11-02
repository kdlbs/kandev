package process

import (
	"errors"
	"testing"
	"time"
)

func TestOpenCodeCredentialAdmissionWaitsForProcessTreeReap(t *testing.T) {
	for _, tc := range []struct {
		name     string
		reapErr  error
		wantOpen bool
	}{
		{name: "clean reap"},
		{name: "failed reap", reapErr: errors.New("process tree remains alive"), wantOpen: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			manager := &Manager{credentialImport: true}
			done := make(chan struct{})
			proc := &PipedProcess{Done: done, process: &commandProcess{reapErr: tc.reapErr}}
			cleanupDone := manager.holdCredentialAdmissionUntilReaped(proc)
			if err := manager.releaseOpenCodeCredentialAdmission(); err != nil {
				t.Fatalf("release credential admission: %v", err)
			}

			manager.startMu.Lock()
			if !manager.credentialImport || !manager.credentialImportBlocked {
				t.Fatal("credential admission opened before process tree reaping completed")
			}
			manager.startMu.Unlock()

			close(done)
			select {
			case <-cleanupDone:
			case <-time.After(time.Second):
				t.Fatal("credential cleanup did not observe process tree completion")
			}

			manager.startMu.Lock()
			defer manager.startMu.Unlock()
			if got := manager.credentialImport; got != tc.wantOpen {
				t.Fatalf("credential admission blocked = %v, want %v", got, tc.wantOpen)
			}
		})
	}
}

func TestOpenCodeCredentialStatusRejectsStoppingAgent(t *testing.T) {
	manager := &Manager{}
	manager.status.Store(StatusStopping)

	if _, err := manager.OpenCodeCredentialStatus(t.Context()); err == nil {
		t.Fatal("credential status accepted an agent that is still stopping")
	}
}

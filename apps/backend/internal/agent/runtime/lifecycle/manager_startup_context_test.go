package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
)

func TestAgentStartupContextPreservesResumeOwnerCancellation(t *testing.T) {
	for _, owned := range []bool{false, true} {
		name := "ordinary caller"
		if owned {
			name = "resume owner"
		}
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				owner, cancelOwner := context.WithCancel(context.Background())
				defer cancelOwner()
				if owned {
					owner = WithResumeAttemptID(owner, "resume-owner")
				}
				shared, cancelShared := context.WithCancel(context.WithoutCancel(owner))
				defer cancelShared()
				startup, release := agentStartupContext(owner, shared)
				defer release()
				if ResumeAttemptIDFromContext(startup) != ResumeAttemptIDFromContext(owner) {
					t.Fatal("startup lost its immutable resume identity")
				}
				cancelOwner()
				synctest.Wait()
				if (startup.Err() != nil) != owned {
					t.Fatalf("startup cancellation = %v, owned = %v", startup.Err(), owned)
				}
				if shared.Err() != nil {
					t.Fatal("owner cancellation ended caller-independent shared work")
				}
				cancelShared()
				synctest.Wait()
				if startup.Err() == nil {
					t.Fatal("manager shutdown did not cancel startup")
				}
			})
		})
	}
}

func TestAgentStartupContextRejectsAlreadyCancelledResumeOwner(t *testing.T) {
	owner, cancel := context.WithCancel(WithResumeAttemptID(context.Background(), "cancelled-owner"))
	cancel()
	shared := context.WithoutCancel(owner)
	startup, release := agentStartupContext(owner, shared)
	defer release()
	if startup.Err() == nil {
		t.Fatal("cancelled owner could admit detached startup work")
	}
}

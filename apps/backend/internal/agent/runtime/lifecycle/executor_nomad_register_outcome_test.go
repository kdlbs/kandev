package lifecycle

import (
	"context"
	"testing"
)

// A registration whose PUT was sent must leave a durable cleanup record when
// the launch cannot confirm the job, so the possibly registered job is stopped.
func TestNomadLaunchRegistrationOutcomeFailuresLeaveCleanupRecord(t *testing.T) {
	cases := map[string]func() *nomadCreateFixture{
		"server error on register": func() *nomadCreateFixture {
			f := newNomadCreateFixture(t, true)
			f.jobHistoryMissing = true
			return f
		},
		"read-back fails after a lost response": func() *nomadCreateFixture {
			f := newNomadCreateFixture(t, false)
			f.loseRegisterResponse, f.jobHistoryMissing = true, true
			return f
		},
	}
	for name, build := range cases {
		t.Run(name, func(t *testing.T) {
			f := build()
			coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
			instance, err := f.runtime.CreateInstance(context.Background(), f.request)
			if instance != nil || err == nil {
				t.Fatalf("CreateInstance() = %v, %v; want a launch error", instance, err)
			}
			if coordinator.launches != 1 {
				t.Fatalf("rollback launches = %d, want 1 durable cleanup after the registration PUT", coordinator.launches)
			}
			if final := f.lastCheckpoint(t); final.Phase != nomadPhaseCleanupPending {
				t.Fatalf("phase = %q, want cleanup_pending", final.Phase)
			}
		})
	}
}

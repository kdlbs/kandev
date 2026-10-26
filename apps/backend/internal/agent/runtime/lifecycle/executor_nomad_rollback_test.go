package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/nomad"
)

func TestNomadDestroyIdentityLaunchRollback(t *testing.T) {
	t.Run("connect failure keeps the original error and retains cleanup", func(t *testing.T) {
		baseline := newNomadCreateFixture(t, false)
		baseline.rejectBootstrap = true
		_, want := baseline.runtime.CreateInstance(context.Background(), baseline.request)
		if want == nil {
			t.Fatal("baseline launch unexpectedly succeeded")
		}

		f := newNomadCreateFixture(t, false)
		f.rejectBootstrap = true
		coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
		instance, err := f.runtime.CreateInstance(context.Background(), f.request)
		if instance != nil || err == nil || err.Error() != want.Error() || errors.Is(err, ErrNomadCleanupRetained) {
			t.Fatalf("CreateInstance() = %v, %v; want the original launch error %q unchanged", instance, err, want)
		}
		if coordinator.launches != 1 || !errors.Is(coordinator.lastErr, ErrNomadCleanupRetained) {
			t.Fatalf("rollback launches=%d err=%v, want one retained cleanup", coordinator.launches, coordinator.lastErr)
		}
		final := f.lastCheckpoint(t)
		if final.Phase != nomadPhaseCleanupPending || final.QuarantineState != nomad.QuarantineMarked {
			t.Fatalf("phase=%q quarantine=%q, want retained cleanup_pending and quarantined", final.Phase, final.QuarantineState)
		}
		assertNoAbsenceWithoutReceipt(t, f)
		if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 {
			t.Fatalf("drain=%d stop=%d release=%d for a launch with no recorded runtime identity", f.cleanup.drainCalls, f.cleanup.stopCalls, f.cleanup.releaseCalls)
		}
		if f.fake.eligibility != "ineligible" || !strings.Contains(f.fake.marker, nomadCleanupReasonIdentityUnrecorded) {
			t.Fatalf("worker not quarantined: eligibility=%s marker=%q", f.fake.eligibility, f.fake.marker)
		}
		if f.registerCalls != 1 || f.createCalls != 0 {
			t.Fatalf("registrations=%d instances=%d", f.registerCalls, f.createCalls)
		}
	})

	t.Run("unrecorded identity or token quarantines without draining", func(t *testing.T) {
		for name, mutate := range map[string]func(*nomadCreateFixture){
			"server identity": func(f *nomadCreateFixture) {
				setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) { s.ServerIdentity = "" })
			},
			"auth token": func(f *nomadCreateFixture) { f.request.AuthToken = "" },
		} {
			t.Run(name, func(t *testing.T) {
				f := prepareNomadCleanupFixture(t)
				mutate(f)
				coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
				f.runtime.rollbackNomadLaunch(context.Background(), f.request)
				if coordinator.launches != 1 || !errors.Is(coordinator.lastErr, ErrNomadCleanupRetained) {
					t.Fatalf("rollback launches=%d err=%v", coordinator.launches, coordinator.lastErr)
				}
				if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 0 || f.cleanup.releaseCalls != 0 {
					t.Fatalf("drain=%d stop=%d release=%d", f.cleanup.drainCalls, f.cleanup.stopCalls, f.cleanup.releaseCalls)
				}
				final := f.lastCheckpoint(t)
				if final.Phase != nomadPhaseCleanupPending || final.QuarantineState != nomad.QuarantineMarked || f.fake.eligibility != "ineligible" {
					t.Fatalf("phase=%q quarantine=%q eligibility=%s", final.Phase, final.QuarantineState, f.fake.eligibility)
				}
				assertNoAbsenceWithoutReceipt(t, f)
			})
		}
	})

	t.Run("recorded identity drains once and reports absence only with a receipt", func(t *testing.T) {
		f := prepareNomadCleanupFixture(t)
		coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
		f.runtime.rollbackNomadLaunch(context.Background(), f.request)
		if coordinator.lastErr != nil {
			t.Fatalf("rollback cleanup = %v", coordinator.lastErr)
		}
		if f.cleanup.drainCalls != 1 || f.cleanup.stopCalls != 1 || f.lastCheckpoint(t).Phase != nomadPhaseAbsent {
			t.Fatalf("drain=%d stop=%d phase=%q", f.cleanup.drainCalls, f.cleanup.stopCalls, f.lastCheckpoint(t).Phase)
		}
		assertNoAbsenceWithoutReceipt(t, f)
	})
}

func TestNomadLaunchFailuresAfterRegistrationRollBack(t *testing.T) {
	cases := map[string]struct {
		setup func(*nomadCreateFixture) context.Context
		want  string
	}{
		"allocation wait timeout": {
			setup: func(f *nomadCreateFixture) context.Context {
				f.allocationsPending = true
				ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
				t.Cleanup(cancel)
				return ctx
			},
			want: "wait for exact running Nomad allocation",
		},
		"multiple allocations": {
			setup: func(f *nomadCreateFixture) context.Context {
				f.multipleAllocations = true
				return context.Background()
			},
			want: "multiple allocations",
		},
		"allocation checkpoint failure": {
			setup: func(f *nomadCreateFixture) context.Context {
				inner := f.request.CheckpointRuntimeInventory
				f.request.CheckpointRuntimeInventory = func(ctx context.Context, metadata map[string]interface{}) error {
					if state, err := nomad.EnvironmentStateFromMetadata(metadata); err == nil && state.Phase == nomadPhaseAllocated {
						return errors.New("checkpoint unavailable")
					}
					return inner(ctx, metadata)
				}
				return context.Background()
			},
			want: "checkpoint running Nomad allocation",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := newNomadCreateFixture(t, false)
			coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
			ctx := tc.setup(f)
			instance, err := f.runtime.CreateInstance(ctx, f.request)
			if instance != nil || err == nil || !strings.Contains(err.Error(), tc.want) || errors.Is(err, ErrNomadCleanupRetained) {
				t.Fatalf("CreateInstance() = %v, %v; want the original launch error containing %q", instance, err, tc.want)
			}
			if coordinator.launches != 1 {
				t.Fatalf("rollback launches = %d, want 1", coordinator.launches)
			}
			final := f.lastCheckpoint(t)
			if final.Phase != nomadPhaseCleanupPending {
				t.Fatalf("phase=%q, want cleanup_pending", final.Phase)
			}
			if f.registerCalls != 1 || f.cleanup.drainCalls != 0 {
				t.Fatalf("registrations=%d drain=%d", f.registerCalls, f.cleanup.drainCalls)
			}
			assertNoAbsenceWithoutReceipt(t, f)
		})
	}
}

func assertNoAbsenceWithoutReceipt(t *testing.T, f *nomadCreateFixture) {
	t.Helper()
	for _, state := range f.checkpoints {
		if state.Phase == nomadPhaseAbsent && (len(state.DrainReceiptDigest) != 64 || state.DrainReceiptGeneration != state.Generation) {
			t.Fatalf("absence recorded without a current-generation receipt: %+v", state)
		}
	}
}

func TestNomadDestroyIdentityRefusedBeforeStageSettlesWithoutQuarantine(t *testing.T) {
	f := prepareNomadCleanupFixture(t)
	setNomadFixtureState(t, f, func(s *nomad.EnvironmentState) {
		s.ServerIdentity = ""
		s.RefusedBeforeStage = true
	})
	coordinator := newNomadFixtureCoordinator(f, &claimStoreWriter{})
	f.runtime.rollbackNomadLaunch(context.Background(), f.request)
	if coordinator.lastErr != nil {
		t.Fatalf("rollback cleanup = %v", coordinator.lastErr)
	}
	if f.fake.eligibility == "ineligible" || f.fake.marker != "" {
		t.Fatalf("refused launch quarantined the node: eligibility=%s marker=%q", f.fake.eligibility, f.fake.marker)
	}
	if f.cleanup.drainCalls != 0 || f.cleanup.stopCalls != 1 {
		t.Fatalf("drain=%d stop=%d, want no drain and one job stop", f.cleanup.drainCalls, f.cleanup.stopCalls)
	}
	if final := f.lastCheckpoint(t); final.Phase != nomadPhaseAbsent || final.QuarantineState != "" {
		t.Fatalf("phase=%q quarantine=%q, want absent without quarantine", final.Phase, final.QuarantineState)
	}
}

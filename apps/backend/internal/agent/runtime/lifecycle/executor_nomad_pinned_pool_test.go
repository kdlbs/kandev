package lifecycle

import (
	"context"
	"testing"
)

func TestNomadPinnedLaunchUsesPinnedNodeActualPool(t *testing.T) {
	f := newNomadCreateFixture(t, false)
	f.fake.nodePool = "kandev"
	launchPinnedNomadFixture(t, f)
	if f.job == nil || f.job.Job.NodePool != "kandev" {
		t.Fatalf("registered job = %+v, want node pool kandev", f.job)
	}
	for _, cp := range f.checkpoints {
		if cp.PinnedNodePool != "kandev" {
			t.Fatalf("checkpoint phase %s pinned pool = %q, want kandev", cp.Phase, cp.PinnedNodePool)
		}
	}
}

func TestNomadPinnedLaunchFallsBackToDefaultPoolWhenNodeReportsNone(t *testing.T) {
	f := newNomadCreateFixture(t, false)
	launchPinnedNomadFixture(t, f)
	if f.job == nil || f.job.Job.NodePool != "default" {
		t.Fatalf("registered job = %+v, want node pool default", f.job)
	}
}

func launchPinnedNomadFixture(t *testing.T, f *nomadCreateFixture) {
	t.Helper()
	instance, err := f.runtime.CreateInstance(context.Background(), f.request)
	if err != nil {
		t.Fatalf("CreateInstance: %v", err)
	}
	t.Cleanup(func() { _ = f.runtime.StopInstance(context.Background(), instance, false) })
}

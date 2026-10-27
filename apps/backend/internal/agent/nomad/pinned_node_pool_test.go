package nomad

import "testing"

func TestPinnedJobCarriesRecordedNodePoolAndRebuildsIdentically(t *testing.T) {
	state := validTestState()
	state.PinnedNodePool = "kandev"
	job, _, state := retainedJob(t, validTestProfileConfig(), state)
	if job.NodePool != "kandev" {
		t.Fatalf("job node pool = %q, want kandev", job.NodePool)
	}
	rebuilt, err := NativeJobFromState(state)
	if err != nil {
		t.Fatalf("NativeJobFromState with retained digest: %v", err)
	}
	if rebuilt.NodePool != "kandev" {
		t.Fatalf("rebuilt node pool = %q, want kandev", rebuilt.NodePool)
	}
	want, _ := NativeJobSpecDigest(job)
	got, _ := NativeJobSpecDigest(rebuilt)
	if want != got {
		t.Fatalf("rebuilt digest %s differs from retained %s", got, want)
	}
	state.PinnedNodePool = "other"
	if _, err := NativeJobFromState(state); err == nil {
		t.Fatal("a changed pinned node pool was accepted against the retained digest")
	}
}

func TestLegacyPinnedStateWithoutNodePoolStillUsesDefault(t *testing.T) {
	job, _, state := retainedJob(t, validTestProfileConfig(), validTestState())
	if job.NodePool != "default" || state.PinnedNodePool != "" {
		t.Fatalf("legacy job node pool = %q, state pool %q", job.NodePool, state.PinnedNodePool)
	}
	rebuilt, err := NativeJobFromState(state)
	if err != nil || rebuilt.NodePool != "default" {
		t.Fatalf("legacy rebuild pool=%q err=%v", rebuilt.NodePool, err)
	}
}

func TestPinnedNodePoolMustBeSafeAndStayOutOfPoolFields(t *testing.T) {
	state := validTestState()
	state.PinnedNodePool = "kandev"
	if err := state.Validate(); err != nil {
		t.Fatalf("pinned state with a node pool rejected: %v", err)
	}
	state.PinnedNodePool = "bad pool/../x"
	if err := state.Validate(); err == nil {
		t.Fatal("unsafe pinned node pool accepted")
	}
}

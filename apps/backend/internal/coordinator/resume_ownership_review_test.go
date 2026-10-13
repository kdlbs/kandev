package coordinator

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type ownedResumeProbe struct {
	entered chan struct{}
	exited  chan struct{}
	release chan struct{}
	calls   atomic.Int32
}

func (r *ownedResumeProbe) ResumeTaskSession(ctx context.Context, _, _ string) (bool, error) {
	r.calls.Add(1)
	close(r.entered)
	defer close(r.exited)
	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case <-r.release:
		return false, nil
	}
}
func TestResumeShutdownCancelsDetachedLaunchAndClosesAdmission(t *testing.T) {
	f := newKindsFixture(t)
	r := &ownedResumeProbe{entered: make(chan struct{}), exited: make(chan struct{}), release: make(chan struct{})}
	f.svc.SetKindDeps(KindDeps{Resumer: r})
	k := &resumeKind{svc: f.svc}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { defer close(done); _, _ = k.launch(ctx, Claim{TargetTaskID: "task-0"}, "s1") }()
	<-r.entered
	cancel()
	<-done
	stopped := make(chan struct{})
	go func() { f.svc.WaitResumeLaunchesStopped(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		close(r.release)
		<-stopped
		t.Fatal("resume shutdown did not cancel owned launch")
	}
	select {
	case <-r.exited:
	default:
		t.Fatal("shutdown returned before resume exited")
	}
	if _, err := k.launch(t.Context(), Claim{TargetTaskID: "task-0"}, "s1"); err == nil {
		t.Fatal("launch admitted after shutdown")
	}
	f.svc.WaitResumeLaunchesStopped()
	if r.calls.Load() != 1 {
		t.Fatal("late launch reached resumer")
	}
}

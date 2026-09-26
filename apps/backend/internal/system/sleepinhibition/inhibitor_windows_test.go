//go:build windows

package sleepinhibition

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestWindowsLeaseReleaseClearsExecutionStateOnce(t *testing.T) {
	recorder := &windowsStateRecorder{}
	inhibitor := newWindowsInhibitor(recorder.call)

	lease, err := inhibitor.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("second release: %v", err)
	}

	if got := recorder.states(); len(got) != 2 || got[0] != esInhibitionState || got[1] != esContinuous {
		t.Fatalf("execution-state calls = %#v, want [%#x %#x]", got, esInhibitionState, esContinuous)
	}
}

func TestWindowsLeaseIgnoresContextCancellationAfterHandoff(t *testing.T) {
	recorder := &windowsStateRecorder{}
	inhibitor := newWindowsInhibitor(recorder.call)
	ctx, cancel := context.WithCancel(context.Background())

	lease, err := inhibitor.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	cancel()
	select {
	case <-lease.Done():
		t.Fatal("request cancellation ended an owned lease")
	case <-time.After(100 * time.Millisecond):
	}
	if got := recorder.states(); len(got) != 1 || got[0] != esInhibitionState {
		t.Fatalf("execution-state calls after cancellation = %#v", got)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
}

func TestWindowsLeaseReportsResetError(t *testing.T) {
	resetErr := errors.New("reset failed")
	recorder := &windowsStateRecorder{resetErr: resetErr}
	inhibitor := newWindowsInhibitor(recorder.call)

	lease, err := inhibitor.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if err := lease.Release(); !errors.Is(err, resetErr) {
		t.Fatalf("release error = %v, want %v", err, resetErr)
	}
}

type windowsStateRecorder struct {
	mu       sync.Mutex
	called   []uint32
	resetErr error
}

func (r *windowsStateRecorder) call(state uint32) (uint32, error) {
	r.mu.Lock()
	r.called = append(r.called, state)
	resetErr := r.resetErr
	r.mu.Unlock()
	if state == esContinuous && resetErr != nil {
		return 0, resetErr
	}
	return state, nil
}

func (r *windowsStateRecorder) states() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]uint32(nil), r.called...)
}

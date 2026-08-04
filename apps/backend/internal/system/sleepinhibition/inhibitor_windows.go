//go:build windows

package sleepinhibition

import (
	"context"
	"runtime"
	"sync"

	"golang.org/x/sys/windows"
)

const (
	esContinuous      uint32 = 0x80000000
	esSystemRequired  uint32 = 0x00000001
	esInhibitionState        = esContinuous | esSystemRequired
)

type windowsStateCaller func(state uint32) (uint32, error)

type windowsInhibitor struct {
	call windowsStateCaller
}

func NewPlatformInhibitor() Inhibitor {
	proc := windows.NewLazySystemDLL("kernel32.dll").NewProc("SetThreadExecutionState")
	return newWindowsInhibitor(func(state uint32) (uint32, error) {
		result, _, err := proc.Call(uintptr(state))
		if result == 0 {
			return 0, err
		}
		return uint32(result), nil
	})
}

func newWindowsInhibitor(call windowsStateCaller) Inhibitor {
	return &windowsInhibitor{call: call}
}

func (i *windowsInhibitor) Platform() Platform { return PlatformWindows }
func (i *windowsInhibitor) Supported() bool    { return true }

func (i *windowsInhibitor) Acquire(ctx context.Context) (Lease, error) {
	result := make(chan windowsAcquireResult, 1)
	handoff := make(chan struct{})
	abort := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		if _, err := i.call(esInhibitionState); err != nil {
			select {
			case result <- windowsAcquireResult{err: NewIssueError(IssueRequestFailed, err)}:
			case <-ctx.Done():
			}
			return
		}
		lease := &windowsLease{release: make(chan struct{}), done: make(chan error, 1)}
		result <- windowsAcquireResult{lease: lease}
		select {
		case <-handoff:
			<-lease.release
		case <-abort:
		}
		_, err := i.call(esContinuous)
		lease.done <- err
		close(lease.done)
	}()

	select {
	case response := <-result:
		if response.err != nil {
			return nil, response.err
		}
		close(handoff)
		return response.lease, nil
	case <-ctx.Done():
		close(abort)
		return nil, ctx.Err()
	}
}

type windowsAcquireResult struct {
	lease Lease
	err   error
}

type windowsLease struct {
	releaseOnce sync.Once
	release     chan struct{}
	done        chan error
}

func (l *windowsLease) Release() error {
	l.releaseOnce.Do(func() { close(l.release) })
	err, ok := <-l.done
	if !ok {
		return nil
	}
	return err
}

func (l *windowsLease) Done() <-chan error { return l.done }

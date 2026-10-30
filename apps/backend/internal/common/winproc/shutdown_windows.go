//go:build windows

package winproc

import (
	"fmt"
	"os"
	"sync"

	"golang.org/x/sys/windows"
)

// ListenForShutdown registers a per-process named event and forwards one
// os.Interrupt to signals when the owning parent sets it. Console-less
// processes cannot receive taskkill's window-close request, so without this
// their parent can only force-kill them and skip the normal teardown.
// The returned function releases the waiter and both event handles.
func ListenForShutdown(signals chan<- os.Signal) (func(), error) {
	name, err := windows.UTF16PtrFromString(shutdownEventName(os.Getpid()))
	if err != nil {
		return nil, err
	}
	request, err := windows.CreateEvent(nil, 1, 0, name)
	if err != nil {
		if request != 0 {
			_ = windows.CloseHandle(request)
		}
		return nil, err
	}
	stop, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		_ = windows.CloseHandle(request)
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		result, waitErr := windows.WaitForMultipleObjects([]windows.Handle{request, stop}, false, windows.INFINITE)
		if waitErr == nil && result == windows.WAIT_OBJECT_0 {
			select {
			case signals <- os.Interrupt:
			default:
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			_ = windows.SetEvent(stop)
			<-done
			_ = windows.CloseHandle(stop)
			_ = windows.CloseHandle(request)
		})
	}, nil
}

// RequestShutdown asks the process with the given PID to start its normal
// teardown. It fails when that process has not registered ListenForShutdown,
// so callers can fall back to their existing termination path.
func RequestShutdown(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid shutdown target pid %d", pid)
	}
	name, err := windows.UTF16PtrFromString(shutdownEventName(pid))
	if err != nil {
		return err
	}
	event, err := windows.OpenEvent(windows.EVENT_MODIFY_STATE, false, name)
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(event) }()
	return windows.SetEvent(event)
}

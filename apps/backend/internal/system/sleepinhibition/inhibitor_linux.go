//go:build linux

package sleepinhibition

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"golang.org/x/sys/unix"
)

const (
	login1Name      = "org.freedesktop.login1"
	login1Path      = dbus.ObjectPath("/org/freedesktop/login1")
	login1Interface = "org.freedesktop.login1.Manager"
)

type linuxDBus interface {
	Object(destination string, path dbus.ObjectPath) linuxDBusObject
	Close() error
}

type linuxDBusObject interface {
	Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call
	CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call
}

type linuxDBusOpener func() (linuxDBus, error)

type linuxInhibitor struct {
	open linuxDBusOpener
}

func NewPlatformInhibitor() Inhibitor {
	return newLinuxInhibitor(func() (linuxDBus, error) {
		connection, err := dbus.ConnectSystemBus()
		if err != nil {
			return nil, err
		}
		return realLinuxDBus{connection: connection}, nil
	})
}

func newLinuxInhibitor(open linuxDBusOpener) Inhibitor { return &linuxInhibitor{open: open} }

func (i *linuxInhibitor) Platform() Platform { return PlatformLinux }
func (i *linuxInhibitor) Supported() bool    { return true }

func (i *linuxInhibitor) Probe(ctx context.Context) error {
	connection, err := i.open()
	if err != nil {
		return NewIssueError(IssueSystemServiceUnavailable, err)
	}
	defer func() { _ = connection.Close() }()
	call := connection.Object(login1Name, login1Path).CallWithContext(ctx, "org.freedesktop.DBus.Peer.Ping", 0)
	if call.Err != nil {
		return NewIssueError(IssueSystemServiceUnavailable, call.Err)
	}
	return nil
}

func (i *linuxInhibitor) Acquire(ctx context.Context) (Lease, error) {
	connection, err := i.open()
	if err != nil {
		return nil, NewIssueError(IssueSystemServiceUnavailable, err)
	}
	call := connection.Object(login1Name, login1Path).CallWithContext(
		ctx,
		login1Interface+".Inhibit",
		0,
		"sleep",
		"Kandev",
		"A Kandev task is running",
		"block",
	)
	if call.Err != nil {
		_ = connection.Close()
		return nil, NewIssueError(IssueRequestFailed, call.Err)
	}
	var fd dbus.UnixFD
	if err := call.Store(&fd); err != nil {
		_ = connection.Close()
		return nil, NewIssueError(IssueRequestFailed, err)
	}
	file := os.NewFile(uintptr(fd), "kandev-sleep-inhibition")
	if file == nil {
		_ = connection.Close()
		return nil, NewIssueError(IssueRequestFailed, os.ErrInvalid)
	}
	return newLinuxLease(connection, file), nil
}

type linuxLease struct {
	connection     linuxDBus
	file           *os.File
	fd             int
	done           chan error
	stop           chan struct{}
	monitorDone    chan struct{}
	releaseErr     error
	releaseOnce    sync.Once
	completionOnce sync.Once
}

func newLinuxLease(connection linuxDBus, file *os.File) *linuxLease {
	lease := &linuxLease{
		connection:  connection,
		file:        file,
		fd:          int(file.Fd()),
		done:        make(chan error, 1),
		stop:        make(chan struct{}),
		monitorDone: make(chan struct{}),
	}
	go lease.monitor()
	return lease
}

func (l *linuxLease) Release() error {
	l.releaseOnce.Do(func() {
		close(l.stop)
		if err := l.file.Close(); err != nil {
			l.releaseErr = err
		}
		<-l.monitorDone
		if err := l.connection.Close(); err != nil && l.releaseErr == nil {
			l.releaseErr = err
		}
		l.complete(l.releaseErr)
	})
	return l.releaseErr
}

func (l *linuxLease) Done() <-chan error { return l.done }

func (l *linuxLease) complete(err error) {
	l.completionOnce.Do(func() {
		l.done <- err
		close(l.done)
	})
}

func (l *linuxLease) monitor() {
	defer close(l.monitorDone)
	fd := l.fd
	// The descriptor returned by logind is a pipe. Non-blocking mode makes the
	// EOF check safe if the platform reports readable data before HUP.
	_ = unix.SetNonblock(fd, true)
	pollFDs := []unix.PollFd{{
		Fd:     int32(fd),
		Events: unix.POLLIN | unix.POLLERR | unix.POLLHUP | unix.POLLNVAL,
	}}
	for {
		_, err := unix.Poll(pollFDs, 100)
		if err != nil {
			if errors.Is(err, unix.EINTR) {
				continue
			}
			if l.stopped() {
				return
			}
			l.unexpectedLoss(err)
			return
		}
		if l.stopped() {
			return
		}
		revents := pollFDs[0].Revents
		if revents&(unix.POLLERR|unix.POLLHUP|unix.POLLNVAL) != 0 {
			l.unexpectedLoss(errors.New("logind inhibitor descriptor closed"))
			return
		}
		if revents&unix.POLLIN == 0 {
			continue
		}
		var buffer [1]byte
		n, readErr := unix.Read(fd, buffer[:])
		if n == 0 {
			l.unexpectedLoss(errors.New("logind inhibitor descriptor reached EOF"))
			return
		}
		if readErr != nil && !errors.Is(readErr, unix.EAGAIN) && !errors.Is(readErr, unix.EWOULDBLOCK) {
			l.unexpectedLoss(readErr)
			return
		}
	}
}

func (l *linuxLease) stopped() bool {
	select {
	case <-l.stop:
		return true
	default:
		return false
	}
}

func (l *linuxLease) unexpectedLoss(err error) {
	if l.stopped() {
		return
	}
	l.complete(NewIssueError(IssueRequestFailed, err))
}

type realLinuxDBus struct{ connection *dbus.Conn }

func (c realLinuxDBus) Object(destination string, path dbus.ObjectPath) linuxDBusObject {
	return realLinuxDBusObject{object: c.connection.Object(destination, path)}
}

func (c realLinuxDBus) Close() error { return c.connection.Close() }

type realLinuxDBusObject struct{ object dbus.BusObject }

func (o realLinuxDBusObject) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return o.object.Call(method, flags, args...)
}

func (o realLinuxDBusObject) CallWithContext(ctx context.Context, method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return o.object.CallWithContext(ctx, method, flags, args...)
}

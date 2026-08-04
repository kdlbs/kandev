//go:build linux

package sleepinhibition

import (
	"context"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
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
}

type linuxDBusOpener func() (linuxDBus, error)

type linuxInhibitor struct {
	open linuxDBusOpener
}

func NewPlatformInhibitor() Inhibitor {
	return newLinuxInhibitor(func() (linuxDBus, error) {
		connection, err := dbus.SystemBus()
		if err != nil {
			return nil, err
		}
		return realLinuxDBus{connection: connection}, nil
	})
}

func newLinuxInhibitor(open linuxDBusOpener) Inhibitor { return &linuxInhibitor{open: open} }

func (i *linuxInhibitor) Platform() Platform { return PlatformLinux }
func (i *linuxInhibitor) Supported() bool    { return true }

func (i *linuxInhibitor) Acquire(context.Context) (Lease, error) {
	connection, err := i.open()
	if err != nil {
		return nil, NewIssueError(IssueSystemServiceUnavailable, err)
	}
	call := connection.Object(login1Name, login1Path).Call(
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
	connection linuxDBus
	file       *os.File
	done       chan error
	once       sync.Once
}

func newLinuxLease(connection linuxDBus, file *os.File) *linuxLease {
	return &linuxLease{connection: connection, file: file, done: make(chan error, 1)}
}

func (l *linuxLease) Release() error {
	var releaseErr error
	l.once.Do(func() {
		if err := l.file.Close(); err != nil {
			releaseErr = err
		}
		if err := l.connection.Close(); err != nil && releaseErr == nil {
			releaseErr = err
		}
		l.done <- releaseErr
		close(l.done)
	})
	return releaseErr
}

func (l *linuxLease) Done() <-chan error { return l.done }

type realLinuxDBus struct{ connection *dbus.Conn }

func (c realLinuxDBus) Object(destination string, path dbus.ObjectPath) linuxDBusObject {
	return realLinuxDBusObject{object: c.connection.Object(destination, path)}
}

func (c realLinuxDBus) Close() error { return c.connection.Close() }

type realLinuxDBusObject struct{ object dbus.BusObject }

func (o realLinuxDBusObject) Call(method string, flags dbus.Flags, args ...interface{}) *dbus.Call {
	return o.object.Call(method, flags, args...)
}

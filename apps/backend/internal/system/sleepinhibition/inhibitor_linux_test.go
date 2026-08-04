//go:build linux

package sleepinhibition

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestLinuxInhibitorRequestsOnlySystemSleep(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = readFile.Close() }()
	defer func() { _ = writeFile.Close() }()

	connection := &fakeLinuxDBus{
		object: &fakeLinuxDBusObject{call: &dbus.Call{Body: []interface{}{dbus.UnixFD(writeFile.Fd())}}},
	}
	inhibitor := newLinuxInhibitor(func() (linuxDBus, error) { return connection, nil })

	lease, err := inhibitor.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if connection.object.method != login1Interface+".Inhibit" {
		t.Fatalf("method = %q", connection.object.method)
	}
	if got := connection.object.args; len(got) != 4 || got[0] != "sleep" || got[1] != "Kandev" || got[2] != "A Kandev task is running" || got[3] != "block" {
		t.Fatalf("inhibit args = %#v", got)
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release: %v", err)
	}
	if !connection.closed {
		t.Fatal("release did not close the D-Bus connection")
	}
}

func TestLinuxInhibitorMapsSystemBusFailure(t *testing.T) {
	inhibitor := newLinuxInhibitor(func() (linuxDBus, error) {
		return nil, errors.New("no system bus")
	})

	_, err := inhibitor.Acquire(context.Background())
	if err == nil || IssueFromError(err) != IssueSystemServiceUnavailable {
		t.Fatalf("acquire error = %v, want system service unavailable", err)
	}
}

type fakeLinuxDBus struct {
	object *fakeLinuxDBusObject
	closed bool
}

func (c *fakeLinuxDBus) Object(string, dbus.ObjectPath) linuxDBusObject { return c.object }
func (c *fakeLinuxDBus) Close() error {
	c.closed = true
	return nil
}

type fakeLinuxDBusObject struct {
	call   *dbus.Call
	method string
	args   []interface{}
}

func (o *fakeLinuxDBusObject) Call(method string, _ dbus.Flags, args ...interface{}) *dbus.Call {
	o.method = method
	o.args = args
	return o.call
}

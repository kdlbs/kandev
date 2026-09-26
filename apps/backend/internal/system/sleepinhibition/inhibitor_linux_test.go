//go:build linux

package sleepinhibition

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	lease, err := inhibitor.Acquire(ctx)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	if connection.object.method != login1Interface+".Inhibit" {
		t.Fatalf("method = %q", connection.object.method)
	}
	if connection.object.context != ctx {
		t.Fatal("acquire did not propagate its context")
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

func TestLinuxInhibitorProbesLogin1(t *testing.T) {
	connection := &fakeLinuxDBus{
		object: &fakeLinuxDBusObject{call: &dbus.Call{}},
	}
	inhibitor := newLinuxInhibitor(func() (linuxDBus, error) { return connection, nil })
	prober, ok := inhibitor.(capabilityProber)
	if !ok {
		t.Fatal("linux inhibitor does not expose capability probe")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := prober.Probe(ctx); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if connection.object.method != "org.freedesktop.DBus.Peer.Ping" {
		t.Fatalf("probe method = %q", connection.object.method)
	}
	if connection.object.context != ctx {
		t.Fatal("probe did not propagate its context")
	}
	if !connection.closed {
		t.Fatal("probe did not close the D-Bus connection")
	}
}

func TestLinuxInhibitorMapsProbeFailure(t *testing.T) {
	connection := &fakeLinuxDBus{
		object: &fakeLinuxDBusObject{call: &dbus.Call{Err: errors.New("name has no owner")}},
	}
	inhibitor := newLinuxInhibitor(func() (linuxDBus, error) { return connection, nil })
	prober, ok := inhibitor.(capabilityProber)
	if !ok {
		t.Fatal("linux inhibitor does not expose capability probe")
	}

	if err := prober.Probe(context.Background()); err == nil || IssueFromError(err) != IssueSystemServiceUnavailable {
		t.Fatalf("probe error = %v, want system service unavailable", err)
	}
	if !connection.closed {
		t.Fatal("failed probe did not close the D-Bus connection")
	}
}

func TestLinuxLeaseReportsRemoteDescriptorLoss(t *testing.T) {
	readFile, writeFile, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	defer func() { _ = readFile.Close() }()

	connection := &fakeLinuxDBus{object: &fakeLinuxDBusObject{call: &dbus.Call{}}}
	lease := newLinuxLease(connection, writeFile)
	if err := readFile.Close(); err != nil {
		t.Fatalf("close remote descriptor: %v", err)
	}

	select {
	case err := <-lease.Done():
		if IssueFromError(err) != IssueRequestFailed {
			t.Fatalf("remote loss error = %v, want request_failed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("lease did not report remote descriptor loss")
	}
	if err := lease.Release(); err != nil {
		t.Fatalf("release after remote loss: %v", err)
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
	call    *dbus.Call
	method  string
	args    []interface{}
	context context.Context
}

func (o *fakeLinuxDBusObject) Call(method string, _ dbus.Flags, args ...interface{}) *dbus.Call {
	o.method = method
	o.args = args
	return o.call
}

func (o *fakeLinuxDBusObject) CallWithContext(ctx context.Context, method string, _ dbus.Flags, args ...interface{}) *dbus.Call {
	o.context = ctx
	return o.Call(method, 0, args...)
}

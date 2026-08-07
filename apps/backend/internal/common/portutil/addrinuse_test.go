package portutil

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
	"testing"
)

// TestIsAddrInUseRealBind is the regression that matters: it binds a real
// socket twice and asserts the helper recognizes the failure. It runs on every
// platform, which is the point — the bug this replaces was a comparison that
// only ever failed on Windows, so no amount of synthetic error fixtures on a
// Linux runner would have caught it.
func TestIsAddrInUseRealBind(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() failed: %v", err)
	}
	defer func() { _ = ln.Close() }()

	second, err := net.Listen("tcp", ln.Addr().String())
	if err == nil {
		_ = second.Close()
		t.Fatalf("second net.Listen(%q) unexpectedly succeeded", ln.Addr())
	}

	if !IsAddrInUse(err) {
		t.Fatalf("IsAddrInUse(%v) = false, want true (errno chain: %s)", err, describeErrno(err))
	}
}

func TestIsAddrInUseRejectsUnrelated(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"plain", errors.New("boom")},
		{"other syscall error", &net.OpError{
			Op:  "listen",
			Net: "tcp",
			Err: &os.SyscallError{Syscall: "bind", Err: syscall.EACCES},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if IsAddrInUse(tc.err) {
				t.Fatalf("IsAddrInUse(%v) = true, want false", tc.err)
			}
		})
	}
}

func describeErrno(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return fmt.Sprintf("syscall.Errno(%d)", uintptr(errno))
	}
	return "no syscall.Errno in chain"
}

package netutil

import (
	"errors"
	"net"
	"testing"
)

func TestIsAddrInUseRecognizesDoubleBind(t *testing.T) {
	first, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for first socket: %v", err)
	}
	t.Cleanup(func() { _ = first.Close() })

	second, err := net.Listen("tcp", first.Addr().String())
	if err == nil {
		_ = second.Close()
		t.Fatal("second listener unexpectedly bound to an occupied address")
	}

	if !IsAddrInUse(err) {
		t.Fatalf("IsAddrInUse(%v) = false, want true", err)
	}
}

func TestIsAddrInUseRejectsOtherErrors(t *testing.T) {
	if IsAddrInUse(errors.New("permission denied")) {
		t.Fatal("IsAddrInUse reported an unrelated error as address-in-use")
	}
}

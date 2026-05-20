package launcher

import (
	"os"
	"testing"
)

func TestCloseParentPipe_ClosesAndNils(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()

	l := &Launcher{parentPipe: w}
	l.closeParentPipe()

	if l.parentPipe != nil {
		t.Error("parentPipe should be nil after closeParentPipe")
	}

	// Writing to the closed write-end should fail.
	_, err = w.Write([]byte("x"))
	if err == nil {
		t.Error("expected error writing to closed pipe")
	}
}

func TestCloseParentPipe_NilIsNoop(t *testing.T) {
	l := &Launcher{}
	// Must not panic.
	l.closeParentPipe()
	if l.parentPipe != nil {
		t.Error("parentPipe should remain nil")
	}
}

func TestCloseParentPipe_Idempotent(t *testing.T) {
	_, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}

	l := &Launcher{parentPipe: w}
	l.closeParentPipe()
	// Second call must not panic (double close).
	l.closeParentPipe()
}

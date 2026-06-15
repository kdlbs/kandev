package launcher

import (
	"bytes"
	"testing"
)

func TestLimitedBufferKeepsOnlyTail(t *testing.T) {
	buf := newLimitedBuffer(5)
	if _, err := buf.Write([]byte("hello")); err != nil {
		t.Fatal(err)
	}
	if _, err := buf.Write([]byte("world")); err != nil {
		t.Fatal(err)
	}

	if got := string(buf.Bytes()); got != "world" {
		t.Fatalf("buffer tail = %q, want world", got)
	}
}

func TestLimitedBufferBytesReturnsSnapshot(t *testing.T) {
	buf := newLimitedBuffer(10)
	if _, err := buf.Write([]byte("abc")); err != nil {
		t.Fatal(err)
	}
	snapshot := buf.Bytes()
	snapshot[0] = 'x'

	if !bytes.Equal(buf.Bytes(), []byte("abc")) {
		t.Fatalf("mutating snapshot changed buffer: %q", buf.Bytes())
	}
}

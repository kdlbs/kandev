package kubernetes

import (
	"net"
	"testing"
)

func TestShouldFallbackStreamingIncludesTimeoutErrors(t *testing.T) {
	err := &net.DNSError{Err: "upgrade timed out", IsTimeout: true}
	if !shouldFallbackStreaming(err) {
		t.Fatal("streaming transport timeout did not permit SPDY fallback")
	}
}

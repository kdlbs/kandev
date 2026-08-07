package websocket

import "testing"

func TestZeroValueClientSendControlBytesReturnsFalseWithoutPanic(t *testing.T) {
	var client Client
	if client.sendControlBytes([]byte("response")) {
		t.Fatal("zero-value client must not report a queued response")
	}
}

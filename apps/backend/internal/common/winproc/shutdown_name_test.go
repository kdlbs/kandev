package winproc

import "testing"

func TestShutdownEventNameMatchesDesktopContract(t *testing.T) {
	if got, want := shutdownEventName(4242), `Local\KandevShutdown-4242`; got != want {
		t.Fatalf("shutdownEventName(4242) = %q, want %q", got, want)
	}
}

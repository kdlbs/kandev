package launcher

import "testing"

func TestPickAvailablePortExceptSkipsUsedPreferredPort(t *testing.T) {
	port, err := pickAvailablePortExcept(defaultBackendPort, map[int]bool{defaultBackendPort: true})
	if err != nil {
		t.Fatal(err)
	}
	if port == defaultBackendPort {
		t.Fatalf("picked reserved preferred port %d", port)
	}
}

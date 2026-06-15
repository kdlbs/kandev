package launcher

import "testing"

func TestPickPortsRejectsDuplicateExplicitPorts(t *testing.T) {
	_, err := pickPorts(12345, 12345)
	if err == nil {
		t.Fatal("expected duplicate explicit ports to fail")
	}
}

func TestPickAvailablePortExceptSkipsUsedPreferredPort(t *testing.T) {
	port, err := pickAvailablePortExcept(defaultBackendPort, map[int]bool{defaultBackendPort: true})
	if err != nil {
		t.Fatal(err)
	}
	if port == defaultBackendPort {
		t.Fatalf("picked reserved preferred port %d", port)
	}
}

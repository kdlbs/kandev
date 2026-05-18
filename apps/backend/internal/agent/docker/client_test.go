package docker

import (
	"fmt"
	"testing"

	"github.com/docker/go-connections/nat"
)

func TestNormalizeDockerHostIP(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{in: "", want: "127.0.0.1"},
		{in: "0.0.0.0", want: "127.0.0.1"},
		{in: "::", want: "127.0.0.1"},
		{in: "127.0.0.1", want: "127.0.0.1"},
		{in: "10.0.0.5", want: "10.0.0.5"},
		{in: "::1", want: "::1"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := normalizeDockerHostIP(tc.in); got != tc.want {
				t.Errorf("normalizeDockerHostIP(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildDockerPortBindings_EmptyReturnsNil(t *testing.T) {
	exposed, bindings := buildDockerPortBindings(nil)
	if exposed != nil {
		t.Errorf("expected nil exposed ports, got %v", exposed)
	}
	if bindings != nil {
		t.Errorf("expected nil bindings, got %v", bindings)
	}
}

func TestBuildDockerPortBindings_AssignsContainerAndHost(t *testing.T) {
	in := []PortBindingConfig{
		{ContainerPort: 8080, HostIP: "127.0.0.1", HostPort: "0"},
		{ContainerPort: 9000, HostIP: "0.0.0.0", HostPort: "9001"},
	}
	exposed, bindings := buildDockerPortBindings(in)

	if got := len(exposed); got != 2 {
		t.Fatalf("exposed ports = %d, want 2", got)
	}
	for _, b := range in {
		key := nat.Port(fmt.Sprintf("%d/tcp", b.ContainerPort))
		if _, ok := exposed[key]; !ok {
			t.Errorf("exposed missing %s", key)
		}
		got := bindings[key]
		if len(got) != 1 {
			t.Fatalf("bindings[%s] = %d entries, want 1", key, len(got))
		}
		if got[0].HostIP != b.HostIP || got[0].HostPort != b.HostPort {
			t.Errorf("bindings[%s] = %+v, want host_ip=%q host_port=%q", key, got[0], b.HostIP, b.HostPort)
		}
	}
}

func TestBuildDockerPortBindings_DeduplicatesContainerPort(t *testing.T) {
	in := []PortBindingConfig{
		{ContainerPort: 7000, HostIP: "127.0.0.1", HostPort: "0"},
		{ContainerPort: 7000, HostIP: "10.0.0.5", HostPort: "7000"},
	}
	_, bindings := buildDockerPortBindings(in)
	got := bindings[nat.Port("7000/tcp")]
	if len(got) != 2 {
		t.Fatalf("want both bindings on port 7000, got %d", len(got))
	}
}

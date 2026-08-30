package docker

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestNormalizeDockerHostIP(t *testing.T) {
	cases := []struct {
		name string
		in   netip.Addr
		want string
	}{
		{name: "unset", in: netip.Addr{}, want: "127.0.0.1"},
		{name: "ipv4 wildcard", in: netip.MustParseAddr("0.0.0.0"), want: "127.0.0.1"},
		{name: "ipv6 wildcard", in: netip.MustParseAddr("::"), want: "127.0.0.1"},
		{name: "ipv4 loopback", in: netip.MustParseAddr("127.0.0.1"), want: "127.0.0.1"},
		{name: "ipv4 host", in: netip.MustParseAddr("10.0.0.5"), want: "10.0.0.5"},
		{name: "ipv6 loopback", in: netip.MustParseAddr("::1"), want: "::1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDockerHostIP(tc.in); got != tc.want {
				t.Errorf("normalizeDockerHostIP(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestBuildDockerPortBindings_EmptyReturnsNil(t *testing.T) {
	exposed, bindings, err := buildDockerPortBindings(nil)
	if err != nil {
		t.Fatalf("buildDockerPortBindings: %v", err)
	}
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
	exposed, bindings, err := buildDockerPortBindings(in)
	if err != nil {
		t.Fatalf("buildDockerPortBindings: %v", err)
	}

	if got := len(exposed); got != 2 {
		t.Fatalf("exposed ports = %d, want 2", got)
	}
	for _, b := range in {
		key := network.MustParsePort(fmt.Sprintf("%d/tcp", b.ContainerPort))
		if _, ok := exposed[key]; !ok {
			t.Errorf("exposed missing %s", key)
		}
		got := bindings[key]
		if len(got) != 1 {
			t.Fatalf("bindings[%s] = %d entries, want 1", key, len(got))
		}
		if got[0].HostIP.String() != b.HostIP || got[0].HostPort != b.HostPort {
			t.Errorf("bindings[%s] = %+v, want host_ip=%q host_port=%q", key, got[0], b.HostIP, b.HostPort)
		}
	}
}

func TestBuildDockerPortBindings_DeduplicatesContainerPort(t *testing.T) {
	in := []PortBindingConfig{
		{ContainerPort: 7000, HostIP: "127.0.0.1", HostPort: "0"},
		{ContainerPort: 7000, HostIP: "10.0.0.5", HostPort: "7000"},
	}
	_, bindings, err := buildDockerPortBindings(in)
	if err != nil {
		t.Fatalf("buildDockerPortBindings: %v", err)
	}
	got := bindings[network.MustParsePort("7000/tcp")]
	if len(got) != 2 {
		t.Fatalf("want both bindings on port 7000, got %d", len(got))
	}
}

func TestBuildDockerPortBindings_RejectsInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		in   PortBindingConfig
	}{
		{name: "container port zero", in: PortBindingConfig{ContainerPort: 0, HostPort: "1"}},
		{name: "container port too large", in: PortBindingConfig{ContainerPort: 70000, HostPort: "1"}},
		{name: "host ip not an address", in: PortBindingConfig{ContainerPort: 8080, HostIP: "localhost"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := buildDockerPortBindings([]PortBindingConfig{tc.in}); err == nil {
				t.Fatal("expected an error, got nil")
			}
		})
	}
}

func TestBuildDockerPortBindings_EmptyHostIPPublishesOnAllInterfaces(t *testing.T) {
	_, bindings, err := buildDockerPortBindings([]PortBindingConfig{
		{ContainerPort: 8080, HostIP: "", HostPort: "8080"},
	})
	if err != nil {
		t.Fatalf("buildDockerPortBindings: %v", err)
	}
	got := bindings[network.MustParsePort("8080/tcp")]
	if len(got) != 1 {
		t.Fatalf("bindings = %d entries, want 1", len(got))
	}
	if got[0].HostIP.IsValid() {
		t.Errorf("host IP = %v, want the zero address (all interfaces)", got[0].HostIP)
	}
}

func TestParseHostPort(t *testing.T) {
	if got, err := parseHostPort("9001"); err != nil || got != 9001 {
		t.Fatalf("parseHostPort(9001) = %d, %v", got, err)
	}
	for _, in := range []string{"", "abc", "70000", "-1"} {
		if _, err := parseHostPort(in); err == nil {
			t.Errorf("parseHostPort(%q) = nil error, want an error", in)
		}
	}
}

func TestContainerTeardownTreatsMissingContainersAsSuccess(t *testing.T) {
	dockerDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"No such container: missing-container"}`))
	}))
	t.Cleanup(dockerDaemon.Close)

	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	client, err := NewClient(config.DockerConfig{
		Host:       "tcp://" + strings.TrimPrefix(dockerDaemon.URL, "http://"),
		APIVersion: "1.44",
	}, log)
	require.NoError(t, err)

	require.NoError(t, client.StopContainer(context.Background(), "missing-container", time.Second))
	require.NoError(t, client.KillContainer(context.Background(), "missing-container", "SIGKILL"))
	require.NoError(t, client.RemoveContainer(context.Background(), "missing-container", true))
}

func TestRemoveContainerTreatsConcurrentRemovalAsSuccess(t *testing.T) {
	dockerDaemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead && r.URL.Path == "/_ping" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"message":"removal of container abc is already in progress"}`))
	}))
	t.Cleanup(dockerDaemon.Close)

	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	client, err := NewClient(config.DockerConfig{
		Host:       "tcp://" + strings.TrimPrefix(dockerDaemon.URL, "http://"),
		APIVersion: "1.44",
	}, log)
	require.NoError(t, err)

	// Docker reports a conflict when another owner is already removing the
	// same container. Removal is a convergence operation, so the in-flight
	// owner has already taken responsibility for reaching the desired state.
	require.NoError(t, client.RemoveContainer(context.Background(), "abc", true))
}

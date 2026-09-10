package lifecycle

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync"
	"testing"
)

func TestOpenSSHRuntimeAPITunnel_LeavesReachableURLsUnchanged(t *testing.T) {
	const rawURL = "https://kandev.example.test/api/v1"

	tunnel, gotURL, err := openSSHRuntimeAPITunnel(nil, rawURL)
	if err != nil {
		t.Fatalf("openSSHRuntimeAPITunnel: %v", err)
	}
	if tunnel != nil {
		t.Fatal("reachable URL unexpectedly created a tunnel")
	}
	if gotURL != rawURL {
		t.Fatalf("URL = %q, want %q", gotURL, rawURL)
	}
}

func TestOpenSSHRuntimeAPITunnel_RequiresSSHClientForLoopbackURL(t *testing.T) {
	_, _, err := openSSHRuntimeAPITunnel(nil, "http://127.0.0.1:38429/api/v1")
	if err == nil {
		t.Fatal("loopback URL without SSH client unexpectedly succeeded")
	}
}

func TestOpenSSHRuntimeAPITunnel_ConcurrentSameHostSessionsGetUniqueRemotePorts(t *testing.T) {
	backendListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve backend port: %v", err)
	}
	backendPort := backendListener.Addr().(*net.TCPAddr).Port
	_ = backendListener.Close()
	rawURL := "http://127.0.0.1:" + strconv.Itoa(backendPort) + "/api/v1"

	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	start := make(chan struct{})
	type result struct {
		tunnel *sshRuntimeAPITunnel
		url    string
		err    error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			tunnel, rewritten, openErr := openSSHRuntimeAPITunnel(client, rawURL)
			results <- result{tunnel: tunnel, url: rewritten, err: openErr}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	seenPorts := make(map[string]struct{}, 2)
	for outcome := range results {
		if outcome.err != nil {
			t.Fatalf("concurrent tunnel open failed: %v", outcome.err)
		}
		if outcome.tunnel == nil {
			t.Fatal("loopback URL did not create a tunnel")
		}
		parsed, parseErr := url.Parse(outcome.url)
		if parseErr != nil {
			t.Fatalf("parse rewritten URL %q: %v", outcome.url, parseErr)
		}
		seenPorts[parsed.Port()] = struct{}{}
		if parsed.Port() == strconv.Itoa(backendPort) {
			t.Fatalf("rewritten URL still uses backend port %d: %q", backendPort, outcome.url)
		}
		_ = outcome.tunnel.Close()
	}
	if len(seenPorts) != 2 {
		t.Fatalf("concurrent tunnels used %d remote ports, want 2", len(seenPorts))
	}
}

func TestOpenSSHRuntimeAPITunnel_ResumeRebindsPersistedRemotePort(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	const rawURL = "http://127.0.0.1:38429/api/v1"
	firstReq := &ExecutorCreateRequest{Env: map[string]string{envKeyKandevAPIURL: rawURL}}

	firstTunnel, firstURL, err := openSSHRuntimeAPITunnelForRequest(client, firstReq)
	if err != nil {
		t.Fatalf("first tunnel: %v", err)
	}
	firstPort := firstTunnel.RemotePort()
	if firstReq.Metadata[MetadataKeySSHRuntimeAPILocalURL] != rawURL {
		t.Fatalf("local URL metadata = %v, want %q", firstReq.Metadata[MetadataKeySSHRuntimeAPILocalURL], rawURL)
	}
	if firstReq.Metadata[MetadataKeySSHRuntimeAPIRemotePort] != firstPort {
		t.Fatalf("remote port metadata = %v, want %q", firstReq.Metadata[MetadataKeySSHRuntimeAPIRemotePort], firstPort)
	}
	if err := firstTunnel.Close(); err != nil {
		t.Fatalf("close first tunnel: %v", err)
	}

	resumedReq := &ExecutorCreateRequest{
		Env:      map[string]string{envKeyKandevAPIURL: firstURL},
		Metadata: cloneSSHMetadata(firstReq.Metadata),
	}
	secondTunnel, secondURL, err := openSSHRuntimeAPITunnelForRequest(client, resumedReq)
	if err != nil {
		t.Fatalf("resume tunnel: %v", err)
	}
	defer func() { _ = secondTunnel.Close() }()
	if secondTunnel.RemotePort() != firstPort {
		t.Fatalf("resume remote port = %q, want persisted %q", secondTunnel.RemotePort(), firstPort)
	}
	if secondURL != firstURL {
		t.Fatalf("resume URL = %q, want the original rewritten URL %q", secondURL, firstURL)
	}
	if resumedReq.Env[envKeyKandevAPIURL] != secondURL {
		t.Fatalf("resume API URL = %q, want %q", resumedReq.Env[envKeyKandevAPIURL], secondURL)
	}
}

func TestOpenSSHRuntimeAPITunnel_ProxiesRoundTripAndTearsDown(t *testing.T) {
	localServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Connection", "close")
		_, _ = io.WriteString(w, "runtime-api-round-trip")
	}))
	t.Cleanup(localServer.Close)

	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	tunnel, rewrittenURL, err := openSSHRuntimeAPITunnel(client, localServer.URL)
	if err != nil {
		t.Fatalf("open tunnel: %v", err)
	}
	t.Cleanup(func() { _ = tunnel.Close() })

	requestClient := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
	response, err := requestClient.Get(rewrittenURL)
	if err != nil {
		t.Fatalf("GET through SSH runtime API tunnel: %v", err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read tunneled response: %v", err)
	}
	if string(body) != "runtime-api-round-trip" {
		t.Fatalf("tunneled response = %q", body)
	}
}

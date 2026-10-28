package executortunnel

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/kandev/kandev/pkg/pluginsdk"
)

const (
	testInstallation = "installation-1"
	testEnvironment  = "environment-1"
	controlPort      = 39000
	instancePort     = 39001
)

func openRequest(opener Opener, generation uint64, port uint32) OpenRequest {
	return OpenRequest{
		Key:          Key{InstallationID: testInstallation, EnvironmentID: testEnvironment, Generation: generation},
		Port:         port,
		AllowedPorts: []uint32{controlPort, instancePort},
		Header:       &pluginsdk.ExecutorTunnelHeader{},
		Open:         opener,
	}
}

type openResult struct {
	conn net.Conn
	err  error
}

func openAsync(registry *Registry, request OpenRequest) <-chan openResult {
	done := make(chan openResult, 1)
	go func() {
		conn, err := registry.Open(context.Background(), request)
		done <- openResult{conn: conn, err: err}
	}()
	return done
}

func awaitOpen(t *testing.T, done <-chan openResult) openResult {
	t.Helper()
	select {
	case result := <-done:
		return result
	case <-time.After(5 * time.Second):
		t.Fatal("Open did not return")
		return openResult{}
	}
}

func openAcked(t *testing.T, registry *Registry, providers chan *fakeProvider, opener Opener, generation uint64, port uint32) (net.Conn, *fakeProvider) {
	t.Helper()
	done := openAsync(registry, openRequest(opener, generation, port))
	provider := <-providers
	provider.ack(provider.header(t).GetBindingNonce())
	result := awaitOpen(t, done)
	if result.err != nil {
		t.Fatalf("Open() error = %v", result.err)
	}
	return result.conn, provider
}

func TestOpenSendsNothingUntilTheProviderAcknowledgesTheBinding(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	done := openAsync(registry, openRequest(newFakeOpener(t, providers), 1, controlPort))
	provider := <-providers
	header := provider.header(t)
	if header.GetPort() != controlPort || header.GetBindingNonce() == "" {
		t.Fatalf("header = %+v, want port %d and a binding nonce", header, controlPort)
	}
	select {
	case result := <-done:
		t.Fatalf("Open returned before the ack: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	provider.ack(header.GetBindingNonce())
	result := awaitOpen(t, done)
	if result.err != nil {
		t.Fatalf("Open() error = %v", result.err)
	}
	defer func() { _ = result.conn.Close() }()
	if _, err := result.conn.Write([]byte("GET /")); err != nil {
		t.Fatalf("write after ack: %v", err)
	}
	select {
	case frame := <-provider.fromHost:
		if string(frame.GetData()) != "GET /" {
			t.Fatalf("data frame = %q", frame.GetData())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("request bytes did not reach the provider after the ack")
	}
}

func TestOpenTimesOutAsUnavailableWhenNoAckArrives(t *testing.T) {
	registry := NewRegistry()
	registry.ackTimeout = 50 * time.Millisecond
	providers := make(chan *fakeProvider, 1)
	done := openAsync(registry, openRequest(newFakeOpener(t, providers), 1, controlPort))
	provider := <-providers
	provider.header(t)
	result := awaitOpen(t, done)
	if !errors.Is(result.err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", result.err)
	}
	if got := registry.Active(testInstallation, testEnvironment); got != 0 {
		t.Fatalf("active streams = %d after ack timeout, want 0", got)
	}
	provider.ack("late")
	waitFor(t, provider.wasCanceled)
}

func TestDataBeforeAckAndSecondAckAreProtocolErrors(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 2)
	opener := newFakeOpener(t, providers)

	done := openAsync(registry, openRequest(opener, 1, controlPort))
	provider := <-providers
	provider.header(t)
	provider.data([]byte("early"))
	if result := awaitOpen(t, done); !errors.Is(result.err, ErrUnavailable) {
		t.Fatalf("data before ack error = %v, want ErrUnavailable", result.err)
	}

	conn, second := openAcked(t, registry, providers, opener, 1, controlPort)
	second.ack("again")
	buf := make([]byte, 1)
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("a second ack must close the stream")
	}
	if got := registry.Active(testInstallation, testEnvironment); got != 0 {
		t.Fatalf("active streams = %d after a protocol error, want 0", got)
	}
}

func TestOversizedProviderFrameClosesTheStream(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	conn, provider := openAcked(t, registry, providers, newFakeOpener(t, providers), 1, controlPort)
	provider.data(make([]byte, MaxFrameBytes+1))
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(make([]byte, 1)); err == nil {
		t.Fatal("an oversized frame must close the stream")
	}
}

func TestHostWritesAreSplitIntoFramesNoLargerThanTheLimit(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	conn, provider := openAcked(t, registry, providers, newFakeOpener(t, providers), 1, controlPort)
	defer func() { _ = conn.Close() }()
	payload := make([]byte, MaxFrameBytes*2+10)
	go func() { _, _ = conn.Write(payload) }()
	total := 0
	for total < len(payload) {
		frame := <-provider.fromHost
		if len(frame.GetData()) > MaxFrameBytes {
			t.Fatalf("frame of %d bytes exceeds %d", len(frame.GetData()), MaxFrameBytes)
		}
		total += len(frame.GetData())
	}
}

func TestDisallowedPortIsRefusedBeforeAnyStreamOpens(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	_, err := registry.Open(context.Background(), openRequest(newFakeOpener(t, providers), 1, 22))
	if !errors.Is(err, ErrPortNotAllowed) {
		t.Fatalf("error = %v, want ErrPortNotAllowed", err)
	}
	select {
	case <-providers:
		t.Fatal("a stream was opened for a disallowed port")
	default:
	}
}

func TestSixtyFifthOpenIsRateLimitedAndOthersAreUnaffected(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, MaxStreamsPerEnvironment+2)
	opener := newFakeOpener(t, providers)
	conns := make([]net.Conn, 0, MaxStreamsPerEnvironment)
	for i := 0; i < MaxStreamsPerEnvironment; i++ {
		port := uint32(controlPort)
		if i%2 == 1 {
			port = instancePort
		}
		conn, _ := openAcked(t, registry, providers, opener, 1, port)
		conns = append(conns, conn)
	}
	if _, err := registry.Open(context.Background(), openRequest(opener, 1, controlPort)); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("65th open error = %v, want ErrRateLimited", err)
	}
	if got := registry.Active(testInstallation, testEnvironment); got != MaxStreamsPerEnvironment {
		t.Fatalf("active = %d, want %d", got, MaxStreamsPerEnvironment)
	}
	if _, err := conns[0].Write([]byte("still works")); err != nil {
		t.Fatalf("an existing stream broke after the refused open: %v", err)
	}
	_ = conns[0].Close()
	waitFor(t, func() bool { return registry.Active(testInstallation, testEnvironment) == MaxStreamsPerEnvironment-1 })
	conn, _ := openAcked(t, registry, providers, opener, 1, controlPort)
	_ = conn.Close()
	other := Key{InstallationID: testInstallation, EnvironmentID: "environment-2", Generation: 1}
	request := openRequest(opener, 1, controlPort)
	request.Key = other
	done := openAsync(registry, request)
	provider := <-providers
	provider.ack(provider.header(t).GetBindingNonce())
	if result := awaitOpen(t, done); result.err != nil {
		t.Fatalf("another environment was limited by this one: %v", result.err)
	}
}

func TestCloseEnvironmentClosesStreamsAndNotifiesTheProvider(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 2)
	opener := newFakeOpener(t, providers)
	connA, providerA := openAcked(t, registry, providers, opener, 1, controlPort)
	_, providerB := openAcked(t, registry, providers, opener, 1, instancePort)
	registry.CloseEnvironment(testInstallation, testEnvironment)
	waitFor(t, providerA.wasCanceled)
	waitFor(t, providerB.wasCanceled)
	if got := registry.Active(testInstallation, testEnvironment); got != 0 {
		t.Fatalf("active = %d after release, want 0", got)
	}
	_ = connA.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := connA.Read(make([]byte, 1)); err == nil {
		t.Fatal("read on a released stream must fail")
	}
}

func TestClientCloseCancelsTheProviderStream(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	conn, provider := openAcked(t, registry, providers, newFakeOpener(t, providers), 1, controlPort)
	_ = conn.Close()
	waitFor(t, provider.wasCanceled)
	waitFor(t, func() bool { return registry.Active(testInstallation, testEnvironment) == 0 })
}

func TestCloseInstallationClosesEveryEnvironmentOfThePlugin(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 2)
	opener := newFakeOpener(t, providers)
	_, providerA := openAcked(t, registry, providers, opener, 1, controlPort)
	request := openRequest(opener, 1, controlPort)
	request.Key.EnvironmentID = "environment-2"
	done := openAsync(registry, request)
	providerB := <-providers
	providerB.ack(providerB.header(t).GetBindingNonce())
	awaitOpen(t, done)
	registry.CloseInstallation(testInstallation)
	waitFor(t, providerA.wasCanceled)
	waitFor(t, providerB.wasCanceled)
}

func TestGenerationChangeClosesOlderStreamsAndRefusesStaleOpens(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 3)
	opener := newFakeOpener(t, providers)
	_, old := openAcked(t, registry, providers, opener, 1, controlPort)
	_, fresh := openAcked(t, registry, providers, opener, 2, controlPort)
	waitFor(t, old.wasCanceled)
	if fresh.wasCanceled() {
		t.Fatal("the newer generation's stream must stay open")
	}
	if _, err := registry.Open(context.Background(), openRequest(opener, 1, controlPort)); !errors.Is(err, ErrStaleGeneration) {
		t.Fatalf("stale open error = %v, want ErrStaleGeneration", err)
	}
}

func TestSameGenerationOpensDoNotCloseExistingStreams(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 2)
	opener := newFakeOpener(t, providers)
	_, first := openAcked(t, registry, providers, opener, 3, controlPort)
	openAcked(t, registry, providers, opener, 3, controlPort)
	if first.wasCanceled() {
		t.Fatal("a same-generation open must not close existing streams")
	}
}

func TestProviderCloseFrameEndsReadsWithEOFOrItsErrorCode(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 2)
	opener := newFakeOpener(t, providers)
	conn, provider := openAcked(t, registry, providers, opener, 1, controlPort)
	provider.data([]byte("tail"))
	provider.closeFrame("")
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	got, err := io.ReadAll(conn)
	if err != nil || string(got) != "tail" {
		t.Fatalf("ReadAll = %q, %v; want tail and EOF", got, err)
	}

	conn2, provider2 := openAcked(t, registry, providers, opener, 1, controlPort)
	provider2.closeFrame("unavailable")
	_ = conn2.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn2.Read(make([]byte, 1)); err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("read after an error close = %v, want a non-EOF error", err)
	}
}

func TestReadDeadlinesApplyToTunnelConnections(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	conn, _ := openAcked(t, registry, providers, newFakeOpener(t, providers), 1, controlPort)
	defer func() { _ = conn.Close() }()
	_ = conn.SetReadDeadline(time.Now().Add(30 * time.Millisecond))
	_, err := conn.Read(make([]byte, 1))
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		t.Fatalf("read error = %v, want a timeout", err)
	}
}

func waitFor(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not reached in time")
}

func TestReleaseBetweenAckAndStartDoesNotLeakAConnection(t *testing.T) {
	registry := NewRegistry()
	tun := &tunnel{registry: registry, key: Key{InstallationID: testInstallation, EnvironmentID: testEnvironment, Generation: 1}, cancel: func() {}}
	if err := registry.admit(tun); err != nil {
		t.Fatalf("admit: %v", err)
	}
	tun.finish(reasonCancel, false)
	if conn, err := tun.start(nil); err == nil || conn != nil {
		t.Fatalf("start after finish = %v, %v; want an unavailable error", conn, err)
	}
}

func TestAckForAnotherBindingIsRefused(t *testing.T) {
	registry := NewRegistry()
	providers := make(chan *fakeProvider, 1)
	done := openAsync(registry, openRequest(newFakeOpener(t, providers), 1, controlPort))
	provider := <-providers
	provider.header(t)
	provider.ack("a-nonce-the-host-never-issued")
	if result := awaitOpen(t, done); !errors.Is(result.err, ErrUnavailable) {
		t.Fatalf("error = %v, want ErrUnavailable", result.err)
	}
}

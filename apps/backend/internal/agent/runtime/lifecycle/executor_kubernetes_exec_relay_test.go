package lifecycle

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	gwebsocket "github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/apimachinery/pkg/util/httpstream/spdy"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	"github.com/kandev/kandev/internal/agentctl/tcpstdio"
)

// fakeKubeletExec stands in for the apiserver and kubelet behind
// pods/exec. It speaks the real streaming protocols (WebSocket v5 and SPDY
// v4) and runs the real `agentctl tcp-stdio` bridge in process, so the test
// covers the whole relay: the backend's ExecPortForwarder, the live
// client-go executor, and the in-Pod bridge.
type fakeKubeletExec struct {
	t        *testing.T
	spdyOnly bool

	mu       sync.Mutex
	requests []*http.Request
}

func (k *fakeKubeletExec) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/exec") {
		http.NotFound(w, r)
		return
	}
	isWebSocket := strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
	if isWebSocket && k.spdyOnly {
		http.Error(w, "websocket exec disabled", http.StatusBadRequest)
		return
	}
	k.mu.Lock()
	k.requests = append(k.requests, r.Clone(context.Background()))
	k.mu.Unlock()
	if isWebSocket {
		k.serveWebSocket(w, r)
		return
	}
	k.serveSPDY(w, r)
}

func (k *fakeKubeletExec) recorded() []*http.Request {
	k.mu.Lock()
	defer k.mu.Unlock()
	return append([]*http.Request(nil), k.requests...)
}

func runBridgeCommand(r *http.Request, stdin io.Reader, stdout, stderr io.Writer) []byte {
	command := r.URL.Query()["command"]
	status := metav1.Status{Status: metav1.StatusSuccess}
	if len(command) < 2 || command[1] != tcpstdio.Subcommand ||
		tcpstdio.Run(command[2:], stdin, stdout, stderr) != 0 {
		status = metav1.Status{Status: metav1.StatusFailure, Message: "bridge failed"}
	}
	encoded, _ := json.Marshal(status)
	return encoded
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func (k *fakeKubeletExec) serveWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := gwebsocket.Upgrader{
		Subprotocols: []string{"v5.channel.k8s.io"},
		CheckOrigin:  func(*http.Request) bool { return true },
	}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	var writeMu sync.Mutex
	send := func(channel byte, data []byte) error {
		writeMu.Lock()
		defer writeMu.Unlock()
		return conn.WriteMessage(gwebsocket.BinaryMessage, append([]byte{channel}, data...))
	}
	stdin, stdinWriter := io.Pipe()
	go func() {
		defer func() { _ = stdinWriter.Close() }()
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch {
			case len(message) > 0 && message[0] == 0:
				_, _ = stdinWriter.Write(message[1:])
			case len(message) == 2 && message[0] == 255 && message[1] == 0:
				_ = stdinWriter.Close() // v5 CLOSE signal for stdin
			}
		}
	}()
	stdout := writerFunc(func(p []byte) (int, error) { return len(p), send(1, p) })
	stderr := writerFunc(func(p []byte) (int, error) { return len(p), send(2, p) })
	status := runBridgeCommand(r, stdin, stdout, stderr)
	_ = send(3, status)
	writeMu.Lock()
	_ = conn.WriteMessage(gwebsocket.CloseMessage,
		gwebsocket.FormatCloseMessage(gwebsocket.CloseNormalClosure, ""))
	writeMu.Unlock()
}

func (k *fakeKubeletExec) serveSPDY(w http.ResponseWriter, r *http.Request) {
	if _, err := httpstream.Handshake(r, w, []string{"v4.channel.k8s.io"}); err != nil {
		return
	}
	streams := make(chan httpstream.Stream, 4)
	conn := spdy.NewResponseUpgrader().UpgradeResponse(w, r,
		func(stream httpstream.Stream, _ <-chan struct{}) error {
			streams <- stream
			return nil
		})
	if conn == nil {
		return
	}
	defer func() { _ = conn.Close() }()
	byType := map[string]httpstream.Stream{}
	for len(byType) < 4 {
		select {
		case stream := <-streams:
			byType[stream.Headers().Get(corev1.StreamType)] = stream
		case <-time.After(5 * time.Second):
			k.t.Errorf("SPDY exec opened only %d of 4 streams", len(byType))
			return
		}
	}
	status := runBridgeCommand(r, byType[corev1.StreamTypeStdin],
		byType[corev1.StreamTypeStdout], byType[corev1.StreamTypeStderr])
	_, _ = byType[corev1.StreamTypeError].Write(status)
	for _, stream := range byType {
		_ = stream.Close()
	}
}

func writeFakeKubeconfig(t *testing.T, serverURL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "kubeconfig")
	content := "apiVersion: v1\nkind: Config\nclusters:\n- name: fake\n  cluster:\n    server: " + serverURL +
		"\ncontexts:\n- name: fake\n  context:\n    cluster: fake\n    user: fake\ncurrent-context: fake\n" +
		"users:\n- name: fake\n  user:\n    token: test-token\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

// startDelayedResponder plays agentctl inside the Pod: it reads the request
// until EOF, pauses, then answers in several delayed chunks and closes.
func startDelayedResponder(t *testing.T, chunks []string) (uint16, <-chan string) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		request, _ := io.ReadAll(conn)
		received <- string(request)
		time.Sleep(300 * time.Millisecond)
		for _, chunk := range chunks {
			_, _ = conn.Write([]byte(chunk))
			time.Sleep(100 * time.Millisecond)
		}
	}()
	return uint16(listener.Addr().(*net.TCPAddr).Port), received
}

// TestKubernetesExecTransportPreservesResponseAfterRequestEnds drives one
// connection through the live exec transport: the client sends its request
// and half-closes, and only then does the in-Pod server answer, after a
// pause and in several chunks. The full response must arrive over both
// client-go stream protocols, and the exec stream must open in the main
// container of a Pod whose first container is a sidecar.
func TestKubernetesExecTransportPreservesResponseAfterRequestEnds(t *testing.T) {
	for _, tc := range []struct {
		name     string
		spdyOnly bool
		method   string
	}{
		{name: "websocket v5", method: http.MethodGet},
		{name: "spdy v4 fallback", spdyOnly: true, method: http.MethodPost},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chunks := []string{"HTTP/1.1 200 OK\r\n", "Content-Length: 4\r\n", "\r\n", "do", "ne"}
			remotePort, received := startDelayedResponder(t, chunks)
			kubelet := &fakeKubeletExec{t: t, spdyOnly: tc.spdyOnly}
			api := httptest.NewServer(kubelet)
			t.Cleanup(api.Close)
			runtime, err := newKubernetesRuntimeClient(kubeexecutor.ExecutorConfig{
				AuthMode: kubeexecutor.AuthModeKubeconfig, KubeconfigPath: writeFakeKubeconfig(t, api.URL),
				Namespace: "kandev-agents", RequestTimeoutSeconds: 30, Transport: kubeexecutor.StreamTransportExec,
			}, nil)
			require.NoError(t, err)
			forward, err := startKubernetesForward(context.Background(), runtime.streams, sidecarSessionPod(true), remotePort)
			require.NoError(t, err)
			t.Cleanup(func() { _ = forward.Close() })

			conn, err := net.Dial("tcp", net.JoinHostPort(kubernetesControlHost, strconv.Itoa(int(forward.LocalPort()))))
			require.NoError(t, err)
			defer func() { _ = conn.Close() }()
			require.NoError(t, conn.SetDeadline(time.Now().Add(15*time.Second)))
			_, err = conn.Write([]byte("GET /health HTTP/1.1\r\n\r\n"))
			require.NoError(t, err)
			require.NoError(t, conn.(*net.TCPConn).CloseWrite())

			select {
			case got := <-received:
				require.Equal(t, "GET /health HTTP/1.1\r\n\r\n", got)
			case <-time.After(10 * time.Second):
				t.Fatal("in-Pod server never saw the request EOF")
			}
			response, err := io.ReadAll(conn)
			require.NoError(t, err)
			require.Equal(t, strings.Join(chunks, ""), string(response))

			requests := kubelet.recorded()
			require.NotEmpty(t, requests)
			last := requests[len(requests)-1]
			require.Equal(t, tc.method, last.Method, "stream protocol under test")
			require.Equal(t, "/api/v1/namespaces/kandev-agents/pods/session-pod/exec", last.URL.Path)
			require.Equal(t, "kandev-agent", last.URL.Query().Get("container"), "exec must skip the sidecar")
			require.Equal(t, []string{kubernetesAgentctlPath, tcpstdio.Subcommand, "--port", strconv.Itoa(int(remotePort))},
				last.URL.Query()["command"])
		})
	}
}

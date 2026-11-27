package codexappserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestClientCorrelationAndClose(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	serverDone := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(serverInput)
		requests := make([]map[string]json.RawMessage, 2)
		for i := range requests {
			line, err := reader.ReadBytes('\n')
			if err != nil {
				serverDone <- err
				return
			}
			if err := json.Unmarshal(line, &requests[i]); err != nil {
				serverDone <- err
				return
			}
		}

		var writeMu sync.Mutex
		for i := len(requests) - 1; i >= 0; i-- {
			var id json.RawMessage
			var params struct {
				Tag string `json:"tag"`
			}
			if err := json.Unmarshal(requests[i]["id"], &id); err != nil {
				serverDone <- err
				return
			}
			if err := json.Unmarshal(requests[i]["params"], &params); err != nil {
				serverDone <- err
				return
			}
			response, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  map[string]string{"tag": params.Tag},
			})
			if err != nil {
				serverDone <- err
				return
			}
			writeMu.Lock()
			_, err = serverOutput.Write(append(response, '\n'))
			writeMu.Unlock()
			if err != nil {
				serverDone <- err
				return
			}
		}
		serverDone <- nil
	}()

	type result struct {
		Tag string `json:"tag"`
	}
	results := make(chan struct {
		tag string
		got result
		err error
	}, 2)
	for _, tag := range []string{"first", "second"} {
		tag := tag
		go func() {
			var got result
			err := client.Call(context.Background(), "test/echo", map[string]string{"tag": tag}, &got)
			results <- struct {
				tag string
				got result
				err error
			}{tag: tag, got: got, err: err}
		}()
	}

	for range 2 {
		select {
		case got := <-results:
			if got.err != nil {
				t.Fatalf("Call(%q): %v; client terminal error: %v", got.tag, got.err, client.Err())
			}
			if got.got.Tag != got.tag {
				t.Errorf("Call(%q) returned tag %q", got.tag, got.got.Tag)
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent calls did not complete")
		}
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake server: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("client did not close")
	}
}

func TestServerRequestResolvedCancelsPendingHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	client := &Client{
		ctx:            ctx,
		cancel:         cancel,
		serverRequests: make(map[string]*serverRequestState),
	}
	state, err := client.registerServerRequest(json.RawMessage(`17`))
	if err != nil {
		t.Fatalf("registerServerRequest: %v", err)
	}
	client.resolveServerRequest(json.RawMessage(`{"requestId":17}`))
	select {
	case <-state.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("serverRequest/resolved did not cancel the handler context")
	}
	if client.beginServerRequestResponse(state) {
		t.Fatal("resolved server request remained eligible for a late response")
	}
}

func TestClientAcceptsCodexAppServerResponseWithoutJSONRPCVersion(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	serverDone := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(serverInput).ReadBytes('\n')
		if err != nil {
			serverDone <- err
			return
		}
		var request map[string]json.RawMessage
		if err := json.Unmarshal(line, &request); err != nil {
			serverDone <- err
			return
		}
		response, err := json.Marshal(map[string]any{
			"id":     request["id"],
			"result": map[string]string{"status": "ready"},
		})
		if err == nil {
			_, err = serverOutput.Write(append(response, '\n'))
		}
		serverDone <- err
	}()

	var result struct {
		Status string `json:"status"`
	}
	if err := client.Call(context.Background(), "initialize", map[string]string{"client": "kandev"}, &result); err != nil {
		t.Fatalf("Codex app-server response without jsonrpc version: %v", err)
	}
	if result.Status != "ready" {
		t.Fatalf("initialize status = %q, want ready", result.Status)
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake Codex server: %v", err)
	}
}

func TestClientFlushInboundWaitsForEarlierNotifications(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	notificationStarted := make(chan struct{})
	releaseNotification := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseNotification) }) }
	t.Cleanup(release)
	client.SetNotificationHandler(func(_ context.Context, _ string, _ json.RawMessage) {
		close(notificationStarted)
		<-releaseNotification
	})

	serverDone := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(serverInput).ReadBytes('\n')
		if err != nil {
			serverDone <- err
			return
		}
		var request map[string]json.RawMessage
		if err := json.Unmarshal(line, &request); err != nil {
			serverDone <- err
			return
		}
		for _, frame := range []any{
			map[string]any{"method": "test/notification", "params": map[string]string{"text": "before response"}},
			map[string]any{"id": request["id"], "result": map[string]string{"status": "complete"}},
		} {
			encoded, err := json.Marshal(frame)
			if err == nil {
				_, err = serverOutput.Write(append(encoded, '\n'))
			}
			if err != nil {
				serverDone <- err
				return
			}
		}
		serverDone <- nil
	}()

	if err := client.Call(context.Background(), "test/call", nil, nil); err != nil {
		t.Fatalf("Call: %v", err)
	}
	select {
	case <-notificationStarted:
	case <-time.After(time.Second):
		t.Fatal("notification handler did not start")
	}
	if err := <-serverDone; err != nil {
		t.Fatalf("fake server: %v", err)
	}

	flushCtx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.FlushInbound(flushCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("FlushInbound while notification is blocked = %v, want context deadline exceeded", err)
	}
	release()
	if err := client.FlushInbound(context.Background()); err != nil {
		t.Fatalf("FlushInbound after notification completed: %v", err)
	}
}

func TestClientAcceptsCodexAppServerRequestWithoutJSONRPCVersion(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})
	client.SetRequestHandler(func(_ context.Context, request ServerRequest) (any, error) {
		return map[string]string{"handled": request.Method}, nil
	})

	if _, err := io.WriteString(serverOutput, `{"id":"request-1","method":"item/commandExecution/requestApproval"}`+"\n"); err != nil {
		t.Fatalf("send Codex app-server request: %v", err)
	}
	line, err := bufio.NewReader(serverInput).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read client response: %v", err)
	}
	var response struct {
		JSONRPC string            `json:"jsonrpc"`
		ID      string            `json:"id"`
		Result  map[string]string `json:"result"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode client response: %v", err)
	}
	if response.JSONRPC != "2.0" || response.ID != "request-1" || response.Result["handled"] != "item/commandExecution/requestApproval" {
		t.Fatalf("client response = %+v", response)
	}
}

func TestClientRejectsUnsupportedExplicitJSONRPCVersion(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	go func() {
		line, err := bufio.NewReader(serverInput).ReadBytes('\n')
		if err != nil {
			return
		}
		var request map[string]json.RawMessage
		if json.Unmarshal(line, &request) != nil {
			return
		}
		response, _ := json.Marshal(map[string]any{
			"jsonrpc": "1.0",
			"id":      request["id"],
			"result":  map[string]string{"status": "invalid"},
		})
		_, _ = serverOutput.Write(append(response, '\n'))
	}()

	var result map[string]string
	err := client.Call(context.Background(), "initialize", map[string]string{"client": "kandev"}, &result)
	if err == nil || !strings.Contains(err.Error(), `unsupported JSON-RPC version "1.0"`) {
		t.Fatalf("Call error = %v, want unsupported explicit JSON-RPC version", err)
	}
}

func TestClientServerRequestsKeepStringAndNumericIDsDistinct(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	client.SetRequestHandler(func(_ context.Context, request ServerRequest) (any, error) {
		return map[string]string{"method": request.Method, "requestID": string(request.ID)}, nil
	})

	for _, id := range []string{`"7"`, `7`} {
		request := fmt.Sprintf(`{"jsonrpc":"2.0","id":%s,"method":"client/test"}`+"\n", id)
		if _, err := io.WriteString(serverOutput, request); err != nil {
			t.Fatalf("send server request: %v", err)
		}
	}

	reader := bufio.NewReader(serverInput)
	seen := make(map[string]bool)
	for range 2 {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			t.Fatalf("read server response: %v", err)
		}
		var response struct {
			ID     json.RawMessage   `json:"id"`
			Result map[string]string `json:"result"`
		}
		if err := json.Unmarshal(line, &response); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		key, err := responseIDKey(response.ID)
		if err != nil {
			t.Fatalf("decode response id key: %v", err)
		}
		if seen[key] {
			t.Errorf("duplicate response id key %q", key)
		}
		seen[key] = true
		if response.Result["method"] != "client/test" || response.Result["requestID"] != string(response.ID) {
			t.Errorf("response result = %#v", response.Result)
		}
	}
	if !seen["s:7"] || !seen["n:7"] {
		t.Fatalf("responses did not preserve string/numeric identities: %v", seen)
	}
}

func TestClientCancellationAndEOFReleasePendingCalls(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})
	frameWritten := make(chan struct{})
	releaseObserver := make(chan struct{})
	var firstFrame sync.Once
	client.SetFrameObserver(func(direction FrameDirection, _ json.RawMessage) error {
		if direction == FrameSent {
			firstFrame.Do(func() {
				close(frameWritten)
				<-releaseObserver
			})
		}
		return nil
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	go func() { cancelled <- client.Call(ctx, "test/cancel", nil, nil) }()
	if _, err := bufio.NewReader(serverInput).ReadBytes('\n'); err != nil {
		t.Fatalf("read first request: %v", err)
	}
	select {
	case <-frameWritten:
	case <-time.After(time.Second):
		t.Fatal("first request write did not reach the frame observer")
	}
	close(releaseObserver)
	cancel()
	select {
	case err := <-cancelled:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled call error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled call remained pending")
	}

	pending := make(chan error, 1)
	go func() { pending <- client.Call(context.Background(), "test/eof", nil, nil) }()
	if _, err := bufio.NewReader(serverInput).ReadBytes('\n'); err != nil {
		t.Fatalf("read second request: %v", err)
	}
	_ = serverOutput.Close()
	select {
	case err := <-pending:
		if err == nil || errors.Is(err, context.Canceled) {
			t.Fatalf("EOF call error = %v, want transport error", err)
		}
	case <-time.After(time.Second):
		t.Fatal("EOF did not release pending call")
	}
}

type blockingWriteCloser struct {
	started chan struct{}
	closed  chan struct{}
	once    sync.Once
}

func (w *blockingWriteCloser) Write([]byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.closed
	return 0, io.ErrClosedPipe
}

func (w *blockingWriteCloser) Close() error {
	select {
	case <-w.closed:
	default:
		close(w.closed)
	}
	return nil
}

func TestClientCancelsStalledWriteAndClosesTransport(t *testing.T) {
	writer := &blockingWriteCloser{started: make(chan struct{}), closed: make(chan struct{})}
	stdout, serverOutput := io.Pipe()
	client := NewClient(writer, stdout, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = stdout.Close()
		_ = serverOutput.Close()
	})

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- client.Notify(ctx, "test/stalled-write", nil) }()
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("writer did not begin the test frame")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Notify error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled Notify remained blocked")
	}
	select {
	case <-client.Done():
	case <-time.After(time.Second):
		t.Fatal("stalled write did not terminate the client")
	}
	select {
	case <-writer.closed:
	default:
		t.Fatal("canceled stalled write did not close the transport writer")
	}
	if err := client.Notify(context.Background(), "test/after-stall", nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("post-stall Notify error = %v, want the terminal write cancellation", err)
	}
}

func TestClientRejectsFramesOverConfiguredLimit(t *testing.T) {
	client := NewClient(io.Discard, io.NopCloser(strings.NewReader(strings.Repeat("x", 33)+"\n")), Options{MaxFrameBytes: 32})
	defer func() { _ = client.Close() }()
	select {
	case <-client.Done():
		if !strings.Contains(client.Err().Error(), "frame exceeds 32 bytes") {
			t.Fatalf("client error = %v, want frame-limit error", client.Err())
		}
	case <-time.After(time.Second):
		t.Fatal("oversized frame did not fail the client")
	}
}

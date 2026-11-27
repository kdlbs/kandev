package codexappserver

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

func TestApprovalWaitDoesNotBlockNotifications(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	notifications := make(chan string, 1)
	client.SetRequestHandler(func(ctx context.Context, _ ServerRequest) (any, error) {
		close(requestStarted)
		select {
		case <-releaseRequest:
			return map[string]string{"status": "resolved"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	client.SetNotificationHandler(func(_ context.Context, method string, _ json.RawMessage) {
		notifications <- method
	})

	frames := strings.Join([]string{
		`{"id":"approval-1","method":"item/commandExecution/requestApproval"}`,
		`{"method":"turn/completed","params":{"turnId":"turn-1"}}`,
	}, "\n") + "\n"
	if _, err := io.WriteString(serverOutput, frames); err != nil {
		t.Fatalf("send approval and notification: %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("approval handler did not start")
	}
	select {
	case method := <-notifications:
		if method != "turn/completed" {
			t.Fatalf("notification method = %q, want turn/completed", method)
		}
	case <-time.After(time.Second):
		t.Fatal("approval wait blocked notification dispatch")
	}

	close(releaseRequest)
	line, err := bufio.NewReader(serverInput).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read approval response: %v", err)
	}
	var response struct {
		ID     string            `json:"id"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode approval response: %v", err)
	}
	if response.ID != "approval-1" || response.Result["status"] != "resolved" {
		t.Fatalf("approval response = %+v", response)
	}
}

func TestServerRequestAdmissionIsBoundedAndRejectsOverflow(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{MaxConcurrentRequests: 1})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	requestStarted := make(chan struct{}, 1)
	releaseRequest := make(chan struct{})
	notifications := make(chan string, 1)
	requestMethods := make(chan string, 2)
	client.SetRequestHandler(func(ctx context.Context, request ServerRequest) (any, error) {
		requestMethods <- request.Method
		requestStarted <- struct{}{}
		select {
		case <-releaseRequest:
			return map[string]string{"status": "resolved"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	client.SetNotificationHandler(func(_ context.Context, method string, _ json.RawMessage) {
		notifications <- method
	})

	if _, err := io.WriteString(serverOutput, `{"id":"approval-1","method":"approval/first"}`+"\n"); err != nil {
		t.Fatalf("send first approval: %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("first approval handler did not start")
	}
	frames := strings.Join([]string{
		`{"method":"turn/completed","params":{"turnId":"turn-1"}}`,
		`{"id":"approval-2","method":"approval/overflow"}`,
	}, "\n") + "\n"
	if _, err := io.WriteString(serverOutput, frames); err != nil {
		t.Fatalf("send notification and overflow approval: %v", err)
	}
	select {
	case method := <-notifications:
		if method != "turn/completed" {
			t.Fatalf("notification method = %q, want turn/completed", method)
		}
	case <-time.After(time.Second):
		t.Fatal("overflow approval blocked notification dispatch")
	}

	reader := bufio.NewReader(serverInput)
	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read overflow response: %v", err)
	}
	var overflow struct {
		ID    string `json:"id"`
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &overflow); err != nil {
		t.Fatalf("decode overflow response: %v", err)
	}
	if overflow.ID != "approval-2" || overflow.Error.Code != -32000 {
		t.Fatalf("overflow response = %+v, want explicit server overload error", overflow)
	}
	if overflow.Error.Message == "" {
		t.Fatal("overflow response omitted its error message")
	}

	close(releaseRequest)
	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read first approval response: %v", err)
	}
	var first struct {
		ID     string            `json:"id"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(line, &first); err != nil {
		t.Fatalf("decode first approval response: %v", err)
	}
	if first.ID != "approval-1" || first.Result["status"] != "resolved" {
		t.Fatalf("first approval response = %+v", first)
	}
	if got := <-requestMethods; got != "approval/first" {
		t.Fatalf("accepted request handler called with %q", got)
	}
	select {
	case method := <-requestMethods:
		t.Fatalf("overloaded request reached handler: %q", method)
	default:
	}
}

func TestResolvedServerRequestCancelsOnlyMatchingRequestID(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{MaxConcurrentRequests: 2})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	started := make(chan string, 2)
	cancelled := make(chan string, 2)
	releaseNumeric := make(chan struct{})
	client.SetRequestHandler(func(ctx context.Context, request ServerRequest) (any, error) {
		key, err := responseIDKey(request.ID)
		if err != nil {
			return nil, err
		}
		started <- key
		select {
		case <-ctx.Done():
			cancelled <- key
			return nil, ctx.Err()
		case <-releaseNumeric:
			return map[string]string{"status": "resolved"}, nil
		}
	})

	for _, id := range []string{`"7"`, `7`} {
		request := fmt.Sprintf(`{"id":%s,"method":"approval/wait"}`+"\n", id)
		if _, err := io.WriteString(serverOutput, request); err != nil {
			t.Fatalf("send server request: %v", err)
		}
	}
	for range 2 {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("server request handler did not start")
		}
	}
	if _, err := io.WriteString(serverOutput, `{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":"7"}}`+"\n"); err != nil {
		t.Fatalf("send serverRequest/resolved: %v", err)
	}
	select {
	case key := <-cancelled:
		if key != "s:7" {
			t.Fatalf("resolved request cancellation key = %q, want s:7", key)
		}
	case <-time.After(time.Second):
		t.Fatal("serverRequest/resolved did not cancel the matching request")
	}

	close(releaseNumeric)
	line, err := bufio.NewReader(serverInput).ReadBytes('\n')
	if err != nil {
		t.Fatalf("read numeric request response: %v", err)
	}
	var response struct {
		ID     json.RawMessage   `json:"id"`
		Result map[string]string `json:"result"`
	}
	if err := json.Unmarshal(line, &response); err != nil {
		t.Fatalf("decode numeric request response: %v", err)
	}
	if string(response.ID) != "7" || response.Result["status"] != "resolved" {
		t.Fatalf("response after resolving string ID = %s; want response for numeric ID 7", line)
	}
}

type gatedResponseWriter struct {
	started chan struct{}
	release chan struct{}
	written chan []byte
}

func (w *gatedResponseWriter) Write(data []byte) (int, error) {
	close(w.started)
	<-w.release
	w.written <- append([]byte(nil), data...)
	return len(data), nil
}

func TestAnswerWinsRequestResolutionWithoutDoubleResponse(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	writer := &gatedResponseWriter{
		started: make(chan struct{}), release: make(chan struct{}), written: make(chan []byte, 1),
	}
	client := NewClient(writer, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
	})

	requestStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	notifications := make(chan string, 1)
	client.SetRequestHandler(func(ctx context.Context, _ ServerRequest) (any, error) {
		close(requestStarted)
		select {
		case <-releaseHandler:
			return map[string]string{"decision": "accept"}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	})
	client.SetNotificationHandler(func(_ context.Context, method string, _ json.RawMessage) {
		notifications <- method
	})
	if _, err := io.WriteString(serverOutput, `{"id":"approval-1","method":"item/commandExecution/requestApproval"}`+"\n"); err != nil {
		t.Fatalf("send approval request: %v", err)
	}
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("approval handler did not start")
	}
	close(releaseHandler)
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("approval response did not reach the writer")
	}
	if _, err := io.WriteString(serverOutput, `{"method":"serverRequest/resolved","params":{"threadId":"thread-1","requestId":"approval-1"}}`+"\n"); err != nil {
		t.Fatalf("send resolution notification: %v", err)
	}
	select {
	case method := <-notifications:
		if method != NotificationServerRequestResolved {
			t.Fatalf("notification method = %q", method)
		}
	case <-time.After(time.Second):
		t.Fatal("resolution notification was not dispatched")
	}

	close(writer.release)
	select {
	case frame := <-writer.written:
		var response struct {
			ID     string            `json:"id"`
			Result map[string]string `json:"result"`
		}
		if err := json.Unmarshal(frame, &response); err != nil {
			t.Fatalf("decode approval response: %v", err)
		}
		if response.ID != "approval-1" || response.Result["decision"] != "accept" {
			t.Fatalf("approval response = %+v", response)
		}
	case <-time.After(time.Second):
		t.Fatal("approval response was suppressed after it claimed the request")
	}
	select {
	case extra := <-writer.written:
		t.Fatalf("request produced a second response: %s", extra)
	default:
	}
}

func TestClientCloseCancelsPendingServerRequestHandler(t *testing.T) {
	clientInput, serverOutput := io.Pipe()
	serverInput, clientOutput := io.Pipe()
	client := NewClient(clientOutput, clientInput, Options{})
	t.Cleanup(func() {
		_ = client.Close()
		_ = serverOutput.Close()
		_ = serverInput.Close()
	})

	started := make(chan struct{})
	cancelled := make(chan struct{})
	handlerDone := make(chan struct{})
	client.SetRequestHandler(func(ctx context.Context, _ ServerRequest) (any, error) {
		close(started)
		defer close(handlerDone)
		<-ctx.Done()
		close(cancelled)
		return nil, ctx.Err()
	})
	if _, err := io.WriteString(serverOutput, `{"id":"approval-close","method":"item/commandExecution/requestApproval"}`+"\n"); err != nil {
		t.Fatalf("send approval request: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("approval handler did not start")
	}
	if err := client.Close(); err != nil {
		t.Fatalf("close client: %v", err)
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("closing the client did not cancel the pending approval")
	}
	select {
	case <-handlerDone:
	case <-time.After(time.Second):
		t.Fatal("pending approval handler did not exit")
	}
}

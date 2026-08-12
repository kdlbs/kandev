package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	sharedlsp "github.com/kandev/kandev/internal/lsp"
)

type recordedFeatureMessage struct {
	method string
	params json.RawMessage
}

type recordingFeatureUpstream struct {
	mu            sync.Mutex
	requests      []recordedFeatureMessage
	notifications []recordedFeatureMessage
	blockMethod   string
	canceled      chan struct{}
	cancelOnce    sync.Once
}

func (u *recordingFeatureUpstream) Request(
	ctx context.Context,
	method string,
	params json.RawMessage,
) (json.RawMessage, error) {
	u.mu.Lock()
	u.requests = append(u.requests, recordedFeatureMessage{method: method, params: cloneRaw(params)})
	block := method == u.blockMethod
	u.mu.Unlock()
	if block {
		<-ctx.Done()
		u.cancelOnce.Do(func() { close(u.canceled) })
		return nil, ctx.Err()
	}
	return json.RawMessage(`{"echo":` + string(params) + `}`), nil
}

func (u *recordingFeatureUpstream) Notify(method string, params json.RawMessage) error {
	u.mu.Lock()
	u.notifications = append(u.notifications, recordedFeatureMessage{
		method: method, params: cloneRaw(params),
	})
	u.mu.Unlock()
	return nil
}

func (u *recordingFeatureUpstream) notificationSnapshot() []recordedFeatureMessage {
	u.mu.Lock()
	defer u.mu.Unlock()
	return append([]recordedFeatureMessage(nil), u.notifications...)
}

func newHubForTest(upstream featureUpstream) (*hub, Snapshot) {
	snapshot := Snapshot{
		Language:      "kotlin",
		Generation:    7,
		Phase:         sharedlsp.PhaseReady,
		Capabilities:  json.RawMessage(`{"hoverProvider":true}`),
		WorkspacePath: "/workspace",
		WorkspaceURI:  "file:///workspace",
		WorkspaceFolders: []WorkspaceFolder{
			{URI: "file:///workspace/repo", Name: "repo"},
		},
		Diagnostics: []json.RawMessage{},
	}
	return newHub("kotlin", 7, upstream), snapshot
}

func TestHubRemapsCollidingRequestIDsPerAttachment(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	first := hub.Attach(snapshot)
	second := hub.Attach(snapshot)
	drainAttached(t, first)
	drainAttached(t, second)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	if err := first.Handle([]byte(
		`{"jsonrpc":"2.0","id":1,"method":"textDocument/hover","params":{"owner":"first"}}`,
	)); err != nil {
		t.Fatal(err)
	}
	if err := second.Handle([]byte(
		`{"jsonrpc":"2.0","id":1,"method":"textDocument/hover","params":{"owner":"second"}}`,
	)); err != nil {
		t.Fatal(err)
	}
	firstResponse := readAttachmentMessage(t, first)
	secondResponse := readAttachmentMessage(t, second)
	if !rawContains(firstResponse, `"id":1`) || !rawContains(firstResponse, `"owner":"first"`) {
		t.Fatalf("first response = %s", firstResponse)
	}
	if !rawContains(secondResponse, `"id":1`) || !rawContains(secondResponse, `"owner":"second"`) {
		t.Fatalf("second response = %s", secondResponse)
	}
}

func TestAttachmentCancellationAndDetachDiscardPendingResponse(t *testing.T) {
	upstream := &recordingFeatureUpstream{
		blockMethod: "textDocument/hover",
		canceled:    make(chan struct{}),
	}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	drainAttached(t, attachment)

	if err := attachment.Handle([]byte(
		`{"jsonrpc":"2.0","id":"same","method":"textDocument/hover","params":{}}`,
	)); err != nil {
		t.Fatal(err)
	}
	if err := attachment.Handle([]byte(
		`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":"same"}}`,
	)); err != nil {
		t.Fatal(err)
	}
	select {
	case <-upstream.canceled:
	case <-time.After(time.Second):
		t.Fatal("upstream request was not canceled")
	}
	attachment.Close()
	for message := range attachment.Messages() {
		if rawContains(message, `"result"`) {
			t.Fatalf("detached attachment received stale success: %s", message)
		}
	}
}

func TestAttachmentBoundsPendingRequestsAndClosesPromptly(t *testing.T) {
	upstream := &recordingFeatureUpstream{
		blockMethod: "textDocument/hover",
		canceled:    make(chan struct{}),
	}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	drainAttached(t, attachment)

	for id := 1; id <= attachmentQueueSize+1; id++ {
		message := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":%d,"method":"textDocument/hover","params":{}}`,
			id,
		)
		if err := attachment.Handle([]byte(message)); err != nil {
			t.Fatal(err)
		}
	}
	overload := readAttachmentMessage(t, attachment)
	if !rawContains(overload, `"id":257`) || !rawContains(overload, `"code":-32000`) {
		t.Fatalf("pending-request overload response = %s", overload)
	}

	closed := make(chan struct{})
	go func() {
		attachment.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("attachment close blocked behind flooded pending requests")
	}
}

func TestHubHandshakeDiagnosticReplayAndNotificationFanout(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	snapshot.Diagnostics = []json.RawMessage{
		json.RawMessage(`{"uri":"file:///workspace/Main.kt","diagnostics":[{"message":"broken"}]}`),
	}
	t.Cleanup(hub.Close)
	first := hub.Attach(snapshot)
	second := hub.Attach(snapshot)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	handshake := readAttachmentMessage(t, first)
	if !rawContains(handshake, `"status":"attached"`) ||
		!rawContains(handshake, `"generation":7`) ||
		!rawContains(handshake, `"hoverProvider":true`) {
		t.Fatalf("handshake = %s", handshake)
	}
	diagnostic := readAttachmentMessage(t, first)
	if !rawContains(diagnostic, `textDocument/publishDiagnostics`) || !rawContains(diagnostic, `broken`) {
		t.Fatalf("diagnostic replay = %s", diagnostic)
	}
	drainAttached(t, second)
	_ = readAttachmentMessage(t, second)

	hub.Broadcast("window/logMessage", json.RawMessage(`{"type":3,"message":"ready"}`))
	for _, attachment := range []*Attachment{first, second} {
		message := readAttachmentMessage(t, attachment)
		if !rawContains(message, `window/logMessage`) || !rawContains(message, `ready`) {
			t.Fatalf("fanout = %s", message)
		}
	}
}

func TestHubAttachmentReplaysEveryCachedDiagnosticBeyondLiveQueueCapacity(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	snapshot.Diagnostics = make([]json.RawMessage, attachmentQueueSize+44)
	for index := range snapshot.Diagnostics {
		snapshot.Diagnostics[index] = json.RawMessage(fmt.Sprintf(
			`{"uri":"file:///workspace/File%d.kt","diagnostics":[{"message":"diagnostic-%d"}]}`,
			index,
			index,
		))
	}
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	t.Cleanup(attachment.Close)

	drainAttached(t, attachment)
	for index := range snapshot.Diagnostics {
		message := readAttachmentMessage(t, attachment)
		if !rawContains(message, fmt.Sprintf(`diagnostic-%d`, index)) {
			t.Fatalf("diagnostic replay %d = %s", index, message)
		}
	}

	hub.Broadcast("window/logMessage", json.RawMessage(`{"type":3,"message":"live"}`))
	if message := readAttachmentMessage(t, attachment); !rawContains(message, `"message":"live"`) {
		t.Fatalf("live notification after replay = %s", message)
	}
}

func TestHubQueueOverflowClosesAttachmentMessageStream(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	drainAttached(t, attachment)
	t.Cleanup(attachment.Close)

overflow:
	for index := 0; index < attachmentQueueSize*2; index++ {
		hub.Broadcast("window/logMessage", json.RawMessage(fmt.Sprintf(`{"message":"%d"}`, index)))
		select {
		case <-attachment.Done():
			break overflow
		default:
		}
	}
	select {
	case <-attachment.Done():
	case <-time.After(time.Second):
		t.Fatal("attachment did not fail after its live queue overflowed")
	}
	hub.mu.RLock()
	_, stillAttached := hub.attachments[attachment.id]
	hub.mu.RUnlock()
	if stillAttached {
		t.Fatal("failed attachment remained registered for live fanout")
	}

	streamClosed := make(chan struct{})
	go func() {
		for range attachment.Messages() {
		}
		close(streamClosed)
	}()
	select {
	case <-streamClosed:
	case <-time.After(time.Second):
		t.Fatal("failed attachment left its message stream open")
	}
}

func TestHubAttachPublishesHandshakeBeforeConcurrentBroadcast(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	hub.documents.mu.Lock()
	attached := make(chan *Attachment, 1)
	go func() { attached <- hub.Attach(snapshot) }()
	time.Sleep(20 * time.Millisecond)

	registered := make(chan bool, 1)
	go func() {
		hub.mu.RLock()
		registered <- len(hub.attachments) != 0
		hub.mu.RUnlock()
	}()
	wasVisibleBeforeReplay := false
	select {
	case wasVisibleBeforeReplay = <-registered:
	case <-time.After(100 * time.Millisecond):
	}
	broadcastDone := make(chan struct{})
	go func() {
		hub.Broadcast("window/logMessage", json.RawMessage(`{"type":3,"message":"ready"}`))
		close(broadcastDone)
	}()
	if wasVisibleBeforeReplay {
		<-broadcastDone
	}
	hub.documents.mu.Unlock()
	attachment := <-attached
	t.Cleanup(attachment.Close)

	first := readAttachmentMessage(t, attachment)
	if !rawContains(first, `"status":"attached"`) {
		t.Fatalf("first attachment message = %s", first)
	}
	<-broadcastDone
}

func TestAttachmentRejectsLifecycleMessagesWithoutClosing(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	drainAttached(t, attachment)
	t.Cleanup(attachment.Close)

	if err := attachment.Handle([]byte(
		`{"jsonrpc":"2.0","id":9,"method":"initialize","params":{}}`,
	)); err != nil {
		t.Fatal(err)
	}
	response := readAttachmentMessage(t, attachment)
	if !rawContains(response, `"id":9`) || !rawContains(response, `"code":-32600`) {
		t.Fatalf("lifecycle rejection = %s", response)
	}
	for _, method := range []string{"initialized", "shutdown", "exit"} {
		if err := attachment.Handle([]byte(
			`{"jsonrpc":"2.0","method":"` + method + `","params":null}`,
		)); err != nil {
			t.Fatal(err)
		}
	}
	if got := upstream.notificationSnapshot(); len(got) != 0 {
		t.Fatalf("lifecycle notifications reached upstream: %#v", got)
	}
	if err := attachment.Handle([]byte(
		`{"jsonrpc":"2.0","id":10,"method":"textDocument/hover","params":{}}`,
	)); err != nil {
		t.Fatal(err)
	}
	if response := readAttachmentMessage(t, attachment); !rawContains(response, `"id":10`) {
		t.Fatalf("attachment closed after rejection: %s", response)
	}
}

func drainAttached(t *testing.T, attachment *Attachment) {
	t.Helper()
	message := readAttachmentMessage(t, attachment)
	if !rawContains(message, `"status":"attached"`) {
		t.Fatalf("first attachment message = %s", message)
	}
}

func readAttachmentMessage(t *testing.T, attachment *Attachment) []byte {
	t.Helper()
	select {
	case message, open := <-attachment.Messages():
		if !open {
			t.Fatal("attachment closed before message")
		}
		return message
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for attachment message")
		return nil
	}
}

func rawContains(message []byte, part string) bool {
	return json.Valid(message) && stringContains(message, part)
}

func cloneRaw(raw json.RawMessage) json.RawMessage {
	return append(json.RawMessage(nil), raw...)
}

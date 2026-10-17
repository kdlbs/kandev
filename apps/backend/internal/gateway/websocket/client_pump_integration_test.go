package websocket

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorillaws "github.com/gorilla/websocket"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/common/logger"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// connectedClient is a live browser connection driving a real Client with both
// pumps running, plus the handles needed to join on their shutdown.
type connectedClient struct {
	browser *gorillaws.Conn
	client  *Client
	hub     *Hub
	stopped <-chan struct{}
}

// newConnectedClient wires a hub, a real WebSocket connection, and both client
// pumps together so tests can exercise the full receive/dispatch/send path.
func newConnectedClient(t *testing.T) *connectedClient {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hub := NewHub(nil, log)

	hubCtx, cancelHub := context.WithCancel(context.Background())
	hubDone := make(chan struct{})
	go func() {
		defer close(hubDone)
		hub.Run(hubCtx)
	}()
	t.Cleanup(func() {
		cancelHub()
		<-hubDone
	})

	ready := make(chan *Client, 1)
	pumpsDone := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, upgradeErr := testWSUpgrader.Upgrade(w, r, nil)
		if upgradeErr != nil {
			return
		}
		client := NewClient("client-1", authn.Identity{}, conn, hub, log)
		hub.Register(client)
		ready <- client

		writeDone := make(chan struct{})
		go func() {
			defer close(writeDone)
			client.WritePump()
		}()
		client.ReadPump(r.Context())
		<-writeDone
		close(pumpsDone)
	}))
	t.Cleanup(srv.Close)

	browser, resp, err := gorillaws.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		t.Fatalf("dial gateway: %v", err)
	}
	t.Cleanup(func() { _ = browser.Close() })

	// Join the pumps from a cleanup registered before the assertions below, so
	// an early t.Fatal cannot leave them running into a later test.
	t.Cleanup(func() {
		_ = browser.Close()
		pumpTimer := time.NewTimer(wsTestTimeout)
		defer pumpTimer.Stop()
		select {
		case <-pumpsDone:
		case <-pumpTimer.C:
			t.Errorf("client pumps did not return within %s", wsTestTimeout)
		}
	})

	readyTimer := time.NewTimer(wsTestTimeout)
	defer readyTimer.Stop()
	var client *Client
	select {
	case client = <-ready:
		waitForClientCount(t, hub, client.ID)
	case <-readyTimer.C:
		t.Fatal("gateway never registered the client")
	}

	return &connectedClient{browser: browser, client: client, hub: hub, stopped: pumpsDone}
}

// roundTrip sends one frame and reads the single reply it produces.
func (cc *connectedClient) roundTrip(t *testing.T, raw []byte) ws.Message {
	t.Helper()
	if err := cc.browser.WriteMessage(gorillaws.TextMessage, raw); err != nil {
		t.Fatalf("browser write: %v", err)
	}
	if err := cc.browser.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
		t.Fatalf("set read deadline: %v", err)
	}
	_, data, err := cc.browser.ReadMessage()
	if err != nil {
		t.Fatalf("browser read: %v", err)
	}
	var msg ws.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("decode gateway frame %s: %v", data, err)
	}
	return msg
}

func (cc *connectedClient) request(t *testing.T, id, action string, payload any) ws.Message {
	t.Helper()
	raw, err := json.Marshal(ws.Message{
		ID:      id,
		Type:    ws.MessageTypeRequest,
		Action:  action,
		Payload: mustJSON(t, payload),
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	return cc.roundTrip(t, raw)
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return raw
}

func TestReadPumpRejectsMalformedFramesWithoutDroppingTheConnection(t *testing.T) {
	cc := newConnectedClient(t)

	bad := cc.roundTrip(t, []byte("{not json"))
	if bad.Type != ws.MessageTypeError {
		t.Fatalf("response = %+v, want an error frame", bad)
	}
	if code := errorCode(t, bad); code != ws.ErrorCodeBadRequest {
		t.Fatalf("error code = %q, want %q", code, ws.ErrorCodeBadRequest)
	}

	// The connection must survive a bad frame and keep serving requests.
	ack := cc.request(t, "req-after-bad", ws.ActionTaskSubscribe, SubscribeRequest{TaskID: "task-1"})
	if ack.Type != ws.MessageTypeResponse {
		t.Fatalf("follow-up response = %+v, want a success response", ack)
	}
}

// MCP actions are PAT-authenticated over /mcp; the raw socket carries no
// credential of its own and must refuse them before dispatch.
func TestReadPumpRefusesMCPActionsOverTheRawSocket(t *testing.T) {
	cc := newConnectedClient(t)

	resp := cc.request(t, "req-mcp", "mcp.tools.list", map[string]string{})
	if resp.Type != ws.MessageTypeError {
		t.Fatalf("response = %+v, want an error frame", resp)
	}
	if code := errorCode(t, resp); code != ws.ErrorCodeForbidden {
		t.Fatalf("error code = %q, want %q", code, ws.ErrorCodeForbidden)
	}
}

func TestConnectedClientSubscriptionRoundTrips(t *testing.T) {
	cc := newConnectedClient(t)

	if resp := cc.request(t, "1", ws.ActionTaskSubscribe, SubscribeRequest{TaskID: "task-1"}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("task.subscribe response = %+v", resp)
	}
	cc.assertTaskSubscribed(t, "task-1", true)

	if resp := cc.request(t, "2", ws.ActionTaskUnsubscribe, SubscribeRequest{TaskID: "task-1"}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("task.unsubscribe response = %+v", resp)
	}
	cc.assertTaskSubscribed(t, "task-1", false)

	if resp := cc.request(t, "3", ws.ActionSystemMetricsSubscribe, map[string]string{}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("system.metrics.subscribe response = %+v", resp)
	}
	cc.assertMetricsSubscribed(t, true)

	if resp := cc.request(t, "4", ws.ActionSystemMetricsUnsubscribe, map[string]string{}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("system.metrics.unsubscribe response = %+v", resp)
	}
	cc.assertMetricsSubscribed(t, false)

	if resp := cc.request(t, "5", ws.ActionRunSubscribe, RunSubscribeRequest{RunID: "run-1"}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("run.subscribe response = %+v", resp)
	}
	if resp := cc.request(t, "6", ws.ActionRunUnsubscribe, RunSubscribeRequest{RunID: "run-1"}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("run.unsubscribe response = %+v", resp)
	}
}

func TestReadPumpPreservesTaskSubscriptionMutationOrder(t *testing.T) {
	cc := newConnectedClient(t)
	subscribeEntered := make(chan struct{})
	releaseSubscribe := make(chan struct{})
	var enteredOnce sync.Once
	cc.hub.setAuthPolicy(AuthPolicy{Subscriptions: SubscriptionAccessPolicy{
		Task: func(context.Context, string) error {
			enteredOnce.Do(func() { close(subscribeEntered) })
			<-releaseSubscribe
			return nil
		},
	}})

	writeRequest := func(id, action string) {
		t.Helper()
		raw, err := json.Marshal(ws.Message{
			ID: id, Type: ws.MessageTypeRequest, Action: action,
			Payload: mustJSON(t, SubscribeRequest{TaskID: "task-ordered"}),
		})
		if err != nil {
			t.Fatalf("marshal %s: %v", action, err)
		}
		if err := cc.browser.WriteMessage(gorillaws.TextMessage, raw); err != nil {
			t.Fatalf("write %s: %v", action, err)
		}
	}

	writeRequest("subscribe", ws.ActionTaskSubscribe)
	select {
	case <-subscribeEntered:
	case <-time.After(wsTestTimeout):
		t.Fatal("task subscribe did not reach authorization")
	}
	writeRequest("unsubscribe", ws.ActionTaskUnsubscribe)

	responses := make(chan ws.Message, 2)
	readErr := make(chan error, 1)
	go func() {
		if err := cc.browser.SetReadDeadline(time.Now().Add(wsTestTimeout)); err != nil {
			readErr <- err
			return
		}
		for range 2 {
			_, data, err := cc.browser.ReadMessage()
			if err != nil {
				readErr <- err
				return
			}
			var response ws.Message
			if err := json.Unmarshal(data, &response); err != nil {
				readErr <- err
				return
			}
			responses <- response
		}
	}()

	select {
	case response := <-responses:
		close(releaseSubscribe)
		t.Fatalf("received %s before the earlier subscribe completed", response.ID)
	case err := <-readErr:
		close(releaseSubscribe)
		t.Fatalf("read response before release: %v", err)
	case <-time.After(100 * time.Millisecond):
		close(releaseSubscribe)
	}

	seen := map[string]bool{}
	for len(seen) < 2 {
		select {
		case response := <-responses:
			seen[response.ID] = true
		case err := <-readErr:
			t.Fatalf("read ordered responses: %v", err)
		case <-time.After(wsTestTimeout):
			t.Fatal("timed out waiting for ordered subscription responses")
		}
	}
	cc.assertTaskSubscribed(t, "task-ordered", false)
}

// A browser disconnect must unwind both pumps and drop the client from the hub,
// otherwise every closed tab leaks a goroutine pair and a subscriber entry.
func TestClientPumpsUnwindOnBrowserDisconnect(t *testing.T) {
	cc := newConnectedClient(t)

	if resp := cc.request(t, "1", ws.ActionTaskSubscribe, SubscribeRequest{TaskID: "task-1"}); resp.Type != ws.MessageTypeResponse {
		t.Fatalf("task.subscribe response = %+v", resp)
	}
	if got := cc.hub.GetClientCount(); got != 1 {
		t.Fatalf("hub client count = %d, want 1", got)
	}

	if err := cc.browser.Close(); err != nil {
		t.Fatalf("close browser: %v", err)
	}
	joinWithin(t, cc.stopped, "client pumps")

	if got := cc.hub.GetClientCount(); got != 0 {
		t.Fatalf("hub client count = %d after disconnect, want 0", got)
	}
	cc.assertTaskSubscribed(t, "task-1", false)
}

func (cc *connectedClient) assertTaskSubscribed(t *testing.T, taskID string, want bool) {
	t.Helper()
	cc.hub.mu.RLock()
	defer cc.hub.mu.RUnlock()
	_, got := cc.hub.taskSubscribers[taskID][cc.client]
	if got != want {
		t.Fatalf("hub task subscription for %q = %v, want %v", taskID, got, want)
	}
}

func (cc *connectedClient) assertMetricsSubscribed(t *testing.T, want bool) {
	t.Helper()
	cc.hub.mu.RLock()
	defer cc.hub.mu.RUnlock()
	if got := cc.hub.systemMetricsSubscribers[cc.client]; got != want {
		t.Fatalf("hub metrics subscription = %v, want %v", got, want)
	}
	if got := cc.client.systemMetricsSubscribed; got != want {
		t.Fatalf("client metrics flag = %v, want %v", got, want)
	}
}

package websocket

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/plugins"
	ws "github.com/kandev/kandev/pkg/websocket"
)

func TestSessionAckIdentityMatchesExactConsumerBranch(t *testing.T) {
	pluginKey := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
	}
	coreKey := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerCore, WireID: "wire-1",
	}
	tests := []struct {
		name string
		req  SessionAckRequest
		key  plugins.SessionDeliveryCursorKey
		want bool
	}{
		{
			name: "plugin identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1"},
			key: pluginKey, want: true,
		},
		{
			name: "plugin cannot also send wire identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", WireID: "wire-1"},
			key: pluginKey,
		},
		{
			name: "plugin id must match",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
				PluginID: "other", Generation: 7, ConsumerID: "consumer-1"},
			key: pluginKey,
		},
		{
			name: "core identity",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "wire-1"},
			key: coreKey, want: true,
		},
		{
			name: "core cannot send plugin fields",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "wire-1", PluginID: "plugin-1", Generation: 7},
			key: coreKey,
		},
		{
			name: "core wire must match",
			req: SessionAckRequest{SessionID: "session-1", ConsumerKind: orderedConsumerCore,
				WireID: "other"},
			key: coreKey,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			identity, valid := sessionAckSubscriptionID(test.req)
			if test.want {
				if !valid || identity == "" || !sessionAckMatchesKey(test.req, test.key) {
					t.Fatalf("valid ACK identity rejected: valid=%v identity=%q", valid, identity)
				}
				return
			}
			if valid && sessionAckMatchesKey(test.req, test.key) {
				t.Fatal("malformed or conflicting ACK identity accepted")
			}
		})
	}
}

func TestPoisonRequeueRequiresOperatorCapability(t *testing.T) {
	tests := []struct {
		name     string
		identity authn.Identity
		want     bool
	}{
		{name: "instance operator", identity: authn.Identity{UserID: "operator-1", Instance: true}, want: true},
		{name: "single user operator", identity: authn.Identity{UserID: "default-user", Role: authn.RoleAdmin, Synthetic: true}, want: true},
		{name: "organization admin has no operator capability", identity: authn.Identity{UserID: "admin-1", Role: authn.RoleAdmin}},
		{name: "anonymous instance bit is insufficient", identity: authn.Identity{Instance: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := canRequeueSessionEvents(test.identity); got != test.want {
				t.Fatalf("canRequeueSessionEvents() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOrderedReplayIsExplicitOnly(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	_, err := service.SessionEvents().Append(
		"session-1",
		nil,
		"message.deleted",
		json.RawMessage(`{"type":"message.deleted","session_id":"session-1","message_id":"message-1"}`),
	)
	if err != nil {
		t.Fatalf("append event: %v", err)
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", UserID: "user-1",
	}

	fresh := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
	}, key)
	if fresh.result != "fresh" || fresh.watermark != 1 || len(fresh.events) != 0 {
		t.Fatalf("fresh result = %+v, want no replay at watermark 1", fresh)
	}

	zero := uint64(0)
	replay := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &zero,
	}, key)
	if replay.result != "replay" || len(replay.events) != 1 {
		t.Fatalf("explicit replay result = %+v, want one replay event", replay)
	}

	invalid := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-1", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &zero, ResumeToken: "invalid",
	}, key)
	if invalid.result != "invalid_resume" || len(invalid.events) != 0 {
		t.Fatalf("invalid resume result = %+v, want replacement without replay", invalid)
	}
}

func TestOrderedReplayRebindsWhenPoisonDeliveryIsAlreadyClaimed(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	poison, err := service.SessionEvents().Append(
		"session-poison-replay",
		nil,
		"unknown.event",
		json.RawMessage(`{"type":"unknown.event","session_id":"session-poison-replay"}`),
	)
	if err != nil {
		t.Fatalf("append poison event: %v", err)
	}
	claim, err := service.SessionDelivery().Claim(
		poison.SessionID,
		poison.ID,
		time.Now().UTC(),
	)
	if err != nil || claim.Disposition != plugins.SessionDeliveryClaimed {
		t.Fatalf("initial poison claim = %q, %v", claim.Disposition, err)
	}
	zero := uint64(0)
	req := SessionSubscribeRequest{
		SessionID: "session-poison-replay", ConsumerKind: orderedConsumerCore,
		WireID: "wire-poison", LastSeenSequence: &zero,
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-poison-replay", ConsumerKind: orderedConsumerCore, WireID: "wire-poison",
	}
	replay := resolveOrderedSessionReplay(service, req, key)
	hub := newTestHub(t)
	hub.SetPluginConversationService(service)
	client := newTestClient("poison-replay-client")
	client.hub = hub
	client.controlSend = make(chan []byte, 1)
	client.orderedSessionSubscriptions = make(map[string]map[string]plugins.SessionDeliveryCursorKey)
	msg := &ws.Message{ID: "subscribe-poison", Type: ws.MessageTypeRequest, Action: "session.subscribe"}

	if !client.acceptOrderedSessionSubscription(msg, req, key, "", replay) {
		t.Fatal("poison replay replacement subscription was rejected")
	}

	var response struct {
		Payload struct {
			Result string `json:"result"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(<-client.controlSend, &response); err != nil {
		t.Fatalf("decode subscription response: %v", err)
	}
	if response.Payload.Result != "invalid_resume" {
		t.Fatalf("poison replay result = %q, want invalid_resume", response.Payload.Result)
	}
	if err := service.SessionEvents().Acknowledge(key, poison.Sequence); err != nil {
		t.Fatalf("replacement cursor did not advance past poison: %v", err)
	}
}

// TestOrderedReplayRebindsOnRetainedGap pins that a replay whose first
// retained row starts past lastSeen+1 (intermediate rows aged out on both
// sides) returns the replacement-cursor result instead of masquerading as a
// contiguous replay that would hide the lost history.
func TestOrderedReplayRebindsOnRetainedGap(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	// A fresh durable-log partition with only a high sequence present mirrors
	// the state after both sides pruned the intermediate rows.
	appended, err := service.SessionEvents().AppendCommitted(plugins.SessionEvent{
		SessionID: "session-gap", ID: "session-gap:10", Sequence: 10,
		EventType: "message.deleted",
		Payload:   json.RawMessage(`{"type":"message.deleted","session_id":"session-gap","message_id":"message-1"}`),
	})
	if err != nil {
		t.Fatalf("mirror event: %v", err)
	}
	if !appended {
		t.Fatal("expected the gap heal to accept the retained row")
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-gap", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", UserID: "user-1",
	}
	lastSeen := uint64(5)
	replay := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-gap", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &lastSeen,
	}, key)
	if replay.result != "invalid_resume" || len(replay.events) != 0 || replay.cursorSequence != replay.watermark {
		t.Fatalf("retained-gap result = %+v, want replacement cursor at watermark without replay", replay)
	}
}

// TestOrderedReplayRebindsWhenPartitionWatermarkAheadWithNoRows pins the
// fully-pruned case: the partition watermark is ahead of the cursor but every
// retained event row aged out, so the reconnect must rebind rather than treat
// the empty replay as a fresh success.
func TestOrderedReplayRebindsWhenPartitionWatermarkAheadWithNoRows(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	base := time.Date(2026, 9, 7, 12, 0, 0, 0, time.UTC)
	appended, err := service.SessionEvents().AppendCommitted(plugins.SessionEvent{
		SessionID: "session-pruned", ID: "session-pruned:10", Sequence: 10,
		EventType: "message.added", ProtocolVersion: 1,
		Payload:   json.RawMessage(`{"type":"message.added","session_id":"session-pruned","message_id":"m1","author_type":"user","content":"x","created_at":"2026-09-07T12:00:00Z"}`),
		CreatedAt: base,
	})
	if err != nil || !appended {
		t.Fatalf("mirror event appended=%v err=%v", appended, err)
	}
	// Age the only retained row out: the partition keeps its watermark.
	if err := service.SessionEvents().CollectExpired(base.Add(plugins.SessionEventRetention + time.Hour)); err != nil {
		t.Fatalf("collect expired: %v", err)
	}
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-pruned", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1", UserID: "user-1",
	}
	lastSeen := uint64(5)
	replay := resolveOrderedSessionReplay(service, SessionSubscribeRequest{
		SessionID: "session-pruned", ConsumerKind: orderedConsumerPlugin,
		PluginID: "plugin-1", Generation: 7, ConsumerID: "consumer-1",
		LastSeenSequence: &lastSeen,
	}, key)
	if replay.result != "invalid_resume" || len(replay.events) != 0 || replay.cursorSequence != replay.watermark {
		t.Fatalf("pruned-gap result = %+v, want replacement cursor at watermark without replay", replay)
	}
	if replay.watermark != 10 {
		t.Fatalf("watermark = %d, want 10", replay.watermark)
	}
}

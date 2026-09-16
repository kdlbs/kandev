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

func TestOrderedReplayGrantAcceptsEveryContiguousAck(t *testing.T) {
	tests := []struct {
		name         string
		consumerKind string
		consumerID   string
		wireID       string
		pluginID     string
		generation   int64
	}{
		{name: "core", consumerKind: orderedConsumerCore, wireID: "wire-replay"},
		{
			name:         "plugin",
			consumerKind: orderedConsumerPlugin,
			consumerID:   "consumer-replay",
			pluginID:     "plugin-replay",
			generation:   7,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
			for sequence := 1; sequence <= 12; sequence++ {
				_, err := service.SessionEvents().Append(
					"session-replay",
					nil,
					"message.deleted",
					json.RawMessage(`{"type":"message.deleted","session_id":"session-replay","message_id":"message"}`),
				)
				if err != nil {
					t.Fatalf("append event %d: %v", sequence, err)
				}
			}

			client := newTestClient("replay-client-" + test.name)
			client.hub = newTestHub(t)
			client.hub.SetPluginConversationService(service)
			client.controlSend = make(chan []byte, 16)
			client.orderedSessionSubscriptions = make(map[string]map[string]plugins.SessionDeliveryCursorKey)
			userID := client.ownUserTopic()
			key := plugins.SessionDeliveryCursorKey{
				SessionID: "session-replay", ConsumerKind: test.consumerKind,
				ConsumerID: test.consumerID, WireID: test.wireID, PluginID: test.pluginID,
				Generation: test.generation, UserID: userID,
			}
			lastSeen := uint64(10)
			req := SessionSubscribeRequest{
				SessionID: "session-replay", ConsumerKind: test.consumerKind,
				PluginID: test.pluginID, Generation: test.generation,
				ConsumerID: test.consumerID, WireID: test.wireID,
				LastSeenSequence: &lastSeen,
			}
			_, initialResume, _, err := service.MintSessionStreamGrant(
				test.pluginID,
				userID,
				test.generation,
				"session-replay",
				test.consumerID,
				test.wireID,
				10,
				10,
			)
			if err != nil {
				t.Fatalf("mint initial resume token: %v", err)
			}
			req.ResumeToken = initialResume
			if err := service.SessionEvents().RegisterCursor(key, 10); err != nil {
				t.Fatalf("register replay cursor: %v", err)
			}
			replay := resolveOrderedSessionReplay(service, req, key)
			if replay.result != "replay" || replay.watermark != 12 || replay.cursorSequence != 10 {
				t.Fatalf("replay = %+v, want cursor 10 and watermark 12", replay)
			}
			msg := &ws.Message{ID: "subscribe-replay", Type: ws.MessageTypeRequest, Action: "session.subscribe"}
			if !client.acceptOrderedSessionSubscription(msg, req, key, userID, replay) {
				t.Fatal("replay subscription was rejected")
			}
			var subscribeResponse struct {
				Payload struct {
					SnapshotCutoff uint64 `json:"snapshot_cutoff"`
					ResumeToken    string `json:"resume_token"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(<-client.controlSend, &subscribeResponse); err != nil {
				t.Fatalf("decode replay response: %v", err)
			}
			if subscribeResponse.Payload.SnapshotCutoff != 12 {
				t.Fatalf("snapshot cutoff = %d, want 12", subscribeResponse.Payload.SnapshotCutoff)
			}

			ack := func(sequence uint64, resumeToken string) string {
				raw, err := json.Marshal(SessionAckRequest{
					SessionID: "session-replay", ConsumerKind: test.consumerKind,
					PluginID: test.pluginID, Generation: test.generation,
					Sequence: sequence, ResumeToken: resumeToken,
					ConsumerID: test.consumerID, WireID: test.wireID,
				})
				if err != nil {
					t.Fatalf("marshal ACK %d: %v", sequence, err)
				}
				client.handleSessionAck(&ws.Message{
					ID: "ack-replay", Type: ws.MessageTypeRequest, Action: "session.ack", Payload: raw,
				})
				var response struct {
					Type    ws.MessageType `json:"type"`
					Payload struct {
						Success      bool   `json:"success"`
						ResumeToken  string `json:"resume_token"`
						Acknowledged uint64 `json:"acknowledged_sequence"`
						ErrorCode    string `json:"code"`
					} `json:"payload"`
				}
				if err := json.Unmarshal(<-client.controlSend, &response); err != nil {
					t.Fatalf("decode ACK %d response: %v", sequence, err)
				}
				if !response.Payload.Success {
					t.Fatalf("ACK %d failed with %s", sequence, response.Payload.ErrorCode)
				}
				if response.Payload.Acknowledged != sequence {
					t.Fatalf("ACK %d acknowledged %d", sequence, response.Payload.Acknowledged)
				}
				return response.Payload.ResumeToken
			}

			resumeToken := subscribeResponse.Payload.ResumeToken
			resumeToken = ack(11, resumeToken)
			resumeToken = ack(12, resumeToken)
			if _, err := service.SessionEvents().Append(
				"session-replay",
				nil,
				"message.deleted",
				json.RawMessage(`{"type":"message.deleted","session_id":"session-replay","message_id":"message-13"}`),
			); err != nil {
				t.Fatalf("append live event: %v", err)
			}
			_ = ack(13, resumeToken)
		})
	}
}

func TestOrderedReplayGrantRejectsInvalidAck(t *testing.T) {
	service := plugins.NewService(nil, plugins.NewRegistry(), nil, testLogger())
	for sequence := 1; sequence <= 2; sequence++ {
		if _, err := service.SessionEvents().Append(
			"session-invalid-ack",
			nil,
			"message.deleted",
			json.RawMessage(`{"type":"message.deleted","session_id":"session-invalid-ack","message_id":"message"}`),
		); err != nil {
			t.Fatalf("append event %d: %v", sequence, err)
		}
	}

	client := newTestClient("invalid-ack-client")
	client.hub = newTestHub(t)
	client.hub.SetPluginConversationService(service)
	client.controlSend = make(chan []byte, 8)
	client.orderedSessionSubscriptions = make(map[string]map[string]plugins.SessionDeliveryCursorKey)
	userID := client.ownUserTopic()
	key := plugins.SessionDeliveryCursorKey{
		SessionID: "session-invalid-ack", ConsumerKind: orderedConsumerCore,
		WireID: "wire-invalid-ack", UserID: userID,
	}
	if err := service.SessionEvents().RegisterCursor(key, 1); err != nil {
		t.Fatalf("register ACK cursor: %v", err)
	}
	_, resumeToken, _, err := service.MintSessionStreamGrant(
		"", userID, 0, key.SessionID, "", key.WireID, 1, 1,
	)
	if err != nil {
		t.Fatalf("mint resume token: %v", err)
	}
	client.orderedSessionSubscriptions[key.SessionID] = map[string]plugins.SessionDeliveryCursorKey{
		key.WireID: key,
	}

	ack := func(sequence uint64, token string) (success bool, code string) {
		raw, err := json.Marshal(SessionAckRequest{
			SessionID: key.SessionID, ConsumerKind: orderedConsumerCore,
			Sequence: sequence, ResumeToken: token, WireID: key.WireID,
		})
		if err != nil {
			t.Fatalf("marshal ACK %d: %v", sequence, err)
		}
		client.handleSessionAck(&ws.Message{
			ID: "ack-invalid", Type: ws.MessageTypeRequest, Action: "session.ack", Payload: raw,
		})
		var response struct {
			Payload struct {
				Success bool `json:"success"`
				Error   struct {
					Code string `json:"code"`
				} `json:"error"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(<-client.controlSend, &response); err != nil {
			t.Fatalf("decode ACK %d response: %v", sequence, err)
		}
		return response.Payload.Success, response.Payload.Error.Code
	}

	if success, code := ack(3, resumeToken); success || code != "forward_gap" {
		t.Fatalf("forward-gap ACK result = success %v, code %q; want forward_gap", success, code)
	}
	if success, code := ack(2, resumeToken+"tampered"); success || code != "invalid_binding" {
		t.Fatalf("tampered ACK result = success %v, code %q; want invalid_binding", success, code)
	}
	if success, code := ack(2, resumeToken); !success || code != "" {
		t.Fatalf("valid ACK after rejected requests = success %v, code %q; want success", success, code)
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

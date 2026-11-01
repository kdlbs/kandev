package statussummary

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
func TestUnreadVisibleTypesMatchSharedManifest(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "packages", "types", "src", "unread-message-visible-types.json"))
	if err != nil {
		t.Fatalf("read shared manifest: %v", err)
	}
	var manifest []string
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("decode shared manifest: %v", err)
	}
	backend := make([]string, 0, len(unreadVisibleMessageTypes))
	for messageType := range unreadVisibleMessageTypes {
		backend = append(backend, string(messageType))
	}
	sort.Strings(manifest)
	sort.Strings(backend)
	if len(manifest) != len(backend) {
		t.Fatalf("backend visible types %v, manifest %v", backend, manifest)
	}
	for i := range manifest {
		if manifest[i] != backend[i] {
			t.Fatalf("backend visible types %v, manifest %v", backend, manifest)
		}
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.10
// @covers AC-TASKS-TASK-UNREAD-DOT-001.18
func TestIsUnreadQualifyingMessage(t *testing.T) {
	for messageType := range unreadVisibleMessageTypes {
		if !IsUnreadQualifyingMessage(messageTypeAgent, string(messageType), "output", nil) {
			t.Errorf("visible type %q does not qualify", messageType)
		}
		if IsUnreadQualifyingMessage(messageTypeUser, string(messageType), "output", nil) {
			t.Errorf("a user-written %q message qualifies", messageType)
		}
	}
	cases := []struct {
		name        string
		messageType string
		content     string
		metadata    map[string]interface{}
		want        bool
	}{
		{name: "empty type is a message", messageType: "", want: true},
		{name: "permission request", messageType: messageTypePermissionRequest},
		{name: "clarification request", messageType: messageTypeClarificationRequest},
		{name: "hidden type", messageType: "log"},
		{name: "setup script", messageType: "script_execution", metadata: map[string]interface{}{"script_type": "setup"}},
		{name: "agent boot script", messageType: "script_execution", metadata: map[string]interface{}{"script_type": "agent_boot"}, want: true},
		{name: "subagent child", messageType: "tool_call", metadata: map[string]interface{}{"parent_tool_call_id": "call-1"}},
		{name: "new session notice", messageType: "status", content: "New session started"},
		{name: "resumed notice", messageType: "status", content: "Session resumed"},
		{name: "other status", messageType: "status", content: "Agent ready", want: true},
		{name: "new session text in a message", messageType: "message", content: "New session started", want: true},
		{name: "recovery card", messageType: "status", metadata: map[string]interface{}{"recovery_actions": true}},
		{name: "decoded footer actions", messageType: "error", metadata: map[string]interface{}{"actions": []interface{}{map[string]interface{}{"type": "ws_request"}}}},
		{name: "in-process footer actions", messageType: "error", metadata: map[string]interface{}{"actions": []map[string]interface{}{{"type": "ws_request"}}}},
		{name: "recovery card with actions", messageType: "status", metadata: map[string]interface{}{"recovery_actions": true, "actions": []interface{}{}}},
		{name: "non-array actions", messageType: "message", metadata: map[string]interface{}{"actions": "none"}, want: true},
	}
	for _, tc := range cases {
		if got := IsUnreadQualifyingMessage(messageTypeAgent, tc.messageType, tc.content, tc.metadata); got != tc.want {
			t.Errorf("%s: qualifies = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnreadRankOrdering(t *testing.T) {
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	event := NewUnreadRank("m-1", base.Add(123456789*time.Nanosecond))
	stored := NewUnreadRank("m-1", base.Add(123456*time.Microsecond).In(time.FixedZone("x", 3600)))
	if event != stored {
		t.Fatalf("event rank %+v != stored rank %+v", event, stored)
	}
	sameTimeLaterID := NewUnreadRank("m-2", base.Add(123456*time.Microsecond))
	if !sameTimeLaterID.After(event) || event.After(sameTimeLaterID) {
		t.Fatal("equal timestamps must order by id")
	}
	if NewUnreadRank("m-1", base).After(NewUnreadRank("m-1", base.Add(time.Second))) ||
		NewUnreadRank("m-1", base.Add(time.Second)).After(NewUnreadRank("m-1", base)) {
		t.Fatal("the same message id must never be after itself")
	}
	deletedCursor := SessionUnreadState{Read: UnreadRank{ID: "gone"}, Newest: NewUnreadRank("m-0", base)}
	if !deletedCursor.Unread() {
		t.Fatal("a cursor whose message was deleted must count every later message")
	}
}

// @covers AC-TASKS-TASK-UNREAD-DOT-001.2
func TestCursorlessSessionUnreadState(t *testing.T) {
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	if !(SessionUnreadState{Newest: NewUnreadRank("m-0", base)}).Unread() {
		t.Fatal("a never-read session with qualifying output must be unread")
	}
	if (SessionUnreadState{}).Unread() {
		t.Fatal("a never-read session without qualifying output must be read")
	}
}

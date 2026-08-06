package lsp

import (
	"encoding/json"
	"testing"
)

const documentURI = "file:///workspace/Main.kt"

func TestDocumentBrokerDeduplicatesOpenAndVersionsInterleavedChanges(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	first := hub.Attach(snapshot)
	second := hub.Attach(snapshot)
	drainAttached(t, first)
	drainAttached(t, second)
	t.Cleanup(first.Close)
	t.Cleanup(second.Close)

	handleNotification(t, first, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": documentURI, "languageId": "kotlin", "version": 40, "text": "first",
		},
	})
	handleNotification(t, first, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": documentURI, "version": 41},
		"contentChanges": []map[string]any{{"text": "newer"}},
	})
	handleNotification(t, second, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": documentURI, "languageId": "kotlin", "version": 1, "text": "stale",
		},
	})
	handleNotification(t, second, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": documentURI, "version": 2},
		"contentChanges": []map[string]any{{"text": "second"}},
	})
	handleNotification(t, first, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": documentURI, "version": 42},
		"contentChanges": []map[string]any{{"text": "latest"}},
	})

	notifications := upstream.notificationSnapshot()
	if len(notifications) != 4 {
		t.Fatalf("upstream notifications = %#v, want open + three changes", notifications)
	}
	if notifications[0].method != "textDocument/didOpen" ||
		!rawContains(notifications[0].params, `"version":1`) ||
		!rawContains(notifications[0].params, `"text":"first"`) {
		t.Fatalf("canonical open = %s", notifications[0].params)
	}
	for index, wantVersion := range []string{`"version":2`, `"version":3`, `"version":4`} {
		change := notifications[index+1]
		if change.method != "textDocument/didChange" || !rawContains(change.params, wantVersion) {
			t.Fatalf("change %d = %s", index, change.params)
		}
	}
	document, ok := hub.documents.Snapshot(documentURI)
	if !ok || document.Text != "latest" || document.Version != 4 || document.References != 2 {
		t.Fatalf("document snapshot = %#v, open=%v", document, ok)
	}
}

func TestDocumentBrokerFinalCloseAndDetachReleaseOnlyDocuments(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	t.Cleanup(hub.Close)
	first := hub.Attach(snapshot)
	second := hub.Attach(snapshot)
	drainAttached(t, first)
	drainAttached(t, second)

	for _, attachment := range []*Attachment{first, second} {
		handleNotification(t, attachment, "textDocument/didOpen", map[string]any{
			"textDocument": map[string]any{
				"uri": documentURI, "languageId": "kotlin", "version": 1, "text": "text",
			},
		})
	}
	handleNotification(t, first, "textDocument/didClose", map[string]any{
		"textDocument": map[string]any{"uri": documentURI},
	})
	if got := countNotifications(upstream.notificationSnapshot(), "textDocument/didClose"); got != 0 {
		t.Fatalf("first close sent %d upstream closes", got)
	}
	second.Close()
	if got := countNotifications(upstream.notificationSnapshot(), "textDocument/didClose"); got != 1 {
		t.Fatalf("final detach sent %d upstream closes", got)
	}
	first.Close()
	if hub.Closed() {
		t.Fatal("last attachment close owned the language-server hub")
	}
}

func TestDocumentBrokerOmitsStaleSaveTextAndHonorsCapability(t *testing.T) {
	upstream := &recordingFeatureUpstream{}
	hub, snapshot := newHubForTest(upstream)
	snapshot.Capabilities = json.RawMessage(
		`{"textDocumentSync":{"change":2,"save":{"includeText":true}}}`,
	)
	t.Cleanup(hub.Close)
	attachment := hub.Attach(snapshot)
	drainAttached(t, attachment)
	t.Cleanup(attachment.Close)

	handleNotification(t, attachment, "textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{
			"uri": documentURI, "languageId": "kotlin", "version": 1, "text": "old",
		},
	})
	handleNotification(t, attachment, "textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": documentURI, "version": 2},
		"contentChanges": []map[string]any{{"text": "new"}},
	})
	handleNotification(t, attachment, "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": documentURI}, "text": "old",
	})
	handleNotification(t, attachment, "textDocument/didSave", map[string]any{
		"textDocument": map[string]any{"uri": documentURI}, "text": "new",
	})

	notifications := upstream.notificationSnapshot()
	var saves []json.RawMessage
	for _, notification := range notifications {
		if notification.method == "textDocument/didSave" {
			saves = append(saves, notification.params)
		}
	}
	if len(saves) != 2 || rawContains(saves[0], `"text":`) || !rawContains(saves[1], `"text":"new"`) {
		t.Fatalf("save notifications = %q", saves)
	}
}

func handleNotification(t *testing.T, attachment *Attachment, method string, params any) {
	t.Helper()
	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatal(err)
	}
	message := []byte(`{"jsonrpc":"2.0","method":"` + method + `","params":` + string(paramsJSON) + `}`)
	if err := attachment.Handle(message); err != nil {
		t.Fatalf("handle %s: %v", method, err)
	}
}

func countNotifications(messages []recordedFeatureMessage, method string) int {
	count := 0
	for _, message := range messages {
		if message.method == method {
			count++
		}
	}
	return count
}

package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newFakeServer returns an httptest.Server whose handler dispatches by URL path
// (the trailing API method name) to the supplied handlers map.
func newFakeServer(t *testing.T, handlers map[string]http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := strings.TrimPrefix(r.URL.Path, "/")
		if h, ok := handlers[method]; ok {
			h(w, r)
			return
		}
		http.Error(w, "no handler for "+method, http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func newFakeClient(srv *httptest.Server) *CookieClient {
	c := NewCookieClient(nil, "xoxc-test", "d-test")
	c.endpoint = srv.URL
	return c
}

func TestCookieClient_AuthTest_Success(t *testing.T) {
	var gotAuth, gotCookie string
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"auth.test": func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			gotCookie = r.Header.Get("Cookie")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"url":"https://acme.slack.com/","team":"Acme","user":"alice","team_id":"T0001","user_id":"U0001"}`))
		},
	})
	client := newFakeClient(srv)
	res, err := client.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("AuthTest: %v", err)
	}
	if !res.OK || res.UserID != "U0001" || res.TeamID != "T0001" {
		t.Errorf("unexpected result: %+v", res)
	}
	if gotAuth != "Bearer xoxc-test" {
		t.Errorf("expected bearer auth header, got %q", gotAuth)
	}
	if gotCookie != "d=d-test" {
		t.Errorf("expected d cookie header, got %q", gotCookie)
	}
}

func TestCookieClient_AuthTest_SlackError(t *testing.T) {
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"auth.test": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
		},
	})
	client := newFakeClient(srv)
	res, err := client.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("AuthTest returned err (should surface as result): %v", err)
	}
	if res.OK {
		t.Errorf("expected OK=false, got %+v", res)
	}
	if !strings.Contains(res.Error, "invalid_auth") {
		t.Errorf("expected slack error in result, got %q", res.Error)
	}
}

func TestCookieClient_SearchMessages(t *testing.T) {
	var gotQuery string
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"search.messages": func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			gotQuery = r.PostForm.Get("query")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"ok": true,
				"messages": {"matches": [
					{"ts":"100.000001","text":"!kandev one","user":"U1","permalink":"https://example/1","channel":{"id":"C1"}},
					{"ts":"100.000002","text":"!kandev two","user":"U1","permalink":"https://example/2","channel":{"id":"C1"}}
				]}
			}`))
		},
	})
	client := newFakeClient(srv)
	msgs, err := client.SearchMessages(context.Background(), `from:U1 "!kandev"`)
	if err != nil {
		t.Fatalf("SearchMessages: %v", err)
	}
	if gotQuery != `from:U1 "!kandev"` {
		t.Errorf("query passthrough failed, got %q", gotQuery)
	}
	if len(msgs) != 2 || msgs[0].TS != "100.000001" || msgs[0].ChannelID != "C1" {
		t.Errorf("unexpected messages: %+v", msgs)
	}
}

func TestCookieClient_ConversationsReplies(t *testing.T) {
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"conversations.replies": func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			if r.PostForm.Get("ts") == "" {
				http.Error(w, "ts required", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"ok": true,
				"messages": [
					{"ts":"100.0001","user":"U1","text":"first"},
					{"ts":"100.0002","user":"U2","text":"second","thread_ts":"100.0001"}
				]
			}`))
		},
	})
	client := newFakeClient(srv)
	out, err := client.ConversationsReplies(context.Background(), "C1", "100.0001", "100.0001")
	if err != nil {
		t.Fatalf("ConversationsReplies: %v", err)
	}
	if len(out) != 2 || out[0].Text != "first" || out[1].ThreadTS != "100.0001" {
		t.Errorf("unexpected thread: %+v", out)
	}
}

func TestCookieClient_ConversationsReplies_PinsHistoryToTriggerTSWhenNoThread(t *testing.T) {
	var got struct{ oldest, latest, inclusive, limit string }
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"conversations.history": func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			got.oldest = r.PostForm.Get("oldest")
			got.latest = r.PostForm.Get("latest")
			got.inclusive = r.PostForm.Get("inclusive")
			got.limit = r.PostForm.Get("limit")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"messages":[{"ts":"200.0001","user":"U1","text":"loner"}]}`))
		},
	})
	client := newFakeClient(srv)
	out, err := client.ConversationsReplies(context.Background(), "C1", "", "200.0001")
	if err != nil {
		t.Fatalf("ConversationsReplies: %v", err)
	}
	// Both window bounds must be the trigger ts and inclusive=true; otherwise
	// concurrent activity in the channel would land in the response instead
	// of the actual trigger message.
	if got.oldest != "200.0001" || got.latest != "200.0001" {
		t.Errorf("expected oldest/latest pinned to trigger ts, got %+v", got)
	}
	if got.inclusive != "true" || got.limit != "1" {
		t.Errorf("expected inclusive=true limit=1, got %+v", got)
	}
	if len(out) != 1 || out[0].Text != "loner" {
		t.Errorf("unexpected fallback: %+v", out)
	}
}

func TestCookieClient_ChatPostMessage(t *testing.T) {
	var got struct{ channel, text, threadTS string }
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"chat.postMessage": func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			got.channel = r.PostForm.Get("channel")
			got.text = r.PostForm.Get("text")
			got.threadTS = r.PostForm.Get("thread_ts")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		},
	})
	client := newFakeClient(srv)
	if err := client.ChatPostMessage(context.Background(), "C1", "100.0001", "hello"); err != nil {
		t.Fatalf("ChatPostMessage: %v", err)
	}
	if got.channel != "C1" || got.text != "hello" || got.threadTS != "100.0001" {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestCookieClient_ReactionsAdd(t *testing.T) {
	var got struct{ channel, ts, name string }
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"reactions.add": func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			got.channel = r.PostForm.Get("channel")
			got.ts = r.PostForm.Get("timestamp")
			got.name = r.PostForm.Get("name")
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		},
	})
	client := newFakeClient(srv)
	if err := client.ReactionsAdd(context.Background(), "C1", "100.0001", "eyes"); err != nil {
		t.Fatalf("ReactionsAdd: %v", err)
	}
	if got.channel != "C1" || got.ts != "100.0001" || got.name != "eyes" {
		t.Errorf("payload mismatch: %+v", got)
	}
}

func TestCookieClient_ReactionsAdd_AlreadyReactedSwallowed(t *testing.T) {
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"reactions.add": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":false,"error":"already_reacted"}`))
		},
	})
	client := newFakeClient(srv)
	if err := client.ReactionsAdd(context.Background(), "C1", "100.0001", "eyes"); err != nil {
		t.Errorf("expected already_reacted to be swallowed, got: %v", err)
	}
}

func TestCookieClient_ReactionsAdd_OtherErrorBubbles(t *testing.T) {
	srv := newFakeServer(t, map[string]http.HandlerFunc{
		"reactions.add": func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":false,"error":"channel_not_found"}`))
		},
	})
	client := newFakeClient(srv)
	err := client.ReactionsAdd(context.Background(), "C1", "100.0001", "eyes")
	if err == nil || !strings.Contains(err.Error(), "channel_not_found") {
		t.Errorf("expected channel_not_found error, got %v", err)
	}
}

func TestCookieClient_RejectsEmptyCredentials(t *testing.T) {
	// AuthTest absorbs ErrNotConfigured into the returned result rather than
	// surfacing it as an error — assert on result.OK explicitly so the test
	// catches the guard regression instead of vacuously passing on err==nil.
	c := NewCookieClient(nil, "", "")
	res, err := c.AuthTest(context.Background())
	if err != nil {
		t.Fatalf("AuthTest: %v", err)
	}
	if res == nil || res.OK {
		t.Errorf("expected ok=false when both credentials are empty, got %+v", res)
	}

	c2 := NewCookieClient(nil, "xoxc-x", "")
	_, err = c2.SearchMessages(context.Background(), "x")
	if err == nil {
		t.Error("expected error when cookie is missing")
	}
}

package webpush

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/store"
)

type sentMessage struct {
	endpoint string
	body     []byte
	subject  string
	opts     SendOptions
	keys     models.VAPIDKeys
}

// fakeSender records pushes and answers with a configurable status per
// endpoint. It never touches the network.
type fakeSender struct {
	mu       sync.Mutex
	sent     []sentMessage
	statuses map[string]int
	errs     map[string]error
}

func (f *fakeSender) Send(_ context.Context, sub *models.PushSubscription, body []byte, keys models.VAPIDKeys, subject string, opts SendOptions) (int, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sent = append(f.sent, sentMessage{sub.Endpoint, body, subject, opts, keys})
	if err := f.errs[sub.Endpoint]; err != nil {
		return 0, err
	}
	if status, ok := f.statuses[sub.Endpoint]; ok {
		return status, nil
	}
	return http.StatusCreated, nil
}

func newTestService(t *testing.T, sender Sender) (*Service, store.WebPushRepository) {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "push.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	database := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = database.Close() })
	repo, _, err := store.Provide(context.Background(), database, database)
	if err != nil {
		t.Fatalf("provide store: %v", err)
	}
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	return NewService(repo, sender, "", log), repo
}

func validInput(endpoint string) SubscribeInput {
	return SubscribeInput{Endpoint: endpoint, P256dh: "key", Auth: "auth", UserAgent: "test-agent"}
}

func TestPublicKeyIsGeneratedOnceAndPersisted(t *testing.T) {
	svc, repo := newTestService(t, &fakeSender{})
	ctx := context.Background()

	first, err := svc.PublicKey(ctx)
	if err != nil || first == "" {
		t.Fatalf("PublicKey() = %q, %v", first, err)
	}
	second, err := svc.PublicKey(ctx)
	if err != nil || second != first {
		t.Fatalf("PublicKey() changed across calls: %q then %q (%v)", first, second, err)
	}
	stored, err := repo.LoadOrCreateVAPIDKeys(ctx, func() (models.VAPIDKeys, error) {
		return models.VAPIDKeys{}, errors.New("must not regenerate")
	})
	if err != nil || stored.PublicKey != first || stored.PrivateKey == "" {
		t.Fatalf("stored keys = %+v, %v", stored, err)
	}
}

func TestSubscribeValidatesInput(t *testing.T) {
	svc, _ := newTestService(t, &fakeSender{})
	cases := map[string]SubscribeInput{
		"http endpoint": {Endpoint: "http://push.example/abc", P256dh: "k", Auth: "a"},
		"empty":         {},
		"no keys":       {Endpoint: "https://push.example/abc"},
		"relative":      {Endpoint: "/abc", P256dh: "k", Auth: "a"},
	}
	for name, in := range cases {
		if _, err := svc.Subscribe(context.Background(), "u1", in); !errors.Is(err, ErrInvalidSubscription) {
			t.Errorf("%s: err = %v, want ErrInvalidSubscription", name, err)
		}
	}
}

func TestSubscribeIsIdempotentPerEndpointAndScopedToUser(t *testing.T) {
	svc, _ := newTestService(t, &fakeSender{})
	ctx := context.Background()
	const endpoint = "https://push.example/device-1"

	for i := 0; i < 2; i++ {
		if _, err := svc.Subscribe(ctx, "u1", validInput(endpoint)); err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	}
	subs, _ := svc.Subscriptions(ctx, "u1")
	if len(subs) != 1 {
		t.Fatalf("u1 subscriptions = %d, want 1", len(subs))
	}
	if other, _ := svc.Subscriptions(ctx, "u2"); len(other) != 0 {
		t.Fatalf("u2 sees %d subscriptions, want 0", len(other))
	}
}

func TestUnsubscribeOnlyRemovesCallersOwnSubscription(t *testing.T) {
	svc, _ := newTestService(t, &fakeSender{})
	ctx := context.Background()
	const endpoint = "https://push.example/device-1"
	if _, err := svc.Subscribe(ctx, "u1", validInput(endpoint)); err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	if err := svc.Unsubscribe(ctx, "u2", endpoint); !errors.Is(err, ErrNotFound) {
		t.Fatalf("other user Unsubscribe err = %v, want ErrNotFound", err)
	}
	if err := svc.Unsubscribe(ctx, "u1", endpoint); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	if err := svc.Unsubscribe(ctx, "u1", endpoint); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Unsubscribe err = %v, want ErrNotFound", err)
	}
}

func TestNotifySendsEncodedPayloadWithTTLUrgencyAndSubject(t *testing.T) {
	sender := &fakeSender{}
	svc, _ := newTestService(t, sender)
	ctx := context.Background()
	if _, err := svc.Subscribe(ctx, "u1", validInput("https://push.example/a")); err != nil {
		t.Fatal(err)
	}

	n, err := svc.Notify(ctx, "u1", Payload{
		Title: "Agent needs permission", Body: "Waiting", Tag: "t", URL: "/t/1", TaskID: "1", SessionID: "s",
	}, SendOptions{Urgency: UrgencyHigh})
	if err != nil || n != 1 {
		t.Fatalf("Notify() = %d, %v", n, err)
	}
	got := sender.sent[0]
	if got.opts.TTL != DefaultTTL || got.opts.Urgency != UrgencyHigh {
		t.Errorf("opts = %+v, want default TTL and high urgency", got.opts)
	}
	if got.subject != DefaultSubject {
		t.Errorf("subject = %q, want %q", got.subject, DefaultSubject)
	}
	if got.keys.PrivateKey == "" || got.keys.PublicKey == "" {
		t.Error("sender did not receive the persisted VAPID keys")
	}
	var payload map[string]string
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	want := map[string]string{"title": "Agent needs permission", "body": "Waiting", "tag": "t", "url": "/t/1", "taskId": "1", "sessionId": "s"}
	for k, v := range want {
		if payload[k] != v {
			t.Errorf("payload[%q] = %q, want %q", k, payload[k], v)
		}
	}
}

func TestNotifyOnlyReachesThatUsersDevices(t *testing.T) {
	sender := &fakeSender{}
	svc, _ := newTestService(t, sender)
	ctx := context.Background()
	_, _ = svc.Subscribe(ctx, "u1", validInput("https://push.example/u1"))
	_, _ = svc.Subscribe(ctx, "u2", validInput("https://push.example/u2"))

	if _, err := svc.Notify(ctx, "u1", Payload{Title: "x"}, SendOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != 1 || sender.sent[0].endpoint != "https://push.example/u1" {
		t.Fatalf("sent = %+v, want only u1's endpoint", sender.sent)
	}
}

func TestNotifyPrunesGoneSubscriptions(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusGone} {
		sender := &fakeSender{statuses: map[string]int{"https://push.example/dead": status}}
		svc, _ := newTestService(t, sender)
		ctx := context.Background()
		_, _ = svc.Subscribe(ctx, "u1", validInput("https://push.example/dead"))
		_, _ = svc.Subscribe(ctx, "u1", validInput("https://push.example/live"))

		n, err := svc.Notify(ctx, "u1", Payload{Title: "x"}, SendOptions{})
		if err != nil || n != 2 {
			t.Fatalf("status %d: Notify() = %d, %v", status, n, err)
		}
		subs, _ := svc.Subscriptions(ctx, "u1")
		if len(subs) != 1 || subs[0].Endpoint != "https://push.example/live" {
			t.Fatalf("status %d: remaining = %+v, want only the live endpoint", status, subs)
		}
	}
}

func TestNotifyKeepsSubscriptionOnTransientFailure(t *testing.T) {
	sender := &fakeSender{statuses: map[string]int{"https://push.example/a": http.StatusServiceUnavailable}}
	svc, _ := newTestService(t, sender)
	ctx := context.Background()
	_, _ = svc.Subscribe(ctx, "u1", validInput("https://push.example/a"))

	n, err := svc.Notify(ctx, "u1", Payload{Title: "x"}, SendOptions{})
	if err == nil || n != 0 {
		t.Fatalf("Notify() = %d, %v; want 0 delivered and an error so the caller can retry", n, err)
	}
	if subs, _ := svc.Subscriptions(ctx, "u1"); len(subs) != 1 {
		t.Fatalf("transient failure must not prune; subscriptions = %d", len(subs))
	}
}

func TestNotifyWithoutSubscriptionsIsANoOp(t *testing.T) {
	sender := &fakeSender{}
	svc, _ := newTestService(t, sender)
	n, err := svc.Notify(context.Background(), "u1", Payload{Title: "x"}, SendOptions{})
	if err != nil || n != 0 || len(sender.sent) != 0 {
		t.Fatalf("Notify() = %d, %v, sent %d; want a silent no-op", n, err, len(sender.sent))
	}
}

func TestEncodePayloadTruncatesLongText(t *testing.T) {
	body, err := encodePayload(Payload{Title: strings.Repeat("t", 500), Body: strings.Repeat("b", 5000)})
	if err != nil {
		t.Fatal(err)
	}
	var p Payload
	if err := json.Unmarshal(body, &p); err != nil {
		t.Fatal(err)
	}
	if len([]rune(p.Title)) > maxTitleRunes || len([]rune(p.Body)) > maxBodyRunes {
		t.Fatalf("payload not truncated: title %d, body %d runes", len([]rune(p.Title)), len([]rune(p.Body)))
	}
}

func TestNormalizeSubject(t *testing.T) {
	cases := map[string]string{
		"":                       DefaultSubject,
		"  ":                     DefaultSubject,
		"mailto:ops@example.com": "mailto:ops@example.com",
		"ops@example.com":        "mailto:ops@example.com",
		"https://example.com":    "https://example.com",
		"not a subject":          DefaultSubject,
	}
	for in, want := range cases {
		if got := NormalizeSubject(in); got != want {
			t.Errorf("NormalizeSubject(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTaskURL(t *testing.T) {
	if got := TaskURL("", ""); got != "/" {
		t.Errorf("no task = %q", got)
	}
	if got := TaskURL("t 1", "s&1"); got != "/t/t%201?sessionId=s%261" {
		t.Errorf("TaskURL escaped = %q", got)
	}
	if got := TaskURL("t1", ""); got != "/t/t1" {
		t.Errorf("TaskURL no session = %q", got)
	}
}

func TestSubscriptionTimestampsAreRecorded(t *testing.T) {
	svc, _ := newTestService(t, &fakeSender{})
	ctx := context.Background()
	_, _ = svc.Subscribe(ctx, "u1", validInput("https://push.example/a"))
	before := time.Now().Add(-time.Minute)
	if _, err := svc.Notify(ctx, "u1", Payload{Title: "x"}, SendOptions{}); err != nil {
		t.Fatal(err)
	}
	subs, _ := svc.Subscriptions(ctx, "u1")
	if subs[0].LastUsedAt == nil || subs[0].LastUsedAt.Before(before) {
		t.Fatalf("LastUsedAt = %v, want a recent time", subs[0].LastUsedAt)
	}
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	// The users table normally comes from internal/user/store; create the
	// slice CountAdminIdentities joins against.
	if _, err := conn.Exec(`CREATE TABLE users (
		id TEXT PRIMARY KEY, email TEXT NOT NULL, role TEXT NOT NULL DEFAULT 'admin',
		status TEXT NOT NULL DEFAULT 'active')`); err != nil {
		t.Fatalf("create users: %v", err)
	}
	s, err := New(conn, conn)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	return s
}

func TestSchemaReplaySafe(t *testing.T) {
	s := newTestStore(t)
	// Re-running initSchema on an existing database must be a no-op.
	if err := s.initSchema(); err != nil {
		t.Fatalf("schema replay: %v", err)
	}
}

func TestIdentityRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	identity := &LoginIdentity{UserID: "u1", Provider: ProviderLocal, Subject: "a@b.c", PasswordHash: "h1"}
	if err := s.CreateIdentity(ctx, identity); err != nil {
		t.Fatalf("create identity: %v", err)
	}
	got, err := s.GetLocalIdentity(ctx, "u1")
	if err != nil {
		t.Fatalf("get identity: %v", err)
	}
	if got.PasswordHash != "h1" || got.Subject != "a@b.c" {
		t.Fatalf("unexpected identity: %+v", got)
	}
	// One local identity per user.
	dup := &LoginIdentity{UserID: "u1", Provider: ProviderLocal, Subject: "other@b.c"}
	if err := s.CreateIdentity(ctx, dup); err == nil {
		t.Fatal("duplicate (user_id, provider) must fail")
	}
	if err := s.UpdatePasswordHash(ctx, "u1", "h2"); err != nil {
		t.Fatalf("update hash: %v", err)
	}
	got, _ = s.GetLocalIdentity(ctx, "u1")
	if got.PasswordHash != "h2" {
		t.Fatal("password hash not updated")
	}
	if err := s.UpdatePasswordHash(ctx, "missing", "h"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestCountAdminIdentities(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	mustExec(t, s.db, `INSERT INTO users (id, email, role, status) VALUES
		('admin1', 'a@x', 'admin', 'active'),
		('member1', 'm@x', 'member', 'active'),
		('admin2', 'd@x', 'admin', 'disabled')`)

	count, err := s.CountAdminIdentities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("no identities yet, got %d", count)
	}
	for _, userID := range []string{"admin1", "member1", "admin2"} {
		if err := s.CreateIdentity(ctx, &LoginIdentity{UserID: userID, Provider: ProviderLocal, Subject: userID}); err != nil {
			t.Fatal(err)
		}
	}
	count, err = s.CountAdminIdentities(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Only active admins count: member1 (role) and admin2 (disabled) excluded.
	if count != 1 {
		t.Fatalf("count = %d, want 1", count)
	}
}

func TestSessionLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	session := &Session{
		UserID: "u1", TokenHash: "hash1",
		CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now,
		UserAgent: "ua", IP: "127.0.0.1",
	}
	if err := s.CreateSession(ctx, session); err != nil {
		t.Fatalf("create session: %v", err)
	}
	got, err := s.GetSessionByTokenHash(ctx, "hash1")
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if got.UserID != "u1" {
		t.Fatalf("unexpected session: %+v", got)
	}
	if _, err := s.GetSessionByTokenHash(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := s.TouchSession(ctx, session.ID, now.Add(time.Minute), now.Add(2*time.Hour), ""); err != nil {
		t.Fatalf("touch: %v", err)
	}
	sessions, err := s.ListSessionsByUser(ctx, "u1")
	if err != nil || len(sessions) != 1 {
		t.Fatalf("list sessions: %v (%d)", err, len(sessions))
	}

	// keep-one deletion
	second := &Session{UserID: "u1", TokenHash: "hash2", CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now}
	if err := s.CreateSession(ctx, second); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSessionsByUser(ctx, "u1", second.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSessionByTokenHash(ctx, "hash1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("hash1 should be revoked")
	}
	if _, err := s.GetSessionByTokenHash(ctx, "hash2"); err != nil {
		t.Fatal("kept session must survive")
	}

	// expiry GC
	expired := &Session{UserID: "u2", TokenHash: "old", CreatedAt: now.Add(-2 * time.Hour), ExpiresAt: now.Add(-time.Hour), LastSeenAt: now}
	if err := s.CreateSession(ctx, expired); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteExpiredSessions(ctx, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetSessionByTokenHash(ctx, "old"); !errors.Is(err, ErrNotFound) {
		t.Fatal("expired session should be gone")
	}
}

// TestTouchSessionIPRefresh pins the conditional-IP half of TouchSession:
// timestamps always update, and the ip column refreshes only when the passed
// ip is non-empty (empty never clobbers; an empty stored IP is backfilled by
// a non-empty touch). Two targets (non-empty and empty seeded IPs) plus a
// same-user_id decoy make the transitions unambiguous: a missing WHERE clause
// or a user_id-scoped WHERE that touches the decoy would trip the untouched
// assertions.
func TestTouchSessionIPRefresh(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Second)

	// First-transition touch args; later transitions advance from these so
	// each transition's Equal() pin is fresh and distinct per column (a
	// SET clause swapping the two columns writes identical values only when
	// the args are identical).
	lastSeen := now
	expires := lastSeen.Add(time.Hour)

	targetA := &Session{
		UserID: "u1", TokenHash: "hashA", IP: "1.1.1.1",
		CreatedAt: now, ExpiresAt: expires.Add(24 * time.Hour), LastSeenAt: lastSeen.Add(-time.Hour),
	}
	targetB := &Session{
		UserID: "u1", TokenHash: "hashB", IP: "",
		CreatedAt: now, ExpiresAt: expires.Add(24 * time.Hour), LastSeenAt: lastSeen.Add(-time.Hour),
	}
	decoy := &Session{
		UserID: "u1", TokenHash: "hashDecoy", IP: "10.0.0.9",
		CreatedAt: now, ExpiresAt: expires.Add(48 * time.Hour), LastSeenAt: lastSeen.Add(-2 * time.Hour),
	}
	for _, session := range []*Session{targetA, targetB, decoy} {
		if err := s.CreateSession(ctx, session); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	// Transition 1: non-empty ip refreshes the stored value on target A.
	lastSeen2 := lastSeen.Add(time.Second)
	expires2 := expires.Add(time.Second)
	if err := s.TouchSession(ctx, targetA.ID, lastSeen2, expires2, "2.2.2.2"); err != nil {
		t.Fatalf("touch: %v", err)
	}
	gotA := mustGetSession(t, s, "hashA")
	if gotA.IP != "2.2.2.2" {
		t.Fatalf("target A IP = %q, want 2.2.2.2", gotA.IP)
	}
	if !gotA.LastSeenAt.Equal(lastSeen2) || !gotA.ExpiresAt.Equal(expires2) {
		t.Fatalf("target A timestamps = %v / %v, want %v / %v", gotA.LastSeenAt, gotA.ExpiresAt, lastSeen2, expires2)
	}
	assertSessionUntouched(t, s, decoy)

	// Transition 2: empty ip preserves the recorded value but still touches.
	lastSeen3 := lastSeen2.Add(time.Second)
	expires3 := expires2.Add(time.Second)
	if err := s.TouchSession(ctx, targetA.ID, lastSeen3, expires3, ""); err != nil {
		t.Fatalf("touch: %v", err)
	}
	gotA = mustGetSession(t, s, "hashA")
	if gotA.IP != "2.2.2.2" {
		t.Fatalf("target A IP = %q after empty touch, want 2.2.2.2", gotA.IP)
	}
	if !gotA.LastSeenAt.Equal(lastSeen3) || !gotA.ExpiresAt.Equal(expires3) {
		t.Fatalf("target A timestamps = %v / %v, want %v / %v", gotA.LastSeenAt, gotA.ExpiresAt, lastSeen3, expires3)
	}
	assertSessionUntouched(t, s, decoy)

	// Transition 3: a non-empty touch backfills an empty stored IP (target B).
	lastSeen4 := lastSeen3.Add(time.Second)
	expires4 := expires3.Add(time.Second)
	if err := s.TouchSession(ctx, targetB.ID, lastSeen4, expires4, "2.2.2.2"); err != nil {
		t.Fatalf("touch: %v", err)
	}
	gotB := mustGetSession(t, s, "hashB")
	if gotB.IP != "2.2.2.2" {
		t.Fatalf("target B IP = %q after backfill touch, want 2.2.2.2", gotB.IP)
	}
	if !gotB.LastSeenAt.Equal(lastSeen4) || !gotB.ExpiresAt.Equal(expires4) {
		t.Fatalf("target B timestamps = %v / %v, want %v / %v", gotB.LastSeenAt, gotB.ExpiresAt, lastSeen4, expires4)
	}
	assertSessionUntouched(t, s, decoy)
}

func mustGetSession(t *testing.T, s *Store, tokenHash string) *Session {
	t.Helper()
	got, err := s.GetSessionByTokenHash(context.Background(), tokenHash)
	if err != nil {
		t.Fatalf("get session %s: %v", tokenHash, err)
	}
	return got
}

// assertSessionUntouched verifies a row outside the touch target was not
// modified: same-IP, last_seen_at, and expires_at all still equal the seeds.
func assertSessionUntouched(t *testing.T, s *Store, seed *Session) {
	t.Helper()
	got := mustGetSession(t, s, seed.TokenHash)
	if got.IP != seed.IP {
		t.Fatalf("decoy IP = %q, want %q", got.IP, seed.IP)
	}
	if !got.LastSeenAt.Equal(seed.LastSeenAt) {
		t.Fatalf("decoy last_seen_at = %v, want %v", got.LastSeenAt, seed.LastSeenAt)
	}
	if !got.ExpiresAt.Equal(seed.ExpiresAt) {
		t.Fatalf("decoy expires_at = %v, want %v", got.ExpiresAt, seed.ExpiresAt)
	}
}

func TestAPITokenLifecycle(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	token := &APIToken{UserID: "u1", Name: "ci", Prefix: "ab12cd34", TokenHash: "th1"}
	if err := s.CreateAPIToken(ctx, token); err != nil {
		t.Fatalf("create token: %v", err)
	}
	got, err := s.GetAPITokenByHash(ctx, "th1")
	if err != nil || got.Name != "ci" {
		t.Fatalf("get token: %v %+v", err, got)
	}
	if err := s.TouchAPIToken(ctx, token.ID, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	tokens, err := s.ListAPITokensByUser(ctx, "u1")
	if err != nil || len(tokens) != 1 {
		t.Fatalf("list tokens: %v (%d)", err, len(tokens))
	}
	// Delete scoped by owner: wrong owner is a no-op.
	if err := s.DeleteAPIToken(ctx, token.ID, "intruder"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPITokenByHash(ctx, "th1"); err != nil {
		t.Fatal("token must survive wrong-owner delete")
	}
	if err := s.DeleteAPITokensByUser(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetAPITokenByHash(ctx, "th1"); !errors.Is(err, ErrNotFound) {
		t.Fatal("token should be revoked")
	}
}

func TestInviteSingleUse(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now().UTC()

	invite := &Invite{TokenHash: "ih1", Email: "new@x", Role: "member", CreatedBy: "admin1", ExpiresAt: now.Add(time.Hour)}
	if err := s.CreateInvite(ctx, invite); err != nil {
		t.Fatalf("create invite: %v", err)
	}
	got, err := s.GetInviteByTokenHash(ctx, "ih1")
	if err != nil || got.Email != "new@x" {
		t.Fatalf("get invite: %v %+v", err, got)
	}
	if err := s.MarkInviteUsed(ctx, invite.ID, "u9", now); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	// Second consume must fail (concurrent-accept guard).
	if err := s.MarkInviteUsed(ctx, invite.ID, "u10", now); !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound on double-use, got %v", err)
	}
	invites, err := s.ListInvites(ctx)
	if err != nil || len(invites) != 1 {
		t.Fatalf("list invites: %v (%d)", err, len(invites))
	}
	if err := s.DeleteInvite(ctx, invite.ID); err != nil {
		t.Fatal(err)
	}
}

func mustExec(t *testing.T, db *sqlx.DB, stmt string) {
	t.Helper()
	if _, err := db.Exec(stmt); err != nil {
		t.Fatalf("exec: %v", err)
	}
}

// TestJSONShapeOmitsDigests pins the API wire shape: credential digests must
// never serialize, and timestamp/name fields use snake_case keys the frontend
// reads (regression for "Invalid Date" / empty name in the tokens UI).
func TestJSONShapeOmitsDigests(t *testing.T) {
	now := time.Unix(1700000000, 0).UTC()
	token := &APIToken{ID: "t1", UserID: "u1", Name: "ci", Prefix: "ab12cd34", TokenHash: "SECRET_HASH", CreatedAt: now}
	raw, err := json.Marshal(token)
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	if strings.Contains(got, "SECRET_HASH") || strings.Contains(got, "token_sha256") {
		t.Fatalf("token hash leaked in JSON: %s", got)
	}
	for _, key := range []string{`"name":"ci"`, `"created_at":`, `"prefix":"ab12cd34"`} {
		if !strings.Contains(got, key) {
			t.Fatalf("missing %s in %s", key, got)
		}
	}

	invite := &Invite{ID: "i1", TokenHash: "INV_HASH", Email: "a@b.c", Role: "member"}
	rawInvite, _ := json.Marshal(invite)
	if strings.Contains(string(rawInvite), "INV_HASH") || strings.Contains(string(rawInvite), "token_sha256") {
		t.Fatalf("invite hash leaked in JSON: %s", rawInvite)
	}

	identity := &LoginIdentity{ID: "id1", UserID: "u1", PasswordHash: "PW_HASH"}
	rawID, _ := json.Marshal(identity)
	if strings.Contains(string(rawID), "PW_HASH") || strings.Contains(string(rawID), "password_hash") {
		t.Fatalf("password hash leaked in JSON: %s", rawID)
	}
}

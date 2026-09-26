package auth

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type serviceFixture struct {
	svc       *Service
	users     userstore.Repository
	backfills *[]string
}

func newServiceFixture(t *testing.T, authEnabled bool) *serviceFixture {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	users, cleanup, err := userstore.Provide(conn, conn)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	authStore, err := store.New(conn, conn)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	claimed := []string{}
	cfg := &config.Config{}
	cfg.Features.Auth = authEnabled
	cfg.Auth.SessionTTLHours = 720
	svc, err := NewService(context.Background(), Deps{
		Cfg:   cfg,
		Store: authStore,
		Users: users,
		Backfills: []BackfillFunc{func(_ context.Context, ownerID string) error {
			claimed = append(claimed, ownerID)
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return &serviceFixture{svc: svc, users: users, backfills: &claimed}
}

// enableAuth flips the features.auth flag on and recomputes the mode, the way
// a restart with the runtime flag enabled would.
func enableAuth(t *testing.T, f *serviceFixture) {
	t.Helper()
	f.svc.cfg.Features.Auth = true
	if err := f.svc.refreshMode(context.Background()); err != nil {
		t.Fatalf("refresh mode: %v", err)
	}
}

func TestModeDisabledByDefault(t *testing.T) {
	f := newServiceFixture(t, false)
	if got := f.svc.Mode(); got != ModeDisabled {
		t.Fatalf("mode = %s, want disabled", got)
	}
}

func TestModeSetupWhenEnabledWithoutAdmin(t *testing.T) {
	f := newServiceFixture(t, true)
	if got := f.svc.Mode(); got != ModeSetup {
		t.Fatalf("mode = %s, want setup", got)
	}
}

func TestEnableThenSetupPromotesDefaultUser(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()

	enableAuth(t, f)
	if f.svc.Mode() != ModeSetup {
		t.Fatalf("mode = %s, want setup", f.svc.Mode())
	}

	user, token, err := f.svc.Setup(ctx, "Admin@Example.com", "hunter2secure", "The Admin", "ua", "127.0.0.1")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if user.ID != userstore.DefaultUserID {
		t.Fatalf("setup must promote the default-user row, got %s", user.ID)
	}
	if user.Email != "admin@example.com" || user.Role != usermodels.RoleAdmin {
		t.Fatalf("unexpected admin: %+v", user)
	}
	if len(*f.backfills) != 1 || (*f.backfills)[0] != userstore.DefaultUserID {
		t.Fatalf("backfills not invoked for admin: %v", *f.backfills)
	}
	if f.svc.Mode() != ModeEnabled {
		t.Fatalf("mode after setup = %s, want enabled", f.svc.Mode())
	}
	// The minted session authenticates.
	identity, ok := f.svc.ResolveSessionToken(ctx, token, "")
	if !ok || identity.UserID != userstore.DefaultUserID || !identity.IsAdmin() {
		t.Fatalf("session identity = %+v ok=%v", identity, ok)
	}
	// Setup is single-shot.
	if _, _, err := f.svc.Setup(ctx, "x@y.z", "password123", "", "", ""); !errors.Is(err, ErrSetupNotAvailable) {
		t.Fatalf("second setup: %v", err)
	}
}

func setupEnabled(t *testing.T, f *serviceFixture) {
	t.Helper()
	enableAuth(t, f)
	if _, _, err := f.svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", ""); err != nil {
		t.Fatal(err)
	}
}

func TestLoginFlow(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()
	setupEnabled(t, f)

	if _, _, err := f.svc.Login(ctx, "admin@x.dev", "wrong-password", "", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("wrong password: %v", err)
	}
	if _, _, err := f.svc.Login(ctx, "ghost@x.dev", "whatever-pass", "", "1.2.3.4"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("unknown email: %v", err)
	}
	user, token, err := f.svc.Login(ctx, "ADMIN@x.dev", "adminpass123", "ua", "1.2.3.4")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if user.Email != "admin@x.dev" {
		t.Fatalf("unexpected user: %+v", user)
	}
	identity, ok := f.svc.ResolveSessionToken(ctx, token, "")
	if !ok || !identity.IsAdmin() {
		t.Fatalf("resolve: %+v ok=%v", identity, ok)
	}
	// Logout revokes.
	if err := f.svc.Logout(ctx, identity.SessionID, identity.UserID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, token, ""); ok {
		t.Fatal("session must be dead after logout")
	}
}

func TestLoginRateLimit(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()
	setupEnabled(t, f)

	var lastErr error
	for i := 0; i < loginRateAttempts+1; i++ {
		_, _, lastErr = f.svc.Login(ctx, "admin@x.dev", "bad-password!", "", "9.9.9.9")
	}
	if !errors.Is(lastErr, ErrRateLimited) {
		t.Fatalf("expected rate limit, got %v", lastErr)
	}
}

func TestChangePasswordRevokesOtherSessions(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()
	setupEnabled(t, f)

	_, tokenA, err := f.svc.Login(ctx, "admin@x.dev", "adminpass123", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, tokenB, err := f.svc.Login(ctx, "admin@x.dev", "adminpass123", "", "")
	if err != nil {
		t.Fatal(err)
	}
	identityA, _ := f.svc.ResolveSessionToken(ctx, tokenA, "")

	if err := f.svc.ChangePassword(ctx, identityA.UserID, identityA.SessionID, "adminpass123", "newpass12345"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, tokenA, ""); !ok {
		t.Fatal("current session must survive password change")
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, tokenB, ""); ok {
		t.Fatal("other sessions must be revoked")
	}
	if _, _, err := f.svc.Login(ctx, "admin@x.dev", "newpass12345", "", ""); err != nil {
		t.Fatalf("login with new password: %v", err)
	}
}

func TestInviteFlow(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()
	setupEnabled(t, f)

	_, token, err := f.svc.CreateInvite(ctx, userstore.DefaultUserID, "new@x.dev", "member", 0)
	if err != nil {
		t.Fatalf("create invite: %v", err)
	}
	// Wrong email is rejected when the invite pins one.
	if _, _, err := f.svc.AcceptInvite(ctx, token, "other@x.dev", "memberpass123", "", "", ""); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("email mismatch: %v", err)
	}
	user, sessionToken, err := f.svc.AcceptInvite(ctx, token, "new@x.dev", "memberpass123", "New User", "", "")
	if err != nil {
		t.Fatalf("accept invite: %v", err)
	}
	if user.Role != usermodels.RoleMember {
		t.Fatalf("role = %s, want member", user.Role)
	}
	identity, ok := f.svc.ResolveSessionToken(ctx, sessionToken, "")
	if !ok || identity.IsAdmin() {
		t.Fatalf("member identity: %+v ok=%v", identity, ok)
	}
	// Single-use.
	if _, _, err := f.svc.AcceptInvite(ctx, token, "again@x.dev", "memberpass123", "", "", ""); !errors.Is(err, ErrInviteInvalid) {
		t.Fatalf("double accept: %v", err)
	}
}

func TestAdminUserManagement(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()
	setupEnabled(t, f)

	member, err := f.svc.AdminCreateUser(ctx, "m@x.dev", "memberpass123", "M", "member")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	if _, err := f.svc.AdminCreateUser(ctx, "m@x.dev", "memberpass123", "M2", "member"); !errors.Is(err, ErrEmailTaken) {
		t.Fatalf("duplicate email: %v", err)
	}
	// Last-admin guard: sole admin cannot be demoted or disabled.
	if _, err := f.svc.AdminSetRoleStatus(ctx, userstore.DefaultUserID, "member", ""); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("demote last admin: %v", err)
	}
	if _, err := f.svc.AdminSetRoleStatus(ctx, userstore.DefaultUserID, "", "disabled"); !errors.Is(err, ErrLastAdmin) {
		t.Fatalf("disable last admin: %v", err)
	}
	// Disabling a member kills their credentials.
	_, memberToken, err := f.svc.Login(ctx, "m@x.dev", "memberpass123", "", "")
	if err != nil {
		t.Fatal(err)
	}
	record, pat, err := f.svc.MintToken(ctx, member.ID, "ci", 0)
	if err != nil || record.ID == "" {
		t.Fatalf("mint token: %v", err)
	}
	if _, ok := f.svc.ResolveBearer(ctx, pat); !ok {
		t.Fatal("fresh PAT must authenticate")
	}
	if _, err := f.svc.AdminSetRoleStatus(ctx, member.ID, "", "disabled"); err != nil {
		t.Fatalf("disable member: %v", err)
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, memberToken, ""); ok {
		t.Fatal("disabled member session must fail")
	}
	if _, ok := f.svc.ResolveBearer(ctx, pat); ok {
		t.Fatal("disabled member PAT must fail")
	}
	if _, _, err := f.svc.Login(ctx, "m@x.dev", "memberpass123", "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("disabled member login: %v", err)
	}
}

func TestResolveBearerIgnoresNonPAT(t *testing.T) {
	f := newServiceFixture(t, false)
	if _, ok := f.svc.ResolveBearer(context.Background(), "eyJhbGciOiJIUzI1NiJ9.agent.jwt"); ok {
		t.Fatal("non-PAT bearer must not resolve")
	}
}

// TestResolveSessionTokenRefreshesChangedIP pins the session-IP refresh on
// the throttled touch path: after the touch interval the request IP replaces
// the stored value when non-empty and differing; same-IP, empty-IP, and
// within-interval requests never change it. Each case runs on a FRESH aged
// session (the first resolve's touch bumps last_seen_at, which would
// throttle later resolves into vacuous passes). Rows are always selected by
// TokenHash, never list index: sessions accumulate on the shared fixture and
// ListSessionsByUser orders by last_seen_at DESC with arbitrary ties (the
// within-interval row's future seed sorts above touched rows).
func TestResolveSessionTokenRefreshesChangedIP(t *testing.T) {
	f := newServiceFixture(t, true)
	ctx := context.Background()

	// seedAgedSession inserts a session whose last_seen_at is far enough in
	// the past that the strict > sessionTouchInterval gate is open, with a
	// 24h future expiry so a slow test cannot trip the delete-before-touch
	// path, under the active DefaultUserID row seeded by userstore.Provide.
	seedAgedSession := func(ip string) (token, hash string, seededLastSeen, seededExpires time.Time) {
		t.Helper()
		now := time.Now().UTC()
		token, hash, err := GenerateSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		seededLastSeen = now.Add(-2 * time.Hour)
		seededExpires = now.Add(24 * time.Hour)
		session := &store.Session{
			UserID: userstore.DefaultUserID, TokenHash: hash,
			CreatedAt: now, ExpiresAt: seededExpires, LastSeenAt: seededLastSeen,
			IP: ip,
		}
		if err := f.svc.store.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		return token, hash, seededLastSeen, seededExpires
	}

	sessionByHash := func(hash string) *store.Session {
		t.Helper()
		sessions, err := f.svc.ListSessions(ctx, userstore.DefaultUserID)
		if err != nil {
			t.Fatalf("list sessions: %v", err)
		}
		for _, s := range sessions {
			if s.TokenHash == hash {
				return s
			}
		}
		t.Fatalf("no session row for hash %s", hash)
		return nil
	}

	t.Run("different IP refreshes stored IP", func(t *testing.T) {
		token, hash, _, _ := seedAgedSession("1.1.1.1")
		if _, ok := f.svc.ResolveSessionToken(ctx, token, "2.2.2.2"); !ok {
			t.Fatal("resolve failed")
		}
		if got := sessionByHash(hash).IP; got != "2.2.2.2" {
			t.Fatalf("stored IP = %q, want 2.2.2.2", got)
		}
	})

	t.Run("non-empty request IP backfills empty stored IP", func(t *testing.T) {
		token, hash, _, _ := seedAgedSession("")
		if _, ok := f.svc.ResolveSessionToken(ctx, token, "2.2.2.2"); !ok {
			t.Fatal("resolve failed")
		}
		if got := sessionByHash(hash).IP; got != "2.2.2.2" {
			t.Fatalf("stored IP = %q, want backfill 2.2.2.2", got)
		}
	})

	t.Run("same IP keeps stored IP and still touches", func(t *testing.T) {
		token, hash, seededLastSeen, seededExpires := seedAgedSession("1.1.1.1")
		if _, ok := f.svc.ResolveSessionToken(ctx, token, "1.1.1.1"); !ok {
			t.Fatal("resolve failed")
		}
		got := sessionByHash(hash)
		if got.IP != "1.1.1.1" {
			t.Fatalf("stored IP = %q, want 1.1.1.1", got.IP)
		}
		// The touch must still advance the gate input (sliding expiry)…
		assertNow := time.Now().UTC()
		if !got.LastSeenAt.After(seededLastSeen) {
			t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, seededLastSeen)
		}
		// …with the resolve-time now (not now+TTL: a swapped-args touch
		// would blow the upper bound), and the expiry must extend past the
		// seed's, not pass it through unchanged.
		if got.LastSeenAt.After(assertNow.Add(time.Minute)) {
			t.Fatalf("last_seen_at = %v, want not after %v", got.LastSeenAt, assertNow.Add(time.Minute))
		}
		if !got.ExpiresAt.After(seededExpires.Add(time.Hour)) {
			t.Fatalf("expires_at = %v, want after %v", got.ExpiresAt, seededExpires.Add(time.Hour))
		}
	})

	t.Run("empty request IP keeps stored IP and still touches", func(t *testing.T) {
		token, hash, seededLastSeen, seededExpires := seedAgedSession("1.1.1.1")
		if _, ok := f.svc.ResolveSessionToken(ctx, token, ""); !ok {
			t.Fatal("resolve failed")
		}
		got := sessionByHash(hash)
		if got.IP != "1.1.1.1" {
			t.Fatalf("stored IP = %q, want 1.1.1.1", got.IP)
		}
		assertNow := time.Now().UTC()
		if !got.LastSeenAt.After(seededLastSeen) {
			t.Fatalf("last_seen_at = %v, want after %v", got.LastSeenAt, seededLastSeen)
		}
		if got.LastSeenAt.After(assertNow.Add(time.Minute)) {
			t.Fatalf("last_seen_at = %v, want not after %v", got.LastSeenAt, assertNow.Add(time.Minute))
		}
		if !got.ExpiresAt.After(seededExpires.Add(time.Hour)) {
			t.Fatalf("expires_at = %v, want after %v", got.ExpiresAt, seededExpires.Add(time.Hour))
		}
	})

	t.Run("within-interval different IP defers refresh to next touch", func(t *testing.T) {
		now := time.Now().UTC()
		// 10 intervals in the future: the strict > gate stays closed for
		// ~11 minutes of wall clock, so the always-touch variants
		// self-reveal (writing 2.2.2.2 or bumping the future seed) while the
		// correct implementation leaves the row untouched. Truncated to the
		// second for the Equal() pin, per the store-test convention.
		seed := now.Truncate(time.Second).Add(10 * sessionTouchInterval)
		future := now.Add(24 * time.Hour)
		token, hash, err := GenerateSessionToken()
		if err != nil {
			t.Fatal(err)
		}
		session := &store.Session{
			UserID: userstore.DefaultUserID, TokenHash: hash,
			CreatedAt: now, ExpiresAt: future, LastSeenAt: seed,
			IP: "1.1.1.1",
		}
		if err := f.svc.store.CreateSession(ctx, session); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.svc.ResolveSessionToken(ctx, token, "2.2.2.2"); !ok {
			t.Fatal("resolve must succeed for future-seed session")
		}
		got := sessionByHash(hash)
		if got.IP != "1.1.1.1" {
			t.Fatalf("stored IP = %q within interval, want 1.1.1.1", got.IP)
		}
		if !got.LastSeenAt.Equal(seed) {
			t.Fatalf("last_seen_at = %v, want seed %v (no touch within interval)", got.LastSeenAt, seed)
		}
		// Spec scenario 5's second half: once the interval elapses the next
		// resolve with the same different IP refreshes the stored value.
		if err := f.svc.store.TouchSession(ctx, session.ID, now.Add(-2*time.Hour), future, ""); err != nil {
			t.Fatal(err)
		}
		if _, ok := f.svc.ResolveSessionToken(ctx, token, "2.2.2.2"); !ok {
			t.Fatal("resolve after aging failed")
		}
		if got := sessionByHash(hash).IP; got != "2.2.2.2" {
			t.Fatalf("stored IP after interval = %q, want 2.2.2.2", got)
		}
	})
}

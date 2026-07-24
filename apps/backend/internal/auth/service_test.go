package auth

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

type fakeSettings struct {
	mu     sync.Mutex
	values map[string][]byte
}

func (f *fakeSettings) Get(_ context.Context, key string) ([]byte, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[key]
	return v, ok, nil
}

func (f *fakeSettings) Save(_ context.Context, key string, value []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.values == nil {
		f.values = map[string][]byte{}
	}
	f.values[key] = value
	return nil
}

type serviceFixture struct {
	svc       *Service
	users     userstore.Repository
	settings  *fakeSettings
	backfills *[]string
}

func newServiceFixture(t *testing.T, required bool) *serviceFixture {
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
	settings := &fakeSettings{}
	claimed := []string{}
	cfg := &config.Config{}
	cfg.Auth.Required = required
	cfg.Auth.SessionTTLHours = 720
	svc, err := NewService(context.Background(), Deps{
		Cfg:      cfg,
		Store:    authStore,
		Users:    users,
		Settings: settings,
		Backfills: []BackfillFunc{func(_ context.Context, ownerID string) error {
			claimed = append(claimed, ownerID)
			return nil
		}},
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return &serviceFixture{svc: svc, users: users, settings: settings, backfills: &claimed}
}

func TestModeDisabledByDefault(t *testing.T) {
	f := newServiceFixture(t, false)
	if got := f.svc.Mode(); got != ModeDisabled {
		t.Fatalf("mode = %s, want disabled", got)
	}
}

func TestModeSetupWhenRequiredWithoutAdmin(t *testing.T) {
	f := newServiceFixture(t, true)
	if got := f.svc.Mode(); got != ModeSetup {
		t.Fatalf("mode = %s, want setup", got)
	}
}

func TestSetEnabledEntersSetupThenSetupPromotesDefaultUser(t *testing.T) {
	f := newServiceFixture(t, false)
	ctx := context.Background()

	mode, err := f.svc.SetEnabled(ctx, true)
	if err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	if mode != ModeSetup {
		t.Fatalf("mode = %s, want setup", mode)
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
	identity, ok := f.svc.ResolveSessionToken(ctx, token)
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
	ctx := context.Background()
	if _, err := f.svc.SetEnabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.svc.Setup(ctx, "admin@x.dev", "adminpass123", "Admin", "", ""); err != nil {
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
	identity, ok := f.svc.ResolveSessionToken(ctx, token)
	if !ok || !identity.IsAdmin() {
		t.Fatalf("resolve: %+v ok=%v", identity, ok)
	}
	// Logout revokes.
	if err := f.svc.Logout(ctx, identity.SessionID, identity.UserID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, token); ok {
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
	identityA, _ := f.svc.ResolveSessionToken(ctx, tokenA)

	if err := f.svc.ChangePassword(ctx, identityA.UserID, identityA.SessionID, "adminpass123", "newpass12345"); err != nil {
		t.Fatalf("change password: %v", err)
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, tokenA); !ok {
		t.Fatal("current session must survive password change")
	}
	if _, ok := f.svc.ResolveSessionToken(ctx, tokenB); ok {
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
	identity, ok := f.svc.ResolveSessionToken(ctx, sessionToken)
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
	if _, ok := f.svc.ResolveSessionToken(ctx, memberToken); ok {
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

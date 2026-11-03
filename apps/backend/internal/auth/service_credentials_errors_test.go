package auth

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/auth/store"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	usermodels "github.com/kandev/kandev/internal/user/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

var credentialFixtureSequence atomic.Uint64

type credentialFixture struct {
	svc        *Service
	authStore  *store.Store
	realUsers  userstore.AccountRepository
	writer     *sqlx.DB
	adminToken string
}

type credentialFixtureOptions struct {
	log            *logger.Logger
	wrapAccounts   func(userstore.AccountRepository) userstore.AccountRepository
	orgStatus      OrgStatusChecker
	skipAdminSetup bool
}

func newCredentialFixture(t *testing.T, opts credentialFixtureOptions) *credentialFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:auth-credential-%d?mode=memory&cache=shared", credentialFixtureSequence.Add(1))
	writer := openCredentialSQLite(t, dsn)
	reader := openCredentialSQLite(t, dsn)
	users, cleanup, err := userstore.Provide(writer, reader)
	if err != nil {
		t.Fatalf("user store: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	authStore, err := store.New(writer, reader)
	if err != nil {
		t.Fatalf("auth store: %v", err)
	}
	accounts := userstore.AccountRepository(users)
	if opts.wrapAccounts != nil {
		accounts = opts.wrapAccounts(accounts)
	}
	cfg := &config.Config{}
	cfg.Features.Auth = true
	cfg.Auth.SessionTTLHours = 720
	svc, err := NewService(context.Background(), Deps{
		Cfg:   cfg,
		Store: authStore,
		Users: accounts,
		Log:   opts.log,
	})
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	if opts.orgStatus != nil {
		svc.SetOrgStatusChecker(opts.orgStatus)
	}
	f := &credentialFixture{
		svc: svc, authStore: authStore, realUsers: users, writer: writer,
	}
	if !opts.skipAdminSetup {
		_, token, err := svc.Setup(context.Background(), "admin@x.dev", "adminpass123", "Admin", "", "")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		f.adminToken = token
	}
	return f
}

func openCredentialSQLite(t *testing.T, dsn string) *sqlx.DB {
	t.Helper()
	conn, err := sqlx.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func (f *credentialFixture) closeWriter() {
	_ = f.writer.Close()
}

func (f *credentialFixture) createSession(t *testing.T, userID string, expiresAt time.Time) string {
	t.Helper()
	token, hash, err := GenerateSessionToken()
	if err != nil {
		t.Fatalf("generate session: %v", err)
	}
	now := time.Now().UTC()
	if err := f.authStore.CreateSession(context.Background(), &store.Session{
		UserID: userID, TokenHash: hash, CreatedAt: now, ExpiresAt: expiresAt, LastSeenAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	return token
}

func (f *credentialFixture) createToken(t *testing.T, userID string) string {
	t.Helper()
	_, raw, err := f.svc.MintToken(context.Background(), userID, "fixture", 0)
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return raw
}

func (f *credentialFixture) setOrg(t *testing.T, userID, orgID string) {
	t.Helper()
	tenancy, ok := f.realUsers.(userstore.TenancyRepository)
	if !ok {
		t.Fatalf("account repository does not support tenancy")
	}
	if err := tenancy.SetUserOrg(context.Background(), userID, orgID); err != nil {
		t.Fatalf("set user org: %v", err)
	}
}

type accountReadErrorRepository struct {
	userstore.AccountRepository
	failUserID *string
	err        error
}

func (r accountReadErrorRepository) GetUser(ctx context.Context, id string) (*usermodels.User, error) {
	if r.failUserID != nil && id == *r.failUserID {
		return nil, r.err
	}
	return r.AccountRepository.GetUser(ctx, id)
}

func assertUnavailable(t *testing.T, identity authn.Identity, ok bool, err error) {
	t.Helper()
	if ok {
		t.Fatalf("resolution succeeded with identity %+v, want unavailable", identity)
	}
	if identity != (authn.Identity{}) {
		t.Fatalf("unavailable result carried identity %+v", identity)
	}
	if !errors.Is(err, ErrCredentialLookupUnavailable) {
		t.Fatalf("error = %v, want ErrCredentialLookupUnavailable", err)
	}
}

func assertInvalid(t *testing.T, identity authn.Identity, ok bool, err error) {
	t.Helper()
	if ok {
		t.Fatalf("resolution succeeded with identity %+v, want invalid", identity)
	}
	if err != nil {
		t.Fatalf("invalid credential returned error %v, want nil", err)
	}
	if identity != (authn.Identity{}) {
		t.Fatalf("invalid result carried identity %+v", identity)
	}
}

func assertSessionInvalid(t *testing.T, f *credentialFixture, ctx context.Context, token string) {
	t.Helper()
	identity, ok, err := f.svc.ResolveSessionToken(ctx, token, "")
	assertInvalid(t, identity, ok, err)
}

func assertBearerInvalid(t *testing.T, f *credentialFixture, ctx context.Context, token string) {
	t.Helper()
	identity, ok, err := f.svc.ResolveBearer(ctx, token)
	assertInvalid(t, identity, ok, err)
}

func assertSessionUnavailable(t *testing.T, f *credentialFixture, ctx context.Context, token string) {
	t.Helper()
	identity, ok, err := f.svc.ResolveSessionToken(ctx, token, "")
	assertUnavailable(t, identity, ok, err)
}

func assertBearerUnavailable(t *testing.T, f *credentialFixture, ctx context.Context, token string) {
	t.Helper()
	identity, ok, err := f.svc.ResolveBearer(ctx, token)
	assertUnavailable(t, identity, ok, err)
}

func TestCredentialLookupFailures(t *testing.T) {
	ctx := context.Background()

	t.Run("missing session token is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		assertSessionInvalid(t, f, ctx, "missing-token")
	})

	t.Run("empty session token is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		assertSessionInvalid(t, f, ctx, "")
	})

	t.Run("revoked session is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		identity, ok, err := f.svc.ResolveSessionToken(ctx, f.adminToken, "")
		if err != nil || !ok {
			t.Fatalf("baseline resolve: identity=%+v ok=%v err=%v", identity, ok, err)
		}
		if err := f.svc.Logout(ctx, identity.SessionID, identity.UserID); err != nil {
			t.Fatalf("logout: %v", err)
		}
		assertSessionInvalid(t, f, ctx, f.adminToken)
	})

	t.Run("expired session is invalid and cleaned up", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		token := f.createSession(t, userstore.DefaultUserID, time.Now().UTC().Add(-time.Hour))
		assertSessionInvalid(t, f, ctx, token)
		if _, err := f.authStore.GetSessionByTokenHash(ctx, HashToken(token)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("expired session cleanup: err = %v, want ErrNotFound", err)
		}
	})

	t.Run("missing account is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		token := f.createSession(t, "account-that-does-not-exist", time.Now().UTC().Add(time.Hour))
		assertSessionInvalid(t, f, ctx, token)
	})

	t.Run("disabling an account revokes its credentials", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		member, err := f.svc.AdminCreateUser(ctx, "revoked@x.dev", "memberpass123", "Member", "member")
		if err != nil {
			t.Fatalf("create member: %v", err)
		}
		memberToken := f.createSession(t, member.ID, time.Now().UTC().Add(time.Hour))
		memberPAT := f.createToken(t, member.ID)
		if _, err := f.svc.AdminSetRoleStatus(ctx, member.ID, "", "disabled"); err != nil {
			t.Fatalf("disable member: %v", err)
		}
		if _, err := f.authStore.GetSessionByTokenHash(ctx, HashToken(memberToken)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("disabled account session: err = %v, want revoked", err)
		}
		if _, err := f.authStore.GetAPITokenByHash(ctx, HashToken(memberPAT)); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("disabled account PAT: err = %v, want revoked", err)
		}
		assertSessionInvalid(t, f, ctx, memberToken)
		assertBearerInvalid(t, f, ctx, memberPAT)
	})

	t.Run("disabled account with retained credentials is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		member, err := f.svc.AdminCreateUser(ctx, "retained@x.dev", "memberpass123", "Member", "member")
		if err != nil {
			t.Fatalf("create member: %v", err)
		}
		memberToken := f.createSession(t, member.ID, time.Now().UTC().Add(time.Hour))
		memberPAT := f.createToken(t, member.ID)
		if _, err := f.realUsers.UpdateUserRoleStatus(ctx, member.ID, usermodels.RoleMember, usermodels.StatusDisabled); err != nil {
			t.Fatalf("disable member without revoking credentials: %v", err)
		}
		if _, err := f.authStore.GetSessionByTokenHash(ctx, HashToken(memberToken)); err != nil {
			t.Fatalf("disabled account session must remain for this case: %v", err)
		}
		if _, err := f.authStore.GetAPITokenByHash(ctx, HashToken(memberPAT)); err != nil {
			t.Fatalf("disabled account PAT must remain for this case: %v", err)
		}
		assertSessionInvalid(t, f, ctx, memberToken)
		assertBearerInvalid(t, f, ctx, memberPAT)
	})

	t.Run("required account read failure is unavailable", func(t *testing.T) {
		secretErr := errors.New("account store secret-marker")
		failID := new(string)
		f := newCredentialFixture(t, credentialFixtureOptions{
			wrapAccounts: func(base userstore.AccountRepository) userstore.AccountRepository {
				return accountReadErrorRepository{AccountRepository: base, failUserID: failID, err: secretErr}
			},
		})
		member, err := f.svc.AdminCreateUser(ctx, "readfail@x.dev", "memberpass123", "Member", "member")
		if err != nil {
			t.Fatalf("create member: %v", err)
		}
		*failID = member.ID
		f.setOrg(t, member.ID, "org-1")
		token := f.createSession(t, member.ID, time.Now().UTC().Add(time.Hour))
		assertSessionUnavailable(t, f, ctx, token)
	})

	t.Run("organization read failure is unavailable", func(t *testing.T) {
		orgErr := errors.New("organization store secret-marker")
		f := newCredentialFixture(t, credentialFixtureOptions{
			orgStatus: func(context.Context, string) (bool, error) { return false, orgErr },
		})
		f.setOrg(t, userstore.DefaultUserID, "org-1")
		assertSessionUnavailable(t, f, ctx, f.adminToken)
		pat := f.createToken(t, userstore.DefaultUserID)
		assertBearerUnavailable(t, f, ctx, pat)
	})

	t.Run("suspended organization is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{
			orgStatus: func(context.Context, string) (bool, error) { return false, nil },
		})
		f.setOrg(t, userstore.DefaultUserID, "org-1")
		assertSessionInvalid(t, f, ctx, f.adminToken)
		pat := f.createToken(t, userstore.DefaultUserID)
		assertBearerInvalid(t, f, ctx, pat)
	})

	t.Run("missing PAT is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		assertBearerInvalid(t, f, ctx, "kandev_pat_missing")
	})

	t.Run("non-PAT bearer is invalid without an error", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		assertBearerInvalid(t, f, ctx, "eyJhbGciOiJIUzI1NiJ9.agent.jwt")
	})

	t.Run("expired PAT is invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		_, raw, err := f.svc.MintToken(ctx, userstore.DefaultUserID, "expiring", 1)
		if err != nil {
			t.Fatalf("mint token: %v", err)
		}
		past := time.Now().UTC().Add(-time.Hour)
		if _, err := f.writer.ExecContext(ctx, f.writer.Rebind(
			`UPDATE auth_api_tokens SET expires_at = ? WHERE token_sha256 = ?`,
		), past, HashToken(raw)); err != nil {
			t.Fatalf("expire token: %v", err)
		}
		assertBearerInvalid(t, f, ctx, raw)
	})

	t.Run("required credential read failures are unavailable", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		pat := f.createToken(t, userstore.DefaultUserID)
		if _, err := f.writer.ExecContext(ctx, `DROP TABLE auth_sessions`); err != nil {
			t.Fatalf("drop sessions: %v", err)
		}
		if _, err := f.writer.ExecContext(ctx, `DROP TABLE auth_api_tokens`); err != nil {
			t.Fatalf("drop tokens: %v", err)
		}
		assertSessionUnavailable(t, f, ctx, f.adminToken)
		assertBearerUnavailable(t, f, ctx, pat)
	})

	t.Run("canceled context is unavailable rather than invalid", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		identity, ok, err := f.svc.ResolveSessionToken(canceled, f.adminToken, "")
		assertUnavailable(t, identity, ok, err)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want wrapped context.Canceled", err)
		}
	})
}

func TestResolveUserIdentityOrganizationOutcomes(t *testing.T) {
	ctx := context.Background()
	cause := errors.New("organization identity read failure")
	usable := true
	var statusErr error
	f := newCredentialFixture(t, credentialFixtureOptions{
		orgStatus: func(context.Context, string) (bool, error) { return usable, statusErr },
	})
	f.setOrg(t, userstore.DefaultUserID, "org-1")

	identity, ok, err := f.svc.ResolveUserIdentity(ctx, userstore.DefaultUserID)
	if err != nil || !ok || identity.UserID != userstore.DefaultUserID {
		t.Fatalf("usable organization: identity=%+v ok=%v err=%v", identity, ok, err)
	}

	usable = false
	identity, ok, err = f.svc.ResolveUserIdentity(ctx, userstore.DefaultUserID)
	if ok || err != nil || identity != (authn.Identity{}) {
		t.Fatalf("unusable organization: identity=%+v ok=%v err=%v, want invalid with nil error", identity, ok, err)
	}

	statusErr = cause
	identity, ok, err = f.svc.ResolveUserIdentity(ctx, userstore.DefaultUserID)
	if ok || identity != (authn.Identity{}) {
		t.Fatalf("unreadable organization: identity=%+v ok=%v err=%v, want no identity", identity, ok, err)
	}
	if !errors.Is(err, ErrIdentityUnavailable) || !errors.Is(err, cause) {
		t.Fatalf("unreadable organization error = %v, want ErrIdentityUnavailable and injected cause", err)
	}
}

func TestCredentialOptionalWritesDoNotReject(t *testing.T) {
	ctx := context.Background()

	t.Run("session touch failure does not reject a valid session", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		identity, ok, err := f.svc.ResolveSessionToken(ctx, f.adminToken, "")
		if err != nil || !ok {
			t.Fatalf("baseline resolve: identity=%+v ok=%v err=%v", identity, ok, err)
		}
		aged := time.Now().UTC().Add(-2 * time.Hour)
		if err := f.authStore.TouchSession(ctx, identity.SessionID, aged, time.Now().UTC().Add(time.Hour), ""); err != nil {
			t.Fatalf("age session: %v", err)
		}
		f.closeWriter()
		got, ok, err := f.svc.ResolveSessionToken(ctx, f.adminToken, "")
		if err != nil || !ok || got.UserID != userstore.DefaultUserID {
			t.Fatalf("resolve after touch failure: identity=%+v ok=%v err=%v", got, ok, err)
		}
	})

	t.Run("expired-session cleanup failure does not make the result unavailable", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		token := f.createSession(t, userstore.DefaultUserID, time.Now().UTC().Add(-time.Hour))
		f.closeWriter()
		assertSessionInvalid(t, f, ctx, token)
	})

	t.Run("PAT touch failure does not reject a valid token", func(t *testing.T) {
		f := newCredentialFixture(t, credentialFixtureOptions{})
		pat := f.createToken(t, userstore.DefaultUserID)
		f.closeWriter()
		got, ok, err := f.svc.ResolveBearer(ctx, pat)
		if err != nil || !ok || got.TokenID == "" {
			t.Fatalf("resolve after touch failure: identity=%+v ok=%v err=%v", got, ok, err)
		}
	})
}

func TestCredentialLookupDiagnostics(t *testing.T) {
	ctx := context.Background()
	core, observed := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	secretErr := errors.New("diagnostic-secret-marker postgres://user:password@host/db")
	failID := new(string)
	orgErr := errors.New("org-diagnostic-secret-marker")
	f := newCredentialFixture(t, credentialFixtureOptions{
		log: log,
		wrapAccounts: func(base userstore.AccountRepository) userstore.AccountRepository {
			return accountReadErrorRepository{AccountRepository: base, failUserID: failID, err: secretErr}
		},
		orgStatus: func(context.Context, string) (bool, error) { return false, orgErr },
	})
	member, err := f.svc.AdminCreateUser(ctx, "diag@x.dev", "memberpass123", "Member", "member")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	*failID = member.ID
	f.setOrg(t, member.ID, "org-1")
	memberToken := f.createSession(t, member.ID, time.Now().UTC().Add(time.Hour))
	assertSessionUnavailable(t, f, ctx, memberToken)
	f.setOrg(t, userstore.DefaultUserID, "org-1")
	pat := f.createToken(t, userstore.DefaultUserID)
	assertBearerUnavailable(t, f, ctx, pat)

	entries := observed.All()
	if len(entries) != 2 {
		t.Fatalf("diagnostic entries = %d, want one per failed read (2): %+v", len(entries), entries)
	}
	want := []map[string]any{
		{"credential_kind": "session", "operation": "account_read", "category": "store_error"},
		{"credential_kind": "pat", "operation": "organization_read", "category": "store_error"},
	}
	for index, entry := range entries {
		if entry.Message != "auth.credential_lookup_failed" {
			t.Fatalf("log message = %q", entry.Message)
		}
		fields := entry.ContextMap()
		if !reflect.DeepEqual(fields, want[index]) {
			t.Fatalf("diagnostic fields = %#v, want %#v", fields, want[index])
		}
		if value := fmt.Sprint(fields); containsSecretMarker(value) {
			t.Fatalf("diagnostic leaked raw error text: %#v", fields)
		}
	}
}

func containsSecretMarker(value string) bool {
	for _, marker := range []string{
		"diagnostic-secret-marker", "org-diagnostic-secret-marker", "postgres://", "password",
	} {
		if strings.Contains(value, marker) {
			return true
		}
	}
	return false
}

func TestCredentialLookupNilLoggerIsSafe(t *testing.T) {
	ctx := context.Background()
	secretErr := errors.New("nil-logger-secret-marker")
	failID := new(string)
	f := newCredentialFixture(t, credentialFixtureOptions{
		wrapAccounts: func(base userstore.AccountRepository) userstore.AccountRepository {
			return accountReadErrorRepository{AccountRepository: base, failUserID: failID, err: secretErr}
		},
	})
	member, err := f.svc.AdminCreateUser(ctx, "nillog@x.dev", "memberpass123", "Member", "member")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	*failID = member.ID
	token := f.createSession(t, member.ID, time.Now().UTC().Add(time.Hour))
	assertSessionUnavailable(t, f, ctx, token)
}

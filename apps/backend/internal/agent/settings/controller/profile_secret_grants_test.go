package controller

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/agent/settings/dto"
	"github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/secrets"
)

// Synthetic accounts: grant-owner owns the global secret, grant-collab is a
// member of the same organization who does not.
const (
	grantOwner    = "grant-owner"
	grantCollab   = "grant-collab"
	grantSecretID = "synthetic-grant-secret"
	grantEnvKey   = "SHARED_TOKEN"
)

func grantCtx(userID string) context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: userID, OrgID: "org-1", Role: authn.RoleMember})
}

func boolPtr(v bool) *bool { return &v }

type grantFixture struct {
	ctrl    *Controller
	repo    store.Repository
	secrets secrets.SecretStore
	agentID string
}

func newGrantFixture(t *testing.T) grantFixture {
	t.Helper()
	ctrl, repo, _, codexID, _ := newProviderTestController(t)
	dir := t.TempDir()
	conn, err := db.OpenSQLite(filepath.Join(dir, "secrets.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	crypto, err := secrets.NewMasterKeyProvider(dir)
	if err != nil {
		t.Fatalf("master key: %v", err)
	}
	raw, cleanup, err := secrets.Provide(sqlxDB, sqlxDB, crypto)
	if err != nil {
		t.Fatalf("provide secrets: %v", err)
	}
	t.Cleanup(func() { _ = cleanup() })
	secretStore := secrets.NewUserVisibleStore(raw)
	if err := secretStore.Create(grantCtx(grantOwner), &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: grantSecretID, Name: "shared", Scope: secrets.ScopeGlobal},
		Value:  "synthetic-grant-value",
	}); err != nil {
		t.Fatalf("seed secret: %v", err)
	}
	ctrl.SetSecretStore(secretStore)
	return grantFixture{ctrl: ctrl, repo: repo, secrets: secretStore, agentID: codexID}
}

func (f grantFixture) create(ctx context.Context, shared *bool) (*dto.AgentProfileDTO, error) {
	return f.ctrl.CreateProfile(ctx, CreateProfileRequest{
		AgentID: f.agentID, Name: "Shared",
		EnvVars: []dto.ProfileEnvVarDTO{{Key: grantEnvKey, SecretID: grantSecretID, Shared: shared}},
	})
}

func (f grantFixture) storedGrant(t *testing.T, profileID string) string {
	t.Helper()
	profile, err := f.repo.GetAgentProfile(context.Background(), profileID)
	if err != nil {
		t.Fatalf("get profile: %v", err)
	}
	for _, envVar := range profile.EnvVars {
		if envVar.Key == grantEnvKey {
			return envVar.SharedByUserID
		}
	}
	t.Fatalf("binding %s missing from %#v", grantEnvKey, profile.EnvVars)
	return ""
}

func (f grantFixture) sharedProfile(t *testing.T) *dto.AgentProfileDTO {
	t.Helper()
	created, err := f.create(grantCtx(grantOwner), boolPtr(true))
	if err != nil {
		t.Fatalf("owner shares secret: %v", err)
	}
	return created
}

func (f grantFixture) updateEnv(ctx context.Context, id string, envVars []dto.ProfileEnvVarDTO) error {
	_, err := f.ctrl.UpdateProfile(ctx, UpdateProfileRequest{ID: id, EnvVars: &envVars})
	return err
}

func TestOwnerSharesSecretWithoutExposingIdentity(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	if got := f.storedGrant(t, created.ID); got != grantOwner {
		t.Fatalf("stored grantor = %q, want %q", got, grantOwner)
	}
	if len(created.EnvVars) != 1 || created.EnvVars[0].Shared == nil || !*created.EnvVars[0].Shared {
		t.Fatalf("response env vars = %#v, want shared", created.EnvVars)
	}
	body, err := json.Marshal(created)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(body), grantOwner) || strings.Contains(string(body), "synthetic-grant-value") {
		t.Fatalf("profile response exposes the grantor or value: %s", body)
	}
}

func TestProfileSecretShareRejectsNonOwners(t *testing.T) {
	cases := []struct {
		name string
		ctx  context.Context
	}{
		{"collaborator", grantCtx(grantCollab)},
		{"auth disabled synthetic admin", authn.WithIdentity(context.Background(), authn.Identity{Synthetic: true, Role: authn.RoleAdmin})},
		{"internal caller without identity", context.Background()},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newGrantFixture(t)
			if _, err := f.create(tc.ctx, boolPtr(true)); !errors.Is(err, ErrInvalidProfileEnvVars) {
				t.Fatalf("create = %v, want ErrInvalidProfileEnvVars", err)
			}
			profiles, err := f.repo.ListAgentProfiles(context.Background(), f.agentID)
			if err != nil || len(profiles) != 0 {
				t.Fatalf("rejected share persisted profiles %#v, %v", profiles, err)
			}
		})
	}
}

func TestProfileSecretShareRejectsLiteralAndNestedCreate(t *testing.T) {
	f := newGrantFixture(t)
	_, err := f.ctrl.CreateProfile(grantCtx(grantOwner), CreateProfileRequest{
		AgentID: f.agentID, Name: "Literal",
		EnvVars: []dto.ProfileEnvVarDTO{{Key: grantEnvKey, Value: "plain", Shared: boolPtr(true)}},
	})
	if !errors.Is(err, ErrInvalidProfileEnvVars) {
		t.Fatalf("literal share = %v, want ErrInvalidProfileEnvVars", err)
	}
	nested := CreateAgentProfileRequest{
		Name:    "Nested",
		EnvVars: []dto.ProfileEnvVarDTO{{Key: grantEnvKey, SecretID: grantSecretID, Shared: boolPtr(true)}},
	}
	if err := validateCreateProfileRequest(nested); !errors.Is(err, ErrInvalidProfileEnvVars) {
		t.Fatalf("nested share = %v, want ErrInvalidProfileEnvVars", err)
	}
}

func TestProfileSecretShareSurvivesUnrelatedEdits(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	name := "Renamed"
	if _, err := f.ctrl.UpdateProfile(grantCtx(grantCollab), UpdateProfileRequest{ID: created.ID, Name: &name}); err != nil {
		t.Fatalf("collaborator rename: %v", err)
	}
	if err := f.updateEnv(grantCtx(grantCollab), created.ID, []dto.ProfileEnvVarDTO{
		{Key: grantEnvKey, SecretID: grantSecretID},
		{Key: "PLAIN", Value: "plain"},
	}); err != nil {
		t.Fatalf("collaborator env edit keeping the binding: %v", err)
	}
	if got := f.storedGrant(t, created.ID); got != grantOwner {
		t.Fatalf("grant after unrelated edits = %q, want %q", got, grantOwner)
	}
}

func TestProfileSecretShareDoesNotFollowAChangedBinding(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	err := f.updateEnv(grantCtx(grantCollab), created.ID, []dto.ProfileEnvVarDTO{{Key: "RENAMED_TOKEN", SecretID: grantSecretID}})
	if !errors.Is(err, ErrInvalidProfileEnvVars) {
		t.Fatalf("collaborator moving the shared secret to a new key = %v, want rejection", err)
	}
	if got := f.storedGrant(t, created.ID); got != grantOwner {
		t.Fatalf("rejected update changed the grant to %q", got)
	}
}

func TestOwnerRevokesProfileSecretShare(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	if err := f.updateEnv(grantCtx(grantOwner), created.ID, []dto.ProfileEnvVarDTO{
		{Key: grantEnvKey, SecretID: grantSecretID, Shared: boolPtr(false)},
	}); err != nil {
		t.Fatalf("owner revoke: %v", err)
	}
	if got := f.storedGrant(t, created.ID); got != "" {
		t.Fatalf("grant after revoke = %q, want none", got)
	}
	err := f.updateEnv(grantCtx(grantCollab), created.ID, []dto.ProfileEnvVarDTO{
		{Key: grantEnvKey, SecretID: grantSecretID, Shared: boolPtr(true)},
	})
	if !errors.Is(err, ErrInvalidProfileEnvVars) {
		t.Fatalf("collaborator re-share after revoke = %v, want rejection", err)
	}
}

func TestProfileSecretShareFailsClosedAfterOwnerDeletesSecret(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	if err := f.secrets.Delete(grantCtx(grantOwner), grantSecretID); err != nil {
		t.Fatalf("delete secret: %v", err)
	}
	err := f.updateEnv(grantCtx(grantCollab), created.ID, []dto.ProfileEnvVarDTO{{Key: grantEnvKey, SecretID: grantSecretID}})
	if !errors.Is(err, ErrInvalidProfileEnvVars) || strings.Contains(err.Error(), grantSecretID) {
		t.Fatalf("stale grant update = %v, want a rejection that does not name the secret", err)
	}
}

func TestDuplicateProfileDropsSecretShare(t *testing.T) {
	f := newGrantFixture(t)
	created := f.sharedProfile(t)
	duplicated, err := f.ctrl.DuplicateProfile(grantCtx(grantOwner), DuplicateProfileRequest{ID: created.ID})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if got := f.storedGrant(t, duplicated.ID); got != "" {
		t.Fatalf("duplicate carried grant %q", got)
	}
	if got := f.storedGrant(t, created.ID); got != grantOwner {
		t.Fatalf("source lost its grant: %q", got)
	}
}

func TestProfileSecretGrantsApplyOnlyToTheirBinding(t *testing.T) {
	grants := profileSecretGrants{profileSecretGrantKey(grantEnvKey, grantSecretID): grantOwner}
	got := grants.apply([]models.ProfileEnvVar{
		{Key: grantEnvKey, SecretID: grantSecretID, SharedByUserID: "forged"},
		{Key: "OTHER", SecretID: grantSecretID, SharedByUserID: "forged"},
		{Key: grantEnvKey, Value: "literal", SharedByUserID: "forged"},
	})
	if got[0].SharedByUserID != grantOwner || got[1].SharedByUserID != "" || got[2].SharedByUserID != "" {
		t.Fatalf("applied grants = %#v", got)
	}
}

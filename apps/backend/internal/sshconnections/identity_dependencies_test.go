package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/auth/authn"
)

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.6
func TestIdentityDependenciesIdentifyBastionConnectionsAndRetainedSessions(t *testing.T) {
	s, ctx := identityService(t)
	testIdentityDependencies(t, s, ctx)
}

func testIdentityDependencies(t *testing.T, s *Service, ctx context.Context) {
	t.Helper()
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "jump key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []Connection{
		{ID: "bastion", Name: "Jump host", IdentityID: identity.ID},
		{ID: "destination", Name: "Build host", IdentityID: "another-key", BastionID: "bastion"},
		{ID: "unrelated", Name: "Unrelated", IdentityID: "another-key"},
	} {
		if _, err := s.store.SaveConnection(ctx, c, 0); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{
		`CREATE TABLE task_sessions(id TEXT PRIMARY KEY, task_id TEXT)`,
		`INSERT INTO task_sessions VALUES ('retained-session','owned-task')`,
		`INSERT INTO ssh_connection_refs VALUES ('runtime','retained-session','destination','stamp')`,
		`INSERT INTO ssh_connection_refs VALUES ('executor','saved-executor','destination','stamp')`,
	} {
		if _, err := s.store.db.Exec(query); err != nil {
			t.Fatal(err)
		}
	}
	router := managerRouter(s, &authn.Identity{UserID: "admin", Role: authn.RoleAdmin})
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/ssh/managed/identities/"+identity.ID, strings.NewReader(`{"expected_version":1}`)))
		wantStatus := 200
		if method == http.MethodDelete {
			wantStatus = 409
		}
		if rec.Code != wantStatus {
			t.Fatalf("%s status=%d", method, rec.Code)
		}
		var body struct {
			Dependencies []struct {
				Kind, ID, Label string
				TaskID          string `json:"task_id"`
			}
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		got := map[string]bool{}
		for _, dep := range body.Dependencies {
			got[dep.Kind+":"+dep.ID] = true
		}
		for _, want := range []string{"connection:bastion", "connection:destination", "executor:saved-executor", "runtime:retained-session"} {
			if !got[want] {
				t.Errorf("%s missing dependency %s", method, want)
			}
		}
		if len(got) != 4 {
			t.Errorf("%s dependencies=%v", method, got)
		}
	}
}

func TestIdentityDependenciesAreAdminOnlyAndDoNotChangeStoredMetadata(t *testing.T) {
	s, ctx := identityService(t)
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.SaveConnection(ctx, Connection{ID: "build-route", Name: "Build server", IdentityID: identity.ID}, 0); err != nil {
		t.Fatal(err)
	}
	var before string
	if err = s.store.db.Get(&before, `SELECT data FROM ssh_identities WHERE id=?`, identity.ID); err != nil {
		t.Fatal(err)
	}
	list, err := s.ListIdentities(ctx)
	if err != nil || len(list) != 1 || len(list[0].Dependencies) != 1 {
		t.Fatalf("list dependencies: %v %v", list, err)
	}
	for _, method := range []string{http.MethodGet, http.MethodDelete} {
		router := managerRouter(s, &authn.Identity{UserID: "member", Role: authn.RoleMember})
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(method, "/api/v1/ssh/managed/identities/"+identity.ID, strings.NewReader(`{"expected_version":1}`)))
		if rec.Code != 403 || strings.Contains(rec.Body.String(), "Build server") {
			t.Fatalf("member disclosure: status=%d", rec.Code)
		}
	}
	var after string
	if err = s.store.db.Get(&after, `SELECT data FROM ssh_identities WHERE id=?`, identity.ID); err != nil {
		t.Fatal(err)
	}
	if before != after || strings.Contains(after, "dependencies") {
		t.Fatal("read/deletion refusal changed durable identity metadata")
	}
}

// @covers AC-EXECUTORS-SSH-CONNECTIONS-001.6
func TestUnusedIdentityDeletionLeavesRemoteAuthorizationUnchanged(t *testing.T) {
	s, ctx := identityService(t)
	identity := newWireIdentity(t, s, ctx, "unused")
	host := newWireHost(t, identity.PublicKey)
	before := host.keys(t)
	if err := s.DeleteIdentity(ctx, identity.ID, identity.Version); err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetIdentity(ctx, identity.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("deleted identity still available: %v", err)
	}
	tx, err := s.store.db.BeginTxx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err = s.vault.Reveal(ctx, tx, "ssh-identity:"+identity.ID); err == nil {
		t.Fatal("deleted identity retained its protected key")
	}
	if host.keys(t) != before || host.executions.Load() != 0 || host.keyCalls.Load() != 0 {
		t.Fatal("local deletion changed or accessed remote authorization")
	}
}

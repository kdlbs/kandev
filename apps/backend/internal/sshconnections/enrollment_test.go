package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"strings"
	"testing"
)

func TestEnrollmentJournalsBeforeRemoteAccessAndNeverReplaysPassword(t *testing.T) {
	s, ctx := identityService(t)
	testEnrollmentJournal(t, s, ctx)
}

func testEnrollmentJournal(t *testing.T, s *Service, ctx context.Context) {
	t.Helper()
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.store.SaveConnection(ctx, Connection{ID: uuid.NewString(), Host: "127.0.0.1", Port: 1, User: "user", IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	err = s.store.write(ctx, func(tx *sqlx.Tx) error { c.Fingerprint = "SHA256:pin"; return putConnection(ctx, tx, c) })
	if err != nil {
		t.Fatal(err)
	}
	route, err := s.readRoute(ctx, c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	req := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: identity.Fingerprint, Mode: "password", Password: "NEVER-PERSIST-THIS"}
	result, err := s.Enroll(ctx, c.ID, req)
	if err != nil {
		t.Fatal(err)
	}
	if result.State != "failed" {
		t.Fatalf("unreachable bootstrap = %s", result.State)
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = s.store.db.Get(&stored, `SELECT data FROM ssh_enrollments`); err != nil {
		t.Fatal(err)
	}
	if containsSecret(stored, string(data), req.Password) {
		t.Fatal("persisted enrollment password")
	}
	req.Password = ""
	retry, err := s.Enroll(ctx, c.ID, req)
	if err != nil || retry != result {
		t.Fatalf("retry did not return recorded outcome: %v", err)
	}
	req.Fingerprint = "different"
	if _, err = s.Enroll(ctx, c.ID, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("changed request accepted: %v", err)
	}
}
func containsSecret(stored, encoded, secret string) bool {
	return strings.Contains(stored, secret) || strings.Contains(encoded, secret)
}

func TestInterruptedEnrollmentRecoversWithoutCredentialReplay(t *testing.T) {
	s, ctx := identityService(t)
	testEnrollmentRecovery(t, s, ctx)
}

func testEnrollmentRecovery(t *testing.T, s *Service, ctx context.Context) {
	t.Helper()
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: "key", Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.store.SaveConnection(ctx, Connection{ID: uuid.NewString(), Host: "host", Port: 22, User: "user", IdentityID: identity.ID}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error { c.Fingerprint = "pin"; return putConnection(ctx, tx, c) }); err != nil {
		t.Fatal(err)
	}
	route, err := s.readRoute(ctx, c.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	req := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: identity.Fingerprint, Mode: "password", Password: "one-use"}
	_, _, _, started, err := s.startEnrollment(ctx, "admin", c.ID, req)
	if err != nil || !started {
		t.Fatalf("start: %v", err)
	}
	if err = s.RecoverEnrollments(ctx); err != nil {
		t.Fatal(err)
	}
	req.Password = ""
	result, err := s.Enroll(ctx, c.ID, req)
	if err != nil || result.State != "outcome_unknown" {
		t.Fatalf("recovery = %+v %v", result, err)
	}
	var refs int
	if err = s.store.db.Get(&refs, `SELECT COUNT(*) FROM ssh_connection_refs WHERE owner_type='enrollment'`); err != nil || refs != 0 {
		t.Fatal("recovery retained operation lock")
	}
}

func TestRouteShowsLatestEnrollmentAfterReload(t *testing.T) {
	s, ctx := identityService(t)
	c, err := s.store.SaveConnection(ctx, Connection{ID: "target"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct{ id, state, created string }{{"old", "failed", "2026-09-20T00:00:00Z"}, {"new", "outcome_unknown", "2026-09-24T00:00:00Z"}} {
		data := `{"request_id":"` + entry.id + `","connection_id":"target","state":"` + entry.state + `","created_at":"` + entry.created + `"}`
		if _, err = s.store.db.Exec(`INSERT INTO ssh_enrollments(actor,request_id,connection_id,digest,data) VALUES ('admin',?,'target','digest',?)`, entry.id, data); err != nil {
			t.Fatal(err)
		}
	}
	route, err := s.GetConnection(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if route.LatestEnrollment == nil || route.LatestEnrollment.RequestID != "new" || route.LatestEnrollment.State != "outcome_unknown" {
		t.Fatalf("latest outcome unavailable: %+v", route)
	}
}

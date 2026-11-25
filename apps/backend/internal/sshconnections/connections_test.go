package sshconnections

import (
	"errors"
	"github.com/google/uuid"
	"testing"
)

func TestConnectionCreateRetryAndAudit(t *testing.T) {
	s, ctx := identityService(t)
	req := SaveConnectionRequest{RequestID: uuid.NewString(), Connection: Connection{Name: "draft", Host: "host", Port: 22, User: "user"}}
	first, err := s.SaveConnection(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.SaveConnection(ctx, req)
	if err != nil || first.Destination.ID != second.Destination.ID {
		t.Fatalf("retry created a new connection: %v", err)
	}
	req.Connection.Host = "different"
	if _, err = s.SaveConnection(ctx, req); !errors.Is(err, ErrConflict) {
		t.Fatalf("request reuse = %v", err)
	}
	req = SaveConnectionRequest{Connection: first.Destination, ExpectedVersion: first.Destination.Version}
	req.Connection.Name = "new name"
	updated, err := s.SaveConnection(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Stamp != first.Stamp {
		t.Fatal("label changed route")
	}
	if _, err = s.store.db.Exec(`DROP TABLE ssh_connection_audit`); err != nil {
		t.Fatal(err)
	}
	req.Connection = updated.Destination
	req.Connection.Host = "another"
	req.ExpectedVersion = updated.Destination.Version
	if _, err = s.SaveConnection(ctx, req); err == nil {
		t.Fatal("unaudited mutation committed")
	}
	saved, err := s.store.GetConnection(ctx, first.Destination.ID)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Host != first.Destination.Host {
		t.Fatal("failed audit left metadata changed")
	}
}

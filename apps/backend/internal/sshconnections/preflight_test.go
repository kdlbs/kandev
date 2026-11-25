package sshconnections

import (
	"github.com/google/uuid"
	"path/filepath"
	"testing"
)

func TestPreflightNamesLocalPrerequisitesWithoutRemoteAccess(t *testing.T) {
	s, ctx := identityService(t)
	route, err := s.SaveConnection(ctx, SaveConnectionRequest{RequestID: uuid.NewString(), Connection: Connection{Name: "draft", Host: "invalid.example", Port: 22, User: "user"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.Preflight(ctx, route.Destination.ID, route.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, d := range result.Checks {
		if d.Hop != "destination" {
			t.Fatal("incorrect hop")
		}
		codes[d.Code] = true
	}
	if !codes["ssh_identity_required"] || !codes["ssh_trust_required"] || result.Ready {
		t.Fatalf("preflight: %+v", result)
	}
}

func TestPreflightDistinguishesUnavailableAgentAndManagedStorage(t *testing.T) {
	s, ctx := identityService(t)
	for _, kind := range []string{"agent", "file", "generate"} {
		req := CreateIdentityRequest{RequestID: uuid.NewString(), Label: kind, Kind: kind}
		if kind != "generate" {
			req.Reference = filepath.Join(t.TempDir(), "missing")
		}
		identity, err := s.CreateIdentity(ctx, req)
		if err != nil {
			t.Fatal(err)
		}
		c, err := s.store.SaveConnection(ctx, Connection{ID: kind, Host: "host", Port: 22, User: "user", IdentityID: identity.ID}, 0)
		if err != nil {
			t.Fatal(err)
		}
		if kind == "generate" {
			if _, err = s.store.db.Exec(`DELETE FROM secrets WHERE id=?`, "ssh-identity:"+identity.ID); err != nil {
				t.Fatal(err)
			}
		}
		route, err := s.GetConnection(ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		result, err := s.Preflight(ctx, c.ID, route.Stamp)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"agent": "ssh_agent_unavailable", "file": "ssh_key_file_unavailable", "generate": "ssh_managed_key_unavailable"}[kind]
		found := false
		for _, check := range result.Checks {
			if check.Code == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s diagnostic missing: %+v", kind, result.Checks)
		}
	}
}

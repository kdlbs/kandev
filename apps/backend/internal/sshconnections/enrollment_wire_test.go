package sshconnections

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func newWireIdentity(t *testing.T, s *Service, ctx context.Context, label string) Identity {
	t.Helper()
	identity, err := s.CreateIdentity(ctx, CreateIdentityRequest{RequestID: uuid.NewString(), Label: label, Kind: "generate"})
	if err != nil {
		t.Fatal(err)
	}
	return identity
}
func trustWireRoute(t *testing.T, s *Service, ctx context.Context, host *wireHost, identity Identity, bastionID string) Route {
	t.Helper()
	address := "127.0.0.1"
	if bastionID != "" {
		address = "fixture-target.invalid"
	}
	route, err := s.SaveConnection(ctx, SaveConnectionRequest{RequestID: uuid.NewString(), Connection: Connection{Name: "wire fixture", Host: address, Port: host.port(), User: "fixture", IdentityID: identity.ID, BastionID: bastionID}})
	if err != nil {
		t.Fatal(err)
	}
	observation, err := s.Observe(ctx, route.Destination.ID, route.Stamp)
	if err != nil {
		t.Fatal(err)
	}
	if observation.Fingerprint != host.fingerprint {
		t.Fatal("observation did not match the independently known fixture key")
	}
	route, err = s.Trust(ctx, route.Destination.ID, route.Stamp, observation.Receipt, host.fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	return route
}

func TestEnrollmentWireUsesSelectedIdentityAndRoute(t *testing.T) {
	for _, routeKind := range []string{"direct", "bastion"} {
		for _, mode := range []string{"password", "identity"} {
			t.Run(routeKind+"/"+mode, func(t *testing.T) {
				s, ctx := identityService(t)
				managed := newWireIdentity(t, s, ctx, "destination")
				bootstrap := newWireIdentity(t, s, ctx, "bootstrap")
				existing := "no-pty " + strings.TrimSpace(bootstrap.PublicKey) + "\n# preserve without newline"
				target := newWireHost(t, existing)
				var bastion *wireHost
				var bastionKeys, bastionID string
				if routeKind == "bastion" {
					gateway := newWireIdentity(t, s, ctx, "gateway")
					bastionKeys = gateway.PublicKey
					bastion = newWireHost(t, bastionKeys)
					bastion.setDestination(target)
					ready := trustWireRoute(t, s, ctx, bastion, gateway, "")
					ready, err := s.TestConnection(ctx, ready.Destination.ID, ready.Stamp)
					if err != nil {
						t.Fatal(err)
					}
					bastionID = ready.Destination.ID
				}
				route := trustWireRoute(t, s, ctx, target, managed, bastionID)
				if target.keyCalls.Load() != 0 || target.passwordCalls.Load() != 0 {
					t.Fatal("host observation authenticated to destination")
				}
				request := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: managed.Fingerprint, Mode: mode}
				if mode == "password" {
					request.Password = "fixture-bootstrap"
				} else {
					request.BootstrapIdentityID = bootstrap.ID
				}
				result, err := s.Enroll(ctx, route.Destination.ID, request)
				if err != nil {
					t.Fatal(err)
				}
				if result.State != "verified" {
					t.Fatalf("enrollment state = %s, code = %s", result.State, result.ErrorCode)
				}
				want := existing + "\n" + managed.PublicKey
				if target.keys(t) != want {
					t.Fatal("enrollment did not preserve existing entries and append exactly the selected public key")
				}
				if target.executions.Load() != 1 || target.lastKey() != managed.Fingerprint {
					t.Fatal("installation or fresh managed authentication missing")
				}
				wantPasswords := int32(0)
				if mode == "password" {
					wantPasswords = 1
				}
				if target.passwordCalls.Load() != wantPasswords {
					t.Fatal("bootstrap password was reused for verification")
				}
				if bastion != nil && (bastion.passwordCalls.Load() != 0 || bastion.executions.Load() != 0 || bastion.keys(t) != bastionKeys || bastion.forwards.Load() < 3) {
					t.Fatal("enrollment did not preserve the separate bastion boundary")
				}
				var saved string
				if err = s.store.db.Get(&saved, `SELECT data FROM ssh_enrollments WHERE request_id=?`, request.RequestID); err != nil {
					t.Fatal(err)
				}
				if strings.Contains(saved, "fixture-bootstrap") {
					t.Fatal("persisted bootstrap password")
				}
				beforeKeys, beforePasswords := target.keyCalls.Load(), target.passwordCalls.Load()
				request.Password = ""
				again, err := s.Enroll(ctx, route.Destination.ID, request)
				if err != nil || again != result {
					t.Fatalf("idempotent result changed: %v", err)
				}
				if target.executions.Load() != 1 || target.keyCalls.Load() != beforeKeys || target.passwordCalls.Load() != beforePasswords {
					t.Fatal("duplicate enrollment performed remote work")
				}
			})
		}
	}
}

func TestEnrollmentWireRetainsPartialOutcomes(t *testing.T) {
	for _, state := range []string{"installed_unverified", "outcome_unknown"} {
		t.Run(state, func(t *testing.T) {
			s, ctx := identityService(t)
			identity := newWireIdentity(t, s, ctx, "destination")
			target := newWireHost(t, "# original\n")
			target.rejectKey.Store(state == "installed_unverified")
			target.disconnectAfterInstall.Store(state == "outcome_unknown")
			route := trustWireRoute(t, s, ctx, target, identity, "")
			request := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: identity.Fingerprint, Mode: "password", Password: "fixture-bootstrap"}
			result, err := s.Enroll(ctx, route.Destination.ID, request)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != state {
				t.Fatalf("state = %s, want %s", result.State, state)
			}
			if state == "installed_unverified" && result.ErrorCode != "ssh_authentication_rejected" {
				t.Fatalf("verification diagnostic = %s", result.ErrorCode)
			}
			if !strings.Contains(target.keys(t), identity.PublicKey) {
				t.Fatal("partial outcome lost the installed public key")
			}
			current, err := s.GetConnection(ctx, route.Destination.ID)
			if err != nil {
				t.Fatal(err)
			}
			if current.Destination.VerifiedRoute != "" {
				t.Fatal("partial enrollment advertised readiness")
			}
			if err = s.RecoverEnrollments(ctx); err != nil {
				t.Fatal(err)
			}
			request.Password = ""
			if _, err = s.Enroll(ctx, route.Destination.ID, request); err != nil {
				t.Fatal(err)
			}
			if target.executions.Load() != 1 || target.passwordCalls.Load() != 1 {
				t.Fatal("recovery replayed enrollment")
			}
		})
	}
}

func TestEnrollmentWireChangedHostKeyBlocksBootstrap(t *testing.T) {
	for _, changedHop := range []string{"direct_destination", "jump_destination", "bastion"} {
		t.Run(changedHop, func(t *testing.T) {
			s, ctx := identityService(t)
			identity := newWireIdentity(t, s, ctx, "destination")
			target := newWireHost(t, "# unchanged\n")
			changed := target
			var bastionID string
			if changedHop != "direct_destination" {
				gateway := newWireIdentity(t, s, ctx, "gateway")
				bastion := newWireHost(t, gateway.PublicKey)
				bastion.setDestination(target)
				route := trustWireRoute(t, s, ctx, bastion, gateway, "")
				if _, err := s.TestConnection(ctx, route.Destination.ID, route.Stamp); err != nil {
					t.Fatal(err)
				}
				bastionID = route.Destination.ID
				if changedHop == "bastion" {
					changed = bastion
				}
			}
			route := trustWireRoute(t, s, ctx, target, identity, bastionID)
			beforeKeys, beforePasswords := changed.keyCalls.Load(), changed.passwordCalls.Load()
			changed.changeHostKey(t)
			request := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: identity.Fingerprint, Mode: "password", Password: "fixture-bootstrap"}
			result, err := s.Enroll(ctx, route.Destination.ID, request)
			if err != nil {
				t.Fatal(err)
			}
			if result.State != "failed" || result.ErrorCode != "ssh_host_key_changed" {
				t.Fatalf("changed host outcome: %s/%s", result.State, result.ErrorCode)
			}
			wantHop := "destination"
			if changedHop == "bastion" {
				wantHop = "bastion"
			}
			encoded, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var response map[string]any
			if err = json.Unmarshal(encoded, &response); err != nil {
				t.Fatal(err)
			}
			if response["hop"] != wantHop {
				t.Fatalf("enrollment failure hop = %v, want %s", response["hop"], wantHop)
			}
			if changed.keyCalls.Load() != beforeKeys || changed.passwordCalls.Load() != beforePasswords || target.passwordCalls.Load() != 0 || target.executions.Load() != 0 {
				t.Fatal("changed host received credentials or enrollment")
			}
			if target.keys(t) != "# unchanged\n" {
				t.Fatal("changed host modified authorization")
			}
		})
	}
}

func TestEnrollmentWireAuditFailureBoundsRemoteEffects(t *testing.T) {
	for _, failAt := range []string{"started", "verified"} {
		t.Run(failAt, func(t *testing.T) {
			s, ctx := identityService(t)
			identity := newWireIdentity(t, s, ctx, "destination")
			target := newWireHost(t, "")
			route := trustWireRoute(t, s, ctx, target, identity, "")
			_, err := s.store.db.Exec(`CREATE TRIGGER fail_enrollment_audit BEFORE INSERT ON ssh_connection_audit WHEN NEW.operation='connection.enroll' AND NEW.result='` + failAt + `' BEGIN SELECT RAISE(ABORT,'fixture audit failure'); END`)
			if err != nil {
				t.Fatal(err)
			}
			request := EnrollmentRequest{RequestID: uuid.NewString(), Stamp: route.Stamp, Fingerprint: identity.Fingerprint, Mode: "password", Password: "fixture-bootstrap"}
			if _, err = s.Enroll(ctx, route.Destination.ID, request); err == nil {
				t.Fatal("enrollment succeeded without its audit record")
			}
			var records int
			if err = s.store.db.Get(&records, `SELECT COUNT(*) FROM ssh_enrollments`); err != nil {
				t.Fatal(err)
			}
			if failAt == "started" {
				if records != 0 || target.passwordCalls.Load() != 0 || target.executions.Load() != 0 || target.keys(t) != "" {
					t.Fatal("initial audit failure admitted remote work or persisted a partial start")
				}
				return
			}
			if records != 1 || target.executions.Load() != 1 || !strings.Contains(target.keys(t), identity.PublicKey) {
				t.Fatal("final audit failure lost evidence of remote effects")
			}
			if _, err = s.store.db.Exec(`DROP TRIGGER fail_enrollment_audit`); err != nil {
				t.Fatal(err)
			}
			if err = s.RecoverEnrollments(ctx); err != nil {
				t.Fatal(err)
			}
			request.Password = ""
			result, err := s.Enroll(ctx, route.Destination.ID, request)
			if err != nil || result.State != "outcome_unknown" {
				t.Fatalf("final audit failure was not recovered as unknown: %s, %v", result.State, err)
			}
			if target.executions.Load() != 1 || target.passwordCalls.Load() != 1 {
				t.Fatal("audit recovery replayed bootstrap access")
			}
		})
	}
}

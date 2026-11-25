package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func TestMigrationRuntimeVerifiesBeforeAdmission(t *testing.T) {
	s, ctx := identityService(t)
	identity := newWireIdentity(t, s, ctx, "destination")
	host := newWireHost(t, identity.PublicKey)
	host.addECDSAHostKey(t)
	route, err := s.SaveConnection(ctx, SaveConnectionRequest{
		RequestID: uuid.NewString(), Connection: Connection{Name: "wire fixture", Host: "127.0.0.1", Port: host.port(), User: "fixture", IdentityID: identity.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinMigrationTestDestination(t, s, ctx, route, host.fingerprint)
	installPendingMigration(t, s, ctx, route, false)
	assertPendingMigrationEditPolicy(t, s, ctx, route)
	if err := s.ValidateSession(ctx, "session"); err != nil {
		t.Fatalf("pending local binding rejected: %v", err)
	}
	if _, _, _, err := s.OpenExecutor(ctx, "executor"); !errors.Is(err, ErrInvalidRoute) || host.keyCalls.Load() != 0 {
		t.Fatalf("background cleanup must leave the bound route pending: %v", err)
	}
	if _, _, _, err := s.OpenConnectionForManagement(context.Background(), route.Destination.ID, route.Stamp); !errors.Is(err, ErrForbidden) || host.keyCalls.Load() != 0 {
		t.Fatalf("management-denied caller probed the route: %v", err)
	}
	_, _, cleanup, err := s.OpenExecutorProbe(ctx, "executor")
	if err != nil {
		t.Fatalf("background reachability could not probe the pinned route: %v", err)
	}
	cleanup()
	_, _, cleanup, err = s.OpenConnectionForManagement(ctx, route.Destination.ID, route.Stamp)
	if err != nil {
		t.Fatalf("management could not test its pending route: %v", err)
	}
	cleanup()
	current, err := s.readRoute(ctx, route.Destination.ID, "")
	if err != nil || current.Destination.VerifiedRoute != "" {
		t.Fatalf("management probe promoted the route: %v", err)
	}
	platformFailure := errors.New("unsupported fixture platform")
	s.SetMigrationPlatformCheck(func(context.Context, *ssh.Client) error { return platformFailure })
	if _, err := s.AdmitRuntime(ctx, "session", route.Destination.ID, ""); !errors.Is(err, platformFailure) {
		t.Fatalf("unsupported platform was admitted: %v", err)
	}
	current, err = s.readRoute(ctx, route.Destination.ID, "")
	if err != nil || current.Destination.VerifiedRoute != "" {
		t.Fatalf("failed platform check promoted the route: %v", err)
	}
	snapshot := admitConcurrentMigrationSessions(t, s, ctx)
	if snapshot.Destination.VerifiedRoute != snapshot.Stamp || host.lastKey() != identity.Fingerprint {
		t.Fatalf("first use did not verify with its saved identity: %+v", snapshot)
	}
}

func TestMigrationObservePrefersSavedPinAndReportsActualReplacement(t *testing.T) {
	s, ctx := identityService(t)
	identity := newWireIdentity(t, s, ctx, "destination")
	host := newWireHost(t, identity.PublicKey)
	host.addECDSAHostKey(t)
	route, err := s.SaveConnection(ctx, SaveConnectionRequest{
		RequestID: uuid.NewString(), Connection: Connection{Name: "wire fixture", Host: "127.0.0.1", Port: host.port(), User: "fixture", IdentityID: identity.ID},
	})
	if err != nil {
		t.Fatal(err)
	}
	pinMigrationTestDestination(t, s, ctx, route, host.fingerprint)
	observed, err := s.Observe(ctx, route.Destination.ID, route.Stamp)
	if err != nil || observed.Fingerprint != host.fingerprint {
		t.Fatalf("inspect reported a false host key change: %+v %v", observed, err)
	}
	replacement := host.changeHostKey(t)
	observed, err = s.Observe(ctx, route.Destination.ID, route.Stamp)
	if err != nil || observed.Fingerprint != replacement {
		t.Fatalf("inspect did not return the actual replacement key: %+v %v", observed, err)
	}
	if host.keyCalls.Load() != 0 || host.passwordCalls.Load() != 0 {
		t.Fatal("inspect authenticated to the destination")
	}
}

func assertPendingMigrationEditPolicy(t *testing.T, s *Service, ctx context.Context, route Route) {
	t.Helper()
	config := map[string]string{
		ConnectionConfigKey: route.Destination.ID, "ssh_connection_stamp": route.Stamp, "ssh_shell": "bash",
	}
	bind := func(kind string) error {
		return s.store.write(ctx, func(tx *sqlx.Tx) error {
			return s.BindExecutorIn(ctx, tx, "executor", kind, config)
		})
	}
	if err := bind("local"); !errors.Is(err, ErrInvalidRoute) {
		t.Fatalf("non-SSH executor retained the imported binding: %v", err)
	}
	if err := bind("ssh"); err != nil {
		t.Fatalf("unrelated edit could not retain its import: %v", err)
	}
}

func installPendingMigration(t *testing.T, s *Service, ctx context.Context, route Route, knownHosts bool) {
	t.Helper()
	for _, q := range []string{
		`CREATE TABLE executors(id TEXT PRIMARY KEY,type TEXT,config TEXT,updated_at TIMESTAMP,deleted_at TIMESTAMP)`,
		`CREATE TABLE task_sessions(id TEXT PRIMARY KEY,executor_id TEXT)`,
		`CREATE TABLE executors_running(session_id TEXT PRIMARY KEY,runtime TEXT)`,
		`INSERT INTO task_sessions VALUES ('session','executor')`,
	} {
		if _, err := s.store.db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	config := map[string]string{ConnectionConfigKey: route.Destination.ID, "ssh_connection_stamp": route.Stamp}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.store.db.Exec(`INSERT INTO executors(id,type,config) VALUES ('executor','ssh',?)`, string(data)); err != nil {
		t.Fatal(err)
	}
	if err = s.store.write(ctx, func(tx *sqlx.Tx) error {
		if _, err := tx.Exec(`INSERT INTO ssh_connection_refs VALUES ('executor','executor',?,?)`, route.Destination.ID, route.Stamp); err != nil {
			return err
		}
		return putMigration(ctx, tx, "executor", sshMigration{ConnectionID: route.Destination.ID, RouteStamp: route.Stamp, SourceDigest: "original", BastionKnownHosts: knownHosts})
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationRuntimeRejectsConcurrentRouteEdit(t *testing.T) {
	s, ctx := identityService(t)
	identity := newWireIdentity(t, s, ctx, "destination")
	host := newWireHost(t, identity.PublicKey)
	route := trustWireRoute(t, s, ctx, host, identity, "")
	installPendingMigration(t, s, ctx, route, false)
	started, changed := make(chan struct{}), make(chan struct{})
	s.SetMigrationPlatformCheck(func(ctx context.Context, _ *ssh.Client) error {
		close(started)
		select {
		case <-changed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := s.AdmitSavedRuntime(ctx, "session")
		result <- err
	}()
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("first use did not reach platform validation")
	}
	c := route.Destination
	c.User = "replacement"
	if _, err := s.store.SaveConnection(ctx, c, c.Version); err != nil {
		t.Fatal(err)
	}
	close(changed)
	if err := <-result; !errors.Is(err, ErrInvalidRoute) && !errors.Is(err, ErrConflict) {
		t.Fatalf("stale platform result accepted: %v", err)
	}
	current, err := s.readRoute(ctx, c.ID, "")
	if err != nil || current.Destination.VerifiedRoute != "" {
		t.Fatalf("replacement route promoted: %+v %v", current, err)
	}
}

func TestMigrationRuntimeUsesStrictKnownHostsForBastion(t *testing.T) {
	s, ctx := identityService(t)
	gateway := newWireIdentity(t, s, ctx, "gateway")
	bastion := newWireHost(t, gateway.PublicKey)
	bastion.addECDSAHostKey(t)
	identity := newWireIdentity(t, s, ctx, "destination")
	destination := newWireHost(t, identity.PublicKey)
	destination.addECDSAHostKey(t)
	bastion.setDestination(destination)
	bastionConnection, err := s.store.SaveConnection(ctx, Connection{
		ID: "gateway", Host: "127.0.0.1", Port: bastion.port(), User: "fixture", IdentityID: gateway.ID,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.store.SaveConnection(ctx, Connection{
		ID: "target", Host: "fixture-target.invalid", Port: destination.port(), User: "fixture",
		IdentityID: identity.ID, BastionID: bastionConnection.ID,
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	route, err := s.readRoute(ctx, "target", "")
	if err != nil {
		t.Fatal(err)
	}
	pinMigrationTestDestination(t, s, ctx, route, destination.fingerprint)
	installPendingMigration(t, s, ctx, route, true)
	s.SetMigrationPlatformCheck(func(context.Context, *ssh.Client) error { return nil })
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, ".ssh", "known_hosts")
	wrong, _, _, _, err := ssh.ParseAuthorizedKey([]byte(identity.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	addr := net.JoinHostPort("127.0.0.1", strconv.Itoa(bastion.port()))
	name := knownhosts.HashHostname(knownhosts.Normalize(addr))
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{name}, wrong)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	_, err = s.AdmitSavedRuntime(ctx, "session")
	var failure *ProbeFailure
	if !errors.As(err, &failure) || failure.Hop != hopBastion || failure.Code != "ssh_host_key_changed" ||
		bastion.keyCalls.Load() != 0 || destination.keyCalls.Load() != 0 {
		t.Fatalf("mismatched bastion allowed authentication: %v", err)
	}
	if err := os.WriteFile(path, []byte(knownhosts.Line([]string{name}, bastion.publicHostKey)+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot := admitConcurrentMigrationSessions(t, s, ctx)
	if snapshot.Stamp == route.Stamp || snapshot.Bastion.Fingerprint != bastion.fingerprint ||
		snapshot.Destination.VerifiedRoute != snapshot.Stamp || bastion.lastKey() != gateway.Fingerprint ||
		destination.lastKey() != identity.Fingerprint {
		t.Fatalf("verified route did not retain both original hops: %+v", snapshot)
	}
}

func admitConcurrentMigrationSessions(t *testing.T, s *Service, ctx context.Context) RuntimeSnapshot {
	t.Helper()
	if _, err := s.store.db.Exec(`INSERT INTO task_sessions VALUES ('second-session','executor')`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	ready, release := make(chan struct{}, 2), make(chan struct{})
	s.SetMigrationPlatformCheck(func(ctx context.Context, _ *ssh.Client) error {
		ready <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	type result struct {
		snapshot RuntimeSnapshot
		err      error
	}
	results := make(chan result, 2)
	for _, sessionID := range []string{"session", "second-session"} {
		go func() {
			snapshot, err := s.AdmitSavedRuntime(ctx, sessionID)
			results <- result{snapshot, err}
		}()
	}
	for range 2 {
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("both admissions must reach the platform check before either promotes")
		}
	}
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("identical first uses did not converge: %v, %v", first.err, second.err)
	}
	if first.snapshot.Stamp != second.snapshot.Stamp || first.snapshot.SessionID == second.snapshot.SessionID {
		t.Fatalf("competing first uses produced the wrong bindings: %+v, %+v", first.snapshot, second.snapshot)
	}
	return first.snapshot
}

func pinMigrationTestDestination(t *testing.T, s *Service, ctx context.Context, route Route, fingerprint string) {
	t.Helper()
	if err := s.store.write(ctx, func(tx *sqlx.Tx) error {
		c := route.Destination
		c.Fingerprint = fingerprint
		return putConnection(ctx, tx, c)
	}); err != nil {
		t.Fatal(err)
	}
}

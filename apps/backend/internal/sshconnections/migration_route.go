package sshconnections

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/kandev/kandev/internal/sshtransport"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type migrationPlan struct {
	destination       migrationHop
	bastion           *migrationHop
	reason            string
	bastionKnownHosts bool
}

type migrationHop struct {
	connection Connection
	identity   Identity
}

func resolveMigration(migration sshMigration) migrationPlan {
	source := migration.Source
	plan := migrationPlan{reason: migration.Reason, destination: migrationHop{connection: Connection{
		Alias: source["ssh_host_alias"], Host: source[sshHostConfigKey], User: source["ssh_user"], Fingerprint: source["ssh_host_fingerprint"],
	}}}
	if migration.Reason != "" && migration.Reason != migrationAlias && migration.Reason != migrationIdentity {
		return plan
	}
	port, err := strconv.Atoi(source["ssh_port"])
	if source["ssh_port"] != "" && (err != nil || port < 1 || port > 65535) {
		plan.reason = migrationInvalid
		return plan
	}
	target, bastion, resolveErr := sshtransport.ResolveMigrationTargets(sshtransport.SSHConnConfig{
		HostAlias: source["ssh_host_alias"], Host: source[sshHostConfigKey], Port: port, User: source["ssh_user"],
		IdentitySource: sshtransport.SSHIdentitySource(source["ssh_identity_source"]), IdentityFile: source["ssh_identity_file"],
		ProxyJump: source["ssh_proxy_jump"], PinnedFingerprint: source["ssh_host_fingerprint"],
	})
	plan.destination = migrationHopFor(target)
	plan.destination.connection.Alias = source["ssh_host_alias"]
	plan.reason = migrationResolveReason(resolveErr)
	if bastion != nil {
		plan.captureBastion(bastion)
	}
	if plan.reason == "" && plan.identityMissing() {
		plan.reason = migrationIdentity
	}
	return plan
}

func (p *migrationPlan) captureBastion(target *sshtransport.SSHTarget) {
	hop := migrationHopFor(target)
	if target.OriginalHost != target.Host {
		hop.connection.Alias = target.OriginalHost
	}
	if p.reason == "" {
		hop.connection.Fingerprint, hop.connection.PublicKey, p.bastionKnownHosts = migrationBastionTrust(target)
		hop.connection.KnownHostsPending = p.bastionKnownHosts
	} else {
		hop.connection.RepairReason = p.reason
	}
	p.bastion = &hop
}

func (p migrationPlan) identityMissing() bool {
	return p.destination.identity.Kind == "" || p.bastion != nil && p.bastion.identity.Kind == ""
}

func migrationResolveReason(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, sshtransport.ErrMigrationAliasMissing):
		return migrationAlias
	case errors.Is(err, sshtransport.ErrMigrationIdentity):
		return migrationIdentity
	default:
		return migrationRoute
	}
}

func migrationHopFor(target *sshtransport.SSHTarget) migrationHop {
	c := Connection{Host: target.Host, Port: target.Port, User: target.User, Fingerprint: target.PinnedFingerprint}
	kind := string(target.IdentitySource)
	reference, reason := migrationIdentityReference(target)
	if reason != "" {
		c.RepairReason = reason
	}
	if kind != identityAgent && kind != identityFile || reason == migrationIdentity {
		return migrationHop{connection: c}
	}
	return migrationHop{connection: c, identity: Identity{Kind: kind, Reference: reference}}
}

func migrationIdentityReference(target *sshtransport.SSHTarget) (string, string) {
	switch target.IdentitySource {
	case sshtransport.SSHIdentitySourceFile:
		if target.IdentityFile == "" {
			return "", migrationIdentity
		}
		path, err := filepath.Abs(target.IdentityFile)
		if err != nil {
			return "", migrationIdentity
		}
		if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() {
			return path, codeKeyFileUnavailable
		}
		return path, ""
	case sshtransport.SSHIdentitySourceAgent:
		socket := target.IdentityAgent
		if strings.EqualFold(socket, "none") {
			return "", migrationIdentity
		}
		if socket != "" && !filepath.IsAbs(socket) {
			var err error
			socket, err = filepath.Abs(socket)
			if err != nil {
				return "", migrationIdentity
			}
		}
		probe := socket
		if probe == "" {
			probe = os.Getenv("SSH_AUTH_SOCK")
		}
		if fi, err := os.Stat(probe); err != nil || fi.Mode()&os.ModeSocket == 0 {
			return socket, codeAgentUnavailable
		}
		return socket, ""
	default:
		return "", migrationIdentity
	}
}

func putMigrationRoute(ctx context.Context, tx *sqlx.Tx, id string, plan migrationPlan) (Route, error) {
	destination := plan.destination
	destination.connection.ImportedPending = true
	if plan.reason != "" && (plan.bastion == nil || plan.bastion.connection.RepairReason != plan.reason) {
		destination.connection.RepairReason = plan.reason
	}
	if plan.bastion != nil {
		bastion, err := putMigrationHop(ctx, tx, uuid.NewString(), *plan.bastion)
		if err != nil {
			return Route{}, err
		}
		destination.connection.BastionID = bastion.ID
	}
	if _, err := putMigrationHop(ctx, tx, id, destination); err != nil {
		return Route{}, err
	}
	return routeIn(ctx, tx, id)
}

func putMigrationHop(ctx context.Context, tx *sqlx.Tx, id string, hop migrationHop) (Connection, error) {
	return saveMigrationHop(ctx, tx, Connection{ID: id}, hop)
}

func saveMigrationHop(ctx context.Context, tx *sqlx.Tx, old Connection, hop migrationHop) (Connection, error) {
	c := hop.connection
	c.ID, c.Name = old.ID, "SSH "+c.Host
	if c.Host == "" || len(c.Name) > 200 {
		c.Name = "SSH connection"
	}
	if old.Version > 0 {
		c.Name = old.Name
	}
	c.Version, c.Revision = old.Version+1, old.Revision+1
	c.IdentityID = old.IdentityID
	if c.IdentityID == "" && hop.identity.Kind != "" {
		i := hop.identity
		i.ID, i.Label, i.Generation, i.Version = uuid.NewString(), c.Name, uuid.NewString(), 1
		data, err := json.Marshal(i)
		if err != nil {
			return Connection{}, err
		}
		if _, err = tx.ExecContext(ctx, tx.Rebind(`INSERT INTO ssh_identities(id,data) VALUES (?,?)`), i.ID, string(data)); err != nil {
			return Connection{}, err
		}
		c.IdentityID = i.ID
	}
	return c, putConnection(ctx, tx, c)
}

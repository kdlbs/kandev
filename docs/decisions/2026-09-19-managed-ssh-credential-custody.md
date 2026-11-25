# ADR-2026-09-19-managed-ssh-credential-custody: Managed SSH credential custody

**Status:** accepted
**Date:** 2026-09-19
**Area:** backend

## Context

[Issue #3735](https://github.com/kdlbs/kandev/issues/3735) exposes a bootstrap
gap: an external agent or unencrypted file must exist before an SSH executor
can be created. Desktop and service processes need not inherit a shell's agent.
SSH access is also a prerequisite for proposed remote Docker execution.
On 2026-09-19 the maintainer explicitly selected unattended use,
admin-managed connections, and assisted enrollment. This accepts the product
boundary. The maintainer authorized the revised package implementation on
2026-09-22. See the [recovery record](../plans/ssh-connection-manager/recovery/recovery.md)
for the preserved approval evidence.

## Decision

Executors owns installation-wide reusable SSH connections and identities.
Administrators manage them through the existing org.config.manage authority;
users consume connections only through authorized executor profiles and task
operations. The connection fixes the endpoint, login, identity, and host trust.
The maintainer subsequently included single-hop ProxyJump in the first release.
A destination may reference one direct bastion connection, with its own login,
identity and explicit host trust. Both records participate in readiness,
mutation/deletion guards, and runtime recovery. No destination credential is
forwarded or silently used as the bastion credential; sharing one identity
requires explicitly selecting it for each hop. Chained jumps remain excluded.

Managed private keys are internal encrypted secrets, unavailable to user
secret reveal/export APIs and agent credential selection. They use the
existing installation protection key and remain usable after unattended
restarts. No new persistent ssh-agent or OS-keychain dependency is introduced.
The backend obtains an in-memory signer for a bounded SSH handshake, never a
private key path or an agent socket to forward into a workload.

Import may accept a passphrase transiently to decode a key. Stored protection
is Kandev's protection, not continuing enforcement of the original passphrase.
The UI states this before import. Backend-host compromise remains outside the
protection boundary; encrypted-at-rest does not mean protected from that host.

Enrollment is a separate administrator operation: observe and verify host
trust each relevant hop first, explicitly install the destination public key using one-time password or
existing identity access, and prove access with the new identity. Password
bootstrap does not become a persistent executor authentication mode.

## Consequences

Service/headless and desktop installs share one behavior. Connections can be
reused without copying pins and key material into profiles. Existing external
agent/file modes remain valid. Backend backups must preserve database and
protection key together. Lost key material requires restoring the matching
backup or explicitly replacing identities and enrolling again.

Shared executor use grants the remote account's workload privileges, including
Docker privileges where configured. It does not grant export of its SSH key.
Audit records distinguish management, host trust, enrollment, and authentication.
Deleting local credentials does not revoke a public key on the remote.

## Alternatives Considered

- **Interactive unlock after restart:** stronger separation from unattended
  host use, but rejects the maintainer's service/headless requirement.
- **Offer two custody modes now:** creates two recovery and scheduling contracts
  before one complete flow works; deferred unless requested later.
- **Depend on an inherited or Kandev-managed ssh-agent:** preserves the original
  process-environment friction or adds a second credential daemon lifecycle.
- **User/workspace-owned identities:** useful for stronger tenant isolation,
  but differs from the selected installation-admin scope and shared executors.
- **Public-key copy only:** remains a fallback but does not fulfill assisted
  enrollment. Bootstrap access is always required; Kandev cannot create trust
  or authorization on an inaccessible host.

## References

- [Requirements](../specs/executors/requirements/ssh-connection-manager.md)
- [System design](../specs/executors/system-design/ssh-connection-manager.md)
- [Existing secret ownership](../specs/workspaces/system-design/repository-secrets.md)

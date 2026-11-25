---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-SSH-MIGRATION-001
---

# SSH connection migration design

## Current code and scope

Earlier versions offered inline settings and an optional shared connection. The cutover moves saved inline settings into shared connections before the backend admits SSH work. The source executor ID identifies each import across restarts. The binder checks route readiness and revision. It does not by itself prove that the platform test passed. Keep [credential custody](../../../decisions/2026-09-19-managed-ssh-credential-custody.md) and the task/profile access boundary.

## Import and bind

The legacy card saves the destination fingerprint only after a successful test and explicit trust. Read `ssh_host_fingerprint` as existing trust, including when the server is offline. API-created rows may have incomplete data. A fingerprint alone is not a current login or platform-test result.

1. In the same version that removes inline writers and dialers, run a local, idempotent upgrade before admitting SSH consumers. Convert each SSH executor once, including executors whose hosts, agents or key files are temporarily unavailable. An already managed binding is a no-op. Separate executors with the same hostname keep separate identities and connection records. Non-SSH executors continue independently.
2. Reuse the old backend account's target/alias and single-bastion resolution. Preserve user, port, file reference and agent selector. Preserve the distinction between an explicit socket and the backend's default agent; resolve the latter at use time. Offline import must not open private keys, contact hosts or enroll credentials. If resolution needs unavailable local configuration, preserve the original selector as inert repair metadata and retry resolution when available. Never drop a jump or silently choose another credential.
3. Copy the destination pin exactly. For a bastion, the old callback consulted the backend user's known_hosts but accepted unrecorded keys. Use the strict positive match for the original host and port when trustworthy local evidence exists. For hashed entries or multiple allowed host keys, validate the actually presented key under strict known_hosts semantics at first use, **before authentication to that hop**, then persist that exact pin against the unchanged route. Honor revocation and mismatch errors. Unsupported trust policies require review. An empty match, unreadable file or historical successful login supplies no trust. Show only the missing hop in Access.
4. Under the executor/connection graph lock, compare the original config digest and commit the identities, connection graph, executor ID/stamp binding, dependency references and removal of inline fields together. Keep profile rows, non-transport fields and ACLs intact. An incomplete source still gets a managed repair record linked to the executor, with a reason and only necessary non-secret source references. Conflicting mixed records fail closed and retain evidence. Use a unique source executor ID plus transactional version checks in SQLite and PostgreSQL. Restarts and concurrent initializers converge. A restored backup or skipped-version upgrade runs the same importer.
5. Separate **migrated binding** from **verified login**. Add a server-owned import-only binding operation, inaccessible to HTTP/WS/MCP callers. Do not set VerifiedRoute during conversion or relax the normal bind/edit checks. On first authorized use of a complete imported binding, resolve the saved identity and check every hop's pin before authentication. Automatically execute the fresh checks required by that operation, including the platform check before a new workload. Record successful evidence and promote the imported binding only for the matching credential generation, source and route revisions. Release the write transaction before network I/O and compare revisions again before use. Competing first uses share or safely repeat checks; failures cannot launch work. A background reachability probe or a user without launch rights cannot promote a binding to workload readiness. Offline or temporarily unavailable routes retry on later use, with no migration button.
6. For changed or missing evidence use the existing shared editor and explicit host trust or route replacement. Never automatically trust a new key, substitute a new route for an old session, or report an executor as verified on the basis of the import. Record import as a system upgrade, separate from actor-attributed trust and later use.

## Settings and API

The normal SSH connection list, executor editor and Access view are the only product surfaces. Fresh and upgraded installations use the same pages. A migrated executor opens its existing URL with its shared connection already selected. A repair link for an incomplete connection leads to the affected field or hop in Access. On phone use the normal full-page detail and wrapping fingerprints. Preserve focus and localize new messages. No batch verification screen, operator startup step or migration wizard.

New writes accept only shared bindings. Reject inline or mixed transport fields at all SSH writer and test entry points. An unrelated profile edit preserves its binding and repair state. Preserve the existing management and task authorization boundaries; only the startup importer may create an unverified historical binding. Disable optional adoption APIs as well as the old editor.

## Launch and retained sessions

Check migration status at executor admission, including queued and scheduled tasks at dispatch time. All new launches use the managed binding, including the automatic first-use path. A repair record cannot authorize a dial through archived fields. Inventory consumers that dial for reachability, executor tests, cleanup, remote Docker and SSH runtime creation. Reuse the managed opener for consumers of an SSH executor. Independent SSH features such as passthrough retain their own contracts.

Treat retained runtime inventory as separate from executor configuration. A running transport may finish. For a reconnect or cleanup, attach only to its authenticated original owner with sufficient persisted endpoint, credential reference and host trust for every hop. Use a verified immutable route snapshot and the existing managed transport. If the stored evidence cannot support that translation, preserve records and surface recovery. Never borrow the executor's latest pin, authorize unknown bastion keys, delete runtime state, or issue cleanup against a replacement machine. Keep the historical format reader for skipped upgrades. Retire general inline dialing.

## Release and rollback

Deliver schema upgrade, import-only bindings, first-use verification and consumer changes together in one PR and one release. Test a direct upgrade from the previous schema. Set the shipped SSH-manager default on wherever SSH is offered. Preserve explicit operator disables as an SSH availability switch: perform local schema/data conversion, suppress all remote work while disabled, and resume normal managed use when re-enabled. Do not turn the legacy UI back on under any toggle. Ensure archived source data cannot be read as executable config. Coordinate default changes, registry metadata, UI gates and public upgrade docs in the same PR.

Take a consistent ordinary Kandev backup before cutover. Document supported rollback by restoring the pre-upgrade backup and matching binary; an older binary must not reinterpret a partially migrated database. Expose `ssh_migration_imports` on the existing debug metrics endpoint. Its pending, verified and blocked counts come from persisted executor bindings. A value of -1 indicates that the query was unavailable. Keep reason codes and actor-attributed trust and binding records. Update the previous section 005 rollout contract and public upgrade docs when the new behavior ships.

## Requirement mapping

Import and bind covers criteria 1–4. Settings and API covers 1, 4 and 6. Launch, retained sessions and release cover 5 and 7. See the [migration plan](../../../plans/ssh-connection-migration/plan.md).

---
created: 2026-11-24
status: done
requirements:
  - REQ-EXECUTORS-SSH-MIGRATION-001
system_design:
  - ../../specs/executors/system-design/ssh-connection-migration.md
legacy_specs: []
---

# Retire inline SSH configuration

[Requirements](../../specs/executors/requirements/ssh-connection-migration.md) and [system design](../../specs/executors/system-design/ssh-connection-migration.md) govern this package. This supersedes the optional-conversion rollout at cutover. Existing SSH settings and desktop-layout work are the implementation baseline.

## Delivery in one PR

1. [Import saved settings](task-01-import.md) and bind once per old executor without dialing hosts.
2. [Verify at first use](task-02-verify.md) automatically. Send only missing or changed evidence to Access.
3. [Cut over consumers](task-03-cutover.md) and retire the inline editor and launch paths. Merge all three work orders together. Validate direct, interrupted, offline and explicitly disabled upgrades.

## ASCII UI preview

Desktop, existing executor after upgrade

```text
SSH connections
Production builds    Imported connection
Release builders     Verify bastion fingerprint       [Review]
```

Phone, after selecting Release builders

```text
< SSH connections
Release builders
Verify bastion fingerprint
SHA256:...
[Review host key]
```

Use real state and values. A healthy imported executor needs no action. Use the ordinary Access review only for an exception. Show SSH verified only after real authentication.

## Verification

Use disposable databases and isolated OpenSSH destination and bastion fixtures. Take upgrade fixtures from the pre-cutover schema. Run commands from each work order sequentially. For resource-intensive commands use a systemd scope, for example `systemd-run --user --scope -p MemoryMax=8000000000 -p MemorySwapMax=0 make -C apps/backend build`. Follow the same aggregate 8 GB cap for disposable container daemons and workloads, which do not inherit the Docker client's cgroup. Tests must not target a developer's main service or SSH credentials.

## Release evidence

Record original and final executor/profile IDs, count of candidate and bound records, classified issues, retention/recovery decisions and successful real checks. Include restart during conversion, concurrent first use, offline hosts, changed pins, and rollback from backup. Provide screenshots of one actionable exception on desktop and phone. The disposable upgrade used executor `old-ssh-executor` and profiles `profile-a` and `profile-b`. A pre-cutover binary initialized the database and reopened the seeded offline SSH executor. With SSH disabled, the new binary produced one shared connection and preserved the profiles and their prepare scripts. Two further starts, one disabled and one enabled, kept the same connection ID and original pin. The route remained unverified. Restoring the pre-cutover backup let the previous binary read the original inline settings and both profiles again.

The real OpenSSH direct-route test started the upgraded backend with the destination offline. Once the server was reachable, the original profile launched a task without a Settings visit. A separate test confirmed that a managed key could be enrolled, used after a backend restart and used to launch a task. The alias fixture exercised an inherited endpoint, an explicit override and a missing alias. An unreachable server and a rejected identity both produced connection errors.

Migration tests passed under Go's race detector with SQLite and an isolated PostgreSQL 17 database. The PostgreSQL transaction checks also passed for binding, runtime references, concurrent bastion edits and deletion dependencies. The HTTP and WebSocket handler checks reject inline SSH writes. The profile service rejects per-profile SSH route overrides.

Two simultaneous first launches using the same imported route both completed admission in the direct and hashed-known-hosts bastion tests. A concurrent route edit rejected the old admission. A native desktop backend verified a managed SSH key after a restart with no inherited SSH agent socket.

The multi-key OpenSSH regression originally failed at the bastion with a host-key-change error even though its trusted key was still present. Independent public fingerprints from the disposable servers were preserved at import. With exact-key negotiation, both the direct executor and an executor using a bastion with a separate identity launched from the original profile without intervention. The native desktop, concurrent admission and rollback checks used disposable fixtures.

The final isolated container run passed all 34 selected SSH cases in one run. It covered the two upgraded workloads, enrollment, rotation, credential failures and retries, concurrent sessions, persistence, both test APIs, and the trust gate. The SSH binaries and web assets were rebuilt. This run used the documented E2E freshness override because building the unrelated Send Now test helper exceeded the 8 GB systemd limit. These SSH cases do not call that helper. The test Docker daemon was stopped after the run.

The repair screenshots for this checkout are in `.kandev/evidence/ssh-cutover/desktop.png` and `.kandev/evidence/ssh-cutover/mobile.png`. Both show the same saved profile and the host fingerprint before the operator decides whether to trust it.

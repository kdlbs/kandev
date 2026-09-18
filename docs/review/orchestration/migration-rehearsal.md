# Private migration and rollback rehearsal

Status: passed on 2026-09-18. The candidate remains
`0.94.0-orchestration.20260918.sha69753564d0d7`, from clean source
`69753564d0d7c0121e8681a72e5b4ce8c384a106`, above exact v0.94.0.
The [redacted receipt](migration-receipt.json) identifies the bundle and snapshot
hashes. No live service configuration, executable or data was replaced.

## Backup and isolation

A read-only connection and SQLite Online Backup API produced a consistent
snapshot while the live service continued running. The snapshot passed SQLite
integrity and foreign-key checks, and task-to-search row alignment. The matching
master key, all 21 claimed attachment files, canvas artifact directory and six
original runtime binaries were preserved with restricted access outside Git.
There are no canvas release rows in this installation. Provider login files and
source repositories were not mounted into the rehearsal.

The snapshot contains 75 tasks, 69 sessions, 20,719 messages, seven profiles,
two workspaces and 19 Automation runs. It has no active sessions, queued messages,
runs, pending moves, review watches, runnable cleanup jobs or expiring quick chats.
Before first startup, copy-only controls disabled one Automation, its trigger,
eight queue auto-run settings and one workflow-step auto-archive setting.
The original settings were retained in the pristine snapshot.

Every runtime used a separate writable restored home, read-only bundle/root,
no host credentials or Docker socket, and no published ports. Only loopback
exists in its network namespace; an attempted external connection returned
`ENETUNREACH`. All owned containers were removed after graceful shutdown.

## Migration, replay and rollback

The actual packaged launcher started with Coordinator, Assistant and Office off.
Authentication remained enabled, readiness reported the exact candidate, and an
unauthenticated workspace request returned 401. Existing row identities and
original column digests were unchanged apart from reviewed native startup
bookkeeping: editor timestamps, process-presence rows, disconnected integration
health, version/projection metadata, planner statistics and temporary-artifact
records. The same categories change when the original v0.94 bundle starts.

Migration adds 25 required schema tables and the two planned Automation reference
columns. It preserves existing task/session/message contents, user/authentication
records, secrets, ownership, workflow/profile references, schedules and Automation
history. No original table is removed. A second startup adds no schema tables and
leaves every orchestration table unchanged, including the single built-in role
and empty legacy-transfer ledger. No duplicate assignments, comments or runs appear.

Rollback restored the pristine pre-candidate snapshot and matching key/blobs into
another directory, then started the original six-binary bundle. Its readiness
reports `v0.94.0`, authentication remains enabled, and retained history/reference
digests match. The old binary was never started against the migrated database.
Candidate and old-bundle startup took approximately 3.1 and 1.9 seconds in these
isolated runs. These are startup timings, not whole-cutover downtime estimates.

All restored copies pass native and FTS integrity checks with zero foreign-key
violations. Both the candidate replay and rollback retain exactly 75 aligned
task/search rows, with no orphan rows or content mismatches. Every backed-up
attachment and the matching key retain their original hashes.

## Coordinator-only check

A separate copy of the replayed database enabled Coordinator and kept Assistant
off. For this network-isolated synthetic API branch only, authentication was
disabled; the three migration/replay/rollback phases retain its real boundary.
Native APIs created a generic new workspace, review task, role and assignment
referencing retained profile/executor identities. Opening its conversation twice
returned the same task, with zero comments. The Coordinator page returned 200;
Assistant admission returned 404. No agent launch was requested.

All existing rows in 214 primary-key tables were compared after those additions.
The only user-setting change was the native last-used workflow entry for the new
synthetic workspace; existing entries, user identities and auth columns remain
unchanged. The two new tasks are the explicit example and its empty conversation.
All 77 task/search rows align. Sessions/messages remain 69/20,719 and runs,
queued messages and pending moves stay at zero.

Rendered desktop/phone interactions were verified in the separate synthetic
[candidate captures](candidate-qualification.md), using identical runtime bytes.
No browser capture or private conversation export was made from the copied data.

## Backup-method finding

The first `VACUUM INTO` snapshot passed SQLite integrity but renumbered implicit
task row IDs, leaving 28 orphan search rows and 45 mismatched task titles in the
snapshot. Both the candidate and original v0.94 bundle reproduce the resulting
search mismatch. The live source has no such mismatch. That snapshot was rejected
as rollback protection and retained only for local diagnosis.

SQLite documents that [VACUUM can change implicit row IDs](https://www.sqlite.org/lang_vacuum.html).
Its [Online Backup API](https://www.sqlite.org/backup.html) instead copies database
pages into a consistent snapshot. A synthetic WAL/row-ID-gap probe and the actual
replacement backup both preserve the task/search mapping. No unrelated native
backup or search implementation was changed. The pilot runbook now requires this
online method for rehearsal and a verified cold copy for the final rollback pair;
SQLite integrity alone is insufficient evidence of application-level consistency.

## Live boundary

The [prepared pilot packet](live-pilot-ready.md) names the exact change. A staged
service override passes systemd's user-unit verifier and preserves the current
port, login, home/data paths, provider PATH, service identity and resource limits.
It has not been installed. The final cold backup must be made at the authorized
change window; this rehearsal snapshot is not the final rollback pair.

Filesystem/Git and external-provider recovery remain separate concerns. Restoring
a database cannot undo external actions or merge later writes automatically.
The first pilot and its observed task/review/restart cycle remain outstanding.

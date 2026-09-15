---
status: draft
system: tasks
requirements:
  - REQ-TASKS-RUNTIME-CLEANUP-001
created: 2026-09-06
updated: 2026-09-06
owners:
  - cfl
---
# Archive Cleanup Evidence

## Authority

This document owns the canonical authenticated legacy-cleanup envelope,
resource content identities, verification, and physical-absence proof.
[Archive Cascade Execution Contracts](archive-cascade-execution-contracts.md)
owns lock/CAS order; [Archive Cascade Boundary Contracts](archive-cascade-boundary-contracts.md)
owns conversion and local destructive sequencing.

## Canonical database identity

`kandev_meta.archive_database_namespace_id` is a canonical lowercase UUID
created once per logical database. Backup/restore preserves it; an import into a
new logical database must reissue bindings under the destination namespace.
Database file paths, DSNs, and process-local IDs never substitute for it.

## Version 1 binding envelope

The closed RFC 8785 object contains exactly:

- `binding_version: 1`, opaque nonempty `key_id`, and
  `database_namespace_id`;
- canonical `operation_id`, `member_id`, `task_id`, and `workspace_id` UUIDs;
- `resource`, the exact subdocument below;
- nullable `group_snapshot_sha256`; and
- canonical RFC3339Nano UTC `committed_at`.

`resource` contains exactly:

- `resource_version: 1`;
- `kind`, one of `runtime|task_environment|worktree|attachment|workspace_group|docker|kubernetes|ssh|sprites`;
- `resource_key_version`, `resource_key`, and canonical `resource_id`;
- `canonical_path_b64`, base64url-no-pad canonical path bytes or JSON null for
  resources without a local path;
- unsigned `ownership_generation` and ownership `state`, one of
  `prepared|active|cleanup_pending|cleaned|restore_pending|restored|quarantined`;
- `marker: {"marker_id","marker_sha256"}`; and
- `content_identity`, the kind-specific closed object below.

UUIDs are lowercase RFC 4122, hashes are 64 lowercase hex, unsigned integers
are JSON integers, nullable fields are present as null, and unknown/missing
fields are invalid. The MAC is lowercase hex
`HMAC-SHA256(key, ASCII("kandev/archive-member-binding/v1") || 0x00 || RFC8785(envelope_without_mac))`.
The retained row stores the exact canonical envelope bytes and MAC; it does not
store an independently selectable subset hash.

## Closed content identities

| Kind | Exact `content_identity` fields |
| --- | --- |
| `runtime` | `executor_running_id`, provider/runtime enum, canonical local PID plus process-start identity or immutable remote execution handle, session ID, and row SHA-256 |
| `task_environment` | task-environment ID, executor/profile IDs, workspace ownership mode, sorted environment-repository IDs, and complete row-set SHA-256 |
| `worktree` | filesystem device/object identity, repository and task-environment-repository registration IDs, canonical containment-root SHA-256, Git common-dir and worktree-admin identities, exact branch, nullable unborn HEAD, expected base commit, repository-default commit, and worktree row SHA-256 |
| `attachment` | attachment ID, exact task/staging owner, byte size, content SHA-256, and row SHA-256 |
| `workspace_group` | group ID, ownership generation, versioned canonical full group/member snapshot SHA-256, and ordered member-set SHA-256 |
| `docker` | container/volume IDs, immutable image/config hash, and exact task/workspace/operation ownership labels |
| `kubernetes` | cluster identity, namespace, object kind/name/UID, spec hash, and exact ownership labels |
| `ssh` | connection identity, remote instance ID, remote canonical path bytes, remote marker ID/hash, and provider observation hash |
| `sprites` | account/connection identity, sprite instance ID, remote marker ID/hash, and provider observation hash |

Every field is mandatory unless explicitly nullable above. Provider enums and
label keys are closed by the resource provider registry. The resource-key
preimage and `content_identity` are recomputed from locked rows and an
authenticated physical/provider observation; a caller-supplied summary or
`resource_snapshot_sha256` without this full object is not evidence.

The version-1 envelope has an inclusive `max_evidence_envelope_bytes=262144`
limit over its canonical UTF-8 bytes before HMAC; larger envelopes are rejected
before allocation, persistence, or physical-guard acquisition. Retained evidence
payloads use `max_retained_payload_bytes=524288`; truncation is forbidden.

## Verification and physical absence

Before acquiring the physical guard, verification selects the retained key by
`key_id`, canonicalizes bytes, checks the MAC, and locks/reloads the database
namespace, operation/member, task/workspace identity, owner rows, tier-8
snapshot, and tier-14 evidence/marker. It persists the exact snapshot hash and
generation in the claim. The worker then acquires the guard and runs only a
short tier-14 marker CAS against that persisted hash/generation, rechecking the
path/provider handle immediately before I/O while retaining the guard. No
tier-8 or earlier lock is acquired while the guard is held. Mismatch, unknown
key/version, deleted key, or ambiguous provider result is `unknown` and permits
no I/O.

An absent captured database metadata row is idempotent metadata removal, never
physical absence. A local/worktree path is `absent_proven` when the valid marker
names the exact envelope and either locked Git registration records removal or
retained snapshot evidence has authenticated
`evidence_kind=git_registration_removal`. The pre-guard snapshot/evidence lock
and guard-held marker CAS require the same authenticated generation and reject
conflicting path/object/branch registrations. Remote absence still requires the
immutable provider handle, scoped authenticated not-found, and marker. Generic
filesystem/Git/provider not-found is `unknown` and remains retryable.

## Issuance, rotation, and verification

Keys are 32 random bytes in the installation secret store. New bindings use only
the current key. Verification-only keys remain while any envelope references
them; removal requires zero references and elapsed retention. Migration issues a
binding only when locked live rows or an existing committed operation/member
marker independently proves every envelope field. Cleanup-only rows, an
operation UUID, a generic row hash, or a partially populated resource document
never qualifies.

Both dialect suites issue each resource kind, reload and verify it, then mutate
each scalar, nullable field, list element/order, marker, generation, state,
database namespace, path byte, and content-identity field independently. Every
mutation fails closed. Tests cover key rotation, canonical encoding, backup
namespace preservation, import reissue, transaction rollback before marker
commit, physical absence with/without exact proof, aliases/symlinks, and a
check-to-use identity change.

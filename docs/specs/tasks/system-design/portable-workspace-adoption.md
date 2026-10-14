---
status: current
system: tasks
created: 2026-10-14
updated: 2026-10-14
requirements:
  - REQ-TASKS-PORTABLE-ADOPTION-001
  - REQ-TASKS-PORTABLE-ADOPTION-002
  - REQ-TASKS-PORTABLE-ADOPTION-003
---

# Portable workspace adoption system design

## Purpose and boundaries

The task backend owns explicit adoption of an independently prepared clone into
an existing damaged host Worktree slot. Deployment SQLite scripts and outer guard
policy remain local. Ordinary healthy-checkout admission and task Docker executor
behavior are separate contracts. No frontend or plugin surface is introduced.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| REQ-TASKS-PORTABLE-ADOPTION-001 | Authority and publication |
| REQ-TASKS-PORTABLE-ADOPTION-002 | Filesystem and Git proof |
| REQ-TASKS-PORTABLE-ADOPTION-003 | Durable receipt and reuse |

## Authority and publication

The task HTTP handlers expose administrator preview, apply and receipt lookup.
The backend composition supplies the caller guard; agent-origin requests and an
unwired guard fail closed. Both composition and service require a verified,
non-synthetic administrator identity. Auth-disabled requests are refused because
they cannot distinguish an operator from an agent omitting credentials. Task access
and workspace management scopes still apply.
The service captures the existing waiting session incarnation, complete canonical
inventory, owner generation and task attachments. Apply acquires a manager lock
and durable recovery claim, rejects in-memory and persisted consumers, then repeats
selection and filesystem inspection before publication. No prompt replay occurs.
Attachment selection uses the same branch identity plans as ordinary launch, so
a generated worktree branch retains the attachment's empty checkout selector.
The receipt captures that selector and its base branch independently from the
materialized worktree branch; publication compares the full attachment set,
including its metadata digest so PR-number mutations cannot change a duplicate
branch identity between selection and the transaction's locked comparison.
Nil and empty metadata objects have the same digest, matching repository decoding.
Publication and receipt lookup use the same active-inventory rule as recovery:
an undeleted slot with an empty legacy status remains active. Competing legacy
bindings prevent publication; adopting a selected legacy slot preserves its status.

## Filesystem and Git proof

The worktree manager pins the owned task root, original checkout, candidate and
registered source metadata. The root is derived from the configured tasks base
and persisted task directory name, independently of the execution working directory.
Native single-repository sessions use the selected checkout as their working
directory; multi-repository sessions use the task root. Both layouts require
the original and candidate checkouts to be direct children of the pinned root.
The original pointer must identify missing linked
administration in the registered source. A present `worktrees` administration
parent is opened without following symlinks and retained in the directory proof;
publication and reuse reject parent replacement or newly restored administration.
If the parent is absent, publication requires it to remain absent. The independent clone must be a direct
child of the same root and retain the selected branch, origin and real ancestry.
Raw Git reads use explicit metadata paths, sanitized Git environment and disabled
replacement objects. Bounded no-follow traversal refuses escaped refs or objects,
grafts and external alternates in both source and candidate; packed refs remain supported.
Both metadata layouts reject symlinked HEAD/config/index files and common-directory
redirects before Git reads. Source object storage receives the same no-follow and
promisor-pack checks as the candidate. Revalidation retains
the root/marker/directory identities. No candidate files, index or old marker are
rewritten. Both directories remain intact.

## Durable receipt and reuse

The task repository atomically updates the selected physical path and source
provenance and inserts a receipt in `task_environment_checkout_adoptions`.
When the environment working directory equals the retired checkout, the same
transaction moves that directory and bound session paths referencing it to the
candidate. Task-root working directories remain unchanged. The receipt retains
the pre-publication snapshot for CAS and validates its deterministic published
working-directory projection on replay and reuse. Logical identities and the
owned task root remain unchanged. Full
selection CAS, claim authority and pinned-path revalidation run in the same
SQLite or PostgreSQL transaction. An operation replay validates current authority
and finalizes only its own committed orphan claim. Cache aliases are invalidated
before the claim is released. Reuse checks the receipt before ordinary metadata
classification, preventing stale bindings from falling into clone recovery.

Receipt validation binds the original session incarnation, owner generation,
workspace, complete branch-driving attachment set, selected slot branch identity
and ordering, and filesystem proof. Publication and receipt reuse share the same
attachment digest comparison; PR metadata or attachment additions cannot retain
old branch authority after publication. HEAD and index may
advance normally; authority or metadata changes fail closed. There is currently
one receipt per environment/slot. Re-attesting or replacing a stale receipt is an
unimplemented lifecycle limitation requiring a separately reviewed operator
contract; reuse checks must not be weakened to conceal it.
PostgreSQL ownership cutover retains receipt bytes and rebinds their environment
foreign key to the replacement table. A receipt referencing an environment that
does not survive the cutover fails the constraint check and rolls back the cutover;
its recorded authority is never rewritten.

## Failure and observability

Invalid JSON returns 400, forbidden operators return 403, and refused selections
return 409. Service/repository errors retain their causes internally. Regression
fixtures exercise HTTP plus persisted state and SQLite/PostgreSQL rollback/replay.
Actual agent process restart and release/deployment acceptance are separate from
these synthetic fixtures. Unsupported filesystem identity platforms fail closed.

## Related documents

- [Requirements](../requirements/portable-workspace-adoption.md).
- [Explicit adoption decision](../../../decisions/2026-10-11-portable-checkout-adoption.md).

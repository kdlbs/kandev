---
status: draft
system: tasks
requirements:
  - REQ-TASKS-EXACT-RETIREMENT-001
  - REQ-TASKS-EXACT-RETIREMENT-002
  - REQ-TASKS-EXACT-RETIREMENT-003
---

# Guarded Exact-Task Retirement System Design

## Purpose and boundaries

The task service owns pair authorization, receipts, operation fencing, and the
old-card lifecycle. The worktree manager owns physical Git identity; the queue,
environment, runtime, and PR systems own their own inspections. Missing an
adapter is an `UNKNOWN` predicate, never an empty inventory. The existing source
manifest is consumed as hash-only evidence and cannot prove archived bytes.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-TASKS-EXACT-RETIREMENT-001` | Data and contracts; Security |
| `REQ-TASKS-EXACT-RETIREMENT-002` | Components; Control flow |
| `REQ-TASKS-EXACT-RETIREMENT-003` | Persistence; Failure and recovery |

## Components and responsibilities

`ExactRetirementPreviewer` authorizes both task IDs before reading either
inventory. Its closed predicate registry covers identity, sessions/FIFO,
move/dispatch, relationships, PR/watch, preservation/Git, environment/runtime/
lease/consumer, and replacement ownership. Adapters provide body-free evidence
and stable generations. The later executor is separate and may not call normal
task delete/archive or broad worktree cleanup.

## Data and contracts

The W02 preview request carries exact old and replacement IDs, workspace ID,
and their expected generations. Later commitment work adds canonical
repository/environment/worktree identities and a platform-verifiable
preservation receipt identifier. A predicate receipt contains status, reason
code, resource ID, observed generation, and SHA-256 evidence digest. Its
payload is redacted: queue bodies, archive bytes, credentials, and provider
tokens never appear.

`PASS` means the adapter proved the required condition. `BLOCKED` means it
proved an unsafe condition. `UNKNOWN` means the adapter was unavailable,
incomplete, contradictory, or failed. A preview is eligible only when every
predicate is `PASS`; W02 exposes no commit route.

## Control flow

1. Authorize the old task from trusted caller context, then authorize and load
   the replacement through the same path.
2. Reject equal IDs, workspace mismatch, and generation mismatch without
   inventory disclosure.
3. Run each read-only predicate and sort receipts by fixed predicate/resource
   order. Adapters do not acknowledge, repair, resume, or consume anything.
4. Return the bounded receipt. Later work rechecks every captured generation
   in one claim transaction before any external action.

## Failure and recovery

An adapter error, Git exit 128, stale linked metadata, absent receipt, or
contradictory identity produces `UNKNOWN`. A live session, unread FIFO item,
active watcher, active consumer, or unresolved relationship produces `BLOCKED`.
Preview has no retry side effect. The later durable operation ledger records
claims and external step receipts; a crash leaves uncertain state blocked until
an independent exact inspection proves it.

## Persistence

W02 persists nothing. Later work adds an idempotent operation ledger whose
request hash, captured generations, audit receipt, and preservation references
survive old-card deletion. The final delete transaction is old-row scoped.

## Security

The service derives authority from request context rather than the request
body. The caller must have an admin identity and `task.write` on both tasks.
Admin status does not grant workspace access. The preview also requires the
old task's workspace to exist and requires workspace write access. A missing or
foreign task does not produce a receipt. When authentication is disabled, the
existing synthetic admin identity follows the same route policy.

Receipts contain only identifiers, generations, reason codes, and evidence
digests.

## Observability

Preview logs only closed reason codes and predicate names. It never logs source
bytes, queue bodies, filesystem paths, or provider credentials.

## Related decisions

- [ADR-2026-09-25-exact-task-retirement-boundary](../../../decisions/2026-09-25-exact-task-retirement-boundary.md)

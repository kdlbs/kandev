---
created: 2026-10-16
status: complete
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
legacy_specs: []
---

# Implementation plan: Repair duplicate legacy worktree ownership

## Overview

Extend the existing offline inventory-repair utility for installations whose
older tasks contain duplicate physical worktree claims. Deliver read-only
discovery, explicit reconciliation, ordinary-store ambiguity guards, and real
archive/delete regression coverage. Execute the four work orders sequentially.
This package prepares the repair. It does not authorize installation maintenance.

The task system owns the contract because task environments own the physical
inventory used by session admission and durable cleanup.

## Evidence and smallest reproduction

The investigated backend runs commit `059260b30`, built on October 6. Two older
multi-repository tasks each have a correct repository slot and a newer slot with
the same physical worktree ID. The newer slot has an empty branch slug and points
to the enclosing task folder, which has no `.git`. The actual checkout below it
is healthy. `GetWorktreesByTaskID` sorts newest first, and `scanWorktrees` retains
the first ID. Archive preparation therefore runs Git in the task folder and fails
at `commit_lookup`. Five matching HTTP 500 failures appear in retained logs.

One child also has a separate environment containing copies of its parent's
physical IDs. Its sessions bind to both environments. `GetWorktreeByID` uses
unscoped `LIMIT 1`, and a parent session reports that the selected worktree
belongs to another task. The exact historical writer is unproven.

Existing fix `a789b16d7` (#4420) lets archive identity capture use the recorded
branch when checkout-local metadata is absent. It is already in this source
tree. A temporary Go overlay test confirmed that current capture succeeds but
`InspectDirtyWorktrees` still refuses that folder at `working_tree_status`.
The version-1 repair utility requires exactly one row per worktree ID, so it
also refuses these duplicate shapes. No failed live DELETE response was captured.

The smallest fixture is one physical ID in two active slots: a registered child
checkout and its enclosing non-Git task root. Add a copied child environment to
exercise cross-task ambiguity. Use generic IDs and temporary paths, never IDs
from the investigated installation.

## Scope

### In scope

- Bounded discovery over saved inventory and a version-2 explicit selection format.
- Exact alias-row removal, shared session rebinding, dependent snapshot preservation,
  and redundant-environment retirement after proof.
- Backup, journal, startup fencing, refusal, rollback, and repeat-application behavior.
- Read/write guards against competing physical claims, including PostgreSQL runtime
  store behavior, without automatic owner election.
- Operator instructions and archive/delete integration evidence after reopening storage.

### Out of scope

- Automatic startup repair, filesystem cleanup, checkout relocation, or Git mutation.
- PostgreSQL repair application, remote executor repair, or application on non-Linux hosts.
- Running the utility against this user's installation, stopping agents, or deployment.
- Rewriting cleanup evidence, merging unrelated branches, or repairing arbitrary corruption.
- Browser component changes or a new repair settings page.

## Technical approach

Use the existing `internal/task/inventoryrepair` command, row observations,
process census, private backup, journal, and startup fence. Add duplicate-specific
validation and mutations rather than weakening version-1 uniqueness checks.
The extension in the linked system design defines canonical proof,
dependent-reference policy, bounded input, transaction boundaries, and rollback.

The canonical row is explicitly selected and independently proven. Neither
timestamp ordering nor a branch-slug spelling chooses it. An entire connected
ownership selection succeeds or fails together. The same-environment repair
retains the canonical slot and removes selected task-root aliases. A copied
child environment additionally requires complete canonical counterparts for
every repository, exact shared membership, explicit session/snapshot rebinding,
and absence of unsupported references before removing the redundant environment.

Keep all task/session identifiers, states, messages, resume data, branch refs,
checkout content, and original cleanup evidence. Version 2 performs SQL metadata
changes only. Do not rerun the legacy schema cutover on a normalized database.
Do not add a uniqueness index that makes unrepaired installations fail startup.

| Installation or identity shape | Behavior | Required evidence |
| --- | --- | --- |
| SQLite, Linux, host Worktree, same-root aliases | Offline selected application | Reciprocal Git/root proof, exact rows, no consumers, rollback |
| SQLite, Linux, copied child environment | Offline application only for complete shared graph | Every slot matched, sessions/snapshots retained, no foreign references |
| SQLite, macOS or Windows | Discovery and read-only preview/verification | Existing native-path and unsupported-apply tests |
| PostgreSQL | Runtime ambiguity guards, repair application unsupported | Real database concurrency tests for store guards, early utility refusal |
| Docker, SSH, Kubernetes, plugins, foreign roots, broken metadata, oversized groups | Report or reject before application | No host repair or guessed ownership |

## Tests

Prefixes below expand to `AC-TASKS-WORKTREE-INVENTORY-REPAIR-`.

| Criteria | Regression evidence |
| --- | --- |
| 004.1-.2, 001.1-.2 | `alias_discovery_test.go` and `alias_preview_test.go`: task-root duplicates, cross-environment duplicates, complete group pagination, forged canonical selections, changed claims |
| 004.3-.6, 001.3-.5 | `alias_apply_test.go` and `alias_recovery_test.go`: shared bindings, retained conversations/snapshots, mixed failed/live slot refusal, inactive consumers, exact rollback, commit/fence failpoints |
| 004.8 | `store_alias_identity_test.go` and `task_environment_alias_identity_test.go`: session joins, active/history mixtures, exact writes, no fan-out. `store_alias_identity_postgres_test.go` and `task_environment_alias_identity_postgres_test.go`: mandatory real-PostgreSQL concurrent claim checks. |
| 004.7, 002.3 | `legacy_alias_lifecycle_test.go`: archive/delete after repair and reopen, parent-only/child-only/cascade cases, dirty and borrowed workspaces, unchanged cancelled cleanup evidence |

Tests must fail before their corresponding implementation. Existing version-1
repair tests, branch slots, and source-evidence guards remain regression inputs.

## E2E tests

Use backend integration tests with temporary SQLite databases and real Git
worktrees. Drive the version-2 utility, reopen through normal repositories,
then invoke archive and confirmed delete through the real HTTP/service paths.
Test each operation on a fresh fixture. The test must assert persisted state and
preserved files, not just a successful HTTP response. Use an actual idle borrower
to verify shared cleanup retention. No real agent is launched.

No Playwright test is required for this package: rendered components do not
change, and existing desktop and phone actions use the tested lifecycle APIs.

## Work orders

- [x] [Task 01: Discover and preview duplicate claims](task-01-discover-and-preview.md)
- [x] [Task 02: Apply and recover exact reconciliation](task-02-reconcile-shared-inventory.md)
- [x] [Task 03: Reject ambiguous runtime identity](task-03-guard-runtime-identity.md)
- [x] [Task 04: Verify lifecycle recovery and document operation](task-04-lifecycle-and-operator-guide.md)

## Companion packages

- [Existing inventory repair](../worktree-inventory-repair/plan.md) remains complete
  for its delivered version-1 scope. This extension has separate work orders and validation records.
- [Missing checkout cleanup preparation](../missing-worktree-cleanup-preparation/plan.md)
  already delivers #4420. Do not duplicate that fix or treat its tests as duplicate repair.
- [Preserve physical inventory](../preserve-task-worktree-inventory/plan.md) protects
  inventory-only refreshes. Its completed status does not establish alias repair.

## Verification results

Design validation on 2026-10-16:

- `python3 scripts/list-docs.py validate`: passed, 384 decisions and 1,616 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` preflight: actual documentation-only diff is exempt.
  A representative planned source path plus the four work orders is covered,
  with no missing requirement, acceptance, design, or plan references. This is
  document-coverage evidence, not implementation validation.
- Work-package relative links resolve. Changed-path whitespace checks passed.

The design turn changed no production or permanent test files. The user later
authorized implementation in this session. All four work orders are complete
and record their actual test results.
Race tests, real PostgreSQL concurrency checks, store conformance, SQL guard,
scoped lint, documentation checks, and actual-diff coverage passed. Live
installation repair and deployment remain outside this delivery.
Task 04 documents canonical lifecycle controls: borrower protection can defer
archive reclaim, and cascade delete can retain a group cleanup error after its
durable task cleanup succeeds. The repair does not change those behaviors.

## Risks

- Correcting only the parent path leaves cross-environment IDs ambiguous. Preview
  must include the whole connected selection.
- A stopped backend can leave agentctl or terminal processes alive. Process
  census and ownership locks remain necessary even for metadata-only changes.
- Redundant-environment deletion can cascade historical Git snapshots. Rebind
  only proven observations first, and reject unknown dependent tables.
- Version-2 graph rollback needs dependency ordering and raw timestamp retention.
  Reusing the version-1 mutation loop unchanged is insufficient.
- Discovery is portable, but existing application support is Linux with SQLite
  host worktrees. The source-built utility is not a new bundled CLI subcommand.
- Old pending cleanup evidence remains an explicit blocker. Reporting a blocked
  candidate is safer than relabelling historical snapshots with new observations.

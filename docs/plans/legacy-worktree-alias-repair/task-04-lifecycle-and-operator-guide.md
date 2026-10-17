---
id: "04-lifecycle-and-operator-guide"
title: "Verify lifecycle recovery and document operation"
status: done
wave: 4
depends_on:
  - "03-guard-runtime-identity"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-002
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-004
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-002.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.3
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.4
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.6
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-004.7
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 04: Verify lifecycle recovery and document operation

## Summary

Prove that the reusable repair restores normal archive and confirmed-delete
behavior after reopening storage. Publish the operator procedure with the
supported installation limits, refusal reasons, and rollback instructions.

## In scope

- Add generic real-Git/SQLite fixtures for same-root aliases and copied child
  environments. Exercise discover, preview, offline apply, verify, storage reopen,
  archive, and confirmed delete through the real lifecycle paths.
- Cover parent-only, child-only, and cascade actions on separate fixtures. Preserve
  a shared workspace with a retained borrower. Verify stable conversation/resume
  identity after rebinding, without launching an agent.
- Include healthy multi-branch controls and dirty tracked, staged, untracked,
  and ignored files. Repair preserves them, and subsequent delete keeps its
  existing consent and safety checks. Preserve cancelled old snapshots verbatim.
- Update `docs/public/git-operations.md`, linking from existing operations recovery
  guidance where useful. Explain source build, discovery/selection, fresh offline
  preview, process blockers, apply/verify/restart, and interrupted repair recovery.
- Synchronize the companion package links and this package's actual validation
  evidence. Keep existing completed work-order results intact.

## Out of scope

Running maintenance on this installation, deployment, binary distribution changes,
rendered UI changes, or claiming unsupported PostgreSQL/non-Linux application.

## Acceptance

1. `TestLegacyAliasLifecycle` proves archive/delete persistence after reopen for
   both incident shapes and proves that child removal preserves borrowed resources.
2. Content, commits, index, snapshots, and conversations survive metadata repair.
   Ordinary dirty/ownership/commit refusals remain intact.
3. The public how-to distinguishes an inventory repair from cleanup completion
   and states the supported platforms, offline requirement, and recovery commands.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/task/inventoryrepair -run TestLegacyAliasLifecycle -count=1)
(cd apps/backend && go test -trimpath -race ./internal/task/inventoryrepair ./cmd/worktree-inventory-repair -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/inventoryrepair/legacy_alias_lifecycle_test.go apps/backend/internal/task/inventoryrepair/legacy_alias_preservation_test.go apps/backend/internal/task/inventoryrepair/legacy_alias_cascade_test.go
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Include any additional fixture/helper files in the scoped lint invocation. Run the local
`validateCoverage` API from `.github/scripts/pr-docs.cjs` over the actual changed
files and their linked documents. Record all commands and results before marking
this package implemented. No generic full-backend or browser audit is added.

## Files likely touched

- New `apps/backend/internal/task/inventoryrepair/legacy_alias_{lifecycle,preservation,cascade}_test.go`.
- Inventory-repair test helpers only where needed for reusable real fixtures.
- `docs/public/git-operations.md` and relevant links in `docs/public/operations.md`.
- The paired inventory-repair requirement/design, this package, and the companion
  `docs/plans/worktree-inventory-repair/plan.md` extension link.

## Dependencies

Tasks 01 through 03, including their complete targeted validation.

## Risks

A successful metadata verification does not prove that later cleanup completed.
Do not weaken file/ownership guards to obtain a successful end-to-end test.

## Parallelism

`sequential`

## Inputs

- Linked design's operator outcome and lifecycle matrix.
- Existing inventory-repair `cleanup_worker_test.go`, handoff service fixtures,
  HTTP delete confirmation tests, and public Git recovery instructions.

## Results

Lifecycle coverage lives in `internal/task/inventoryrepair` so it can reuse the
private isolated-host audit fixture. It invokes the real task service, handoff
service, worktree manager, and registered HTTP routes. No production test hook
or duplicate fixture package was added. The verification commands above use
these final test locations.

The initial cascade fixture omitted the restore configuration saved by normal
materialization. The fixture now uses the real Office schema and a complete
single-repository group record. The first assertions also treated queued cleanup
as synchronous and assumed cascade archive removes a borrower's checkout.
The final tests exercise the durable cleanup worker and preserve the existing
borrower guard.

Canonical controls reproduce two existing lifecycle outcomes without repair:

- Cascade archive persists both archive states but retains the checkout while
  the child's terminal session still binds to the shared environment. Reclaim
  remains `waiting_for_clean`.
- Confirmed cascade delete returns HTTP 503 with `pending: true` after deleting
  both tasks. The durable cleanup jobs subsequently succeed and remove the
  checkout. The workspace group retains `cleanup_failed` with `worktree wt:
  worktree not found`, because its ID-based cleanup runs after inventory deletion.

This package does not change these existing lifecycle policies or group-status
handling. The tests assert task persistence, actual filesystem outcome, and
separate job/group evidence. The operator guide states this limitation.

Final targeted lifecycle race tests passed (68.173s). They also cover preserved
messages/turns/conversation IDs, provider resume tokens, snapshots, healthy branch
slots, staged/unstaged/untracked/ignored content, deletion confirmation, discard
consent, and immutable cancelled cleanup evidence. The full repair/CLI race run passed with `-timeout 20m`.
The final package results are recorded below.

Documentation checks passed: public-doc tests (63), public-doc validation,
catalog validation (384 decisions and 1,616 specifications), spec tests (36),
full spec lint, actual-diff `validateCoverage`, and `git diff --check`.
Final scoped inventory-repair lint passed with zero issues.
Screenshots are not required because this package changes no rendered UI.

Full race-suite output:

```text
ok  	github.com/kandev/kandev/internal/task/inventoryrepair	321.432s
ok  	github.com/kandev/kandev/cmd/worktree-inventory-repair	1.126s
```

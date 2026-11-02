---
created: 2026-10-23
status: implemented
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008
system_design:
  - ../../specs/agents/system-design/workspace-inventory-repair.md
legacy_specs: []
---

# Implementation plan: Resume after an attested workspace changes branch

## Outcome and checkpoint

Allow ordinary Resume and additional-session launch after a completed workspace
repair and a subsequent branch change. Preserve the same checkout, original
repair evidence, and independent delivery-recovery checks.

Implementation was authorized after the design handoff, with the scope simplified
to reuse existing admission checks without additional environment-wide locking.
Design baseline: verified remote main `c15867d270e3cddcae5334262ba5b31a6ef9ecce`.
This task's branch was advanced cleanly
to that commit; the affected MCP task's workspace was not changed.

## Evidence and root cause

Read-only inspection of the affected task showed a completed positive receipt
for `feature/unify-built-in-mcp-m-3po`. Its canonical environment row now names
`feature/builtin-mcp-management-6d1b`. The worktree ID and canonical path hash
still match. Resume reports:

```text
workspace inventory recovery conflicts with current state: repaired row receipt identity no longer matches
```

`attestedWorkspaceInventoryRowsReceipt` calls
`workspaceInventoryRowReceiptMatches` before validating completed attestation.
`workspaceInventoryReceiptCheckoutMatches` treats historical branch/ref identity
as a permanent physical-identity constraint. Live branch synchronization updates
the row but correctly leaves the historical receipt unchanged.

The smallest reproduction is: repair a stale inventory slot, complete positive
attestation, switch or rename the actual branch, persist its observed branch,
and invoke ordinary Resume. Keep the environment row, worktree ID, and path.
The original code rejected the receipt before launch. The new regression must fail
for this reason before production changes.

The running installation was built from `e29a5b364488`. Main contains the
separate silent-turn fix in PR #4623 and delivery-notice fix in PR #4598.
Neither changes this receipt predicate. Resolving workspace admission does not
prove the affected prompt's unknown outcome is resolved.

## Contract and scope

Agents owns the existing online repair requirement/design pair. Tasks owns the
storage implementation and separate offline repair utility. Extend the Agents
pair rather than introduce an offline repair or UI specification.

The existing AC-008.3 through .10 remain safeguards. New AC-008.11 and .12 define
branch continuity and mixed-inventory refusal. The design's historical-branch
constraint is incomplete for completed repairs; the accepted
[decision](../../decisions/2026-10-23-attested-worktree-branch-continuity.md)
records its correction explicitly.

In scope: positive-receipt admission, current checkout proof, both launch entry
points, old persisted receipts, regression coverage, and precise public guidance.

Out of scope: receipt rewriting, schema changes, live database repair, deployment,
backend restart, new sessions, provider recovery changes, and UI rendering.
The MCP task's seven design documents remain owned by that task.

## Technical approach

Separate immutable scope and physical identity from historical branch/ref
identity in the executor receipt gate. Permit changed branch/ref values only
after validating the stored positive attestation and proving the current
canonical row against Git within existing admission. Retain strict
incomplete-attestation and explicit-key retry behavior.

Place the new helper in
`apps/backend/internal/orchestrator/executor/executor_inventory_branch_continuity.go`.
Keep the shared call site in `executor_workspace_inventory_recovery.go` small.
Reuse current ownership and safe inspection helpers. Actual repair and incomplete
attestation retain their existing claims. No new claim or repeated store reads
are required for this read-only completed-repair check.
Do not add unrelated changes to branch-observation writers.

| Boundary | Supported behavior | Refusal/evidence |
| --- | --- | --- |
| Local managed worktree, SQLite or PostgreSQL | Completed positive receipt plus current reciprocal branch proof | Real Git/SQLite integration; existing store and claim contracts retained |
| Owner or authorized borrower | Bind the receipt to the environment owner | Cross-owner, wrong current branch, or ambiguous slot refuses |
| Multiple repositories or repeated repository slots | Every required slot must pass | Mixed safe/blocked inventory admits zero launches |
| Incomplete or negative receipt | Existing exact historical attestation only | Branch evolution cannot finish or overwrite failed evidence |
| Remote/container or unsupported source | Existing behavior | No new host inspection or fallback admission |
| Unknown prompt outcome | Existing delivery recovery | Workspace admission does not clear or replay it |

## Regression and end-to-end evidence

The new tests live in `executor_inventory_branch_continuity_test.go` and
`executor_inventory_branch_continuity_integration_test.go`. Use real temporary
Git worktrees and SQLite, following `executor_inventory_claim_test.go` and the
existing workspace inventory attestation fixtures.

Primary red test: `TestResumeSessionWithOptionsAllowsAttestedBranchEvolution`.
Pair it with `TestLaunchPreparedSessionAllowsAttestedBranchEvolution`. Cover
rename and switch, later commits, staged/dirty/untracked files, old receipt
bytes, and a borrower using its owner's receipt. Count exactly one launch.

`TestAttestedBranchEvolutionRejectsUnprovenIdentity` covers branch disagreement,
changed owner/path/worktree/repository slot, missing Git registration, symlinks,
and unsupported sources. `TestAttestedBranchEvolutionRequiresCompletedRepair`
covers missing, negative, and inconsistent post-repair evidence.
`TestAttestedBranchEvolutionChecksEverySlot` covers a changed safe slot with a
second unattested, failed, deleted, or duplicate slot. Retain existing repair-claim tests. A valid live sibling must not prevent this
read-only check merely because the historical branch changed.
All refusals launch nothing and preserve receipts and checkouts.

The backend integration tests exercise the real Resume and prepared-launch
paths with a stubbed agent launcher. No browser test or visual preview is needed:
the existing controls and rendering do not change. Do not use the live task as
a test fixture or bypass its blocked delivery.

## Public documentation

`docs/public/websocket-api.md` now distinguishes ordinary Resume after completed
repair from explicit same-key replay and unknown-prompt recovery. Existing
key-conflict semantics remain documented.

## Work order

- [x] [Task 01: Admit proven branch evolution](task-01-proven-branch-evolution.md)

## Verification results

Implementation and focused verification are complete. Changed-file lint passed
with zero issues. The [work order](task-01-proven-branch-evolution.md#results) records the
red/green result, executor race suites, preservation tests, delivery-block
checks, and the unrelated umask-sensitive test selected by the original broad
filter. Production changes are limited to the shared executor receipt gate
and its helper, plus a shared Git identity inspector extracted from the existing
full preservation inspector. Existing claims and database operations are unchanged.

Design validation on 23 October:

- Catalog validation passed for 400 decisions and 1,718 specifications.
- All 36 specification-linter tests passed, and full specification lint passed.
- Package links, whitespace, and prospective implementation documentation
  coverage passed. The coverage check includes the proposed executor change.
- The design checkpoint preceded all production and permanent test changes.

## Risks

- Accepting a current row without Git proof would weaken branch admission.
- Rechecking current content against repair-time hashes would reject normal work.
- Reuse existing owner/borrower rules without introducing a new writer-exclusion policy.
- A successful fix can reveal a separate prompt-delivery blocker in the live task.
- Existing tests that model arbitrary receipt corruption must retain rejection
  coverage; do not simply remove their branch cases to make the new path pass.

---
status: active
system: agents
created: 2026-09-01
owners:
  - kandev
---
# Preserved workspace inventory repair Requirements

## Overview

A reusable task environment's durable `task_environment_repos` inventory can
drift from the repository/branch slots required by session resume or a fresh
additional-session launch, even though the on-disk checkout remains valid and
untouched. Recovery must repair only provably stale or missing server-owned
inventory metadata and must never delete, reseed, rematerialize, clean, reset,
or rewrite the preserved checkout.

## Requirements

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008: Preserved workspace inventory repair

**Intent:** Recover a reusable checkout whose canonical inventory metadata drifted without changing the checkout.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.1:** Normal workspace reuse shall fail closed when any required repository and branch slot lacks exactly one active canonical inventory row.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.2:** An authorized recovery shall repair one missing or stale row only when server-owned task, workspace, environment, repository, session, runtime, path, and Git metadata prove one reciprocal checkout identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.3:** Recovery shall preserve Git objects, refs, index state, tracked modifications, and untracked files byte-for-byte and shall return hashes that attest to the before and after state. Inspection shall not execute repository-configured callbacks.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.4:** The same task-scoped idempotency key and payload shall return the existing receipt; the same key with a different derived identity, including session identity, shall conflict without mutation even after inventory becomes valid.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.5:** Duplicate, conflicting, cross-task, cross-workspace, wrong-repository, wrong-branch, wrong-environment, deleted, failed, symlinked, or ambiguously dirty evidence shall return a stable non-leaking result and preserve every resource.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.6:** A committed repair shall permit at most one orchestrator-owned resume or start attempt, and concurrent calls shall not create duplicate rows, receipts, sessions, or live writers. Repair and incomplete attestation shall exclude active consumers of the same environment across owning and borrowing tasks.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.7:** Recovery shall never delete, clean, reset, reseed, rematerialize, or broadly discover a checkout, and a post-repair launch failure shall remain retryable and audited.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.8:** Unauthorized and unrelated-task callers shall receive no repository path, worktree path, branch, or cross-workspace existence information.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.9:** A same-idempotency-key retry after a committed repair shall return the stored receipt, including from a fresh or additional-session launch that reaches the same repair path automatically.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.10:** The stored receipt shall carry non-secret evidence captured both before the repair and after the repair transaction commits, so that a divergence between the two is itself an auditable result. Every borrower shall honor the environment owner’s receipt before launching.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.11:** After a repair completes with matching before-and-after evidence, a later branch rename or switch shall not alone prevent Resume or additional-session launch. The same owned checkout and repository slot must remain provable. The currently recorded branch must match the checkout even when it still names the repair-time branch. Completed-repair admission shall prove checkout identity without scanning file content or the staged index. Later commits and local file changes shall remain intact. Original repair evidence shall remain unchanged. This behavior shall also apply to completed repairs recorded before an upgrade.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.12:** Later branch changes shall not make an incomplete, negative, or internally inconsistent repair result usable. Every required repository slot shall pass admission independently. A safe changed-branch slot shall not conceal another slot with uncertain ownership or incomplete repair. Workspace admission shall neither resolve an uncertain prompt outcome nor resend the interrupted prompt.

The branch-continuity extension is tracked in the
[attested branch continuity package](../../../plans/attested-worktree-branch-continuity/plan.md).
Its work order records implementation and validation status.

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-008.13:** When a worktree launch requests a checkout branch that has no exact canonical slot, but exactly one active canonical row of the same repository holds a preserved worktree on that exact branch and no other requested slot owns that row, reuse shall bind the launch to that row's slot and reuse its worktree instead of failing closed; a different branch, an ambiguous, deleted, or failed holder, or a row owned by another requested slot shall still fail closed.

## Migrated source detail

This requirement was split out of
[Agent Resume and Runtime Recovery](agent-resume-runtime-recovery.md) so the
inventory-repair feature has its own vertical requirement/design pair, per
[`docs/specs/guide`](../../guide/) conventions.

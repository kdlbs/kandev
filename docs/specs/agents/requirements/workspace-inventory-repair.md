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

### REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004: Preserved workspace inventory repair

**Intent:** Recover a reusable checkout whose canonical inventory metadata drifted without changing the checkout.

#### Acceptance criteria

- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.1:** Normal workspace reuse shall fail closed when any required repository and branch slot lacks exactly one active canonical inventory row.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.2:** An authorized recovery shall repair one missing or stale row only when server-owned task, workspace, environment, repository, session, runtime, path, and Git metadata prove one reciprocal checkout identity.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.3:** Recovery shall preserve Git objects, refs, index state, tracked modifications, and untracked files byte-for-byte and shall return hashes that attest to the before and after state.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.4:** The same task-scoped idempotency key and payload shall return the existing receipt; the same key with a different payload shall conflict without mutation.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.5:** Duplicate, conflicting, cross-task, cross-workspace, wrong-repository, wrong-branch, wrong-environment, deleted, failed, symlinked, or ambiguously dirty evidence shall return a stable non-leaking result and preserve every resource.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.6:** A committed repair shall permit at most one orchestrator-owned resume or start attempt, and concurrent calls shall not create duplicate rows, receipts, sessions, or live writers.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.7:** Recovery shall never delete, clean, reset, reseed, rematerialize, or broadly discover a checkout, and a post-repair launch failure shall remain retryable and audited.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.8:** Unauthorized and unrelated-task callers shall receive no repository path, worktree path, branch, or cross-workspace existence information.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.9:** A same-idempotency-key retry after a committed repair shall return the stored receipt, including from a fresh or additional-session launch that reaches the same repair path automatically.
- **AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-004.10:** The stored receipt shall carry non-secret evidence captured both before the repair and after the repair transaction commits, so that a divergence between the two is itself an auditable result.

## Migrated source detail

This requirement was split out of
[Agent Resume and Runtime Recovery](agent-resume-runtime-recovery.md) so the
inventory-repair feature has its own vertical requirement/design pair, per
[`docs/specs/guide`](../../guide/) conventions.

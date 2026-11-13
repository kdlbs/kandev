---
id: "01-implementation"
title: "feat(runtime): add opt-in automatic execution recovery policies"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
acceptance_criteria:
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.2
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.3
  - AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# feat(runtime): add opt-in automatic execution recovery policies

## Outcome and scope

Expose independent default-OFF automatic start, native-history continuation and proven-unsent retry options; retain existing owner, recovery and admission checks.

Out of scope: Unknown-outcome replay, model or capacity changes, installation activation, and historical outcome rewriting.

## Implementation acceptance

1. The affected native entrypoint follows the linked requirement and design while preserving unrelated behavior and history.
2. Rejected, stale and failed operations retain their existing refusal or rollback semantics.
3. The focused implementation checks and linked documentation coverage have honest recorded results; unexecuted follow-up checks remain pending.

## Files and dependencies

The source files are the existing PR #4712 diff. This work order adds its delivery record and links existing requirements and system design; it introduces no runtime gate or permanent test.

- [System design](../../specs/platform/system-design/session-recovery-convergence.md)
- [Plan](plan.md)

## Verification and results

Validation:
- Selected existing Go checks passed across flags, profiles, task models, recovery/queue behavior, lifecycle environments and both Codex transports.
- Temporary port checks passed for automatic origin, retry spacing, current identity/candidate reads, authoritative ON/OFF environments, structured bootstrap causes and Windows DOS/UNC working-directory boundaries.
- 27 existing frontend tests, full TypeScript checking and all normal commit hooks passed. Original source archive-wire/native-evidence checks were retained as prior evidence; no permanent tests or CI jobs were added.

Documentation-only correction: git diff --check and the existing .github/scripts/pr-docs.cjs validateCoverage entrypoint with the candidate changed paths and the linked candidate documents supplied on stdin. No GitHub status is synthesized locally.

The public-port follow-up passed the existing primary-promotion, native-failure and bounded restart-queue selection (orchestrator package: 0.513s). The original source fixtures separately reproduced both primary-policy failures and both same-session action-join collisions, then passed after the source fixes; those are source-owner results rather than newly repeated public-port checks.

Current public-port command (from apps/backend):

```text
go test -tags fts5 -trimpath -p 2 ./internal/orchestrator -run "^(TestPrimaryPromotionAfterStop|TestNativeResumeStartFailureLeavesUnknownDeliveryUnacknowledged|TestAgentStoppedAfterNativeResumeStartFailureDoesNotCompleteTurn|TestSilentRestorePass|TestSilentRestoreQueue|TestSilentRestoreFocusUsesTheSharedBoundedQueue)" -count=1
```

Result: PASS (0.513s). Normal commit hooks must pass before publication.

---
status: done
created: 2026-10-23
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# feat(runtime): add opt-in automatic execution recovery policies

Expose independent default-OFF automatic start, native-history continuation and proven-unsent retry options; retain existing owner, recovery and admission checks.

## Delivery order and dependencies

The implementation is one existing PR slice. Preserve its native dependencies and existing feature defaults, then verify the narrow affected paths. No dependent PR is imported into this package.

- [Implementation and verification](task-01-implementation.md)

## Scope and risks

Unknown-outcome replay, model or capacity changes, installation activation, and historical outcome rewriting. These remain outside this delivery slice. Preserve existing files, configuration and history; report unavailable integration coverage without inventing a successful result.

## Verification strategy

Reuse the completed focused implementation checks recorded below and the existing documentation-coverage evaluator. Only remaining review fixes require their affected checks to run again.

Current public-port command (from apps/backend):

```text
go test -tags fts5 -trimpath -p 2 ./internal/orchestrator -run "^(TestPrimaryPromotionAfterStop|TestNativeResumeStartFailureLeavesUnknownDeliveryUnacknowledged|TestAgentStoppedAfterNativeResumeStartFailureDoesNotCompleteTurn|TestSilentRestorePass|TestSilentRestoreQueue|TestSilentRestoreFocusUsesTheSharedBoundedQueue)" -count=1
```

Result: PASS (0.513s). Normal commit hooks must pass before publication.

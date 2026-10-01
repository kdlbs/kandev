---
id: "05-safe-live-adoption"
title: "Reattach surviving agents without initialization or destructive cleanup"
status: done
wave: 5
depends_on: ["04-persistent-recovery-ui"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-004
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-005
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-007
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-004.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-005.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.3
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.1
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-007.6
system_design:
  - ../../specs/platform/system-design/durable-agent-reattachment.md
---

# Task 05: Reattach surviving agents without initialization or destructive cleanup

## Summary

Reattach surviving agents without initialization or destructive cleanup. Preserve original ownership and the no-resend contract.

## In scope

Carry created_by_attempt versus reattached_existing disposition and immutable cleanup ownership through startup success and error results.
Replace reuseExisting-to-initializeAgentSession routing with authenticated association verification and the shared reconciler.
Verify original native session, owner, generation, stream, and capability; preserve existing submission/turn.
A proven initialized survivor receives no ACP initialize/load/new or prompt. Unknown association blocks without force-stop.
Only genuine creation failures may invoke new-start process teardown. Reattach failure may dispose its own tunnel/client only.
Audit executor_execute.go, earlier startup errors, timeout paths, REVIEW writes, cancellation, and delayed cleanup after a successor.
Retain explicit Stop authorization. Support positive legacy association or report unsupported reattachment safely.
Add transport-survivor integration coverage without implementing contributor-owned executor redial policy.

## Out of scope

Other work orders, contributor-owned executor redial, long-horizon scheduling, and detached MCP policy.

## Acceptance

- All named regressions exercise the actual owning boundary and pass.
- No stale owner, unrelated recovery cause, or automatic resend crosses the repaired path.
- Partial failure remains visible, bounded, and restart-reconcilable without deleting retained evidence.

## Tests

TestReuseExistingAdoptsWithoutInitialize records every ACP call to a busy survivor and asserts no initialize/load/new/prompt.
TestReattachFailurePreservesSurvivor forces timeout/auth/identity failure and checks the pre-existing process remains alive.
TestCreatedAttemptFailureStillCleansUp proves the ownership distinction does not leak newly created processes.
TestLateReattachCleanupPreservesSuccessor races old failure and successor ownership.
Extend the Task 04 browser specs to resume a live survivor through the actual reuse path, preserve output, and avoid REVIEW/duplicate completion.

## Verification

Run from repository root after implementation. All commands are independently rooted.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator/executor ./internal/orchestrator ./internal/agentctl/server/api -count=1)
make -C apps/backend lint
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-recovery-actions-guard.test.ts)
(cd apps/web && pnpm lint)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/durable-reattachment.spec.ts tests/session/durable-stream-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-durable-reattachment.spec.ts tests/session/mobile-durable-stream-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/manager_startup.go`
- `apps/backend/internal/agent/runtime/lifecycle/manager_launch.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_adoption.go`
- `apps/backend/internal/agent/runtime/lifecycle/types.go`
- `apps/backend/internal/orchestrator/executor/executor_execute.go`
- `apps/backend/internal/agent/runtime/lifecycle/executor_standalone.go`

Add the named regression files beside the owning source packages.

## Dependencies

Task 04: Persist truthful session recovery and render it on desktop and phone.

## Risks

Concurrent old events and new ownership can race settlement or cleanup. Use immutable identity and compare-and-set at the mutation boundary.

## Parallelism

`sequential`

## Inputs

- [Manifest](plan.md).
- [Requirements](../../specs/platform/requirements/durable-agent-delivery.md).
- [Design](../../specs/platform/system-design/durable-agent-reattachment.md).

## Results

Completed 2026-09-28.

- Existing-agent startup now verifies authenticated owner, generation, session, native session, stream, submission, and negotiated durable capability before restoring state and attaching the updates stream. It does not issue ACP initialize/load/new or prompt dispatch for a proven initialized survivor.
- Startup disposition follows ownership through errors and cancellation. Reattachment failure disposes only its own client/tunnel and preserves the peer and task session. Newly created startup failures retain their cleanup path; late cancellation cannot stop a reattached successor.
- Lifecycle and executor regressions passed under `go test -race`, including `TestReuseExistingAdoptsWithoutInitialize`, identity mismatch preservation, reattachment failure preservation, and late cancellation. The positive new-start cleanup case passed through `TestStartAgentProcessAsyncNotifiesAfterProcessStartFailure`.
- Desktop survivor-restart E2E passed 2/2 with `pnpm e2e:run --host --no-build --project chromium tests/session/agent-survival-restart.spec.ts`; mobile passed 1/1 with `pnpm e2e:run --host --no-build --project mobile-chrome tests/session/mobile-agent-survival-restart.spec.ts`. They exercise the live reuse path, retain the original session, avoid duplicate completion/output, and allow follow-up interaction.
- The focused race command passed all matching tests in lifecycle, orchestrator, executor, and SQLite; the agentctl API package compiled without a matching test:

  ```bash
  go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator ./internal/orchestrator/executor ./internal/task/repository/sqlite ./internal/agentctl/server/api -run "^(TestDeliveryReconcile.*|TestReuseExisting.*|TestRunAgentProcessAsync_.*|TestStartAgentProcessAsync.*|TestReplayed.*|TestDeliveryTerminal.*|TestDeliverySettlement.*|TestDeliveryRecoveryBlocks.*|TestAgentctlDisconnectPersistsRecoveryAndPublishesWithoutSettlingSession|TestAgentDeliveryRecovery.*|TestDispatchPromptPreservesSubmission.*|TestDispatchPromptRejectsCallbackManagerWithoutSubmissionCapability|TestExistingWorkspaceStart_BindsInitialDeliveryBeforeAdmission)$" -count=1
  ```

- `make -C apps/backend lint`, Web recovery tests/lint/typecheck/i18n checks, desktop/mobile durable-recovery E2Es, and docs/spec validation passed. PostgreSQL was unavailable and native Windows/macOS process tests remain release gates.
- No executor redial policy, remote reachability scheduler, or detached MCP wait/offline policy was added. No changes were committed.

### CI remediation, 2026-10-01

- Lazy execution creation now forwards the retained journal root and stable session owner, falling back to the task environment owner when no session is bound. Backend restart/resume browser validation passes.
- The concrete backend lifecycle adapter forwards initial delivery submission binding. Its optional-interface regression and Monitor browser validation pass.
- The focused race command recorded in task 03 passes. Current-head CI remains pending publication.

- CI reproduced forced deletion of established Docker environments after recoverable agent failure. Lifecycle now preserves that environment for authorized resume; bootstrap rollback, deletion, and explicit force remain destructive. Passed `go test -race -tags fts5 ./internal/agent/runtime/lifecycle -run 'TestDockerRecoverableFailureRetains|TestKubernetesRecoverableFailure|Test.*Docker.*Stop|Test.*StopAgentWithReason' -count=1`. All three real-container browser regressions passed with `KANDEV_E2E_CONTAINERS=1 pnpm e2e:raw --project=containers e2e/tests/docker/docker-launch.spec.ts --grep 'externally stopped|external stop' --retries=0 --reporter=line`. Final backend lint passed; public executor guidance was updated.

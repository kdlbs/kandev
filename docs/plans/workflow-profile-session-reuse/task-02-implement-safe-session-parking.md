---
id: "02-implement-safe-session-parking"
title: "Route source and destination behavior"
status: pending
wave: 2
depends_on:
  - "01-add-portable-workflow-policy"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-PROFILE-SESSIONS-001
acceptance_criteria:
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.1
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.2
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.3
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.4
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.5
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.6
  - AC-TASKS-WORKFLOW-PROFILE-SESSIONS-001.11
system_design:
  - ../../specs/tasks/system-design/workflow-profile-session-lifecycle.md
---

# Task 02: Route Source and Destination Behavior

## Summary

Use the destination step for session selection and the source step for session
retirement.

## In scope

- Split profile-switch routing into start and end inputs.
- Thread the source step or normalized end setting through every entry path.
- Keep credential preflight, promotion, queue rollback, parking, stop intent,
  guard release, and callback suppression.
- Cover legacy, manual, queued, and direct engine transitions.

## Acceptance

- Tests prove all four start-and-end combinations.
- A destination setting cannot change source retirement.
- The workflow engine core gains no action, event, or state.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/orchestrator -run 'Test(SwitchSessionForStep|PrepareWorkflowStepSession|HandleAgentCompleted|HandleAgentStopped|HandleAgentFailed).*ProfileSession' -count=1 -v
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium tests/workflow/queued-session-ownership.spec.ts -- --retries=0 --repeat-each=3
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/workflow/mobile-queued-session-ownership.spec.ts -- --retries=0 --repeat-each=3
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome tests/task/mobile-task-create-workflow-step-previews.spec.ts -- --grep 'touch scrolls ten workflow options and selects either end' --retries=0 --repeat-each=3
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_workflow.go`
- `apps/backend/internal/orchestrator/workflow_callbacks.go`
- `apps/backend/internal/orchestrator/event_handlers_workflow_profile_session_policy_test.go`
- `apps/backend/internal/orchestrator/workflow_profile_session_lifecycle.go`

## Dependencies

Task 01.

## Risks

A direct engine entry can lose the source step after the task moves.

## Parallelism

`sequential`

## Results

### CI regression follow-up, 2026-10-03

Hosted queue-ownership failures showed that an `agent.failed` callback from the
stream closed by a deliberate profile-switch stop bypassed the exact execution
teardown claim. That callback created a recovery message for a parked session.
The handler now consumes only the matching park stop intent or teardown owner,
retires that execution, and suppresses generic session-failure handling. A
different execution cannot consume the stop intent. The owning requirement and
design now state that this is an intentional parked outcome; no storage or
protocol change was needed.

The matching regression failed before the handler change and passes afterward.
The test also verifies the turn remains open, the session remains
`WAITING_FOR_INPUT`, and no recovery message appears. The mismatch test verifies
that a successor execution leaves the old stop intent untouched. The focused
profile-session test command and the orchestrator package suite passed. Changed
scope Go lint reported zero issues.

All three failed hosted E2E cases passed three repetitions with retries
disabled: desktop queue ownership, mobile queue ownership, and the mobile
workflow-preview scroll case. The preview assertion again checks that expected
step names appear in order while allowing additional configured steps.

```bash
cd apps/backend && go test ./internal/orchestrator -run 'TestHandleAgentFailed_ProfileSessionStopIntent' -count=1
cd apps/backend && go test ./internal/orchestrator/... -count=1
cd apps/backend && golangci-lint run ./internal/orchestrator/... --timeout=5m
cd apps/web && pnpm lint
cd apps/web && pnpm exec prettier --check e2e/tests/task/workflow-step-previews-helpers.ts
```

### Live-main rebase and CI follow-up, 2026-10-03

Rebased the branch onto `origin/main` at `a81c68fe838a`. Conflict resolution
preserved both native-restore identity and live-agent startup disposition in
the lifecycle execution, and kept both explicit cancellation ownership and
exact resume-startup termination. In the executor start path, a failed live
reattachment continues to avoid newly-created-start cleanup; native restore
still returns its actual startup error to its recovery owner. The rebase also
exposed an outdated test call after the restore API gained its execution
argument; that test now supplies the expected nil execution.

The workflow preview-title matcher and mobile fixture passed again after the
rebase. The mobile preview scroll case and mobile workflow-agent switch each
passed three repetitions with retries disabled. The hosted dynamic-unclassified
fallback case passed once and then three further repetitions with retries
disabled. A first overlapping Go package run exposed one lifecycle cache-prune
temporary-directory cleanup failure; the case passed five isolated repetitions,
the lifecycle packages passed in isolation, and the complete targeted recovery
suite then passed without concurrent runs.

```bash
cd apps/backend && go test ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... ./internal/agent/runtime/agentctl/... -count=1
cd apps/backend && golangci-lint run ./internal/orchestrator/... ./internal/agent/runtime/lifecycle/... ./internal/agent/runtime/agentctl/... --timeout=5m
cd apps && pnpm --filter @kandev/web lint
cd apps/web && pnpm run typecheck && pnpm run e2e:sleep-ratchet && pnpm run i18n:ratchet
cd apps/web && pnpm exec vitest run components/workflow-selector-row.test.tsx
cd apps/web && pnpm exec prettier --check e2e/tests/task/workflow-step-previews-helpers.ts components/workflow-selector-row.tsx
cd apps/backend && make build && make e2e-plugin-package
cd apps && pnpm --filter @kandev/web build:vite
```

The targeted Go suite, changed-scope lint, web lint/typecheck/build, selector
unit tests (9), E2E ratchets, formatting, backend/E2E artifact builds, and all
three browser regressions passed locally. New-head hosted CI and review status
remain pending publication; this local evidence does not claim hosted CI green.

### PR #3598 exact-head CI fixup, 2026-10-03

The previous hosted head exposed two independent recovery failures and a
browser layout failure. ACP reports a transient `session/load` network error
as a generic `-32603` error, so it did not carry the typed restore code. The
restore policy now treats only high-confidence `network_unavailable` evidence
with the short-retry decision as transport failure; other untyped and provider
failures remain blocked. Automatic continuation prompts now receive a unique
durable delivery-submission ID before dispatch, allowing SQL and a retained
agentctl to agree on the active submission during restart adoption. The dialog
layout test closes its option menu with Escape instead of clicking a possibly
disabled option.

Validation passed: both targeted Go regression suites; complete lifecycle and
orchestrator package suites; changed-scope Go lint (0 issues); the modified
E2E file's ESLint and Prettier checks; backend and E2E plugin artifact builds;
and `git diff --check`. The two affected continuation/restart browser cases
passed together (2/2) with retries disabled, and the long-text dialog browser
case passed (1/1) with retries disabled. The transient case preserved its
native conversation and did not replay the original prompt. These are local
results only; fresh hosted CI and review evidence remain pending publication.

```bash
cd apps/backend && go test ./internal/agent/runtime/lifecycle -run 'TestRestorePolicy(UsesTypedAgentctlReason|RecognizesTransientUnstructuredACPTransportError)|TestInitializeSessionNativeLoadFailure' -count=1
cd apps/backend && go test ./internal/orchestrator -run 'TestInterruptionContinuationPreparation' -count=1
cd apps/backend && go test ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... -count=1
cd apps/backend && golangci-lint run ./internal/agent/runtime/lifecycle/... ./internal/orchestrator/... --timeout=5m
cd apps/web && pnpm exec eslint e2e/tests/task/dialog-long-text-overflow.spec.ts
cd apps/web && pnpm exec prettier --check e2e/tests/task/dialog-long-text-overflow.spec.ts
cd apps/backend && make build && make e2e-plugin-package
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/session/provider-interruption-continuation.spec.ts -- --grep 'read-restore-transient restores|backend restart, agent survival=true' --retries=0
cd apps/web && pnpm e2e:run --host --no-build --shards 1 --project chromium e2e/tests/task/dialog-long-text-overflow.spec.ts -- --retries=0
git diff --check
```

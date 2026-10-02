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

### CI remediation, 2026-10-02

- A newly authorized launch without an explicit message submission uses an execution-bound initial submission identity. Native restore preserves the harness generation while distinct authorized launches cannot collide in the retained journal. Legacy initial submission identities remain recognizable during adoption.
- The launch-identity regression failed before the fix. The complete lifecycle, process, and task-service packages passed with the race detector, using the command recorded in task 03.
- Added Korean translations for the existing recovery surfaces after main introduced that locale. Complete locale validation passes; composition and interaction remain unchanged. Published-head CI verification remains pending.


### Adjacent browser CI regression (2026-10-02)

E2E Shard 3/14 on `a71fe177551` (run `36944067239`, job `110654853231`)
reported 258 passing tests, three existing skips, and one strict flaky failure.
The Git changes Revert test exposed its hover-only action, then Playwright's
click scroll removed hover. The action stayed hidden until the 60-second
fixture deadline. Its retry passed, which still fails the strict CI policy.

The fixture now centers the row before hovering. It retains the same real
button click, commit-removal assertion, and staged-file assertion. Product
components, mobile composition, operation semantics, timeouts, retry policy,
and skip policy are unchanged. This is test geometry preparation for the
existing desktop pointer interaction, not a new UI contract.

Commands from `apps/web`:

- `GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host
  --project chromium tests/git/git-changes-panel.spec.ts -- --grep
  'revert commit undoes commit and stages changes' --repeat-each=3 --retries=0`
  passed: three tests in 33.1 seconds, with a managed fresh build.
- `GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host
  --no-build --project chromium tests/git/git-changes-panel.spec.ts --
  --retries=0` passed: 27 tests in 4.4 minutes.
- `pnpm exec eslint e2e/tests/git/git-changes-panel.spec.ts --max-warnings 0`
  passed.

Other current-head hosted checks were still finishing when these local results
were recorded. The final CI outcome belongs to the exact subsequently pushed
head and is recorded in the task plan and PR checks.

### Remaining browser-shard remediation, 2026-10-02

The first complete browser run on `a71fe1775513` reported three flaky cases in
shards 3 and 6. Strict flaky-test failure remains enabled. The Revert hover case
is recorded above. Shard 6 also exposed two missing fixture checkpoints:

- The right-pane maximize/reload fixture selected Files and immediately entered
  maximize before its debounced layout save was observable. It now waits on the
  existing persisted-layout helper for the selected Files view, then performs the
  original maximize, disabled-control, exit, reload, visibility, and healthy-layout
  assertions. It does not activate Files or toggle the pane after reload.
- The mobile PR-only fixture mutated native Git history before proving the agent
  had settled or the local commit snapshot had arrived. It now waits for the
  existing quiet-session completion barrier and observes the shared commit through
  the production mobile Changes panel before installing the provider-only overlay.
  The remote-only SHA remains absent from native Git, and pushed provenance,
  warning absence, remote detail, patch, and sheet-dismissal assertions remain.

All 70 existing commits were rebased without content conflicts onto verified main
`68542f03983a56b9c9c42fd1afed10842e1beff0`. Before these two checkpoint changes,
both reported cases passed three repetitions with zero retries against that main;
the actual hosted shard failures supply the red evidence for the fixture races.
The owning desktop file then passed all 10 tests with zero retries:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/layout/right-panel-visibility.spec.ts -- --retries=0
```

The rebased Git-status handler tests passed 22 tests in one file. Browser setup
changes preserve the existing desktop/mobile interactions and do not change product
UI composition or weaken recovery contracts.

The owning mobile Changes file passed all 9 tests with zero retries:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --retries=0
```

Zero-warning ESLint for all three changed browser fixtures, Prettier checks,
full specification lint, and whitespace validation passed. Native Windows
journal throughput is now proven by the hosted `a71fe1775513` run; native
desktop containment/OS smoke and a live harness version matrix remain distinct
release validation limits. Final pushed-head CI is recorded in the task plan.

The final checkpoint changes also passed three repetitions each with zero retries:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-changes-panel.spec.ts -- --grep 'PR-only commit opens the remote commit sheet' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/layout/right-panel-visibility.spec.ts -- --grep 'keeps the toggle disabled while maximized' --repeat-each=3 --retries=0
```

Both commands passed 3/3. Catalog validation also passed after the result updates.


### Additional current-head fixture readiness, 2026-10-02

Hosted head `21392d854d8` exposed four fixture readiness failures across browser
shards 3, 9, and 12. Strict flaky-test enforcement remains enabled. The workflow
picker fixture now uses the deterministic response used by its desktop counterpart,
preserving all primary, parked-session, selection, touch-target, and overflow checks.
The profile-warning fixture waits for the host catalog probe to complete before
loading the page. The commit-body fixture waits for the exact session to settle
and modifies its persisted workspace path. The nested mobile Review fixture waits
for root and both nested README sections before asserting their repository labels,
preserving the original diff, sticky-header, and viewport checks.

The pre-change desktop probe/commit cases and mobile nested case passed locally;
the hosted failures provide red evidence. The mobile workflow baseline passed
three zero-retry repetitions in 31.6–50.5 seconds. All 72 existing commits then
rebased without conflicts onto main `0ec0538aa038f2e8b8617fb4f5be6128f0cedbac`.
The updated mobile workflow and nested Review each passed three zero-retry
repetitions (six tests total). Targeted zero-warning ESLint and whitespace checks
passed. Exact commands:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/review/mobile-submodule-review.spec.ts tests/workflow/mobile-workflow-agent-switch.spec.ts -- --repeat-each=3 --retries=0
pnpm exec eslint --max-warnings 0 e2e/tests/git/git-commit.spec.ts e2e/tests/settings/pr3473-qa-visual-capture.spec.ts e2e/tests/workflow/mobile-workflow-agent-switch.spec.ts e2e/tests/review/mobile-submodule-review.spec.ts
```

The first desktop repetition run passed five tests and failed once during backend
fixture startup, before test assertions. A diagnostic rerun and final pushed-head
CI outcomes are recorded below or in the task plan.

The diagnostic desktop rerun passed all six tests (three repetitions of each
case), zero retries, in 50.5 seconds. The earlier startup failure did not recur;
its cause remains unconfirmed. This rerun enabled only existing fixture logging:

```bash
cd apps/web
E2E_DEBUG=1 GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/settings/pr3473-qa-visual-capture.spec.ts tests/git/git-commit.spec.ts -- --grep 'captures desktop warning|commit dialog includes body' --repeat-each=3 --retries=0
```

Hosted shard 10 also reported a native conversation-fork setup race: idle controls
were visible before the browser store had an assistant message with a turn ID.
The shared desktop/mobile helper now waits for the exact backend session to settle
and for that session's assistant turn to hydrate before applying fixture-only native
metadata. The real fork confirmation, selected-session, geometry, and overflow
assertions remain unchanged. The pre-change mobile case passed locally; the hosted
failure supplies red evidence. The updated desktop case passed three zero-retry
repetitions in 22.3 seconds. Targeted ESLint passed.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/chat/codex-app-server.spec.ts -- --repeat-each=3 --retries=0
```

The updated mobile fork case also passed three zero-retry repetitions in
22.9 seconds:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/chat/mobile-codex-app-server.spec.ts -- --repeat-each=3 --retries=0
```

Catalog validation and full specification lint passed after these records.

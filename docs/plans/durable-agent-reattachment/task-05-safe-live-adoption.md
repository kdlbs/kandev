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

### Refreshed main integration checks, 2026-10-02

A complete local Git/history integration run passed 30 tests and exposed two
further fixture failures with zero retries. The Amend action had the same
click-time auto-scroll/hover loss as Revert; centering the row before physical
hover preserves the real click and updated commit-message assertions. Main's
new header geometry test assumed its default 280px pane could fit the full
Local checkout commits label on one line. Diagnostic measurement observed a
33px wrapped row, with matching control/wrapper bounds and no top offset.
The fixture now uses the existing Dockview resize helper to give the single-line
28px assertions a 400px pane. Its narrower-viewport dynamic wrapping, refresh,
Changes reopen, descendant adjacency, and overflow checks remain unchanged.
Temporary geometry logging was removed. No product styling changed.

Both updated cases passed three repetitions each, zero retries (six passes in
59.7 seconds). Targeted zero-warning ESLint, formatting, and whitespace passed:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/git/changes-history-regression.spec.ts tests/git/git-changes-panel.spec.ts -- --grep 'keeps collapsed history headers compact|amend commit updates commit message' --repeat-each=3 --retries=0
pnpm exec eslint --max-warnings 0 e2e/tests/git/changes-history-regression.spec.ts e2e/tests/git/git-changes-panel.spec.ts
```

Hosted head 31c0f263f9e failed trusted walkthrough context preparation after
three minutes. The already validated separate PR #4147 fixes that trusted
main-owned helper; its fresh snapshot has no failed/pending checks or unresolved
threads. That snapshot is historical: the user subsequently authorized merging only
PR #4147. Its verified merge commit is d22fbaab1fcdf35add3272c0a6fcafebb4ed8e2b.
PR #3598 remains open; an all-green outcome is not claimed.


### Post-helper main reconciliation, 2026-10-02

Rebased onto main including the authorized helper merge and Windows process
startup changes. The Windows conflict resolution retains both `HideWindow`
and suspended startup before Job Object assignment. Existing tests for both
invariants remain. Windows launcher cross-compilation passed:

```bash
cd apps/backend
GOOS=windows GOARCH=amd64 GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -c -o /tmp/kandev-launcher-conflict-windows-oct2.test.exe ./internal/agent/runtime/agentctl/launcher
```

The rebase exposed a missing startup-attempt argument in synchronous resume
failure rollback. Supplying an empty identity to make the old behavior compile
reproduced an owned attempt left in STARTING. The regression now covers restoration
of the prior WAITING_FOR_INPUT state and preservation of a successor attempt.
Passing the current attempt identity fixes rollback while retaining its ownership
fence. The first green run exposed an incorrect FAILED expectation in the new
fixture; correcting it to the existing prior-state contract yields a passing
resume/workspace race suite. Scoped Go lint reports zero issues.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race ./internal/orchestrator/executor -run 'Test.*(Resume|WorkspaceBinding)' -count=1
GOCACHE=/tmp/kandev-go-build-preserved-20261001 golangci-lint run ./internal/orchestrator/executor/...
```

Hosted browser failures exposed setup races in file transfer, reused workflow
sessions, live settings persistence and the Office negative refetch baseline.
Fixtures now wait for exact session settlement, both initial Git reads, accepted
configuration writes and initial page hydration. The shared Git readiness helper
is moved from existing navigation coverage. Original behavior and geometry
assertions remain; no timeout or retry allowance was increased.

The desktop batch passed twelve cases across four fixtures (three repetitions
of each), with three failures caused by an incorrect model-write endpoint in the
new wait. Correcting it to the actual ACP config-option endpoint passed all three
reset-setting repetitions. Zero-warning targeted ESLint and formatting passed.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/workflow/workflow-step-proceed.spec.ts tests/office/realtime-dashboard.spec.ts tests/task/workspace-file-transfer.spec.ts tests/task/create-task-workflow-agent-overrides.spec.ts tests/task/task-navigation-responsiveness.spec.ts -- --grep 'preserves session settings|does not refetch on cross-workspace|uploads picked files|keeps grouped replacements|Files stays usable' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/workflow/workflow-step-proceed.spec.ts -- --grep 'preserves session settings' --repeat-each=3 --retries=0
```

A fresh mobile build passed all nine session-refresh cases, zero retries.
The three file-header repetitions exposed an accidentally removed `node:path`
fixture import. Restoring it preserves directory and filename assertions.
The corrected file-header rerun passed three repetitions in 29.8 seconds,
zero retries. Current-head hosted CI remains pending.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/review/mobile-review-file-status.spec.ts tests/session/mobile-session-refresh-efficiency.spec.ts -- --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/review/mobile-review-file-status.spec.ts -- --repeat-each=3 --retries=0
```
Native Windows/macOS containment smoke and live harness relocation coverage
remain release evidence limits; cross-compilation does not replace native checks.


The final main refresh incorporated literal Git path handling and bounded archive
manifest cleanup without conflicts. All four affected backend package race suites
passed (executor 6.9s, process 182.7s, task service 103.6s, worktree 101.2s).
A fresh managed desktop build then passed all five affected cases, zero retries,
in 1.3 minutes. Normal commit hooks passed without bypass. Hosted checks remain
pending until the rebased head is published and validated.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race ./internal/orchestrator/executor ./internal/agentctl/server/process ./internal/task/service ./internal/worktree
cd ../web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/workflow/workflow-step-proceed.spec.ts tests/office/realtime-dashboard.spec.ts tests/task/workspace-file-transfer.spec.ts tests/task/create-task-workflow-agent-overrides.spec.ts tests/task/task-navigation-responsiveness.spec.ts -- --grep 'preserves session settings|does not refetch on cross-workspace|uploads picked files|keeps grouped replacements|Files stays usable' --retries=0
```


### Further hosted fixture remediation, 2026-10-02

Hosted checks on the preceding head passed the trusted walkthrough helper,
Linux backend/static checks, both PostgreSQL databases and native Windows build,
vet and all targeted tests. Windows was cancelled during setup-go's cache archive
after tests completed, at the existing 40-minute job limit. GitHub rejected a
rerun while its aggregate was queued; the same-head rerun was accepted after
that workflow finished. A cancelled job is not reported as passing CI.

Four browser shards exposed one flaky case each. The short-phone question's saved
page recorded its real MCP call timing out after one minute. Its fixture now
creates the neighboring task first and the target last, preserving the existing
MCP timeout, native touch actions, clipping assertions and exact answer receipts.
Both phone sizes passed three repetitions each, zero retries, in 1.7 minutes.
The pre-change short-phone baseline passed locally; hosted evidence supplies red.

The workflow shard failed during fixture reset with exactly `session transfer in
progress`. The existing four-attempt reset poll now recognizes that exact 500
JSON response as transient cleanup ownership. New unit coverage reproduced the
old failure, then passed transient success, persistent four-attempt exhaustion
and immediate rejection of unrelated failures. All eleven helper tests passed.
No prompt submission or production retry behavior changed.

The PR switcher inherited earlier LSP checkout history. It now resets the owned
seed checkout to its baseline before creating the PR commit and uses the declared
repository path. The mobile symlink fixture waits for initial Git hydration before
opening its action menu. The PR and both adjacent workflow cases passed nine
repetitions total, zero retries, in 1.4 minutes. Mobile symlink verification passed three repetitions, zero retries, in 19.7s. Scoped ESLint passed; no test retry allowance or timeout was increased.

```bash
cd apps/web
pnpm exec vitest run --config vitest.config.ts e2e/helpers/api-client.test.ts
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-threads-composer-disclosure.spec.ts -- --grep 'scrolls and submits a long required question' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/pr/pr-switcher-changes.spec.ts tests/workflow/workflow-agent-switch.spec.ts -- --grep 'shows correct PR data|manual step move updates chat UI|on_turn_start transition' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/git/mobile-symlink-identification.spec.ts -- --repeat-each=3 --retries=0
```


### Latest startup-recovery main integration, 2026-10-02

Rebased onto main's standalone listener and startup-failure explanations changes.
Recovery-hook conflicts preserve main's current-request result fence and apply it
to context continuation details. Canvas conflicts retain source cleanup in a
finally block and the existing branch publication completion checks. Markdown
conflicts retain both repository selection and post-load setup; those options
share one argument to satisfy the five-parameter lint limit.

Six focused frontend files passed all 99 tests, including recovery actions,
presentation, service and reset helpers. Full affected runtime/lifecycle and
orchestrator race suites passed. A fresh managed desktop build passed four
conflict checks in 51.1s, zero retries; mobile failure-history recovery passed in
12.3s, zero retries. Conflict-file ESLint, formatting, catalog/specification lint
and whitespace passed. Current-head hosted CI remains pending after publication.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race ./internal/agent/runtime/agentctl/... ./internal/agent/runtime/lifecycle ./internal/orchestrator/...
cd ../web
pnpm exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts lib/active-session-recovery.test.ts lib/session-recovery-presentation.test.ts lib/session-last-agent-error.test.ts lib/services/session-recovery-service.test.ts e2e/helpers/api-client.test.ts
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/chat/markdown-preview.spec.ts tests/canvas/plugin-canvas.spec.ts tests/task/launch-failure-recovery.spec.ts -- --grep 'open markdown preview from diff|first visual row|reconciles a published task canvas|retains session failure' --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts -- --grep 'retains session failure history' --retries=0
```


### Windows CI job budget correction, 2026-10-02

Two successive hosted heads passed every Windows build, vet and native test
step but were cancelled during setup-go cache publication at the 40-minute
workflow cap. On the latest run, native checks took 28 minutes including
compilation, and final targeted tests ended at 39m38s. Only 25 seconds remained
for cache publication. The Windows job now has a finite 50-minute total budget.
Its 25-minute package timeout, assertions, test selections and all browser
timeouts/retry allowances remain unchanged.

The existing job-headroom workflow contract was changed first and failed RED.
The configuration correction passed all ten workflow contract tests and action
pinning lint. Hosted validation is pending on the next published revision.

```bash
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
```

### Current-main integration, 2026-10-02

Rebased onto main `341f8941376e29f21b0873ae94a989cf2a595c58`, including
Changes-loading feedback, Git-read retry behavior, and Go artifact reuse.
Japanese and Korean catalog conflicts preserve both Git and recovery copy;
strict duplicate-key validation passed. History fixture reconciliation retains
exact 28px checks in a 400px pane, dynamic narrow-pane wrapping checks, and
main's separate residual-spacing assertions. PR-switcher fixture checkout
isolation remains intact.

Validation: 24 Git-refresh/Changes unit tests across five files passed;
localization checks, web typecheck, full web lint, scoped fixture lint, and
Makefile shell checks passed. Three desktop browser cases (compact history,
residual spacing, PR switching) and two mobile history cases passed with zero
retries. Existing native runtime/recovery checks from the preceding integration
remain recorded above; these checks do not replace native OS containment smoke
or live harness relocation validation.

```bash
cd apps/web
pnpm exec vitest run hooks/domains/session/use-session-git-refresh.test.tsx hooks/domains/session/git-status-refresh-coordinator.test.ts components/task/changes-panel-refresh-status.test.tsx components/task/mobile/mobile-changes-panel.test.tsx components/task/changes-panel-header.test.tsx
pnpm run i18n:check
pnpm run typecheck
pnpm run lint
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/git/changes-history-regression.spec.ts tests/pr/pr-switcher-changes.spec.ts -- --grep 'keeps collapsed history headers compact|preserves residual history spacing|shows correct PR data' --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/git/mobile-changes-history-regression.spec.ts tests/git/mobile-changes-panel-refresh-recovery.spec.ts -- --grep 'preserves residual PR spacing|keeps collapsed history|retry' --retries=0
cd ../..
scripts/check-make-shells
```

### Hosted browser follow-ups, 2026-10-02

The preceding hosted head exposed three fixture assumptions. Conditional-read
coverage compared a 304 with the first 200 despite intervening session metadata
updates. It now requires that the conditional request's exact validator was
observed on a full response, and that the 304 returns that same validator.
The mobile host-model warning fixture now waits for the advertised catalog,
matching the existing desktop prerequisite. File-transfer fixtures open their
known task ID directly; the failed artifact showed the task in the sidebar but
an empty filtered home board.

The complete conditional-read suite passed nine cases across three runs;
the complete file-transfer suite passed fifteen cases across three runs.
Retries were disabled. The separate hosted workflow-switch recovery failure
was inspected and reproduced three times with backend diagnostics: all three
passed with unchanged lifecycle assertions. Its hosted outcome remains pending;
no speculative lifecycle change or timeout increase was made.

Go cache integration checks also passed: `go test -trimpath ./internal/testutil`
and Makefile shell-dispatch checks. A freshly rebuilt mobile Git-recovery test
passed, preserving pending diff selection through automatic recovery.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -trimpath ./internal/testutil
cd ../web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts -- --grep 'retries automatically after failure' --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/session/session-refresh-efficiency.spec.ts -- --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/task/workspace-file-transfer.spec.ts -- --repeat-each=3 --retries=0
E2E_DEBUG=1 GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project chromium tests/task/change-workflow.spec.ts -- --grep 'updates the open task stepper after changing workflow' --repeat-each=3 --retries=0
```

The mobile warning follow-up passed three runs with zero retries:

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-pr3473-qa-visual-capture.spec.ts -- --repeat-each=3 --retries=0
```

### Inventory preservation integration, 2026-10-02

Rebased onto main `14d473d3e0cc747ab3cb9e8455da812a665d6b90`, which adds
preserved workspace inventory repair. Merged recovery requests retain inventory
idempotency, receipt propagation, guarded preflight admission, native-only
inventory repair, explicit context-continuation authority, checkpoint settlement,
and the independent retry-connection action. PostgreSQL retains main's 20-minute
package deadline and this branch's 40-minute total setup/test/cleanup budget.

A compile check caught a stale error variable after conflict resolution; it was
corrected to preserve joined preflight/settlement errors. Validation passed:
executor and remaining orchestrator subpackage race tests, lifecycle race tests
(100.893s), orchestrator race tests (123.290s), handlers race tests (6.944s),
changed-scope orchestrator lint (zero issues), and all ten workflow contract tests.
Freshly rebuilt browser coverage passed conditional revalidation, workflow
switching, and file upload (three cases, zero retries).

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race ./internal/orchestrator/... ./internal/agent/runtime/lifecycle
# Re-run packages that failed compilation after correcting the merge variable:
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -race ./internal/orchestrator ./internal/orchestrator/handlers
GOCACHE=/tmp/kandev-go-build-preserved-20261001 golangci-lint run ./internal/orchestrator/... --new-from-rev=14d473d3e0cc747ab3cb9e8455da812a665d6b90 --timeout=5m
cd ../web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/change-workflow.spec.ts tests/task/workspace-file-transfer.spec.ts tests/session/session-refresh-efficiency.spec.ts -- --grep 'updates the open task stepper after changing workflow|uploads picked files|revalidates an unchanged session' --retries=0
cd ../..
python3 .github/scripts/backend-tests-workflow-contract_test.py
```

Hosted harness lint found the rebased backend guide at 301 lines. Removed one
sentence duplicated by its existing backend i18n routing bullet; the stable
error-code rule, document link, and locale ADR remain intact. The exact whole-tree
lint failed before this correction and passed afterward. Harness linter tests
also passed. No product behavior or CI limit was changed.

```bash
python3 .github/scripts/lint-harness-files.py --all
python3 scripts/lint-harness-files.test.py
```

### Browser fixture isolation follow-up, 2026-10-02

Run `37035996596` passed Windows native tests and cache publication. Its twenty
browser/container blob reports recorded seven failed attempts across six browser
shards. All seven were inspected. Navigation task setup now resets only the
fixture checkout before launching; a new dirty-checkout regression failed before
this change and passed afterward. Independent branch seeding still preserves a
dirty checkout. Settings navigation uses an owned, unreferenced profile instead
of disabling a shared profile with retained dynamic-profile references. Workflow
preview expectations read current seed metadata. The mobile branch refresh test
waits for selector closure and popover animation before measuring its touch area.
Context reset waits for its new response before checking idle and persisted
settings, rather than accepting the previous turn's idle state.

The unchanged mobile workflow-move case passed three diagnostic runs; the
unchanged drag-cancellation case passed in the desktop sequence. No production
change was inferred from those isolated successes. Initial reset coverage also
passed three runs. Focused ESLint, catalog validation, and full spec lint passed.
A local animation-wait edit incorrectly selected an ancestor of the popover;
that setup failure was corrected before final validation. Hosted checks remain
a publication gate. Native operating-system containment and live harness resume
matrix limitations recorded above remain unchanged.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/task-navigation-responsiveness.spec.ts -- --grep 'task setup restores a dirty' --retries=0
# RED: dirty-checkout assertion failed before the fixture reset.
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/task-navigation-responsiveness.spec.ts -- --grep 'task setup restores a dirty|branch setup leaves a dirty' --retries=0
# GREEN: both cases passed.
GOCACHE=/tmp/kandev-go-build-preserved-20261001 E2E_DEBUG=1 pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-workspace-repository-sets.spec.ts tests/task/mobile-create-task-workflow-agent-overrides.spec.ts -- --grep 'contained full-height drawer|routes the selected profile' --repeat-each=3 --retries=0
# Diagnostic baseline: six cases passed.
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/kanban/swimlane-height.spec.ts tests/settings/hide-disabled-agent-profiles-nav.spec.ts tests/task/task-create-workflow-step-previews.spec.ts tests/workflow/workflow-step-proceed.spec.ts -- --grep 'live content grows|off by default|scrolls ten|preserves session settings' --retries=0
# Four cases passed; preview metadata change requires the final run below.
```

Final fixture validation passed six mobile cases (55.5s) and twelve desktop cases
(1.9m), with three repetitions and zero retries. Targeted ESLint and Prettier
checks passed. The original hosted run reached terminal state with 55 passing
checks and eight failed checks (six browser shards and their two report gates);
its snapshot was complete with no API errors.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/settings/mobile-workspace-repository-sets.spec.ts tests/task/mobile-task-route-responsiveness.spec.ts -- --grep 'contained full-height drawer|cached conversation' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/kanban/swimlane-height.spec.ts tests/settings/hide-disabled-agent-profiles-nav.spec.ts tests/task/task-create-workflow-step-previews.spec.ts tests/workflow/workflow-step-proceed.spec.ts -- --grep 'live content grows|off by default|scrolls ten|preserves session settings' --repeat-each=3 --retries=0
pnpm exec eslint e2e/tests/settings/mobile-workspace-repository-sets.spec.ts e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/task-navigation-helpers.ts e2e/tests/task/task-navigation-responsiveness.spec.ts e2e/tests/workflow/workflow-step-proceed.spec.ts
pnpm exec prettier --check e2e/tests/settings/mobile-workspace-repository-sets.spec.ts e2e/tests/settings/hide-disabled-agent-profiles-nav.spec.ts e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/task-navigation-helpers.ts e2e/tests/task/task-navigation-responsiveness.spec.ts e2e/tests/workflow/workflow-step-proceed.spec.ts
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Latest Git metadata and ACP mode integration, 2026-10-02

Rebased onto main `8403b464b42719d4f0357c9376a33a11a88f7416`, including Git diff
status metadata and confirmed Auggie mode startup. All 83 branch commits were
preserved without new conflicts. Range-diff changed only documentation context
headers. Catalog validation, full spec lint, and whitespace checks passed.
Affected backend packages passed with race detection: ACP (28.905s), process
(208.755s), and lifecycle (133.990s). A fresh managed browser build passed the
context-reset persistence case with zero retries (32.7s). Publication and final
exact-head hosted checks remain the next gates.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle
cd ../web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/workflow/workflow-step-proceed.spec.ts -- --grep 'preserves session settings across context reset' --retries=0
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Published browser fixture follow-up, 2026-10-02

The published `c2bf20` backend workflow passed, including native Windows and
PostgreSQL. Browser shards 11 and 13 reported one first-attempt failure each.
The promoted-tab screenshot shows both seeded tasks in the sidebar and no board
cards. Its setup now opens the task's stable route, as the adjacent case does.
The workflow helper searched concatenated text with `indexOf`, so valid repeated
or overlapping titles could never satisfy it. A real picker regression with
`Review complete`, `Review`, `Review` failed before the fix. The helper now
compares the individual rendered titles exactly and in order. This also provides
an actual/expected diff if the hosted failure has another cause. The initial
hosted screenshot was captured after cleanup and does not identify its mismatched
workflow. No timeout or product assertion was relaxed.

The new regression and both affected cases passed three repetitions each:
9 passed (1.4m), zero retries. Scoped ESLint and Prettier checks passed.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts -- --grep 'titles repeat' --retries=0
# RED: one failure with the old substring assertion.
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts tests/layout/preview-tab-session-switch.spec.ts -- --grep 'titles repeat|scrolls ten|promoted file tab' --repeat-each=3 --retries=0
pnpm exec eslint e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/workflow-step-previews-helpers.ts e2e/tests/layout/preview-tab-session-switch.spec.ts
pnpm exec prettier --check e2e/tests/task/task-create-workflow-step-previews.spec.ts e2e/tests/task/workflow-step-previews-helpers.ts e2e/tests/layout/preview-tab-session-switch.spec.ts
```

### Exact dirty-path monitor integration, 2026-10-02

Rebased all 85 branch commits onto main
`69fa561795f52ee69ef15513e9f55c3c65ddc3f7` without conflicts. Range-diff preserves
all branch changes. The affected process package passed with race detection
(163.197s). A fresh browser build passed the three affected/regression cases
with zero retries (35.2s). Full web typecheck and lint, documentation catalog,
full specification lint, and whitespace checks passed.

The preceding published cohort is terminal: 59 passed, 11 skipped, 1 neutral,
4 failed, 0 pending, complete evidence. All 20 browser blob reports were audited;
the only failed attempts are the two fixtures described above (one failed and
one timed out, both passed their retry). New-head hosted checks remain required.
Native Windows/macOS product containment smoke tests and the live harness
restore matrix remain outside these checks.

```bash
cd apps/backend
GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -trimpath -race ./internal/agentctl/server/process
cd ../web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/task/task-create-workflow-step-previews.spec.ts tests/layout/preview-tab-session-switch.spec.ts -- --grep 'titles repeat|scrolls ten|promoted file tab' --retries=0
pnpm run typecheck
pnpm run lint
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Remaining tab fixture paths and container diagnostics, 2026-10-02

The `f29b4` hosted run exposed the same board-card setup dependency in the active
center-tab round-trip case. Its screenshot shows both tasks in the sidebar and
zero board cards under changed workflow columns. The two remaining cases in
this file now open their seeded task IDs directly. The complete four-case file
passed three repetitions: 12 passed (2.6m), zero retries. Scoped ESLint and
Prettier passed. Tab identity, duplicate prevention, active state, and refresh
persistence assertions are unchanged.

Containers shard 5 failed only while building its Docker fixture image; build
output was suppressed, and its hosted retry passed. The image built locally.
The first local product probe then failed because an ignored build receipt from
revision `34de930` no longer matched rebuilt binaries. That stale receipt was
preserved outside the build directory. With a CI-style version and current
build identity, the native Codex Docker case passed in 41.0s without retries.
No product or fixture build behavior changed on this evidence. New-head hosted
checks must still pass; a local pass does not settle the original build failure.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/layout/preview-tab-session-switch.spec.ts -- --repeat-each=3 --retries=0
pnpm exec eslint e2e/tests/layout/preview-tab-session-switch.spec.ts
pnpm exec prettier --check e2e/tests/layout/preview-tab-session-switch.spec.ts
VERSION=0.0.0-e2e.f29b4bd1a1859ce72d0056dc4d23c0ac54c71a55 GOCACHE=/tmp/kandev-go-build-preserved-20261001 KANDEV_E2E_CONTAINERS=1 E2E_DEBUG=1 pnpm e2e:run --host --project containers tests/docker/codex-app-server.spec.ts -- --retries=0
```

### Controlled initial Git loading fixture, 2026-10-02

The final hosted browser failure held fresh refresh requests but forwarded
background ready snapshots. Those snapshots could clear the initial loading
indicator between its text assertion and its layout measurement. The bridge
now supports an explicit opt-in hold for ready notifications. Only the initial
loading test enables it, and it releases queued notifications before completing
the refresh and again during cleanup. Normal forwarding and deliberately dropped
pending-event behavior remain unchanged. Production code did not change.

A bridge unit regression failed because ready snapshots escaped the hold; both
bridge tests then passed. The unit test mocks the Playwright assertion module
because it exercises socket forwarding without a browser. The complete desktop
Git recovery file passed (3 cases, 18.9s), the affected initial case passed three
more repetitions (28.4s), and both mobile recovery cases passed (17.2s). Every
browser run used zero retries. Typecheck, scoped ESLint, and Prettier passed.

All 20 reports from `f29b4` were audited: three failed first attempts, exactly the
two browser fixture issues and the Docker image build described above. The
terminal cohort has 58 passed, 11 skipped, 1 neutral, 5 failed checks (three leaf
jobs and two report gates), and no pending checks. Backend CI is fully green.
Publication of these verified fixture fixes and fresh complete CI remain gates.

```bash
cd apps/web
pnpm exec vitest run e2e/scripts/git-status-refresh-bridge.test.ts
# RED: one failing hold assertion and one passing default-forwarding case.
# GREEN: both pass after the bridge hold is implemented.
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/git/changes-panel-refresh-recovery.spec.ts -- --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/git/changes-panel-refresh-recovery.spec.ts -- --grep 'keeps initial pending' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/git/mobile-changes-panel-refresh-recovery.spec.ts -- --retries=0
pnpm run typecheck
pnpm exec eslint e2e/scripts/git-status-refresh-bridge.test.ts e2e/tests/git/git-status-refresh-helpers.ts e2e/tests/git/changes-panel-refresh-recovery.spec.ts
pnpm exec prettier --check e2e/scripts/git-status-refresh-bridge.test.ts e2e/tests/git/git-status-refresh-helpers.ts e2e/tests/git/changes-panel-refresh-recovery.spec.ts
```

### Final browser fixture failures, 2026-10-02

The `fc571214694` cohort finished with two failed first browser attempts and
three failed checks (one shard and its two report gates). All 20 blob reports
were audited: 3672 passing attempts, 47 skipped, two failed first attempts, and
two successful retries. Backend, Windows, PostgreSQL and container checks passed.

The preview fixture now explicitly persists the older session as primary before
reloading, preserving all default-selection, switching, content and URL checks.
Three repetitions passed (33.6s), zero retries. The earlier diagnostic had two
passes and an unrelated workspace-setup 404, not a reproduced selection failure.

The mobile queue fixture starts its generating turn after navigation/readiness,
so slow setup cannot consume the sleep turn. This exposed the shared helper's
keyboard-only submission: three local failures left the prompt unsent on mobile.
Using the existing pointer-aware submit-button method fixed all three repetitions
(47.6s). Desktop cancel and delayed-admission cases also passed (2 cases, 38.7s).
No prompt retry, product change or timeout increase was added. Scoped ESLint and
Prettier passed for all three changed files.

```bash
cd apps/web
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/kanban/preview-session-tabs.spec.ts -- --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts -- --grep 'keeps queued row controls ordered and touchable' --repeat-each=3 --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/chat/cancel-turn-availability.spec.ts tests/chat/queue-admission-reliability.spec.ts -- --grep 'direct input cancel control|clears an attached Task draft' --retries=0
pnpm exec eslint e2e/helpers/generating-session.ts e2e/tests/chat/mobile-message-queue-management.spec.ts e2e/tests/kanban/preview-session-tabs.spec.ts
pnpm exec prettier --check e2e/helpers/generating-session.ts e2e/tests/chat/mobile-message-queue-management.spec.ts e2e/tests/kanban/preview-session-tabs.spec.ts
```

### Latest main integration, 2026-10-02

Rebased onto verified main `87dcd788a8f43faa72445bd35c0e86b270e2ebfd`
(runtime-indicator removal and shared branch-read ordering). No conflicts; all
89 patches match the prior series in range-diff. The affected upstream unit
suites passed all 33 tests. Fresh managed desktop checks passed three cases
(39.5s); mobile queued controls and branch-policy drawer passed two (13.9s),
all with retries disabled. Full web typecheck, lint and i18n checks passed,
along with documentation catalog, full specification lint and whitespace.
No backend source changed in this base increment. Exact-head hosted CI remains
a delivery gate; native Windows/macOS product containment smoke and live-harness
resume-matrix limitations remain as previously recorded.

```bash
cd apps/web
pnpm exec vitest run components/update-available-toast-bridge.test.tsx hooks/domains/workspace/use-repository-branches.test.tsx
pnpm run typecheck
pnpm run lint
pnpm run i18n:check
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project chromium tests/kanban/preview-session-tabs.spec.ts tests/chat/cancel-turn-availability.spec.ts tests/chat/queue-admission-reliability.spec.ts -- --grep 'shows all sessions as tabs|direct input cancel control|clears an attached Task draft' --retries=0
GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts tests/task/mobile-create-task-branch-policy.spec.ts -- --grep 'keeps queued row controls ordered and touchable|keeps the policy marker and fresh-branch state visible' --retries=0
cd ../..
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Latest main Windows matrix reconciliation, 2026-10-02

Main advanced to `8a1229e6d70a`, whose earlier workflow change separates the
Windows process race suite from the native build, vet and targeted tests in a
40-minute non-fail-fast matrix. The rebase conflict was resolved in favor of
that complete matrix; branch-only 50-minute edits were superseded. The main
workflow contract suite passes all 12 tests after resolution. The 08ab run's
Windows job had reached its 50-minute ceiling during the combined suite, so the
matrix will be validated by fresh CI on this rebased head.

The same 08ab run's rich-output chart case failed once when the lazy bar plot
was absent and passed on automatic retry. Four isolated local attempts passed
with retries disabled. It remains unchanged pending a fresh hosted result; any
repeat on the rebased head is actionable.

```bash
python3 .github/scripts/backend-tests-workflow-contract_test.py
python3 .github/scripts/lint-action-pinning.py
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

### Persisted session-setting replay and latest-main rebase, 2026-10-03

The b861 run exposed a reload race in the adjacent workflow context-reset test:
the agent's live model cache could still report `High` after the synchronous
setting endpoint had durably recorded the explicit `Max` override. Session boot
projection preferred that stale cache and omitted the persisted override. The
projection now overlays valid persisted model and option choices onto a cloned
boot snapshot, while rejecting values absent from the current option catalog.
The regression test failed before the change, then passed with the fix; it also
checks that the cache remains unchanged and unsupported values are ignored.

- `GOCACHE=/tmp/kandev-go-build-preserved-20261001 go test -trimpath ./internal/backendapp -count=1`
  passed (46 seconds).
- `GOCACHE=/tmp/kandev-go-build-preserved-20261001 pnpm e2e:run --host
  --project chromium tests/workflow/workflow-step-proceed.spec.ts --
  --grep 'preserves session settings across context reset' --repeat-each=3
  --retries=0` passed all three runs (one minute).
- Normal commit hooks passed, including changed-package Go lint and gofmt.
- Rebased onto main `b330ad97a8712eb7b1a8ee2863b46ba302a92acb` without
  conflicts. `git range-diff` retained the preceding 91 branch patches
  unchanged and includes the new backend fix as patch 92.
- The superseded b861 hosted run was cancelled after strict browser flakes; its
  checks are not current-head evidence. Fresh hosted CI and review evidence for
  the published rebased head remain pending.


### Latest-main continuation integration and CI remediation, 2026-10-06

Merged main's command autocomplete and Cursor native continuation changes while
preserving both cancellation regression tests. Cancellation uses the shared
continuation-aware waiting policy, including retained-runtime cancellation,
without workflow completion.

- `go test -race -tags sqlite_fts5 ./internal/orchestrator/... -count=1`
  passed after resolving the duplicate cancellation-policy declaration.
- The parallel race run passed journal, process, ACP transport, lifecycle, and
  routing-error packages. Its initial orchestrator build failure is superseded
  by the successful command above.
- `make -C apps/backend build e2e-plugin-package` and
  `golangci-lint run ./... --allow-serial-runners
  --new-from-rev=dcdff28b4700b9db0d86fa6483cff16e87d6c36b --timeout=10m`
  passed.
- Desktop managed E2E passed 10/10 with `--retries=0`: queued-message attribution,
  cross-workspace dashboard isolation with a same-workspace positive control,
  workflow auto-launch, six native continuation scenarios, and continuation
  reload/cancellation. The command selected
  `tests/chat/agent-message-attribution.spec.ts`,
  `tests/office/realtime-dashboard.spec.ts`,
  `tests/workflow/workflow-agent-switch.spec.ts`, and
  `tests/session/provider-interruption-continuation.spec.ts` through
  `pnpm e2e:run --host --no-build --shards 1 --project chromium --
  --retries=0 --grep 'running target task: queued message|dashboard does not refetch|auto-launches agent when step has profile override and prompt|integration:|desktop: accepted continuation'`.
- Queue assertions now observe a persisted first-turn message rather than the
  brief idle interval between turns. Dashboard coverage isolates independent
  Office producers while forwarding actual cross-workspace events through the
  production handler and requiring a same-workspace HTTP refetch.
- Workflow startup retained its original deadline. Failure diagnostics now keep
  first-attempt traces and attach the backend log; no reproduced workflow defect
  justified changing production behavior or its deadline.
- Feature-specific PostgreSQL conformance, native Windows/macOS containment,
  and live harness probes remain outstanding release gates. Hosted CI for the
  delivered remediation remains pending until the exact new head finishes.

- `go test -race -tags sqlite_fts5 ./cmd/mock-agent ./internal/backendapp
  ./internal/agentctl/types/streams -count=1` passed.

### Windows CI result-cache remediation, 2026-10-06

Current-head Windows process job `112307710263` reached the unchanged
40-minute job deadline after delayed test startup. Its single rerun,
`112331370974`, passed every test and printed the package `ok` result at
15:14:04 UTC, then stalled before the final package JSON event until the job
was cancelled at 15:27:27 UTC. No individual assertion failed.

Go 1.26's test runner prints that result after the test process has exited,
then performs result-cache input hashing before closing its JSON converter.
The Windows process command now uses `-count=1` to run the live subprocess
suite on every invocation and bypass result-cache lookup and storage. Build
caching, race detection, the complete package selection, the 25-minute test
deadline, and the 40-minute job deadline remain unchanged.

- The workflow contract test failed against the previous command, then passed
  all 13 tests with the uncached invocation.
- `TMPDIR=/root/.cache/kandev-agentctl-runtime-tmp GODEBUG=gocachetest=1
  /root/go/pkg/mod/golang.org/toolchain@v0.0.1-go1.26.0.linux-amd64/bin/go
  test -p 1 -race -count=1 -v -json -timeout 25m
  ./internal/agentctl/server/process/...` passed locally and emitted terminal
  package success events. Its diagnostic output confirmed result caching was
  disabled by `-test.count=1`. This is Linux evidence, not a Windows receipt.
- `python3 .github/scripts/lint-action-pinning_test.py` passed all nine tests.
- `zizmor .github/workflows` reported existing repository findings. An offline
  JSON comparison of the changed workflow against its pre-change version
  found the same single low-severity Windows CMD analysis limitation and no
  added findings.

Fresh hosted validation of this workflow change remains pending. The separate
PostgreSQL, native containment, and live harness release gates remain open.

### Main integration and conflict resolution, 2026-10-06

Integrated main `7d55a599502530501f9308c21cb63f9c64d06ac8`. The sole merge
conflict was the former load-failure fallback in `session.go`. The resolution
retains this branch's typed restore coordinator and native-only recovery, which
also preserves main's OpenCode prohibition on silently creating a replacement
conversation. The incoming OpenCode missing-session regression now asserts
`RestoreReasonNativeStateMissing` alongside no new-session call. Main's
independent Windows job deadline increase to 60 minutes is retained with this
branch's uncached live-process test invocation.

- Focused lifecycle/OpenCode recovery tests passed with race detection.
- `pnpm run typecheck` and `pnpm run lint` passed from `apps/web`.
- Six affected Vitest files passed all 78 tests in the resolved workspace.
- `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all` passed;
  `python3 scripts/lint-spec-files.test.py` passed all 36 tests.
- Repository documentation coverage preflight passed for 54 changed work
  orders against the integration base.

Historical head `9d116356` mobile canvas failed-job rerun `112345332469`
succeeded without a source change. This receipt does not substitute for CI on
the integration head. PostgreSQL, native containment, and live harness release
gates remain open.

- Affected backend packages passed with race detection: agents, managedruntime,
  settings/controller, runtime/activity, hostutility, and backendapp. The
  initial lifecycle run failed two local scratch-path-sensitive tests: an
  overlong Unix socket path and an SSH cleanup fixture. Both focused tests and
  the complete lifecycle package passed using `TMPDIR=/root/k3598-tmp`; the
  complete lifecycle rerun took 131.228 seconds. No production workaround was
  introduced. All runs used `go test -p 1 -race ... -count=1`.
- `golangci-lint run ./... --allow-serial-runners
  --new-from-rev=7d55a599502530501f9308c21cb63f9c64d06ac8 --timeout=10m`
  passed with zero issues.

### CI follow-up: portal focus ownership

The current browser check reproduces a model picker closing after selection.
A bounded browser focus trace proves that the option's mouse-down bubbles
through React to the session panel although its DOM node is in a portal outside
the panel. The panel then focuses itself, causing Radix to dismiss the picker.
Persisted opening-turn readiness alone did not repair this failure.

Repair scope: the existing shared panel mouse-down/click router must only
claim non-interactive targets physically contained in its current panel. Keep
normal transcript focus, controls, deferred Quick Chat focus, model changes,
and picker-close policy unchanged. Extend the existing router unit test with
a real portal DOM target, run it RED before the containment guard, and prove
GREEN afterward. Re-run the actual desktop picker scenario and its phone
counterpart with no retries, plus shared router/control tests, lint and types.
No new UI composition or copy is needed; phone model settings retain their
existing entry point and overlay. Fresh pushed-head CI remains a delivery gate.

The router unit regression failed on unwanted panel focus before the guard.
`pnpm --dir apps/web exec vitest run components/task/chat/route-panel-mouse-down.test.ts components/task/chat/clarification-custom-input.test.tsx components/model-config-selector.test.tsx components/task/model-selector-consecutive-switch.test.tsx`
then passed all 53 tests. The two persistence fixtures now also wait for their
exact session's persisted opening response and WAITING_FOR_INPUT state before
changing configuration. The picker check additionally confirms the saved
model while requiring the same effort control to remain visible.

Parent-head hosted validation is historical: all backend checks, including
PostgreSQL 16/18 and the uncached Windows process job, passed at `48f19138`.
The Windows process step completed normally in 24m46s and emitted final package
PASS records. These checks do not substitute for the next pushed head or close
manual native containment and live-harness release gates. No public docs
change is required for this repair to existing focus and picker behavior.

Rebuilt browser validation passed all nine cases (three independent runs of
each changed model scenario):
`TMPDIR=/root/.cache/kandev-pr3598-e2e-tmp GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/chat/model-selector-error.spec.ts -- --grep 'agent tab keeps|changed values stay|stays open after' --repeat-each=3 --retries=0 --trace=retain-on-failure`.
The rebuild included backend, web E2E assets and the fixture plugin. Web
typecheck including the new phone test and focused ESLint passed. Catalog
validation, full specification lint, and all 36 linter tests passed; local
documentation coverage accepted 55 changed work orders against current main.

Phone validation passed all six cases with no retries:
`TMPDIR=/root/.cache/kandev-pr3598-e2e-tmp GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-model-selector.spec.ts -- --repeat-each=3 --retries=0 --trace=retain-on-failure`.
The new native touch case verifies saved model identity, continued picker
visibility, and entry into the effort submenu. The existing long-menu and
provider-description touch case also passed three times. The old per-suite
retry override was removed so the runner's retry policy applies. No temporary
focus probe remains. Fresh pushed-head CI and advanced-base compatibility are
still required; manual release gates retain their existing status.

### Follow-up hosted CI fixture repairs

The hosted stale-error model test and context-reset settings test clicked the
picker trigger again after a successful selection, closing the picker that
correctly remains open. Preserve the production open policy. The stale-error
fixture now observes the newer successful HTTP response before releasing the
older failed response, then drains its owned route before cleanup. Opening
model fixtures wait for their exact persisted response and idle session.

The shared seed repository cleanup now restores only the origin URL during
integration teardown. Tracking references refresh after task reset, before a
new browser scenario. Direct branch-recovery callers retain the default
refresh. Fetch failures retain stderr. Two actual remote-reference-lock
regressions failed before the helper change and passed afterward. The original
hosted Git error had suppressed stderr; the controlled lock does not establish
that the hosted failure had the same cause.

`cd apps/web && pnpm exec vitest run e2e/helpers/seed-repository-origin.test.ts e2e/helpers/mobile-threads-swipe-helpers.test.ts`
passed five tests. Web typecheck and focused ESLint passed after fixture wiring.
Rebuilt picker and workflow cases passed; retained-runtime follow-up is
recorded in the owning continuity work order;
fresh pushed-head hosted CI remains required. No release gate is closed by
these fixture checks.

The timed-out hosted shard includes the retained-runtime model-change case.
The local 19-case batch reproduced its eight-minute timeout: the native trace
recorded the successful model change but no follow-up prompt, and the failure
snapshot retained the open model-picker dialog. The other 18 cases passed,
including all workflow transitions. The case now closes the picker with Escape
and asserts it is hidden before sending the follow-up. Retain all same-runtime,
message-count, and native ACP trace assertions. Neither test nor job timeouts
were increased. The rebuilt combined desktop run passed the repaired case
and all picker, port-forwarding and workflow cases; the subsequent retained-
runtime diagnostic run passed all 12 cases;
the original hosted shard's missing terminal artifact remains a limitation on
attributing its full timeout until the rerun or fresh CI supplies evidence.


Final no-retry phone integration check:
`KANDEV_RUN_QUIET_DIR=/root/.cache/kandev-pr3598-quiet-owned E2E_PORT_OFFSET=0 GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-create-task-workflow-agent-overrides.spec.ts tests/session/mobile-port-forwarding.spec.ts tests/pr/mobile-pr-watcher-missing-branch.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/session/mobile-transient-turn-runtime-continuity.spec.ts --retries=0 --trace=retain-on-failure`
passed all 21 cases in 10.7 minutes. This covers the shared seed-origin fixture,
phone cancellation before any retry dispatch, continuation across viewers,
retry exhaustion, launch recovery, port actions and native thread navigation.
Web typecheck, full web ESLint, all twelve changed TypeScript files' Prettier
checks, and the five seed-origin/swipe unit regressions passed on this source.
These results do not close manual release gates or substitute for pushed-head CI.


Final desktop native-runtime check:
`KANDEV_RUN_QUIET_DIR=/root/.cache/kandev-pr3598-quiet-owned E2E_PORT_OFFSET=0 GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium -- tests/session/transient-turn-runtime-continuity.spec.ts --retries=0 --trace=retain-on-failure`
passed all four cases in 3.7 minutes after the shared observer and causal
cancellation setup. Actual retry counts, exact native prompt counts, retained
execution identity, completed-effect counts and backend restart cleanup passed.
The earlier isolated cleanup startup failure remains unconfirmed; this result
does not claim a production startup fix. Catalog/full spec lint and coverage
preflight passed: 57 work orders against merged main, 65 against the recorded
PR base, with zero coverage errors.


Current-main conflict remediation (2026-10-07): canonical source diagnostic evidence
is separate from marker-free persisted-message notifications; the focused
regression failed before repair and passed afterward. The merge reuses the
shared semantic JSON metadata CAS and preserves main's initial-submission and
cancellation ownership contracts. `GOMAXPROCS=4 go test -p 2 -count=1
-timeout=10m ./internal/agent/runtime/lifecycle
./internal/task/repository/sqlite ./internal/orchestrator/...` passed, as did
`GOMAXPROCS=4 go test -p 2 -count=1 -timeout=15m
./internal/agentctl/server/process`. Focused canonical/diagnostic and executor
race checks passed. Full web typecheck/lint/i18n, diff-scoped Go lint,
catalog/spec and harness checks passed.

The accepted lazy-resume browser fixture used an uninterruptible `/slow 8s`
command and could exceed the cancellation deadline. It now uses the existing
cancellation-aware inline delay and requires agent-authored acceptance output
and the persisted boot receipt. Two helper regressions failed before the
agent-author filter and passed after it:
`pnpm --dir apps/web exec vitest run
e2e/helpers/session-resume-prompt-queue.test.ts`. The corrected mobile case
passed three first attempts with `--repeat-each=3 --retries=0`; the earlier
mobile batch with a retry remains failed evidence. Desktop merge-fixture
validation passed all 19 cases with retries disabled, using the managed host
runner with one worker. Pushed-head hosted CI remains pending at this checkpoint.
Manual native Windows/macOS and targeted durable PostgreSQL/live-harness
release gates remain open.

Latest-main workflow integration: focused model/repository/service/handler
race checks passed using `GOMAXPROCS=4 go test -p 2 -race -count=1
-timeout=10m ./internal/task/models ./internal/task/repository/sqlite
./internal/task/service ./internal/task/handlers -run 'Workflow|Process'`.
The final fresh-build mobile recovery file passed all five first attempts with
retries disabled. Full spec/catalog/harness checks and the 58-work-order
coverage preflight passed. Pushed-head CI remains an external pending gate.

### Continued PR #3598 CI fixture isolation (2026-10-07)

A forced non-main checkout reproduced shard 1's missing `movable.ts` before
drag-and-drop. The fixture committed its file on that checkout while task setup
selected main. The DnD suite now checks out main and removes untracked scratch
files before seeding, after the shared test reset has stopped previous tasks.
All on-disk and tree movement assertions remain. The rebuilt related desktop
suite passed nine tests with no retries:

```sh
E2E_PORT_OFFSET=0 pnpm e2e:run --project chromium tests/settings/sidebar-direct-customization.spec.ts tests/task/create-task-url-reopen-no-branches.spec.ts tests/task/file-tree-drag-drop.spec.ts -- --retries=0
```

Main `8feffe1e17` merged without conflicts. Focused race checks passed in
workflow controller, handlers, repository, service and MCP handlers for start
selection. Sidebar settings now reset per test using the current SQL revision;
this does not change persistence within a scenario. Mobile sidebar validation
passed one test, and ESLint/typecheck plus 72-work-order coverage passed.
Hosted replacement-head validation remains pending.

The full DnD spec also passed three repetitions (six tests) with `--retries=0`
using the rebuilt artifacts. This is local evidence, not a hosted CI verdict.

### PR #3598 current-main conflicts and validator budget (2026-10-07)

Integrated main's task-navigation, Git-refresh, task-ranking, native MCP,
disk-usage and plugin-focus changes. The Git bridge conflict now retains both
`holdReadyNotifications` and `dropPendingStatusEvents`. The walkthrough runner
keeps its deterministic shared-deadline test rather than restoring wall-clock
sleep sensitivity. Its 17 tests passed:
`python3 .github/scripts/pr-walkthrough-runner_test.py`.

Merged runtime race checks passed through MCP config, lifecycle and task
handlers: from `apps/backend`, `go test -race ./internal/agent/mcpconfig
./internal/agent/runtime/lifecycle ./internal/task/handlers -run
'Test.*(CursorMCP|CursorProjectMCP|RuntimeReplacement|StreamDisconnectDoesNotReplaceCompletedPromptOutcome)'
-count=1`. Focused ESLint and web typecheck passed. Harness validation passed
19 harness-linter tests, all 203 harness files, 36 spec-linter tests, full spec
lint and the targeted `harness-lint` pre-commit hook for `apps/web/AGENTS.md`.

Rebuilt Git-refresh desktop tests passed five first attempts; matching mobile
tests passed four first attempts, sequentially from `apps/web`:
`E2E_PORT_OFFSET=0 pnpm e2e:run tests/git/changes-panel-refresh-recovery.spec.ts
tests/git/diff-refresh-continuity.spec.ts --project=chromium --retries=0`, then
`E2E_PORT_OFFSET=0 pnpm e2e:run --no-build
tests/git/mobile-changes-panel-refresh-recovery.spec.ts
tests/git/mobile-diff-refresh-continuity.spec.ts --project=mobile-chrome
--retries=0`.

The trusted live documentation evaluator reproduced the hosted generic error:
references exceeded its 200-document limit. A local exact-loader candidate-tree
preflight also failed at 200 reads. Moved only this PR's sidebar isolation
appendix into the focused [sidebar browser isolation package](../sidebar-browser-isolation/plan.md),
preserving the receipt verbatim and the original feature work order unchanged.
The same loader then passed 74 work orders with 199 referenced documents.
Catalog validation passed (365 decisions, 1444 specs). This is local evidence;
fresh pushed-head CI, native containment and targeted durable PostgreSQL/live
harness gates remain external and pending.


### Subsequent main integration: initial brief and coordinator control

Preserved incoming initial-brief dispatch ownership, accepted reference context,
and durable direct-message identity together. Regression assertions reproduced
missing delivery IDs for recovered ready-session dispatch and its compound
resume retry; both now pass. Ordinary queue drain retains its durable recovery
fence and the initial-brief ownership gate. The Windows process cohort runner
retains uncached subprocess execution. Shared browser helpers preserve readiness,
layout restoration, held notifications, and pending-or-completed history.

Integrated the coordinator-control increment without changing recovery policy.
The Korean catalog conflict retains both translation sets; duplicate-key checks
passed across all task catalogs. No automatic resend or destructive survivor
cleanup was introduced.

Final integrated-tree validation passed (commands from repository root unless
an explicit working directory is shown):

- From `apps/backend`: `go test -race ./internal/coordinator
  ./internal/backendapp ./internal/agent/runtime/lifecycle
  ./internal/orchestrator/executor ./internal/task/handlers
  ./internal/persistence/requiredstores -run
  'Test.*(Coordinator|InitialTaskBrief|Delivery|RuntimeReplacement|MCPIdentity|RequiredStore|Migration|Boot)'
  -count=1`. The required-store package then passed its full suite separately:
  `go test -race ./internal/persistence/requiredstores -count=1`.
- From `apps/backend`: `go test -race ./internal/mcp/profile
  ./internal/mcp/handlers ./internal/agentctl/server/process -run
  'Test.*(Coordinator|Durable|Delivery)' -count=1`; full portable cohort-runner
  suite `go test -race ./cmd/windows-process-tests -count=1`.
- Workflow contract suite: 13 tests passed with
  `python3 .github/scripts/backend-tests-workflow-contract_test.py`.
- From `apps/backend`: changed-scope `golangci-lint run ./...
  --new-from-rev=7c6fcf1b9d0e9f2e91e524c44dc710a319fd24c2
  --allow-serial-runners --timeout=5m` passed with zero issues. From `apps/web`,
  `pnpm run lint`, `pnpm run typecheck`, and `pnpm run i18n:check` passed.
- `python3 scripts/lint-spec-files.py --all` and
  `python3 scripts/list-docs.py validate` passed. Exact-loader preflight passed
  74 work orders with 199 referenced documents. PR-introduced whitespace passes
  against the authoritative base; incoming base-only whitespace is preserved.
- Rebuilt desktop browser suite passed 33 first attempts. From `apps/web`:
  `E2E_PORT_OFFSET=0 pnpm e2e:run --host
  tests/git/changes-panel-refresh-recovery.spec.ts
  tests/session/long-prepare-panels.spec.ts
  tests/session/transient-turn-runtime-continuity.spec.ts
  tests/task/directory-browser-hidden-folders.spec.ts
  tests/task/subtask.spec.ts tests/task/sidebar-filter-selected-first.spec.ts
  tests/task/task-create-workflow-step-previews.spec.ts
  tests/coordinator/policy-enforcement.spec.ts --project=chromium --retries=0`.
- After `make -C apps/backend build e2e-plugin-package`, desktop production
  recovery coverage passed three first attempts from `apps/web`:
  `E2E_PORT_OFFSET=0 pnpm e2e:run --no-build --host
  tests/session/durable-reattachment.spec.ts
  tests/session/durable-stream-recovery.spec.ts --project=chromium --retries=0`.
- Sequential mobile coverage passed 17 first attempts from `apps/web`:
  `E2E_PORT_OFFSET=0 pnpm e2e:run --no-build --host
  tests/session/mobile-durable-reattachment.spec.ts
  tests/session/mobile-durable-stream-recovery.spec.ts
  tests/session/mobile-transient-turn-runtime-continuity.spec.ts
  tests/settings/mobile-workspace-repository-sets.spec.ts
  tests/task/mobile-directory-browser-hidden-folders.spec.ts
  tests/task/mobile-sidebar-filter-selected-first.spec.ts
  tests/task/mobile-task-create-workflow-step-previews.spec.ts
  tests/coordinator/mobile-proposal-kinds.spec.ts
  --project=mobile-chrome --retries=0`.

A later conflict-free review-disposition increment was checked in an owned
synthetic worktree: `pnpm exec vitest run
hooks/domains/review/use-finding-actions.test.tsx` passed 20 tests; focused
ESLint and `pnpm run typecheck` passed. Latest-head CI/review and final combined
base evidence remain external until delivery. Native Windows/macOS containment
and targeted durable PostgreSQL/live-harness release gates remain open. The
queue runtime-loss flake remains unconfirmed; failed tests retain backend logs.

### Subsequent hosted browser remediation

The hosted handoff capability test selected an arbitrary retained agent for its
second profile and received a profile-create 500. Its fixture now selects the
registered mock adapter for both profiles. A unit regression rejected the old
selection with an unrelated unregistered agent present, then passed after the
correction. The browser capability update affects every profile owned by the
mock agent, including retained profiles. Production handoff behavior is unchanged.

From `apps/web`, the final checks were:

```bash
pnpm exec vitest run e2e/helpers/session-handoff-profile-fixtures.test.ts e2e/helpers/agent-fixtures.test.ts
pnpm exec eslint e2e/helpers/session-handoff-profile-fixtures.ts e2e/helpers/session-handoff-profile-fixtures.test.ts e2e/tests/session/session-handoff-unhealthy-profile.spec.ts e2e/tests/workflow/queue-limit-navigation.spec.ts
pnpm run typecheck
E2E_PORT_OFFSET=0 pnpm e2e:run --host tests/session/session-handoff-unhealthy-profile.spec.ts tests/session/session-handoff.spec.ts tests/workflow/queue-limit-navigation.spec.ts --project=chromium --retries=0 --repeat-each=3
```

The unit check passed three tests; the managed browser check passed 12 cases on
three fresh workers without retries. ESLint and typecheck passed. An intermediate
browser check caught a fixture `agentId`/`agent_id` mismatch; the final command
above covers the corrected boundary.

The hosted queue-navigation setup also failed once before passing on retry.
Its isolated six-case repetition passed. A one-worker CI-image replay with
2 CPUs and 4 GiB memory preserved the failed shard's preceding desktop order;
both queue-navigation cases passed on their first attempts. The owned replay
was stopped after that prefix, so this is not a full-shard completion receipt.
The queue cause remains unconfirmed. Failed navigation attempts now attach the
isolated backend log; waits and admission behavior are unchanged. Fresh hosted
CI and its complete artifact audit remain pending after delivery. Native and
targeted PostgreSQL/live-harness release gates remain open.


### Empty Git scope identity remediation

Hosted head `56d014af044b1dfbd8728dbd9a92d75653a88b1a` failed the
expand-all browser check. Retries-disabled CI-image reproduction failed twice
in five runs. Waiting for the mock turn to finish did not repair it and was
removed. Browser event and store traces showed a delivered click followed by a
section replacement as an empty comparison target alternated between `null`
and an empty string. Empty scoped values now have one canonical representation;
a real comparison-target change still replaces the scope.

Validation in the primary conversation:

- The final typed regression failed against the original helper, then passed
  with the fix. `pnpm exec vitest run
  lib/state/slices/session-runtime/git-status-display-state.test.ts` passed all
  12 tests.
- `pnpm exec vitest run lib/state/slices/session-runtime/
  hooks/domains/session/` passed 979 tests in 102 files.
- Focused ESLint and `pnpm run typecheck` passed. Public documentation review
  found no changed command, configuration, UI entry point, or public contract.
- After a fresh managed build, the CI runtime image with two CPUs, 4 GiB,
  one worker, and the spec's retry override temporarily set to zero ran
  `bash e2e/scripts/run-raw-e2e.sh --project=chromium --workers=1
  e2e/tests/git/diff-expansion.spec.ts --repeat-each=5 --retries=0
  --reporter=list --output=/owned-output`: 40 first-attempt passes. The override
  and diagnostic probes were removed; the original spec is unchanged.
- `E2E_PORT_OFFSET=0 pnpm e2e:run --host --no-build
  --project mobile-chrome -- tests/git/mobile-diff-refresh-continuity.spec.ts
  --workers=1 --retries=0` passed both phone cases on their first attempt.

Other hosted navigation, preview-feedback, and hidden-backfill failures remain
under investigation. Main integration, new-head CI and full artifact audit are
pending. Native Windows/macOS containment and targeted durable-delivery
PostgreSQL/live-harness release gates remain open.


### Latest-main integration and hidden-backfill fixture

Integrated main `33133eb0e0be5a11db123abad9b98a3ce796f40b` without conflicts
at `65af9b42f111a5cb1c0225396c6994e215e1678d`. Its backend tree matches the
previously validated synthetic `42ceb2fc5b565b9d7d623ff2043b54ba9fb47a67`.
`pnpm exec vitest run lib/state/slices/session-runtime/ hooks/domains/session/`
passed 986 tests in 102 files after integration. Focused ESLint and typecheck
passed. Exact loader preflight against the integrated main base passed with
74 work orders and 199 referenced documents; the older-base preflight is not
valid for this delivery.

Hosted hidden-tab backfill reproduced twice on the integrated branch. Tracing
showed the periodic timer paused correctly; initial-history repair and mock
`/slow` tool activity made an all-history-request count ambiguous. The fixture
now establishes a conversation and starts one quiet `/sleep 60` turn. It waits
for persisted RUNNING state and a visible refresh before hiding the document.
It still checks a 12-second hidden window, foreground refresh, resumed periodic
refresh, and a persistently RUNNING session. There is no automatic recovery,
resubmission, new timeout allowance, or production visibility change.

- `E2E_PORT_OFFSET=0 pnpm e2e:run --host --no-build --project chromium --
  tests/chat/hidden-running-backfill.spec.ts --workers=1 --repeat-each=3
  --retries=0`: three first-attempt passes.
- The same spec in the CI runtime image, two CPUs, 4 GiB, one worker:
  `bash e2e/scripts/run-raw-e2e.sh --project=chromium --workers=1
  e2e/tests/chat/hidden-running-backfill.spec.ts --repeat-each=3 --retries=0
  --reporter=list --output=/owned-output`: three first-attempt passes.
- The CI-image command with `--project=mobile-chrome
  e2e/tests/task/mobile-sidebar-shared-task-state.spec.ts --grep
  "phone rejects old workspace pages" --repeat-each=3 --retries=0` passed
  all three runs.
- The CI-image command with `--project=chromium
  e2e/tests/session/long-prepare-panels.spec.ts
  e2e/tests/task/task-navigation-responsiveness.spec.ts --grep
  "file tree and terminal keep waiting|Files stays usable" --repeat-each=3
  --retries=0` passed six cases. Both specs now attach backend logs on failure.
- Two hosted-failure reproductions of preview-feedback passed after main
  integration. Their command selected `tests/preview/preview-feedback.spec.ts`
  with `--grep "persists multi-route" --repeat-each=2 --retries=0` alongside
  the other hosted desktop failures. That combined run was not green: it also
  reproduced hidden-backfill twice and one navigation Git-hydration timeout.

The intermittent preparation/navigation causes remain unconfirmed. Fresh-head
hosted CI, reviews and the all-report audit remain pending; earlier-head CI is
not a delivery receipt. No new feature flag, delegation, PR merge, or release
approval. Native and targeted PostgreSQL/live-harness release gates stay open.

### Preparation gate prerequisite remediation

Hosted run `37797695064`, head `bde926610567cc0f27847b63c4bb0a0f942cc77b`,
shard 9 job `113396605142` failed its first preparation-gate attempt and passed
on retry. The attached backend log showed a worktree launch and successful
preparation without a fetch. The test relied on the worker-scoped repository's
mutable `pull_before_worktree` setting. It now reads that setting, explicitly
enables fetching before launch, and restores the prior value in cleanup.
Gate files are removed even when restoring the repository setting fails.

- RED: temporarily set `pull_before_worktree: false` before the unchanged
  gate setup; the CI-image test reproduced the exact 30-second gate failure.
- GREEN: keep that disabled-fetch setup and run
  `bash e2e/scripts/run-raw-e2e.sh --project=chromium --workers=1
  e2e/tests/session/long-prepare-panels.spec.ts --repeat-each=5 --retries=0
  --reporter=list --output=/owned-output`: five first-attempt passes.
- Remove the temporary disabled-fetch setup and repeat the final fixture with
  `--repeat-each=3`: three first-attempt passes. Both runs used the CI runtime
  image, two CPUs, 4 GiB and one worker.
- `pnpm exec eslint e2e/tests/session/long-prepare-panels.spec.ts` and
  `pnpm run typecheck` passed after the final cleanup change.

New main `b232b2931a330864975d90d867d320a3239db8e4` combines without conflicts.
A temporary merge passed 1,025 web tests in 105 files and typecheck via
`pnpm exec vitest run lib/state/slices/session-runtime/ hooks/domains/session/
lib/services/session-launch-service.test.ts lib/ws/handlers/tasks.deleted.test.ts
components/task/simple/task-chat.comment-send.test.tsx`. Its focused backend
check, `go test -race ./internal/agent/runtime/dynamic
./internal/agent/runtime/routingerr ./internal/agent/agents ./internal/orchestrator
./internal/agent/runtime/lifecycle -run
'Superseded|Stale|Failure|OpenCodeNative|Dynamic|Delivery|InitialTaskBrief' -count=1`,
passed. The temporary merge worktree was removed without changing the branch.

Other intermittent browser causes remain unconfirmed. New-head hosted CI,
reviews and all-report audit remain pending. Native Windows/macOS containment
and targeted durable-delivery PostgreSQL/live-harness gates remain open.

### Configuration-chat deletion diagnostics

The same hosted run's shard 2 job `113396605311` failed its first configuration
chat deletion attempt while waiting for an HTTP DELETE success, then passed on
retry. Its available trace covers the successful retry, not the failed attempt.
No cause or production fix is claimed. Failure-only backend-log attachment was
added; deletion behavior, retry policy and timeout remain unchanged.

`bash e2e/scripts/run-raw-e2e.sh --project=chromium --workers=1
 e2e/tests/settings/config-chat-popover.spec.ts --repeat-each=3 --retries=0
 --trace=retain-on-failure --reporter=list --output=/owned-output` passed all
18 cases on their first attempt in the CI runtime image with two CPUs, 4 GiB
and one worker. Hosted verification remains necessary.

### Main 56cc19514e integration (2026-10-08)

Integrated main `56cc19514e20b1c78c366005a9358ba9a8857393` through the
existing merge history. Five conflicts preserved context continuation,
submission uncertainty, cancellation, inspection deadlines, and upstream's
retirement of the interruption-continuation toggle. No new toggle was added.

Combined recovery priorities were tested before the resolution: two cases
failed and one passed when an older workspace relocation action could obscure
uncertain delivery or inspection contention. Uncertain delivery now retains
Retry connection and Stop; inspection contention retains same-session Retry.
Workspace relocation remains available for its independent eligible failures.
The existing workspace identity helpers moved into the extracted recovery fence
module to satisfy the hook's lint size limit without changing their predicates.

Validation:

- `pnpm exec vitest run components/task/chat/session-stopped-banner.test.tsx
  lib/services/session-recovery-service.test.ts hooks/domains/session/
  lib/state/slices/session-runtime/ lib/state/slices/features/` passed 1,041
  tests in 109 files. The final banner-only run passed all 24 cases.
- `pnpm run typecheck`, focused ESLint with `--max-warnings 0`,
  `pnpm run i18n:check`, full specification lint, and catalog validation passed.
  All eight merged locale catalogs parsed without duplicate keys.
- `go test -race ./internal/orchestrator ./internal/orchestrator/executor
  ./internal/agentctl/server/adapter/transport/acp ./internal/runtimeflags
  ./internal/worktree -run
  'ResumeAttempt|CancelAgent|Inspection|Continuation|ProviderInterruption|Recovery|Delivery|Retired'
  -count=1` passed all five packages.
- Rebuilt desktop browser checks passed both durable reattachment cases and
  all 15 provider-continuation cases. The inspection case initially failed
  because task state was checked immediately after streamed response text;
  desktop and phone now poll the separate task completion transition.
- The corrected inspection case passed first attempt in the isolated CI
  runtime image with two CPUs, 4 GiB, one worker, and `--retries=0`.
  Two intervening host runs failed respectively at the contention notice and
  fixture workspace setup; neither is claimed fixed by the completion poll.
  The temporary incorrect response index was reverted before delivery.

All four phone cases passed first attempt in the same isolated CI image: actual
disconnect after reload, eligible continuation, cancellation with history, and
inspection retry with draft preservation. New-head hosted CI remains pending.
Native Windows/macOS containment and targeted durable-delivery
PostgreSQL/live-harness release gates remain open.

The normal merge hook exposed a cross-branch rollback contract change: the
rollback helper now returns a classified error, while this PR's synchronous
start failure caller discarded it. Extending the existing attempt-identity test
failed because the successor-attempt cause was absent. The caller now joins the
start and rollback errors and leaves the successor's state and identity intact.
No lint hook was bypassed.

`go test -race ./internal/orchestrator/executor -run
'Synchronous|ResumeAttempt|Inspection|Recovery|Delivery|Continuation' -count=1`
passed after this error-preservation change. Browser receipts above precede this
backend-only failure-path correction; they do not claim coverage of that path.

### Directory browser hosted flake follow-up

Run `37820281227`, shard 13 job `113463972650`, failed its first
fine-pointer directory-browser case because Files remained hidden after its tab
was clicked. Retry succeeded, so the fail-on-flaky gate correctly failed the
job. Its first-attempt screenshot shows Changes active; only the retry has a
trace. Source inspection confirms new Git output intentionally activates
Changes. The exact hosted timing cause remains unconfirmed.

The unchanged full spec passed 12 first attempts across three fresh CI-image
workers. The desktop picker fixture did not wait for its initial turn to settle;
the narrow-width case already did. Desktop setup now waits for the original
session's terminal turn state and the task's Review state before navigation.
Existing click, keyboard, geometry and reveal assertions remain unchanged.
Failure-only backend logs were added. Product focus behavior, assertion
timeouts, and retry policy remain unchanged.

`bash e2e/scripts/run-raw-e2e.sh --project=chromium --workers=1
 e2e/tests/task/directory-browser-hidden-folders.spec.ts --repeat-each=3
 --retries=0 --trace=retain-on-failure --reporter=list --output=/owned-output`
passed all 12 updated cases on first attempt using the isolated CI runtime image,
two CPUs and 4 GiB. `pnpm exec eslint
 e2e/tests/task/directory-browser-hidden-folders.spec.ts --max-warnings 0` and
`pnpm run typecheck` passed. Fresh hosted verification is still required.

The corresponding phone check,
`bash e2e/scripts/run-raw-e2e.sh --project=mobile-chrome --workers=1
 e2e/tests/task/mobile-directory-browser-hidden-folders.spec.ts --retries=0
 --trace=retain-on-failure --reporter=list --output=/owned-output`, passed both
cases on first attempt in the same constrained image. Full specification lint,
catalog validation and whitespace checks passed. Native and targeted live-state
release gaps recorded above remain open.

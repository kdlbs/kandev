---
created: 2026-09-30
status: in_progress
requirements:
  - REQ-EXECUTORS-FAILURE-VISIBILITY-001
system_design:
  - ../../specs/executors/system-design/executor-failure-visibility.md
legacy_specs: []
---

# Implementation Plan: Executor failure visibility

## Overview

Preserve actionable executor evidence, then admit durable scoped episodes and
settle only proven lost executions, then expose tested recovery guidance on every
task surface. Work orders are sequential. Initial implementation and local acceptance are complete. PR CI remediation
and review remain active delivery gates; Task 03 records the affected checks.
Backend and frontend regressions, rendered acceptance and final checks are tracked
in each work order. No production task or cluster resources are used for testing.

## Scope

In scope: typed observations, idle/shared inventory reconciliation, safe durable
history, exact-execution settlement, desktop/phone visibility, localization,
backend/frontend regression coverage and public recovery documentation.

Out of scope: storage sizing/provisioning, binary compatibility/upgrades, colors
task recovery, destructive automatic repair, prompt replay, production cluster tests.

## Evidence and coordination

Read-only source trace at `ff917a370` confirms Pod cause overwritten by container
projection, refresh short-circuiting inspection, memory-only status, tracked-only
regular polling, and generic stream-failure/startup waiting transitions. See the
[design](../../specs/executors/system-design/executor-failure-visibility.md) for exact symbols. Minimal regression fixture: Failed Pod with
reason Evicted and docker-data >12Gi message, two exit137 containers with generic
termination reasons, matching Bound workspace PVC. Assert the eviction explanation
survives normalization, episode storage, restart, summary readback, and UI reload.
Never infer an OOM from that fixture.

Additional parent-reported evidence: node reboot at 2026-09-29T23:03:53Z,
Running Pod with required-container CrashLoopBackOff after volatile bootstrap files
vanished, exit 2, recovered Docker sidecar and 126+ restarts. Duplicate-execution,
cleanup timeout and cleanup 401 hid the cause. Reported repair resumed successfully
at 2026-09-30T09:42:41Z with the same workspace, but a missing provider rollout caused
a fresh conversation. The [design](../../specs/executors/system-design/executor-failure-visibility.md#additional-incident-evidence)
records provenance and scope. No repair is authorized in this task by that evidence.

Related work inspected on 2026-09-30:

- PR [#3849](https://github.com/kdlbs/kandev/pull/3849), head
  `d1ca72f6f34f855eadc64a41c299dfe31df2f777`: admission/execution fences overlap.
  Re-read current head before implementation; this is not a dependency to blindly
  cherry-pick or replace. PR #4068 changes plugin instance creation, not this contract.
- Storage sibling `35c03538-ee40-4bb0-a361-8654d2aaffb4` was notified of ownership.
- Compatibility sibling `88948fb5-914f-4254-87cf-6c7e57024e1c` has a completed
  design package; its session was terminal at initial inspection. The parent later
  reported successful repair/resume with lost provider conversation. Its plan was
  read; this task did not create or repair a session.
  It owns safe helper refresh, activity gating, and planned main-container restart.
  This observer consumes eventual exact maintenance identity; it does not implement upgrades.

Confirmed intent is sufficient for planning. Missing provider facts are modeled
as uncertainty. No unresolved product question blocks the package.

## Technical approach

1. Typed resource inspection before control operations, preserving Pod/container
   evidence and sanitizing at the boundary. Compatibility matrix is in the design.
2. Durable episode and affected-session tables with unique admission fences;
   paged retained-inventory inspection, correlated turn settlement, typed recheck,
   and additive summary projection. Existing launch-error fields remain independent.
3. Reuse current session composer recovery/history surfaces, wire
   localization and navigation, prove reload/recovery behavior, publish user guidance.

The design's provider compatibility matrix is normative for work order scope.
All providers must distinguish unsupported/unknown from confirmed terminal facts;
only Kubernetes promises Kubernetes-specific diagnostics. No shared generic
transport error grants recovery or retry authority.

## ASCII UI preview

UI-01: Desktop shared or session-owned interruption, at the composer recovery location.

```text
Task title
Chat | Plan | Files | PR
<conversation>

[!] Executor evicted
    Recorded storage-limit cause
    Repair guidance; reset can delete data
    [Recheck status] [Show details]
    expanded: workspace / conversation / observation time
              [Technical details v]
```

UI-02: Phone uses the same inline recovery card and bounded scroll region.

```text
< Task title
<conversation>
+--------------------------------------+
| [!] Executor connection lost         |
| Worker unavailable; agent unverified |
| Restore connectivity, then recheck   |
| [Recheck status] [Show details]       |
| expanded: workspace and conversation |
| [Technical details v]                |
+--------------------------------------+
```

UI-03: Recheck preserves the recorded cause when current status is unverified.
Healthy recovery clears active controls, without asserting provider continuity.

UI-04: Partial recovery is a distinct warning card in transcript history.

```text
[!] Executor recovery confirmed: <time>
    Original conversation could not be restored.
    A new provider conversation was started.
    Recorded workspace evidence / retained Kandev transcript
[Normal working composer]
```

Shared ownership remains one durable episode; composers present that episode's
read-only recheck. Session-only episodes remain fenced to their matching session.
Task launch errors remain independently owned. No Reset or blind Resume is added.
Technical facts use readable bounded rows, not a floating JSON menu. Phone actions
have 44px targets and reuse the composer safe-area container; the card has one
bounded scroller. Tasks without sessions retain the card at the workbench bottom.
Fresh or unknown provider continuity uses warning styling; confirmed restored
continuity uses status/success styling. All copy remains localized.

## Tests

Names below are proposed regression tests to write first, not existing results.

| AC suffix | Regression evidence |
| --- | --- |
| .1, .6, .10 | `executor_failure_observation_test.go`: `TestExecutorFailurePreservesPodEviction`, `TestExecutorFailureExit137IsNotOOM`, `TestExecutorFailureSanitization`; terminal Pod rejects exec in reconnect/refresh tests |
| .2, .5, .8 | `manager_executor_failure_test.go`: `TestExecutorFailureTransientReconnect`, `TestExecutorFailureStaleGeneration`, `TestExecutorFailureExpectedStop`, successful refresh racing disconnect |
| .3, .5, .11 | `executor_failure_episodes_test.go` in repository/sqlite: reopen/duplicate/concurrent admission; `executor_failure_reconciliation_test.go` in task/service: idle/shared/zero-session/legacy inventory, API unavailable, crash retry |
| .4, .5, .8 | `executor_failure_test.go` in orchestrator: lost active turn, idle sibling, stale callback, dead predecessor/live successor, no workflow retry/completion, queue/question preservation |
| .6, .7, .8 | task service recheck/admission tests: stale stamp/foreign task/identity mismatch, retained workspace versus conversation, original cause survives failed retry |
| .3, .9 | `executor_failure_test.go` in task/statussummary: boot/rebuild/WS resolution revision and unrelated shared error coexistence |
| .12 | `executor_failure_observation_test.go`: `TestExecutorFailureRunningPodCrashLoop`; healthy sidecar/unready required container, last exit 2, optional reboot evidence, no inferred reboot from SandboxChanged |
| .13 | orchestrator `executor_failure_test.go`: `TestExecutorFailureCleanupCauseChain`; cleanup timeout then 401, verified-live duplicate sentinel preserved; confirmed loss blocks duplicate cleanup/relaunch; STARTING with empty error still shows episode |
| .5, .12 | task/service `executor_failure_reconciliation_test.go`: `TestExecutorFailureCrashLoopDeduplicates`; 126 repeated restarts yield one unresolved episode, no duplicate history/alerts |
| .14 | lifecycle/orchestrator `TestExecutorFailureFreshConversationOutcome`: missing original rollout creates permitted fresh conversation, successful RPC/WAITING_FOR_INPUT/empty error cannot report restored; persistence/reload and stale-attempt guards |
| .7, .9, .10 | web `task-shared-error.test.tsx`, `task-status-summary.test.ts`, proposed `executor-failure.test.ts`: action eligibility, unknown codes, old payloads, episode revisions, safe details |

## E2E tests

Proposed `tests/session/executor-failure-visibility.spec.ts` (chromium) and
`tests/session/mobile-executor-failure-visibility.spec.ts` (mobile-chrome):
open persisted eviction after navigation/reload, switch session/Files/Plan, recheck
failure and success, transcript history after recovery, no duplicate actions,
coexisting independent errors, zero-session task preview, unknown status and no
false OOM. Assert phone drawer/targets, long localized details, scroll containment,
focus return and no horizontal overflow. Also show Running Pod/CrashLoopBackOff
with secondary cleanup cause, and successful partial recovery with a fresh provider
conversation despite empty session error. Reload retains the result marker, working
composer and no stale recovery controls. AC .3, .5-.10, .12-.14.

Proposed `tests/kubernetes/executor-failure-visibility.spec.ts` (containers):
use an isolated disposable Kind cluster and owned task fixture, write workspace
sentinel, terminate the owned Pod or restart its container, verify shared/idle
visibility and retained PVC, restart isolated backend, recheck history and resource
identity, prove no prompt replay or destructive recovery. Use fake Kubernetes API
fixtures for the exact reported 12Gi eviction message. The owned Kind case explicitly deletes its test-owned Pod. A tiny-volume
experiment did not produce an Evicted reason reliably, so it is not acceptance
evidence. Do not reuse production kubeconfig/context/resources. Fake API/controller fixtures also
model Running Pod/exit2/CrashLoopBackOff and volatile bootstrap loss while the disk
marker survives; never reboot a host or erase real credentials to reproduce it.
Use the mock provider missing-rollout path for fresh-conversation outcome evidence.
AC .1-.8, .11-.14.

## Work orders

- [x] [Task 01: Preserve and classify executor observations](task-01-observations.md)
- [x] [Task 02: Persist and reconcile executor failure episodes](task-02-durable-episodes.md)
- [x] [Task 03: Deliver visible recovery and regression evidence](task-03-visible-recovery.md)

## Verification results

Implementation and affected local checks are complete. The final owned Kind
Pod-loss acceptance passed with retries disabled; see task 03 for the proof and
preceding red regressions. Remote PR CI/review remain separate delivery gates. Design-package validation passed on 2026-09-30:
`python3 scripts/list-docs.py validate` (333 decisions, 1262 specifications),
`python3 scripts/lint-spec-files.py --all`, and `git diff --check`.
Each work order contains independently rooted commands and its actual results.
Runtime, persistence, frontend, localization and public documentation changes
have been exercised; do not interpret the blocked Kind setup as passed acceptance.

## Risks

- Existing automatic refresh and generic AgentctlError paths can race; retirement
  must not run under locks that event subscribers reacquire.
- A vanished Pod may leave no termination details. Preserve captured evidence;
  explicitly report unknown cause if none was captured.
- Persisted inventory lacks some historic baselines. No synthetic backfill or
  claiming provider conversation retention based on PVC state.
- An API partition cannot prove a process dead. Preserve capacity/admission safety
  while showing uncertainty, even if that delays user recovery.
- Open admission/compatibility work may change exact fencing seams; refresh evidence
  before integration. Public docs now describe implemented failure reporting.

## Verification checkpoint

Observation and durable-state work orders are complete with targeted, race and
PostgreSQL evidence. The desktop/phone implementation, locales and public docs
are verified. Task 03 remains open pending isolated Kubernetes acceptance.
Image loading now succeeds with bounded cold-import budgets and an isolated image
export daemon. Startup events identified cold local-path provisioning consuming
27 seconds of the fixture's 30-second Pod readiness deadline. The owned test now
uses the existing executor setting for a 180-second readiness budget; production
defaults are unchanged. Production evidence is not substituted.
The exhaustive repository rerun with a 20-minute budget passes: 1,775 cases,
no failed assertions (1,168.022s). It supersedes the initial 10-minute timeout.
See each work order's Results for passed checks and the remaining Kind blocker.

## Attributed recovery evidence checkpoint

The parent's 2026-09-30 pinned-task report records legacy readiness-timeout cleanup
deleting a managed Pod and PVC during Resume/Continue, followed by reconstruction
from published commits or recorded edits. Requirements, design incident evidence
and public guidance now distinguish historical volume retention from original
workspace continuity after replacement. The reported empty-operation `end_turn`
gap remains adjacent completion-correlation work, not a new executor cause or an
expansion of this implementation. No production inspection or repair was performed.
This documentation checkpoint does not change the open Kind acceptance gate.

### October 6 acceptance checkpoint

Portability cleanup removed private environment names from repository artifacts
and SSH test examples. The affected SSH live-status regression passes. The full
source/documentation scan has no matches for the named private environments.
Frontend typecheck, changed-fixture ESLint, 11 fixture helper/policy tests,
specification catalog validation and specification lint pass.

The latest isolated attempt exceeded the 600-second image-load budget before
task launch. Local runtime cleanup also reported a missing container exit event.
Earlier attempts reached startup and exposed cold provisioning and interrupted
control-plane leases, addressed by test-only readiness budgets and checks. The
real Pod-loss acceptance assertions have not passed; do not mark task 03 or the
overall plan complete. No production timeout defaults or resources were changed.

Owned infrastructure cleanup completed after local runtime event handling recovered:
test node, isolated image-daemon container and data volume, owned bridge/network,
partial archive and temporary dispatcher were removed. No shared daemon restart
or production mutation was performed. Kubernetes acceptance remains pending.

### Worker connectivity verification results

The responsive-API/unavailable-worker regression is implemented and verified.
Pod readiness and disruption evidence override stale container readiness without
inferring process death. Cached executor status becomes unknown immediately;
a stable observation persists after the disconnect grace window. Ownership and
active turns remain held, with primary connectivity evidence preserved separately
from cleanup blockers. Bound volume inventory does not imply workspace access.

Affected backend tests and focused worker race regressions pass; Go static analysis
reports zero issues. Frontend coverage passes (25 tests), as do typecheck, changed
file lint, localization checks, public documentation tests and specification checks.
Freshly built desktop and mobile suites each pass both scenarios, including an
active held turn through reload and failed recheck without completion or replay.

The original real Kubernetes Pod-loss acceptance gate remains open. This focused
regression uses fake Kubernetes resources and isolated UI fixtures, not production
evidence. Task 03 and the overall plan remain in progress; changes are uncommitted.

### October 7 presentation correction

User feedback supersedes the top executor strip and floating details surface.
Active executor incidents now use the regular composer recovery location, with
inline details and bounded readable technical facts. Shared persistence ownership
and all read-only recheck safeguards remain unchanged. No-session tasks retain
a bottom recovery card. Fresh-conversation history is a prominent warning card;
confirmed continuity uses success/status styling. See task 03 for the revised
desktop/phone preview and focused validation.

### October 7 correction verification results

The revised composer/history presentation is implemented. Three new behavioral
component regressions failed before implementation; the final focused cohort
passes 22 tests across six files, including shared composer ownership and recovery
warning/status semantics. Typecheck, changed-file ESLint, localization checks and
new-copy ratchet pass. Public documentation tests pass (62) and 47 published pages
validate; specification catalog/lint and diff whitespace checks pass.

Managed freshly built desktop acceptance passes both scenarios (33.2s), and
mobile acceptance passes both scenarios (19.4s). Coverage proves bottom composer
placement, absence of a duplicate top executor banner/modal, inline details,
read-only recheck/reload, active-turn ownership without replay, distinct fresh
conversation warning, contained scrolling and phone target sizing.

The isolated user-requested instance serves the updated build at its existing
URL. Browser checks verify all six seeded routes, failed worker recheck/reload and
inline phone recovery. The recovered example copy now describes history rather
than directing users to nonexistent active error controls. No production instance
or Kubernetes resource was changed. The original real Kubernetes Pod-loss gate
remains open; task 03 and the overall plan remain in progress and uncommitted.

### Final Kubernetes acceptance

The initial real scenario exposed missing connection hydration, then unsafe
passive workspace recreation after restart. Both defects now have red/green
backend regressions. The fresh managed Kubernetes scenario passed with retries
disabled, proving durable missing-Pod evidence, retained volume identity,
restart/reload, the regular composer card, read-only recheck, no replacement Pod,
and no prompt replay. Its owned cluster, image, and backend were cleaned up.
Task 03 records the final local regression, race, lint, and documentation results.
PR #4345 current-head remote CI/review remain delivery gates, not local acceptance.

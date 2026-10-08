---
id: "03-visible-recovery"
title: "Deliver visible recovery and regression evidence"
status: in_progress
wave: 3
depends_on:
  - "02-durable-episodes"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-FAILURE-VISIBILITY-001
acceptance_criteria:
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.1
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.3
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.5
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.6
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.7
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.8
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.9
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.10
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.11
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.12
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.13
  - AC-EXECUTORS-FAILURE-VISIBILITY-001.14
system_design:
  - ../../specs/executors/system-design/executor-failure-visibility.md
---

# Task 03: Deliver visible recovery and regression evidence

## Summary

Consume the durable episode through existing task-shell and session recovery owners.
Deliver localized desktop/phone recovery guidance, isolated full-flow proof, and
public docs that distinguish retained files from lost or unknown conversation state.

## In scope

Inline composer recovery cards, session history/recovery references, navigation decoration,
frontend normalization and revisions, six-locale copy, E2E fixtures, isolated Kind
acceptance, executor/task public docs. Compare rendered views with UI-01..04.

Cover Running Pod/CrashLoopBackOff, secondary cleanup blockers and UI-04's successful
partial recovery/fresh conversation on desktop and phone, including reload with
empty active error. Extend public guidance to separate restored compute, retained
workspace, retained Kandev transcript and native provider conversation continuity.
Do not imply bootstrap credentials or global Git configuration survived a restart.

## Out of scope

No storage provisioning, binary upgrades, destructive shortcut or production recovery.

## Acceptance

- Failure cause survives reload, backend restart and tab/session switching, with one authoritative episode, independent errors preserved, and no false OOM or unsupported Resume.
- Desktop and phone execute authorized recheck and eligible recovery with matching results; history persists, stale resolution cannot clear a successor, and phone geometry/accessibility/localization meet UI-01..04.
- Isolated Kubernetes evidence proves retained workspace and no unsafe replay/reset; public documentation matches shipped behavior and all linked implementation checks pass.

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

## Verification

Commands run from repository root; new test files named here are implementation
outputs, not existing checks. Demonstrate each regression failing before its fix.
Run the affected baseline tests as well if implementation touches additional suites.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/executor-failure.test.ts lib/task-status-summary.test.ts components/task/task-shared-error.test.tsx components/task/session-error-details.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/executor-failure-visibility.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-executor-failure-visibility.spec.ts)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/kubernetes/executor-failure-visibility.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/task/task-shared-error.tsx`
- `apps/web/components/task/task-launch-error-context.tsx`
- `apps/web/components/task/simple/components/task-launch-error-entry.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/lib/task-status-summary.ts`
- `apps/web/lib/executor-failure.ts (new)`
- `apps/web/lib/types/ and lib/state/ (status-summary types and replacement reducers)`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja}/`
- `apps/web/e2e/tests/session/{executor-failure-visibility,mobile-executor-failure-visibility}.spec.ts (new)`
- `apps/web/e2e/tests/kubernetes/executor-failure-visibility.spec.ts (new)`
- `docs/public/executors.md`
- `docs/public/tasks-and-workflows.md`

## Dependencies

02-durable-episodes. Refresh open admission/compatibility changes before coding.

## Risks

Identity and prompt-generation races must fail closed. Inspect/recheck cannot mutate
resources or cause prompt replay. Tests must use owned disposable data and cluster;
if container prerequisites are absent, record the exact blocker and leave completion
pending rather than substituting production. Reuse existing database test harnesses
for supported dialects and ensure a regex command actually discovers the new tests.

## Parallelism

`sequential`. This work order does not authorize delegation.

## Inputs

- [Requirements](../../specs/executors/requirements/executor-failure-visibility.md)
- [System design](../../specs/executors/system-design/executor-failure-visibility.md)
- [Plan, fixture matrix, source baseline and coordination](plan.md)
- Scoped AGENTS.md, /tdd, and /mobile-parity plus /e2e for rendered changes.

## Results

Implemented shared and session-owned recovery surfaces, localized desktop dialog
and phone drawer, immutable recovery history, revision-safe HTTP/WS merging,
request coalescing/response fencing, navigation decoration, and public guidance.
All six shipped locales plus pseudo cover the new copy.

Validation passed:

- Focused frontend cohort: six files, 37 tests. The shared-error component cohort
  was rerun after its test-only literal cleanup and passes.
- Changed TypeScript ESLint with zero warnings, typecheck, i18n check and ratchet.
- Rebuilt desktop and phone E2E, then both rerun after the final backend edit:
  one test each, passing reload, failed/confirmed recheck, fresh-conversation
  history, recorded retention timestamp, drawer sizing and touch targets.
- Public-doc validator tests: 62 passed. Public docs: 47 pages validated.
  Specification/decision validation and specification lint pass.

Kubernetes acceptance remains pending. Image export and import now succeed using
isolated image-build infrastructure and a bounded 600-second load budget. The
worker fixture allows 1,200 seconds for cold setup. Startup events identified
27-second local-path provisioning consuming the 30-second test executor deadline;
this test uses the existing executor setting for a 180-second readiness budget.
A subsequent cold import interrupted scheduler/controller leases and runtime
restart handling. The real test now waits for both control-plane Pods to be ready
before task launch, with a bounded 240-second readiness check. Production defaults
are unchanged. Fixture helper/policy checks pass: 11 tests.

The real case explicitly deletes only its owned Pod, asserts `PodNotFound`, retained
PVC identity, reload/backend-restart durability, read-only recheck and no prompt
replay. Exact Evicted classification remains covered by backend fixtures. A tiny
volume experiment ended with a Succeeded Pod and does not count as eviction proof.
No production task or cluster was mutated.

Task 03 and the overall plan remain in progress. No commit or PR delivery is
claimed while the Kubernetes acceptance gate is pending.

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

### Worker connectivity regression

Extend AC .2/.6/.13 coverage with responsive API, owned Running Pod, stale main
container Ready=true, Pod Ready=false and explicit worker/disruption evidence.
Prove uncertain observation and safe preflight, durable dedup/reload, secondary
cleanup failure preservation, no settlement/replay, and shared desktop/phone
connection-lost guidance with Bound volume inventory separated from accessibility.
Use fake Kubernetes resources and isolated UI data, never production reproduction.

UI-05 (desktop dialog and phone drawer reuse UI-01/02 geometry):

```text
[!] Executor connection lost                 [Details]
Kubernetes reports the worker is unavailable.
Agent status cannot be verified.
Volume inventory retained; access and integrity unverified.
Restore worker connectivity, then recheck.
[Recheck status] [Technical details]
```

### Worker connectivity regression results

UI-05 is implemented through the shared desktop dialog and mobile drawer. Six
localized catalogs and the pseudo-locale provide connection-lost guidance, agent
status uncertainty and separate workspace access/integrity uncertainty. Technical
details retain bounded Pod conditions and the secondary cleanup blocker.

The freshly rebuilt desktop suite and mobile suite each pass both scenarios. The
worker scenario holds a real mock-agent turn active, seeds isolated display
evidence, reloads and rechecks, then verifies unchanged reply identifiers and a
RUNNING session without completion or replay. Mobile target sizing and overflow
checks pass. These display fixtures complement backend admission tests; they do
not reproduce a production incident or close the real Kubernetes acceptance gate.

Frontend tests pass (25 tests across three files), along with typecheck, changed
file ESLint, localization checks, 62 public documentation tests and validation of
47 public pages. Specification catalog validation and full specification lint pass.
The original real Pod-loss gate remains pending. Task 03 remains in progress and
the implementation remains uncommitted.

### October 7 user-requested composition correction

This supersedes UI-01/02/05 executor strip and dialog/drawer geometry. Use the
existing session recovery card location above the composer on desktop and phone,
with cause, one guidance paragraph, recheck and inline expansion. Technical facts
use readable rows rather than JSON. Unrelated task launch errors keep their current
handling. The transcript recovery card is visibly framed; fresh conversation loss
uses warning styling and restored continuity uses status/success styling.

```text
Composer recovery (desktop and phone):
[!] Executor no longer available
    Cause and repair guidance
    [Recheck status] [Show details]
    expanded: workspace / conversation / technical facts

Transcript recovery:
[!] Executor recovery confirmed: timestamp
    A new provider conversation was started.
    Workspace evidence / retained Kandev history
```

Phone composition reuses SessionRecoveryCard: one bounded card scroller, wrapping
actions with at least 44px targets, existing composer safe-area container, no
executor overlay. Regression coverage includes no duplicate top executor notice,
inline expansion, reload/recheck, distinct recovered notice and containment.

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


### PR #4345 review remediation (2026-10-08)

The ready PR reconciles current runtime conversation guards, retained prompt failures, and sidebar running-session projection without discarding executor episodes. New regressions cover unsupported-runtime disconnect fallbacks, unavailable ownership authority, Docker not-found versus API failure, retained Remote Docker inspection, graceful Pod deletion, bounded inspection paging, background startup reconciliation, and unverified launch safety. Ownership uncertainty never authorizes prompt queuing, cleanup, or replay.

Desktop and mobile visibility scenarios pass, including restoring the composer from an HTTP recheck before websocket delivery, persisted outage guidance, and the fresh-conversation recovery notice. Both scenarios restore user settings. The isolated Kubernetes acceptance test uses the composer card and inline technical disclosure. Public docs now name Show details on that card. Korean joins the complete locale set.

The status-summary projector avoids durable executor reads for unrelated events after initial hydration; the regression and full projector suite pass. The public documentation, localization, frontend unit/type/lint, specification, lifecycle, reconciliation, SQL persistence, backend app, and orchestrator contract checks pass. Full backend lint and the real isolated Kubernetes Pod-loss gate remain in progress; broad package-suite timeouts are not claimed as passes. Refreshed desktop/mobile screenshots are isolated synthetic examples, with local terminal output hidden.


The renewed real Kubernetes attempt first encountered a missing default Docker bridge during the image's permission-only build step. The fixture now builds with `--network none` and successfully creates an isolated control plane. A transient backend fixture setup timeout passed on the diagnostic retry. Image import still failed before the loss assertions, and automatic cluster cleanup also failed. The exact fixture-owned node was then verified by its cluster label and removed; its image is absent and its backend port is closed. The main instance and its resources were untouched. This is an infrastructure-blocked acceptance gate, not a pass. Expanded backend executor, reconciliation, projection, and model regressions pass; the full projection suite also passes.

Full backend lint (`./...`, comparison base `56cc19514e20b1c78c366005a9358ba9a8857393`, five-minute deadline) passed with zero new issues after cache warming. The successful run completed in 1m50s. Earlier loading timeouts and the interrupted stalled analyzer run remain failed attempts, not passes.

The first normal merge commit was blocked by the hook resolver after the remote main ref advanced beyond the recorded merge input. Its old equality check selected the pre-merge branch point and reported upstream-only lint findings. The resolver now accepts an incoming merge commit that remains an ancestor of the canonical base ref. Regression coverage verifies both remote-ref advancement and rejection of an unrelated merge input. The resolver tests and shell syntax checks pass; no hook was bypassed and no unrelated upstream source was modified for those findings.


### Docker recovery acceptance reconciliation

The container-backed CI regression still clicked the pre-episode Restart control
while disconnect classification was in flight. Confirmed executor loss now owns
the composer, and read-only Recheck must never start compute. The focused scenario
now waits for the durable card, verifies no Resume is advertised while stopped,
checks that Recheck leaves compute stopped, explicitly repairs only its owned test
container, and checks recovery with unchanged environment/container identity and a
usable terminal. This exercises the approved repair-then-recheck contract rather
than treating a transient old control as recovery authority.

The focused composer/recheck unit tests passed (10 tests across three files), and
the changed E2E helpers/spec passed ESLint. The first local attempt was blocked by
missing shared Go-cache artifacts. Rebuilding with the task-owned cache succeeded,
but Docker fixture image creation failed before the scenario ran. The revised
container scenario is therefore pending current-head CI verification; local setup
failure is not a pass. The real Kubernetes acceptance gate remains open as well.

### Idle Docker observation cadence

The current-head container CI fixture failed the durable-card assertion in all
three attempts because its five-second default timeout preceded the approved
one-minute idle-environment reconciliation sweep. The assertion now waits up to
90 seconds for admission, retaining every stopped-compute, read-only recheck,
identity-preservation, and terminal recovery assertion. No production timing or
recovery behavior changed.

The focused real Docker scenario passed with retries disabled (one test, 1.8
minutes total). Local verification used a disposable daemon with its own socket,
data directory, and bridge; the existing daemon was not reconfigured. A temporary
host-network fixture image build accommodated its isolated network policy, and
that fixture source change was restored before delivery. The normal managed
backend, web, and plugin builds had completed immediately before this run. The
current-head CI Docker assertion supplies the red evidence; the earlier default
daemon local attempt failed at image setup and is not assertion evidence.

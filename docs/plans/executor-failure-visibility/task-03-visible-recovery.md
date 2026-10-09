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

### Persisted Kubernetes inspection correction

The retries-disabled, freshly built Kubernetes acceptance test reached provider
acceptance, wrote its managed-workspace sentinel, and deleted only the fixture's
owned Pod. It then failed to record `PodNotFound` within the admission deadline.
A backend regression reproduces the underlying status-client configuration
error: durable resource inventory omits connection settings, while inspection
previously tried to reconstruct a client directly from that inventory.

Inspection now overlays current executor connection settings while preserving
recorded namespace, Pod UID, and ownership. Resource-only executor identity
supports restored inventory; conflicting identities fail closed before an API
query. Regression coverage verifies current connection changes, immutable target
metadata, no control handshake or agent creation, and no Pod/PVC deletion. A
second red regression proves legacy Kubernetes inventory must use its Pod UID
without requiring a Docker-style container ID; the portable inventory query now
admits those retained session records.

These fixes restore the existing documented inspection contract. Public recovery
controls, workspace uncertainty, and provider continuity semantics do not change.
Focused backend regression and real Kubernetes acceptance results remain pending
at this checkpoint; the earlier Kubernetes failure is not claimed as a pass.

The next real Kubernetes run passed missing-Pod admission and durable reload, but
failed the final absence assertion: a passive workspace request from the rendered
Files/Terminal area created a replacement Pod after restart. The fixture's owned
cluster and image were cleaned up. This run is a red safety regression, not a
pass. New backend regressions cover cold and cached workspace entry points,
failed failure-inventory reads, and exact environment/generation versus legacy
session scope. Passive workspace admission now refuses an active incident before
runtime creation or registration; unrelated scopes and resolved incidents retain
ordinary workspace admission. Public executor guidance records that boundary.

### Final local acceptance results

The fresh managed-build Kubernetes Pod-loss scenario passed with retries disabled
(one scenario, 2.8 minutes; 9.4 minutes including isolated setup and cleanup). It
verified persisted `PodNotFound` evidence and retained PVC identity before and
after backend restart, the regular composer card and inline details, read-only
Recheck, no replacement Pod, and unchanged provider reply IDs. Passive terminal
requests were refused before runtime creation while the episode remained active.
The exact owned cluster, worker image, and backend were removed by the fixture.
No production task, instance, cluster resource, or provider conversation was used.

The final affected lifecycle, workspace admission, persistence, and task-service
regressions passed. Source lint for both changed Go packages reported zero issues.
SQL guard and the persistence store-conformance race suite passed. Public docs
validation passed 63 tests and 47 pages; specification catalog and lint passed.
Accepted requirements/design statuses now reflect the approved contract. No new
rendering or translated copy was introduced by this backend correction; existing
composer/history desktop and phone assets remain representative.

Local implementation acceptance is complete. PR #4345 remote CI and review
readiness remain separate delivery gates, tracked in the live PR validation block.

### Automatic session-open admission regression

The container CI shard exposed an outdated refresh expectation that implicitly
authorized restarting an externally stopped executor. The repaired browser case
checks physical container state, unchanged environment and episode identity, and
unchanged provider reply IDs through reload and read-only Recheck, followed by
explicit fixture-owned repair and terminal reconnection. Its stronger assertion
also exposed a real automatic `session.launch` path that bypassed workspace
admission and started the container on reload.

Focused persistence-backed tests reproduced automatic-resume eligibility allowing
an active legacy or shared environment incident. Automatic eligibility now reads
the matching active incident, preserves scope and generation, and fails closed
when the incident read is unavailable. Session-open launch admission and session
focus reuse that eligibility check; explicit recovery retains its existing owner.
The focused Go regressions and existing session-open tests passed; the complete
orchestrator package also passed. Both affected real Docker scenarios passed with
retries disabled after a fresh managed build. Temporarily removing automatic
admission made the repaired refresh case fail at its physical-state assertion
(expected `exited`, observed `running`). The source was restored byte for byte
before final verification. Go and browser-test lint passed. Documentation and
specification validation passed. No rendered copy changed. Remote CI/review
readiness remains a separate delivery gate for the pushed head.

### Shared-scenario CI coverage correction

The executor/container shards passed at the published fixup head with zero
retries (145 passed, seven skipped). Several normal browser shards reached their
45-minute job cap; their cancellation is not a passing test result. Comparison
with the selected successful main timing baseline and an exact-source ordered
reproduction are separate from the coverage correction below.

Manifest inspection and a read-only Playwright selection exposed shared scenario
tests keyed by their helper definition path rather than the runnable importing
spec. This omitted the desktop and mobile executor recovery scenarios from the
CI file selection. A real Playwright discovery fixture reproduced the mismatch;
blob timing regression reproduced the inconsistent file key. Catalog and timing
collection now retain the outer owning spec through nested helper suites. The
27 affected planner, runner, and timing tests passed. Reverting catalog ownership
made the repaired discovery assertion fail; the source was restored exactly.
Lint and formatting passed. Regenerated manifests select both recovery wrappers
and no helper paths. Independent Playwright list discovery for all 14 normal
shards matched every assigned project/spec count exactly. The ordered, exact-CI
source browser reproduction was stopped after preserving the independent
intentional-stop race evidence below; this coverage correction does not claim
to repair the job-cap cancellations.

### Intentional-stop disconnect race remediation

Exact-source ordered browser reproduction captured intentional agent deletion
followed by the new healthy-disconnect reconnection waiting ten seconds for the
deleted instance. The run was interrupted after preserving that evidence (127
passed, one skipped, one interrupted, 160 not run); it is not a full shard pass.
The concurrent main run completed all 14 normal shards within the existing cap.
A lifecycle stop regression reproduced two extra inspections during teardown.
The fix suppresses classification during intentional stop and idle suspension,
revalidates queued callbacks, and serializes stream reconnection with teardown.
Successful and failed stop cases, unexpected healthy disconnect, and uncertainty
coverage passed. Failed stops release suppression for a later recovery. Changed
Go lint passed. The full lifecycle package passed (114.908 seconds), and the
focused user-stop, cleanup, failed-stop, and disconnect cases passed under the
race detector (2.452 seconds). Catalog and specification validation passed.
All 27 focused desktop/mobile browser cases passed with retries disabled,
including all nine cases whose earlier CI attempt failed. The independent
mobile navigation geometry assertion now waits for the finite sheet entrance
animation before measuring its unchanged 44-pixel minimum touch target. Both
mobile failure areas passed three repetitions each with retries disabled (six
cases, 1.4 minutes). Remote CI remains a separate delivery gate.

### Recovery owner and terminal-agent CI remediation

The next published-head CI run completed every normal shard within its existing
job limit. All six container shards passed (145 passed, seven skipped, no
retries), and backend/frontend checks passed. The remaining browser failure was
an automatic recovery workspace pane without its session recovery link; the
complete blob audit also identified six retry-resolved scenarios. Documentation
coverage rejected legacy shard work-order metadata. Aggregate browser gates
correctly remained red.

A workspace-only automatic failure now resolves its matching session owner even
when boot status metadata is absent. An unrelated session and an independent
workspace restoration attempt retain their own errors. The missing-link unit
regression failed before the correction and passed afterwards. Desktop and phone
browser scenarios passed without retries.

Repeated isolated fixtures reproduced fresh recovery incorrectly blocked after
ordinary agent crash: terminal local agent inventory clears its process handle,
while its controller remains attached. Observation now uses that controller only
for the same owned terminal standalone execution and still verifies attachment
and availability. Healthy-controller recovery, unknown/terminated-controller
blocking, live-row rejection, rotated execution rejection and remote-row rejection
have focused regression coverage. The full lifecycle package passed (118.126
seconds); the final boundary cases passed under the race detector.

Native Kanban task menus no longer compete with their confirmation's modal
pointer lock. A 60-second closing-menu animation made the original lock failure
deterministic; four corrected desktop repetitions passed with retries disabled.
Context menus keep their existing interaction policy. Alert-dialog recovery now
uses Radix's closing-animation cleanup and preserves another modal's lock when an
unrelated closed root unmounts. Six phone confirmation/body-lock cases passed
without retries. The 53 focused frontend regressions passed across eight files.

Controlled command snapshots exclude only late real snapshots for their own
session. Other notifications, sessions, binary frames and malformed frames still
flow. Workflow preview waits for the reused review session to become idle; diff
fixtures wait for the complete seeding turn; dialog geometry setup expands the
actual sidebar before clicking its visible New Task control. These fixture fixes
do not inflate timeouts or force clicks. Suite-local retry overrides were removed
so an explicit no-retry run remains authoritative. Legacy shard plan metadata was
migrated to its existing requirement/design references and passed the validator
from the trusted base.

No new product copy or public command/configuration contract was added in this
fixup batch. Existing public executor/recovery guidance remains accurate. Remote
CI, review disposition and final-commit screenshots remain delivery gates.

Final affected browser verification passed all 12 cases (two repetitions of each
of the six CI failure scenarios, 3.7 minutes) with retries disabled. Final
frontend typecheck, localization ratchet, changed-file lint, trusted-base
coverage validation, catalog validation and specification lint passed. The final
controller ownership matrix passed under the race detector (1.526 seconds).

### Mobile menu and fixture readiness CI remediation

The next published-head CI run passed all six container shards (145 passed,
seven skipped, no retries), but one normal shard failed the phone Kanban outside
tap regression and another exhausted its existing job limit. The partial
reports also identified a retry-resolved task-creation setup failure. Remote
browser aggregate gates remained red.

Phone Kanban menus retain their modal backdrop while open and during ordinary
dismissal. Selecting an enabled action releases the menu's modal ownership
before invoking the action, allowing the native confirmation to take ownership.
Desktop menus retain their existing non-modal policy. Mouse and keyboard
selection handoff regressions failed before the fix and passed afterwards;
disabled actions do not transfer ownership. A 60-second closing-menu animation
exposes stuck locks without inflating the test timeout. All five phone scenarios
passed twice with retries disabled (ten cases, 1.1 minutes), covering ordinary
fade, outside-tap focus return, confirmation handoff, nested task sheets and
workspace selection. The 58 focused frontend tests passed across six files;
changed-file lint, typecheck and localization ratchet passed.

Task-creation QA expands a collapsed sidebar through its existing UI helper
before clicking New Task. The regression explicitly collapses navigation first;
the original click timed out and the corrected setup passed. The final
desktop verification after the selection-handoff change passed all 13 affected
cases with retries disabled (4.2 minutes).

The timed-out shard was reproduced using its original assignment and exact CI
checkout/runtime artifacts. Its first reproduced failure was a Bitbucket
fixture whose initial agent turn moved the task to another workflow step while
its context menu was open. Waiting for that turn's successful idle state before
opening the menu made the complete plugin flow pass with retries disabled
(32.4 seconds). The remaining original assignment is being checked with only
that fixture readiness correction. Missing CI artifacts are not evidence that
the unreported tests passed.

These changes restore documented interaction behavior and settle test fixtures;
no public command, configuration, terminology or localization contract changes.
Existing task and mobile documentation remains accurate. Complete shard
investigation, final-commit captures and fresh remote CI/review evidence remain
delivery gates.

The continued original-shard run reproduced a retained retry cancellation
failure. The request reached the backend but returned `cancelled: false` after
one automatic attempt: the next waiting reservation retained the episode's
cumulative count and was mistaken for a currently dispatched attempt. Cancelling
a waiting reservation now fences its dispatch claim and retires automatic work
without cancelling the idle agent runtime, preserving the actual count in its
durable error. The regression failed before the correction and passed afterwards;
the cancellation/status matrix passed under the race detector (2.851 seconds).
This restores the existing waiting-cancellation contract in
[transient runtime continuity](../../specs/platform/requirements/transient-turn-runtime-continuity.md).
The browser regression observes the cancellation response and compares reported
attempts to the ACP trace instead of assuming startup finishes before the first
retry. All four continuity scenarios passed against the recorded diagnostic runtime,
including cancellation and all five retries ending in exhaustion. An earlier
local build refused retry admission; no fail-closed policy was weakened to make
the reproduction pass. Final merged-head browser evidence remains pending.


Current-base integration preserves the newer native restore coordinator and
durable stream identities alongside executor observation. Missing native state
blocks automatic replacement; successful explicit continuation records a fresh
conversation only after generation commit, context submission, and recovery
settlement. Historical fresh-conversation notices remain durable. Schema startup
initializes executor episodes, session continuity, and delivery journals together.
The focused regression rejects uncommitted or replaced continuation candidates.

The continued original-shard diagnostic run passed 42 cases, skipped one gated
fixture, and stopped at a storage policy notification locator. Consecutive saves
left two legitimate success notifications visible. The test now waits on the
save response and checks the latest notification; the focused case passed with
retries disabled. The remaining 139 original cases are being checked explicitly.
The next diagnostic segment passed 21 cases and exposed a sidebar reorder
regression; 117 cases were not reached. Escape during an active reorder also
closed its enclosing editor. Desktop and phone surfaces now consume that Escape
while allowing ordinary dismissal after the reorder ends, including the existing
inner confirmation handler. Two new surface regressions failed before the fix;
the focused sidebar unit matrix passed all 21 cases afterwards. Browser tests
cover cancel, retained focus and unchanged order on both surfaces. Pointer tests
observe the move announcement and current target bounds before release. The
complete desktop and phone sort scenarios passed with retries disabled against
the recorded diagnostic runtime (52.7 and 49.9 seconds). Two prior runs failed
at backend readiness during measured disk pressure before entering the test;
neither timeout nor retry policy changed. Fresh merged-tree scenarios follow.

Merged-tree executor failure, cancellation, and explicit continuation package
tests passed across the lifecycle, orchestrator, SQLite, task service, handlers,
and summary packages. The missing native rollout test verifies restore admission
fails without silently starting a fresh provider conversation. Frontend unit
verification passed 86 cases across ten files; the native backend and plugin
package builds passed. Strict merged-tree browser verification passed six cases
with retries disabled: executor failure and worker outage on both viewports,
plus desktop and phone sort editing (2.2 and 1.6 minutes). Typecheck, full
localization checks, changed-file lint and trusted documentation coverage passed.
Final captures and new remote checks remain delivery gates; old-head success is
not final delivery proof.

The ninth CI E2E build stopped in test discovery before scheduling the browser
shards. A marker-free base merge left two imports of the same readiness helper
in the workflow override fixture. Full Playwright discovery reproduced the
syntax error; removing the duplicate preserved both sides' readiness assertions.
Full discovery and the exact CI shard planner then passed, followed by the
fixture's ESLint check. After future base integrations, full discovery and shard
planning supplement focused browser tests so unexecuted fixtures are parsed too.
The final native capture build passed using a task-owned Go cache after a shared
cache file disappeared during compilation. No shared cache was cleared and no
test timeout, retry setting, or source gate was weakened.

The tenth-head CI report audit covered all twenty shard reports and matching
retry metadata. It found three underlying browser failures; the four failed
check rows included report and suite aggregates. All current failures were
reproduced with retries disabled before remediation:

- A preceding workflow-navigation test persisted a hidden Review column in the
  shared worker. Per-test settings now clear hidden columns. The packaged
  provider test uses the native Add panel path when the saved layout does not
  retain its review tab. The ordered two-test reproduction passed afterwards.
- Retained-runtime retry accounting now compares the recorded retry count with
  capacity-failing ACP prompts, excluding the earlier successful turn and the
  initial failed dispatch. Cancellation and all five exhausted retries passed
  while preserving the original runtime and conversation identity assertions.
- The workspace dropdown inside the phone task picker added a second modal
  owner. Closing the enclosing picker during selection could leave pointer
  input blocked. The existing Drawer/Sheet now owns modality; the dropdown is
  nonmodal. A real component regression failed with the original modal menu
  and passed with the fix. Phone and desktop shared-workspace browser suites
  each passed all three cases. Their trigger checks observe actual hit targets
  before normal taps rather than forcing interaction through an overlay.

The workspace-picker correction follows the existing workspace-sidebar UI
contract and its shipped Drawer/Sheet composition. It changes no labels, sizes,
scroll ownership, data semantics, or recovery policy. Current-head lint,
typecheck, full discovery, documentation validation and fresh remote CI remain
delivery checks; successful local reproduction is not a remote CI verdict.

The eleventh-head CI passed the earlier retained-runtime, phone workspace, and
packaged-provider regressions. Two different first-attempt failures surfaced:
Quick Chat cancellation pending observation and Azure DevOps watch reset feedback.
Both passed on retry, so the flake gate correctly kept their shard checks red.
The first nineteen reports contain two failed results and four retry attempts
(including serial-suite repetitions); the last report and aggregate remain pending.

The Azure failure snapshot shows the reset confirmation still processing task
deletion when the feedback assertion timed out. The existing test now awaits
the actual successful reset response and confirmation dismissal before checking
the same success message, without increasing any timeout. The cancellation
observation helper now buffers from arm, forwards the real pending notification,
and releases all remaining frames after the pending UI assertion. It handles
batched gateway frames and preserves unarmed/binary traffic. Three focused
regressions failed on the original helper and passed after the correction. The
full Quick Chat cancellation browser suite passed all three cases with retries
disabled. Original first-attempt CI traces were unavailable; isolated baselines
passed, so local helper regressions establish the fixture boundary without
claiming an exact transport ordering was captured in the incident. Phone
cancellation and the complete Azure integration flow then passed with retries
disabled (one case each). Changed-file ESLint, typecheck, all twenty shard
manifests and documentation validators passed. The final full-report audit and
fresh remote CI remain delivery gates. Product cancellation and integration
reset policy are unchanged.

The next CI run passed all six container shards except the workspace-source
observation test. Its first-failure screenshot shows the added repository in
Changes while the test waits for a visible Files viewport; the physical file
assertion inside the container had already passed. The test now awaits initial
native Git status, then the actual new Changes folder and panel activation
before selecting Files. Production panel activation policy is unchanged.
Changed-file ESLint and web typecheck passed. Local retries-disabled execution
was blocked before the test by image-build networking: the Docker daemon could
not attach a build-container interface to its missing bridge. This is an
unavailable local browser check, not a passing reproduction. No host daemon or
production resources were changed. The exact updated integration flow requires
fresh container CI evidence before delivery can be considered ready.

The same run also reported archived-transcript scroll observation and mobile
workflow-return failures in a normal shard. The archived flow lacked the
settled-bottom precondition used by its active-transcript counterpart. It now
observes the latest reply and native bottom position before issuing its wheel
input; both chat cases passed with retries disabled. The mobile workflow flow
passed unchanged in isolation and after its preceding command-palette test
(three cases total, retries disabled). Its CI failure remains unexplained:
the manual-move marker persisted while the original session was starting.
The existing assertion now includes bounded session states and error summaries
when that marker remains, without extending its timeout or accepting a busy
move. No workflow production behavior changed. Fresh CI must validate this
flow; local passes do not establish the original failure's cause.

Fresh container CI exposed a setup omission in the Git-status precondition:
the Docker page fixture does not enable the read-only store bridge that the
normal page fixture enables. The added poll therefore returned false in every
attempt before repository attachment. This test now enables that existing
bridge in its pre-navigation init script. This fixes the observation setup,
without changing store data, production behavior or assertion timeouts. Local
container execution remains unavailable because of the host Docker bridge;
fresh remote execution is still required.

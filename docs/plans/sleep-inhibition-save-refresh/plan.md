---
created: 2026-10-06
status: implemented
requirements:
  - REQ-PLATFORM-TASK-SLEEP-INHIBITION-001
system_design:
  - ../../specs/platform/system-design/task-sleep-inhibition.md
legacy_specs: []
---

# Implementation Plan: Preserve Saved Sleep Settings During Status Refreshes

## Overview

Deliver one browser correction in one sequential work order. A successful sleep
settings PATCH acknowledgement must outlive older status reads in the same
local store, including their error and loading finalizers. Preserve current
refresh, failed-save, draft, caller-return and shared-save behavior.

Design checkpoint completed; ROOT reviewed all four artifacts and released test
authoring, then granted exclusive GLOBAL LOCAL-HEAVY after child59's audited
return. Task 01 is done in this same primary session. Hosted delivery and merge
verification remain task-level gates. No delegation or
new task/session/tab/model. Merge requires its own later ROOT SERIAL MERGE
INTERRUPT. ROOT coordinates one global local-heavy slot and one serial merge
across the three persistent fixes.

## Scope

### In scope

- Existing [platform requirement](../../specs/platform/requirements/task-sleep-inhibition.md),
  criteria .9-.11, and its paired design.
- Module-private store-scoped acknowledgement/read boundary in the live hook.
- Independently authored deferred real-hook/store and real-card/save-provider
  integration regressions, including the active grouped consumer's attention.
- Only minimal immediate card completion glue if those causal tests establish
  that the hook correction alone cannot preserve the existing caller contract.

### Out of scope

Backend/native power policy, task admission, authentication, API shape,
server/cross-tab revisions, runtime flags, defaults, polling cadence, UI copy,
layout, navigation, probe aborts and a global request framework. Do not change
the common save provider, store or API. No browser/E2E/build/Go run or broad
writer audit; no optional polish, moving-main rebase or protected-proof replay.

## Technical approach

The source of the defect is unconditional GET publication in
`apps/web/hooks/domains/settings/use-sleep-inhibition-settings.ts`: refresh and
save race through the same `setSleepInhibition` action, while GET catch/finally
also write shared error/loading after save. The hook's per-instance inFlight
boolean cannot express acknowledged-save ordering or sharing across mounts.

Keep coordination local to that file. Use actual `useAppStoreApi` identity for
an entry with an acknowledgement generation/current read identity, and a
committed binding plus own flight identity per hook instance. PATCH admission
does not discard useful reads. On fulfillment, advance the store generation
before publishing/returning the actual response. All old GET terminal paths
lose publication ownership; admission can replace an obsolete pending flight.
Failed PATCH keeps the generation. Commit-time retirement closes old callback
admission and old read publication before passive timer cleanup. An already
admitted save retains its actual captured-store result after close/unmount.
See [design control flow](../../specs/platform/system-design/task-sleep-inhibition.md#acknowledged-save-publication).

The active `TaskBehaviorSettings` Runtime tab and the legacy standalone
`TaskActionsSettings` use the same immediate card and store. The first visit
mounts Runtime; visited hidden tabs stay mounted. Route changes recreate the
pathname-keyed save provider, while root/nested StateProviders keep the shared
store. Therefore do not add per-workspace state or a new visibility/tab policy.
Normal card save admission still respects canEdit/loading and contributor
canSave; use direct hook tests for a GET already pending before save rather
than bypassing that real-provider guard.

| Boundary | Intended behavior | Evidence |
| --- | --- | --- |
| Same store, two hook bindings or remount | Shared successful-ACK fence; newest admitted read owns pending state | Deferred real StateProvider tests |
| Independent root stores | No response/error/loading or generation leakage | A/B and A-B-A binding controls |
| Retired view/callback | No new GET/PATCH; no retired GET publication; admitted PATCH still returns actual outcome to caller/captured store | Commit, close, unmount, StrictMode cases |
| Current save provider | Preserve revisions, dirty state, failedIds and canLeave | Actual SettingsSaveProvider/card integration |
| Browser transport | Real API clients, only exported JSON transport mocked | Request URL/method/body and return assertions |

## Tests

Two independently authored suites cover the changed contract. Existing suites
remain controls; their mocks are not accepted as causal evidence for this fix.

| Criteria | New suite and named case family |
| --- | --- |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.9 | `use-sleep-inhibition-settings.test.tsx`: pre-save/during-save GET success and rejection after ACK; sibling shared store. `sleep-inhibition-save-refresh.test.tsx`: rendered toggle, actual saveAll, visibility-event GET, stale success/error |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.10 | Both suites: GET before ACK, GET after ACK while old GET pending, failed PATCH with useful read, latest loading/error/retry, unsaved and in-save edit/revision controls |
| AC-PLATFORM-TASK-SLEEP-INHIBITION-001.11 | Hook suite: A-B/A-B-A, retained callbacks before passive cleanup, unmount/StrictMode, independent stores, admitted save after close. Integration suite: grouped TaskBehavior attention and close/reopen on shared StateProvider |

Use explicit deferred promises, assertions on admission before settlement, then
observable results. Never copy/import/replay the protected ROOT candidate,
write predicate tests, or mock hook/provider/store/API/contributor behavior.
Test real API options and actual caller values, and settle all deferred work in
cleanup. Use at most the transport and necessary navigation boundary mocks.

## E2E and mobile evidence

This is state/data publication inside the existing card. The mobile-parity
data-only exception applies: the targeted rendered card/provider tests cover
the same save/status outcome on desktop and phone. No changed layout/touch/
scroll/navigation/viewport interaction, new Playwright tests, browser runs,
screenshots, build or ASCII layout preview are warranted. The original
[implemented package](../task-sleep-inhibition/plan.md) records the historical
phone save/containment evidence; it does not prove this new race correction.

## Public documentation

Audited `docs/public/operations.md`, `README.md`, and `docs/screenshots.md`.
The guide already names Task Behavior > Runtime and correctly describes
persistence and host limitations. No public guide/copy/localization change is
needed; this correction restores save behavior without a new operator action.
The existing implemented companion plan is retained as historical delivery;
none of its done work orders is reopened or assigned new test counts.

## Work orders

- [x] [Task 01: Save and refresh publication boundary](task-01-save-refresh-boundary.md)

Only one wave, sequential; no dependencies on another work order. Local-heavy
slot acquisition and later merge interruption are operational barriers, not
additional implementation orders.

## Verification results

Implementation conforms to the current design. Actual verification:

- One conditional pinned pnpm 9.15.9 frozen install passed.
- Independent transport-boundary RED: nine causal failures and six passing
  controls. The first hook correction passed 80 of 82 selected tests, exposing
  one fixture selector and a real reopened-card baseline gap. The narrow card
  glue and selector repair passed all 18 affected card tests.
- An additional reopened cached-error Retry case failed before resolving the
  card's callback at click time. Final affected GREEN passed 37 tests across
  the real-hook, real-card/provider/Runtime and existing card suites. The other
  46 unchanged provider, Runtime and store controls passed in the prior full
  selection: 83 distinct tests passed across the recorded runs.
- Changed-file ESLint with zero warnings and Prettier checks passed. Own test
  group-size warnings were fixed by splitting behavior groups. The initial
  lint invocation used the wrong cwd and produced no verdict; ROOT received
  the setup checkpoint before the corrected reviewed `apps/web` invocation.
- Normal project `pnpm run typecheck`, including its generation prehook, passed
  after the final card/test change. No generated files changed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed; no copy/catalog changes.
- Final catalog validated 357 decisions and 1411 specifications; all 36 spec
  linter tests and full specification lint passed. The paired owner appears in
  the platform catalog. Final links, whitespace, actual changed-path coverage
  and normal hooks are recorded in private delivery receipts. Hosted delivery
  and merge remain task-level gates.
- Private original receipts/logs use `/tmp/kandev-child60-<NN-name>.{json,log}`:
  install `01`, RED `02`/`13`, initial full selection `03`, affected card `04`,
  final affected GREEN `14`, final lint `15`, format `16`, typecheck `17`,
  i18n check/ratchet `18`/`19`. Every original through `19` actually joined;
  its native handle and owned PID/group absence are recorded. Setup/format/lint
  repair originals `05`-`12` remain recorded separately, never overwritten.

Prior design-only validation passed:

- `python3 scripts/list-docs.py validate`: 357 decisions / 1411 specifications.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- Platform catalog contains the existing requirement and new paired design.
- All four actual artifact paths checked; 15 repository Markdown links resolve;
  tracked and untracked whitespace checks passed.
- Actual `.github/scripts/pr-docs.cjs` `validateCoverage` on the four current
  artifact contents plus all projected owned code/test paths: `covered`,
  `ok=true`, `errors=[]`, one pending order and accepted platform references.
- Original lightweight runner native handle `9063` ACTUALLY JOINED via
  `6df4d4`, exit 0; all seven command PID/groups joined/gone. Receipts and logs
  use private prefix `/tmp/kandev-child60-design-20261006-`, with check suffixes
  `catalog-validation`, `linter-tests`, `spec-lint`, `catalog-owner`,
  `diff-whitespace`, `links-projected-coverage`, and `status`.

Final artifact edits receive an affected links/coverage/whitespace recheck;
its receipt prefix is `/tmp/kandev-child60-design-final-20261006-`.
The completed ACTUAL DESIGN END checkpoint preceded permanent test and product
changes. Later ROOT reviewed-package and exclusive-heavy releases authorized
the current implementation phase. No Go, browser, E2E or build verification is
within this data-only repair. Delivery and separate serial merge gates remain.

## Evidence and source boundary

Actual design HEAD: `72a840a860143c6b55e401fe27c92167415b1e4a`.
Protected proof base: `5d3bc32cd240d36c1c5f780f64890172f4836f9d`.
The hook/card/API/save-provider blobs exactly match ROOT's supplied blobs
(`721cf0f6`, `fe15962b`, `a9e6574d`, `41e4509b` respectively). This authorizes
using the supplied local causal classification without rebase or replay.

Accepted private evidence: `/tmp/kandev-root-sleep-inhibition-save-order-next-candidate.test.tsx`
(0400, SHA256 `3edad7d177dc32f3665c40295432a5397d4bd166af7b3eb4048648f817ed1a02`),
classification `/tmp/kandev-root-sleep-inhibition-save-order-proof-classification.json`,
original same-prefix receipt/log. ROOT original `24336/1d090f` was ACTUALLY JOINED
through `eb96b9`, exit 1, 14.334s: two causal failures and two controls passed.
Rendered real card plus actual saveAll/store/API demonstrated stale GET success
and rejection after successful PATCH. This does not establish backend persistence
failure, cross-tab ordering or a full General route mount. All protected files,
managed worktrees, dependencies and foreign resources remain untouched.

## Risks

- Hook-local generations miss shared-store/remount reads; process-global ones
  conflate independent stores. Keep the key equal to the actual store API.
- Advancing on PATCH start drops useful reads on failure; advancing after
  publication allows a stale terminal write. Advance immediately before ACK
  publication.
- Retiring loading without checking current ownership clears a newer pending
  GET. Assert pending state before and after every stale terminal path.
- Tests must respect real save admission/revision and bool-draft semantics.
  No synthetic stronger editing policy or provider bypass.
- A broader contract or setup/resource/unknown failure requires a ROOT checkpoint,
  not scope expansion or blind retry. No material unresolved design question.

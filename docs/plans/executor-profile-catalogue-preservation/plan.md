---
created: 2026-10-09
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve executor catalogue during profile mutations

## Overview

Preserve catalogue changes received during a normal existing-profile save so
eligible task choices remain available. One sequential work order owns the
normal page's publication and independent real-page regressions, with the
sibling remove correction gated on its own causal RED.

The existing executor-owned [requirement](../../specs/executors/requirements/profile-editor.md)
and [design](../../specs/executors/system-design/profile-editor.md) own the
profile lifecycle. Add AC .13 through .15 to that pair, retaining .5, .6 and
.8 through .12. The reusable UI owner retains save coordination and layout;
the tasks owner retains selection/launch policy. No parallel UI pair or ADR.

## Evidence and requirement history

Admitted checkout and live remote main both read
`ca69af0bd062d2fcff5bb155b6d37b41b9e10e7d`; the normal page blob is
`177a810d0fc674faf176df4f73191ad2717caa2b`, identical to ROOT's proof.
Accept `/tmp/kandev-root-executor-catalogue-discovery-20261008/qualified-proof.json`
and `future-design-brief.md` without replay. Original `aad5c0`, native session
`53797`, actual terminal `7d1fca`, exit 1 proved one causal case and two passing
controls. Both catalogue disappearance and real task-option disappearance
assertions reached. Real page, store/providers and save coordinator ran; only
external transport and Monaco visual capability were stubbed.

The earlier detached launcher gave **NO VERDICT**. The earlier attached Monaco
failure was a **fixture failure**, not a product pass. Those receipts remain
historical. ROOT owns protected 0400 `candidate.test.tsx`, SHA256
`35b1c658ecc430f79419ad83266b6d7e8306dd0dae1f41a64f012a781f467da2`.
Never copy, import, replay, edit, chmod or remove it. Permanent tests below
must be authored independently after implementation release.

Before implementation, `useProfilePersistence.save` awaited acknowledgement
then passed its captured catalogue to `upsertExecutorProfile`; `setExecutors` replaces the
catalogue. A later executor/profile row is consequently dropped although the
server entity exists. The sibling remove maps its captured catalogue too;
source established the seam. The independently authored corrected RED now
proves delete loss through real confirmation and navigation (see work-order results).

History: `c277822ac` unified editor routes (#3762, addressing #3740);
`202d48bce` preserved omitted scripts in partial server saves (#4309).
AC .5 supplies existing Save/discard compatibility but did not explicitly
quantify unrelated catalogue preservation. The linked unification work order
is `done`; script-preservation artifacts retain their recorded `in_progress`
status/results as historical delivery records despite the merged commit.
Do not rewrite those statuses or replay their suites for this frontend fix.
Neither task-cache executor-field merging nor task default selection owns
this executor-profile catalogue contract.

## Scope

### In scope

- Normal `ProfileEditForm/useProfilePersistence` save publication from current
  store state, using the existing helper.
- Current unrelated additions, updates and removals, including matching
  executor metadata and sibling profiles; accepted target acknowledgement.
- Same-hook remove only with independently demonstrated causal coverage.
- Real page/providers/transport/coordinator tests and actual picker derivation;
  unchanged-success, rejection, dirty and delete/navigation controls.

### Out of scope

Backend, APIs, schemas, migrations, public guide edits, layout, copy, navigation
design, credentials, provider rules, permissions, target-resurrection guarantees,
same-profile server ordering, stale-page request ownership, global revisions,
writer inventory/migration, or plugin/Kubernetes connection-hook edits.

## Technical approach

Use `useAppStoreApi` inside `useProfilePersistence`. Immediately before each
successful publication, obtain `appStore.getState().executors.items`.
Save supplies it to `upsertExecutorProfile`; remove maps it and filters only
the acknowledged target ID. Remove the captured catalogue selector/dependencies
from these persistence callbacks. Preserve page subscriptions, target/executor
identity, helper fallback, payload/contributors and existing success/failure
order. There is no await between the current read and `setExecutors`.

| Boundary | Existing behavior / compatibility | Evidence and disposition |
| --- | --- | --- |
| Normal built-in form | REST update/delete through settings API, ordinary contributor | Independently authored real-page causal tests; only publication changes |
| Normal Kubernetes profile | Same persistence hook, combined profile/connection contributor, admin gate | Preserve contributor/permissions; scoped existing contributor controls |
| `plugin_remote` page | Separate page and hook; load/save/delete already use current store | Read-only positive compatibility boundary; no migration |
| Kubernetes connection resource | Create/update/remove already read current store | Read-only positive compatibility boundary; no mutation changes |
| Existing missing-target fallback | Helper may insert target/executor from accepted response | Retain existing semantics; no new deletion/save ordering promise |
| Task and subtask options | Catalogue projection plus actual `useExecutorProfileOptions` | Assert current values, labels and executor metadata, including absence of removed options |
| New-session context | Reads executor label and `useTaskExecutorProfile`, no executor-options picker | Consumer audit only; no invented picker claim or changes |

## Tests

New independently authored file:
`apps/web/components/settings/profile-edit/profile-edit-catalogue-publication.test.tsx`.
The names below defined the planned coverage. The independent execution receipts
are recorded in the work order; they now cover all nine cases.

| Criteria | Test name / scenario |
| --- | --- |
| .5, .13 | `save retains later executor and profile additions`: hold PATCH, publish new executor and same-executor sibling, acknowledge; accepted target and actual selectable options retained |
| .13 | `save retains current executor metadata and sibling updates`: change owner name/config/status plus sibling name/config and unrelated executor/profile while pending; current metadata and option names win |
| .13 | `save does not restore unrelated removals`: remove unrelated executor and sibling while target remains; neither catalogue rows nor their options reappear |
| .13 | `save preserves a mixed current catalogue`: additions, updates and removals together; all unrelated current entries retain values, all unrelated removed entries stay absent |
| .5, .14 | `unchanged catalogue save accepts target and clears dirty`: real edited Name, accepted profile, shared contributor clean, existing success notification |
| .5, .14 | `rejected save preserves current catalogue and dirty draft`: later catalogue mutation, reject PATCH; no publication, edited Name retained, contributor dirty, error and no success notification |
| .15 | `delete retains mixed current catalogue and task options`: actual confirmation, held DELETE, later mixed unrelated changes, acknowledge; only target removed, current options retained and real navigation reaches executor hub |
| .15 | `unchanged catalogue delete removes only target`: successful baseline control with real confirmation/navigation |
| .15 | `rejected delete leaves current catalogue and route`: held DELETE rejection after later changes; no publication/navigation, existing dialog/deleting recovery |

Each interleaving must observe transport admission before publishing the later
store state, then acknowledge/reject and join completion. Inputs include a
surviving target executor/profile and both surviving and removed unrelated rows.
Do not resolve by sleep or test private callbacks. Save and delete expected
failures must reach the catalogue and option assertions before production edits.
The delete RED is independent of ROOT's accepted save diagnostic.

## End-to-end and mobile boundary

Mount real `ProfileEditPage` with `StateProvider`/`createAppStore`,
`ToastProvider`, `SettingsSaveProvider` and existing UI providers. Change Name
through the actual input and execute the actual coordinator's `saveAll` or its
rendered Save action. Confirm deletion through the real dialog. A subscribed
test consumer applies the exact production flatten/fallback projection from
`task-create-dialog-computed.ts` and `new-subtask-dialog.tsx`, then calls actual
`useExecutorProfileOptions`; assert values, labels, eligibility and executor
metadata from its results. Do not stub that hook, store actions, internal form,
contributor, routing or persistence. Stub only external `fetchJson` and, if
needed, external `@monaco-editor/react` visual rendering; retain script wrappers.

This component integration proves edit/request/later-catalogue/acknowledgement/
task-choice outcome. It does not prove a live server write or remote runtime.
Pure state/data normalization leaves layout, touch, scrolling, navigation and
viewport-dependent behavior unchanged. The mobile-parity exception explicitly
permits these component tests with this note. No ASCII layout preview, new
Playwright file, browser/build/full local suite is planned. New rendered or
viewport-sensitive evidence would require a ROOT scope/resource checkpoint.

## Public documentation audit

Searched `docs/public/executors.md`, root README, screenshot catalogue and
adjacent specs/decisions. The public executor reference/how-to guidance already
describes the complete editor and profile selection accurately. The fix restores
existing behavior without new settings, API contracts, labels, commands or
screenshots; no public-guide addition is warranted. Internal docs updated only.

## Work orders

- [x] [Task 01: Publish normal profile mutations over the current catalogue](task-01-preserve-current-catalogue.md) (`done`, sequential)

## Verification results

Design validation on 2026-10-09, all original commands terminal with exit 0:

- `python3 scripts/list-docs.py validate`: 365 decisions and 1,477 specifications.
- `python3 scripts/lint-spec-files.test.py`: all 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `git diff --check`: passed. Status enumerates exactly four unstaged artifacts,
  two modified owning documents and two new delivery documents; index empty.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: actual docs-only diff
  `exempt`, `ok=true`, `errors: []`. Separate planned normal-page source trigger
  preflight `covered`, `ok=true`, one work order, correct owner/design references,
  `errors: []`. This is reference validation, not an actual production edit or
  implementation coverage result. Existing Node 24.21.0 ran the cheap helper;
  no dependency installation occurred.

At the DESIGN checkpoint, implementation checks had not run. ROOT later
released this package in the same primary with an exclusive local-heavy lease.
The [work-order results](task-01-preserve-current-catalogue.md#results) preserve
every original RED/GREEN, fixture/lint/type failure and correction receipt.
Independent corrected RED proved four save interleavings and real-dialog delete
loss (five causal failures, four passing controls, zero unhandled errors).
The minimal normal-page correction passed the prescribed four suites (20 tests),
and the final typed fixture passed all nine affected cases. Changed ESLint,
typecheck, i18n check/ratchet and documentation checks passed. Actual changed-path
coverage returned `covered`, `ok=true`, one work order and `errors=[]`.

The sole work order is `done` and this manifest `implemented` for local
implementation acceptance. Normal active hooks, ready publication, hosted review
and ROOT serial merge/closure remain delivery gates; the persistent task is
incomplete. No protected proof replay, broad passing replay or foreign-resource
mutation occurred. The public guide remains accurate; internal artifacts record
the restored contract and state/data mobile exception.

## Release, resource and delivery gates

The DESIGN checkpoint required these four artifacts unstaged/uncommitted and
ending that turn. ROOT later released implementation and the exclusive lease
in this same primary; the release and execution history is recorded in the
work order and durable task plan. ROOT's later explicit implementation interrupt in task
`71d65c60-71e6-42a4-b0fe-e96050d47ec2`, primary session
`130ac83a-f599-4a8b-8870-a2322c6fb68a`, plus the exclusive global local-heavy
lease, is required before permanent tests, production, install or heavy checks.
Persist identity/system marker, user edits, phase, native handles and crash next
action in the live task plan. No agents, sessions, tabs or model changes.

After release, use pinned pnpm 9.15.9 and one frozen install from `apps/` only
if dependencies are absent. Execute one original local-heavy command at a time;
retain each native handle, actually join it and prove fresh owned groups absent.
Resource/timeout/transport/unknown/out-of-scope results checkpoint ROOT before
alternatives. No cache wipes, foreign kills, weakened assertions or passing replay.
Normal hooks, commit/push and ready PR are standing eventual delivery authority;
merge is not. Freeze the ready head; never rebase solely because main moved.

Return local-heavy resources before one attached original
`scripts/pr-await <PR> --mode all-terminal --deadline-min 90 --interval-sec 60`
under GNU `timeout --kill-after=10s 91m`. Preserve that SAME original and
deadline through crashes. Actual END/join/fresh gone precede any ROOT-authorized
replacement; no blind retry, duplicate timer/manual review or recursive observer.
All six actual required contexts AND actual Backend/Frontend/E2E parent runs
must succeed on the exact head. Require fresh complete report, `errors: []`,
zero visible/hidden/actionable/human findings, and authenticated configured
CodeRabbit App 347564 substantive FULL CURRENTHEAD ALLfiles review evidence
(`source=covered`, `kind=reviewed`). Accept sufficient automatic review with
ZERO requests; one request only for a real inspected gap. Ground and disposition
every finding.

A separate ROOT SERIAL MERGE grant follows hosted-ready actual END/all joins.
Only then use normal expected-head squash and verify actual SHA/tree/parent,
all owned blobs, main inclusion and owned joins/END. ROOT owns independent
verification, archive, ABSENT readback and proof release; this task remains
incomplete until actual merge and closure. Preserve Office4355 original live
monitor95296, all foreign resources, paused oversized task and unproved Docker
volume. Optional callback/queue fullness never gates the durable checkpoint;
do not retry messages. Only critical unsafe/uninferable decisions go to the
direct parent question tool, which ends the turn. No operator approval request.

## Risks

- Reading the current catalogue too early recreates the lost-update bug.
- A page rerender does not update a closure already awaiting acknowledgement.
- Target-deletion races and same-profile concurrent ordering remain existing
  policies; do not turn unrelated preservation into arbitration guarantees.
- Fixtures must join held transport, navigation and owned timers without masking
  causal assertions; Monaco capability failure is not a product RED.
- The remove branch remains conditional on its independent causal RED; a
  noncausal outcome requires a ROOT checkpoint before changing scope or checks.

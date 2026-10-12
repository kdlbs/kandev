---
created: 2026-10-10
status: done
requirements:
  - REQ-AGENTS-PROFILE-DELETION-CATALOGUE-001
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
legacy_specs: []
---

# Implementation Plan: Preserve profile choices after list-row deletion

## Overview

Correct the active `ProfileRow` successful-delete publication so only the
accepted target disappears from both current inventories. This is one bounded
vertical repair, delivered by exactly one sequential work order after the
design checkpoint and ROOT's later explicit implementation release.

Agents owns the failed contract: configured profile membership and selectable
options. Extend the existing [catalogue requirement](../../specs/agents/requirements/creation-catalogue.md#list-row-deletion)
and [catalogue design](../../specs/agents/system-design/creation-catalogue.md#accepted-list-row-deletion-evidence-and-inventory).
The creation contract and its completed package remain separate and unchanged.
No new ADR is required for this local conformance correction.

## Confirmed evidence and assumptions

At main/checkout `c176df170bf2ceee5ab4e3f55dc3b296ab187b55`, the active
`AgentProfilesSubList` renders `ProfileRow`. Successful deletion already reads
current settings rows, but then flattens them to replace all picker options.
`registerAgentsHandlers` can legitimately publish a global option for an owner
absent from settings rows; existing duplication merges preserve that inventory.
The rebuild drops it. It can also replace unrelated newer option metadata with
older settings metadata.

ROOT's qualified receipt records native46666, joined `cb7205`, exit 1, one causal
failure losing `root-new-profile`, and two passing controls (ordinary delete
and rejected delete). Receipt:
`/tmp/kandev-root-profile-delete-options-discovery-20261010/qualified-proof.json`.
The protected `candidate.test.tsx` must never be read, copied, modified or deleted.
Independent regressions will be written later. The unused
`useProfileEnabledToggle` candidate was rejected and is excluded.

Confirmed intent is to preserve every unrelated option, including absent-owner
options, newer metadata and ineligibility. No unresolved product decision blocks
this package. The supplied reproduction is accepted evidence, not a new local
test result. No tests, install, lint, build or hooks run in this design turn.

## Scope

### In scope

- Successful list-row deletion and independently authored row/store/handler/
  actual task/subtask selector regressions.
- Current-slice preservation, overlapping deletes, failure/conflict navigation,
  admin gating, unchanged mobile confirmation controls and eligibility boundaries.
- The existing agents requirement/design pair, this manifest and one work order.

### Out of scope

- Backend, APIs, global handlers, list fetches, absent-agent materialization,
  selection policy, new hooks/actions/merges, UI layout or copy changes.
- Adjacent creation, duplicate, save or enablement repairs; child102's PR4387
  connection-save hook and executor specifications.
- Delegation, recursive tasks, new sessions, model switches or extra work orders.

## Technical approach

In `apps/web/components/settings/agents/agent-profiles-section.tsx`, keep
`ProfileRow.handleDelete` and all non-success branches. After the accepted
response, derive each list from the owning store's current corresponding slice,
filtering only `profile.id`. Use the existing setters synchronously without an
await between reading and publication. Preserve other values/order and slice
metadata/version. Remove the unused `toAgentProfileOption` import and correct
the comment that assumes the picker inventory is fully represented by settings.
No timestamp comparison is needed to preserve values that deletion does not own.

The same existing `AgentProfileOption` shape is retained for concrete, dynamic
and CLI-passthrough profiles. Current enablement, dynamic-routing and executor
gates determine actual eligibility. An absent settings owner is permitted; an
Office-scoped created event remains ignored by the existing global handler.

## ASCII UI preview

UI-01: Existing Agents list row, confirmed deletion and retained choices.
Entry: `/settings/agents`. Data names below are illustrative profile data;
controls retain current localized copy. No geometry or composition changes.

```text
Desktop:  [Target profile] [Duplicate icon] [Delete icon]
          anchored confirmation: [Cancel] [Delete]
Phone:    [Target profile] [...]
          actions menu -> existing confirmation surface
          [Cancel] [Delete]

After successful deletion:
                     Before repair       Proposed
Target row/choice    removed              removed
Sibling choice       retained             retained
Received global      lost after ACK       retained with current label
Disabled option      may regress          remains unavailable to new work

Task / Subtask agent selector (both viewports):
  [Selected retained profile label v]
  choices: sibling, received global
  absent: deleted target, disabled/Office-ineligible choices
```

Structural requirements: existing row and confirmation controls, selected label
and actual eligible choices. ASCII spacing is illustrative. Phone composition
uses existing `AgentProfileDeleteConfirmation`/`MobileActionConfirmation`; the
row menu closes before confirmation. Existing page/picker scroll owners, safe
areas, touch sizes, cancellation and focus return remain. This is the mobile
skill's state/data-only exception, with real rendered row/selector proof and
affected row/admin/confirmation controls. Maps to .1, .2, .5 and .6.

## Tests

New file: `apps/web/components/settings/agents/agent-profiles-section-delete-inventory.test.tsx`.
Mount the real row and subscribed app store, run real registered events and
render actual task/subtask selectors with their production option derivation.
Do not substitute an array renderer or assert IDs alone. Hold the external
delete response, prove the received option is selectable before resolution,
then prove its visible choice and selected label survive success.

| Planned regression/control | Acceptance mapping |
| --- | --- |
| `retains an absent-owner global choice after delete ACK` | .1, .5; primary RED, desktop and phone shared data path |
| `removes only the accepted target without an intervening event` | .1; ordinary-success control |
| `preserves received choices after rejected or handled deletion` | .4, .5; failure control |
| `preserves choices and navigates to guided resolution on conflict` | .4 |
| `preserves newer option metadata and mixed eligibility` | .2, .5; enabled absent-owner option plus disabled newer option and Office-scoped event |
| `does not resurrect targets when two deletions finish in either order` | .3; retain unrelated options throughout |
| Existing row confirmation and `agent-admin-gating.test.tsx` controls | .6 |

New tests use unique profile IDs to avoid module-level event tombstone collisions,
settled hook readiness and actual administrator state; all deferred operations
and subscriptions must settle/clean up. The first failure must be the loss of
the received choice, with ordinary/failure controls passing. The existing
minimal fake-store row tests alone cannot prove event publication or labels.

## Existing mobile compatibility context

`apps/web/e2e/tests/settings/mobile-agent-profile-delete.spec.ts`,
`mobile-chrome` project, test `keeps list-row deletion controls above the row
link and returns focus on cancel` documents the existing active row menu,
confirmation, cancellation and focus owner. It is read-only compatibility
context, not a mandatory replay or build gate. The changed inventory behavior
is covered by the focused real rendered row/store/handler/task/subtask selector
tests and affected row/admin/confirmation controls above. The state/data-only
mobile-parity exception applies: no new or repeated mobile E2E run is required.
Any actual viewport, layout or control change requires a ROOT scope reassessment.

## Work orders

- [ ] [Task 01: Preserve current inventories on accepted row deletion](task-01-preserve-delete-inventory.md) (`in_progress`, sequential).

## Execution and delivery constraints

Design-only first turn: leave these four artifacts unstaged/uncommitted and
return the external checkpoint. ROOT reviews and sends a later explicit
implementation interrupt to this same primary session. No redundant approval
question or model switch. Local-heavy capacity is free but not granted: wait
for ROOT's actual lease before install/tests/lint/build/hooks. Install once
from `apps/` after implementation release and lease grant; preserve dependencies,
shared caches, foreign refs and other users' resources.

Preserve all live command handles through interruption, join original handles,
and never rerun a duplicate observer/test to infer status. No commands are live
at this design checkpoint. Never access/delete/prune protected paused task
`a6032d95`, its `serialize-workspace` worktree, or volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.

After later authorized implementation, record actual results and use normal
hooks without bypass. Return the actual local-heavy lease, then continue hosted
delivery in that same turn with one observer: 90-minute GNU timeout, kill after
10 seconds, 60-second cadence. Keep communication possible between waits.
Read automatic findings promptly; request no routine fresh full review per
addition/push/SHA. Further focused review needs a concrete material reason.
Require the six required checks plus actual Backend/Frontend/E2E parent jobs
current SUCCESS, zero actionable findings and no human gates. Normal merge
requires ROOT's one static/current-main compatibility and serial expected-head
squash grant. No admin merge, branch deletion, main-drift-only rebase, optional
polish or replay of passing broad checks. Extra scope requires a concrete ROOT
checkpoint. These gates do not authorize implementation during this turn.

## Verification strategy and results

Design: lightweight Python catalogue/spec validation, cross-reference and diff
checks only. PR-documentation helper preflight is deferred to released execution
because only Python/diff checks are granted now. It must enumerate the actual
tracked and untracked diff against admitted base
`c176df170bf2ceee5ab4e3f55dc3b296ab187b55`, including the new regression and
every real changed path. Require `ok`, status `covered`, no errors and exactly
one work order. The work order supplies the available managed Node executable,
repo-root invocation, bounded command timeouts and original-handle join rules.

Implementation release received from ROOT on 2026-10-10 in the same primary
session, with exclusive local-heavy lease103. Accepted ROOT evidence remains
separate from the independent local results below.

Corrected design checkpoint receipts (2026-10-10, all joined exit 0):

- `list-docs.py validate`: 368 decisions and 1496 specifications validated
  (command receipt `1ca78c`).
- `lint-spec-files.py --all`: all specification files passed (`711213`).
- Python cross-reference check: one work order, all six criteria, owning design
  declared by the manifest; no mobile replay/build gate; actual tracked/untracked
  coverage preflight and exact covered/one-order conditions present (`461d30`).
- `git diff --check`: clean (`f30864`); status contains only the owning pair and
  this new plan directory (`68d1fc`). All four artifacts are unstaged/uncommitted.

No install, production/test edit, product lint/build/hooks, Node preflight,
local-heavy lease or live command/observer handle at this checkpoint. ROOT's
two requested design corrections are applied; later implementation remains
pending explicit release to this same primary.

## Risks

- Capturing either slice before awaiting repeats a stale-write defect; rebuilding
  options loses independent values even if settings rows are read at write time.
- A fake producer/store/picker test can pass while the active rendered flow is
  broken. Use the real registered handler and actual choices, and prove setup
  readiness before releasing the request.
- Changing non-success paths or store metadata expands this repair. Preserve
  their semantics and stop at a concrete ROOT checkpoint if that is necessary.

## Implementation receipts

- Bootstrap: original handle19436 joined exit0, one frozen-lockfile install from
  `apps/`; `/tmp/kandev-child103-implementation-20261010/install.log`.
- Independent causal RED: original14864 joined exit1; one actual lost retained
  task label after ACK, two ordinary-success/rejected-delete controls passed.
  `/tmp/kandev-child103-implementation-20261010/red.log`. No setup/unhandled error.
- Expanded GREEN65261 and1730 joined exit1 due to asynchronous selector portal
  cleanup in the new assertion helper. Awaiting real listbox closure corrected
  it; GREEN51871 then passed 30/30. Both failing attempts are retained honestly.
- New-fixture lint/type errors were corrected within this test only. ESLint89353
  passed; typecheck51339 exit2 identified the fixture issues. No adjacent source
  or contract change was required.
- Final affected GREEN34730 joined exit0: three suites, 30 tests, including nine
  independent real-row/store/registered-handler/task+subtask-selector scenarios.
  `/tmp/kandev-child103-implementation-20261010/green-final.log`.
- Docs/preflight43843 joined exit0: catalogue 368 decisions/1496 specifications,
  all spec files passed. Actual tracked+untracked diff from admitted base contains
  six owned paths including the new regression; coverage returns ok, covered,
  errors=[], exactly this one work order. Logs: catalogue.log, spec-lint.log and
  coverage.log under the same receipt directory.

Final sequential check20538 joined exit0: scoped ESLint with zero warnings,
web typecheck and i18n ratchet all passed (eslint-final.log, typecheck-final.log,
i18n.log). Implementation is done. Lease103 remains held for the normal-hook
commit; publication and hosted readiness receipts continue in the external task
plan. Merge still requires the separate ROOT expected-head grant.

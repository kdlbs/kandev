---
created: 2026-11-02
status: done
requirements:
  - REQ-TASKS-TURN-CHANGES-005
  - REQ-TASKS-TURN-CHANGES-006
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
  - ../../specs/tasks/system-design/turn-change-history-integrity.md
legacy_specs: []
---

# Implementation plan: Hide unavailable turn change summaries

## Overview

Hide the entire transcript card when a turn summary is unavailable or failed.
Tasks owns this change because summaries and transcript anchors belong to the captured turn.
One sequential work order changes the shared visibility predicate and proves desktop and phone behavior.
Implemented on 2026-11-02; results are recorded below.

The user supplied an unavailable-card screenshot and requested that this state stay hidden.
The existing specification and tests deliberately show this card for unavailable history.
This package changes that presentation rule. It preserves the retained history state.

## Scope

### In scope

- Summary-level `unavailable` and `failed` states, with zero or retained file entries.
- Reply anchors, fallback anchors, live settlement, reconnect, and reload.
- Partial summaries with usable changes and explicit historical diagnostics.
- Focused desktop and phone regression coverage and the existing public guide.

### Out of scope

- Capture, comparison, provenance, backend data, retention, and branch-rename behavior.
- New settings, localization keys, warning surfaces, or card styles.
- Changes to pending or expired summaries, workflow gates, or autopilot.

## Technical approach

Update `apps/web/lib/turn-changes/projection.ts::isTurnChangeCardVisible`.
Reject `unavailable` and `failed` before admitting pending or nonzero summaries.
Remove the history-specific visibility exception from this predicate.
Keep `isUnavailableHistoryComparison`: the historical viewer and its controls still use it.
Keep `projectTurnChangeSummary`: `resolveTurnChangeScope` uses it to retain historical selections.

`message-list-native.tsx` already applies the predicate to reply cards, fallback rows, and summary admission.
Filter at these existing boundaries before `TurnChangedFilesCard` mounts.
No card border, footer, disabled action, file request, or padding remains for a rejected summary.
The card renderer can retain its defensive unavailable state for direct internal rendering.

Mixed results stay visible when the aggregate summary remains usable.
Repository-level failure, missing file content, or `summary_complete: false` does not independently hide a card.
No stored availability, reason, count, or historical selection changes.

Update the public how-to section in `docs/public/tasks-and-workflows.md` during implementation.
Remove its unavailable-history exception. Explain hidden unavailable summaries and retained partial cards.
The root README and screenshot catalog contain no matching behavior to update.

The completed [history-integrity package](../turn-change-history-integrity/plan.md)
and [branch-continuity package](../turn-change-title-branch-continuity/plan.md)
retain their original results. Their unavailable-card expectations are superseded by this package.
No new ADR is required: the existing separation between history state and transcript eligibility remains authoritative.

## ASCII UI preview

### UI-01: Unavailable summary, desktop and phone

Entry point: a completed task turn in Chat.

```text
Before:
[Final assistant reply]
[Open diff (disabled)]
[This turn's change summary is unavailable.]
[Repository status: unavailable]
[Next message]

After:
[Final assistant reply]
[Next message]
```

The omitted card and its reserved spacing are required. Sample text and spacing are illustrative.
Desktop and phone share this composition and their existing transcript scroll owner.
No replacement banner, drawer, control, or focus target appears.
Map: AC-TASKS-TURN-CHANGES-005.1, .7, .9, and .13.

### UI-02: Usable partial summary

```text
Desktop:
[Final assistant reply]
[1 available change +2 -1] [Partial] [Open diff]
[healthy repository and affected repository status]

Phone:
[Final assistant reply]
[1 available change +2 -1] [Partial]
[Open diff]
[healthy repository and affected repository status]
```

Reuse the current inline card and `MobileDiffSheet` exemplar for historical navigation.
Phone actions retain 44-pixel targets. The diff drawer retains safe areas and one internal scroll owner.
Shared projection owns eligibility. Responsive wrappers retain their current presentation.
Map: AC-TASKS-TURN-CHANGES-005.7, .12, .13, and AC-TASKS-TURN-CHANGES-006.2, .7.

## Tests

- `projection.test.ts`: unavailable and failed summaries stay hidden with zero or nonzero counts and any reason.
- `projection.test.ts`: pending, ready, expired, and usable partial summaries retain their established eligibility.
- `history-scope.test.ts`: explicit and latest unavailable historical selections remain available despite hidden chat cards.
- Existing historical viewer controls tests protect unavailable diagnostics and count wording.

These cases cover AC-005.7 and AC-006.2/.7. All identifiers use the owning requirement prefix in the work order.

## E2E tests

- `turn-changes-history-conflict.spec.ts`, Chromium: target the exact change-set ID, require no card after settlement and reload, and retain the assistant reply.
- `turn-changes-title-rename.spec.ts`, Chromium: change the branch-switch assertion to an absent card while preserving API unavailability checks.
- `turn-changes-history-mixed.spec.ts`, Chromium: retain the usable partial card and historical navigation.
- `mobile-turn-changes-history-integrity.spec.ts`, mobile-chrome: add the existing conflict scenario, require no card before and after reload, and retain existing usable-diff tests.

Wait for the terminal summary through existing causal helpers before asserting absence.
Scope selectors to the active chat and `data-change-set-id`. Earlier healthy cards can remain.
Record focused desktop and phone captures of UI-01 and compare the result with the preview.
Require no horizontal document overflow. Existing scroll ownership remains authoritative.

## Work orders

- [x] [Task 01: Hide unavailable summary cards and prove history continuity](task-01-hide-unavailable-cards.md)

## Verification results

Design-turn results (2026-11-02):

- `python3 scripts/list-docs.py validate`: passed, 425 decisions and 1885 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: covered the changed work orders and planned `projection.ts` change, with no errors.
- Catalog lookup confirmed the owning requirement and both designs.
- `git status --short`: specification and plan files only, including the new untracked package.

Implementation results (2026-11-02):

- `projection.ts` rejects `unavailable` and `failed` before admitting pending or nonzero summaries; the unavailable-history exception moved out of transcript eligibility while `isUnavailableHistoryComparison` and `projectTurnChangeSummary` remain for viewer diagnostics and explicit history selection.
- `projection.test.ts` covers unavailable/failed summaries with zero or retained files and history-conflict, branch-switch, and capture-failure reasons, plus retained unavailable history projection and pending/ready/expired/partial eligibility. `history-scope.test.ts` keeps latest and explicit unavailable selections while the chat card is hidden.
- Chromium specs require the exact `data-change-set-id` card to stay absent after settlement and reload while the reply remains visible. The phone history-integrity spec adds the same conflict scenario with no horizontal overflow. The mixed partial card remains unchanged.
- `docs/public/tasks-and-workflows.md` now states that unavailable or failed summaries have no card and that usable partial summaries remain.
- Discovery: 5 Chromium tests in 3 files; 3 mobile-chrome tests in 1 file.
- Managed Chromium run: 5 passed. Managed mobile-chrome run: 3 passed.
- Captured desktop and phone UI-01 screenshots: the earlier healthy card remains, the final reply is visible, and no unavailable card, footer, action, or reserved spacing follows it.
- Focused Vitest: 35 passed across the three work-order files; 86 passed across 12 turn-change files. Typecheck and `rich-output:check` passed with an increased Node heap. ESLint on all changed TypeScript files reported zero warnings.
- `node --test scripts/validate-public-docs.test.mjs` (63 passed), `node scripts/validate-public-docs.mjs` (54 pages), `python3 scripts/list-docs.py validate` (425 decisions, 1885 specifications), `python3 scripts/lint-spec-files.test.py` (36 passed), `python3 scripts/lint-spec-files.py --all`, the coverage preflight, and `git diff --check` passed.

Review remediation (2026-11-02): superseded packages now describe this package as completed, the public how-to separates partial cards from hidden unavailable or failed summaries, the core design drops failure-row anchor wording, the phone conflict scenario resets the shared seed checkout, and the linked presentation requirement and core system design advance to `active` and `current`. The phone conflict scenario, ESLint, and documentation/specification checks passed again.

## Risks

- Changing the history projection would remove diagnostics. Change only transcript eligibility.
- Hiding repository failures would remove healthy partial results. Test aggregate and repository states separately.
- Existing conflict and branch-switch tests require updated expectations. Preserve their backend correctness assertions.
- An unscoped absence assertion can match an earlier card. Correlate the exact change-set ID.

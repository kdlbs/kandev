---
created: 2026-10-01
status: in_progress
requirements:
  - REQ-UI-CHANGES-LOADING-001
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
  - REQ-UI-BOUNDED-CHANGES-001
system_design:
  - ../../specs/ui/system-design/changes-loading-feedback.md
  - ../../specs/platform/system-design/changes-refresh-recovery.md
  - ../../specs/ui/system-design/bounded-changes-rendering.md
legacy_specs: []
---

# Implementation Plan: Changes Toolbar Feedback and Section Spacing

## Overview

Replace loading and refresh-failure banners with one toolbar status beside Review.
Failed Git reads retry automatically after a delay, preserving the last valid changes.
Verify PR and Commits geometry after PR #4145, then repair only remaining spacing defects.
Two sequential work orders deliver the shared desktop/phone behavior and regression coverage.

## Scope

### In scope

- Git refresh, detail enrichment, and inline commit-file loading feedback.
- Shared toolbar placement, accessible status, hover/focus tooltip, and phone visibility.
- A localized "Review" tooltip on the eye button in both header compositions.
- Warning in the spinner position, delayed read recovery, and preserved prior data.
- Empty-state, inline commit Retry, shared ownership, and content geometry regressions.
- Remaining inter-section gaps, sibling row spacing, content indentation, and expanded commit boundaries after PR #4145.

### Out of scope

- Mutation feedback, diff-viewer states, backend changes, or unrelated fetch scheduling.
- General toolbar redesign, new dependencies, or new runtime settings.
- Header padding removal, disclosure sizing, and alignment already implemented in PR #4145.

## Assumptions and source evidence

The user requests a small status after the left icon group, directly beside the eye when it is the group's final action.
The latest screenshot clarification requires a visible loading tooltip on hover.
The eye button is Review in `ChangesPanelHeaderLeft`.
`GitStatusNotice` adds a card above the timeline during refresh or enrichment.
`CommitStatusHistoryRow` adds a padded row during commit-file loading.
This package treats those passive states and repeated file-row loading text as the requested simplification.
Action feedback and full diff-viewer placeholders retain their separate purpose.

The user's follow-up removes the refresh-failure banner and manual Retry requirement.
Recovery must be automatic, with a small warning in the toolbar between attempts.
The agent selects 5, 10, 20, then capped 30-second delays as a routine recovery policy.
UI owns reusable toolbar presentation. Platform owns Git read recovery and retry admission.
No material open question blocks this local change.

### Coordination with PR #4145

The user requests no duplicated work. The latest check on 2026-10-02 at
03:41 WEST found [PR #4145](https://github.com/kdlbs/kandev/pull/4145) still
open at head `c5038084dc3d2a3f05294c0797e48905d5fb8953`. Its changed files
include the history row and working-tree rendering, geometry E2E coverage, and
the bounded-rendering docs. Task 02 remains deferred until this PR lands; do not
duplicate its header or history-spacing work.

Task 01's toolbar, tooltip, and refresh-recovery changes remain independent.
After PR #4145 integrates, measure its rendered section geometry and repair only
defects that remain. Preserve its header design and tests. Do not restore the
superseded 4px header gap or negative top margin.

### Section-spacing regression evidence before PR #4145

Commit `0be0fd65aae` (`perf(ui): virtualize Changes panel rows (#4071)`) moved the
section footer padding onto the standalone history header. Separately rendered
children lost the section content gutter and their grouped-list sibling spacing.
The later measurement repair in `61d791ebd74` (`#4088`) repopulates measured row
sizes, but it does not restore this inner section geometry.

A disposable Chromium fixture rendered the actual normal-flow `PRFilesSection`
and `CommitsSection` beside the actual virtual history model, rows, and viewport.
Both used the production stylesheet and the same five PR files and two collapsed
commits. The viewport was 1440px wide with a 700px panel. No live data was changed.
The fixture, server, and browser were removed after recording these CSS-pixel measurements:

| Geometry | Earlier normal-flow rendering | Current virtual rendering |
| --- | --- | --- |
| PR heading to first file | 4 | 16 |
| Last PR file to Commits heading | 10 | -2 (overlap) |
| Commits heading to first commit | 4 | 16 |
| PR sibling gap | 2 | 0 |
| Commit sibling gap | 2 | 0 |
| PR/commit row left edge relative to heading | -4 | -20 |

This is a diagnostic for existing AC-UI-BOUNDED-CHANGES-001.6 and `.8`.
These measurements predate PR #4145 and do not describe its rendered result.
Expanded commit and phone geometry still require the implementation's browser checks.

## Technical approach

Extract the existing inline-detail hook and lift its invocation into `useChangesPanelData`.
Share that owner with the timeline and expose its pending snapshot projection to the toolbar.
Combine it with `gitStatusPresentation.loading` and `detailsPending`.
Use a separate toolbar loading prop, leaving mutation `isLoading` unchanged.

Append the shared status after all actions in both `ChangesPanelHeaderLeft` branches, before the toolbar spacer.
Render it when review controls are absent, and keep it outside the overflow menu.
Reuse the shared Spinner, Tooltip, warning icon, and `task:loadingChanges` translation.
Add localized warning copy to English and all six supported locales.
Give the Review eye button its own hover/focus tooltip through `t(REVIEW_LABEL_KEY)`.
Preserve its action and accessible name on desktop and phone.

Remove `GitStatusNotice`, omit pending file statistics, and remove loading commit descriptors.
Update `beforeLayoutKey` for the remaining geometry-changing states.
Keep existing source identity, request retirement, retry, and virtualizer contracts.

Extend the shared Git refresh coordinator with one delayed recovery timer per client/environment scope.
Use increasing delay after each failed finite attempt, with no overlap or retry during hidden/inactive state.
Reset backoff on accepted recovery and cancel timers on release, unfocus, visibility loss, or disconnect.
Keep `fresh`, one `recover`, and detail `replay` semantics and existing request/source guards.
Retain the explicit refresh API for other callers, while removing its summary-panel Retry button.

After integrating PR #4145, follow with verification of its section geometry.
Repair only remaining inter-section, sibling, gutter, or expanded-commit defects.
Preserve its compact headers, direct descendant adjacency, and touch controls.
Assign intentional spacing inside measured shells and include it in estimates.
Keep virtual wrappers contiguous and preserve the measurement refresh algorithm.

## Existing contract reconciliation

Update Platform criteria `.28` and `.29`, and add `.36` through `.38`, for the new recovery outcome.
The original [Git refresh package](../changes-panel-git-refresh/plan.md) remains an implemented historical record.
This follow-up supersedes its manual summary-panel Retry and body-notice presentation.
Its source authority, deadlines, data preservation, and diff-viewer recovery guarantees remain required.
Update the existing Platform and Tasks design summaries to reference the delayed recovery policy.
Reconcile the publication ADR's presentation consequence without changing tracker publication ownership.

## ASCII UI preview

UI-01: Desktop Changes, pending refresh or expanded commit files.

```text
BEFORE (source-backed)
| Diff | Review (eye) | Walkthrough | ... |
| [spinner] Checking Git / detail notice |
| Changed files                         |
| Commit                                |
|   [spinner] Loading files...          |

AFTER
| Diff | Review (eye) | Walkthrough | (spinner)     Branch | Pull | ... |
| Changed files                                                       |
| Commit                                                              |
```

UI-02: Phone Changes or narrow panel, pending and ready.

```text
PENDING
| [route] [eye] (spinner)           ... |
| Files or commit headers              |
|          content scroller            |
| existing bottom navigation           |

READY
| [route] [eye]                     ... |
| Files, commit files, or empty state   |

INITIAL PENDING, NO REVIEWABLE CONTENT
| (spinner)                        ... |
|          empty content area          |
```

UI-03: Failed refresh, delayed retry, and automatic recovery.

```text
BACKOFF
| Diff | Review (eye) | Walkthrough | (warning)     Branch | Pull | ... |
| Last available files and commits     |

RETRY ACTIVE
| Diff | Review (eye) | Walkthrough | (spinner)     Branch | Pull | ... |
| Last available files and commits     |

RECOVERED
| Diff | Review (eye) | Walkthrough |               Branch | Pull | ... |
| Current files and commits            |
```

The header remains fixed. Only the existing body scrolls.
The status follows the complete left action group before the empty toolbar space.
It does not split action icons or follow the right-side branch, Pull, and overflow group.
That placement and the absence of passive status rows are required.
Labels, other icons, and ASCII spacing are illustrative.
Hover/focus shows localized loading copy on desktop.
Hover/focus on the eye button shows "Review", including when its text label is hidden.
Phone visibility and accessible status do not depend on hover.
Retain 44px targets for existing phone actions. The spinner is passive.
Phone UI-03 uses `[route] [eye] (warning) ...` in the same shared status position.
Warning hover/focus explains stale changes and automatic retry. No body banner or manual Retry appears.
UI-01 through UI-03 cover AC-UI-CHANGES-LOADING-001.1 through 001.7 and Platform criteria `.28` and `.29`.

UI-04: PR and Commits section geometry, desktop and phone.

```text
BEFORE #4145                   PROPOSED REMAINING REPAIR
| PR CHANGES (5)            |   | PR CHANGES (5)            |
|       16px gap            |   |   file                    |
| file                     |   |   file                    |
| file (no sibling gap)     |   |       2px sibling gaps    |
| file                     |   |   file                    |
| COMMITS (2) touches files |   |       10px section gap    |
|       16px gap            |   | COMMITS (2)               |
| commit                   |   |   commit                  |
| commit (no sibling gap)   |   |   commit                  |
                               |       2px sibling gap     |
                               |   commit                  |
```

Header compaction is owned by PR #4145. Preserve its direct-child adjacency.
Remaining gap/gutter repairs require reproduction after integrating that PR.
The 2px sibling gap, 10px expanded-section gap, and 16px gutter are reference targets.
They describe inner content spacing, not fixed row heights.
Phone uses the same grouping with wrapped content and existing touch-sized controls.
UI-04 covers AC-UI-BOUNDED-CHANGES-001.6 and `.8`.

## Tests

Use TDD for changed state and row logic. Add assertions to these existing suites:

| Evidence | Criteria |
| --- | --- |
| `changes-inline-commit-state.test.ts`: overlapping requests, collapse, settlement, errors, context reset | 001.1, 001.4 |
| `changes-inline-commit-state-owner.test.tsx`: shared owner retirement and late response rejection | 001.4 |
| `changes-panel-header.test.tsx`: full/narrow order, loading/warning tooltip, Review tooltip, initial loading, ready state | 001.1, 001.2, 001.5, 001.7 |
| `changes-panel-body-context.test.tsx`: no refresh banner or Retry, preserved prior rows, initial versus completed empty | 001.3, 001.4, 001.6 |
| `changes-panel-file-row.test.tsx`: no pending text or fabricated counts, unavailable label retained | 001.3 |
| `changes-timeline-model.test.ts`, `changes-panel-timeline-history.test.tsx`, `changes-timeline-history-row.test.tsx`: no loading descriptors, retained errors/empty rows | 001.3, 001.4 |
| `mobile/mobile-changes-panel.test.tsx`: shared loading and inline-detail inputs | 001.1, 001.4 |
| `git-status-refresh-coordinator.test.ts`: delay sequence, duplicate owners, no overlap, partial failure, notification recovery, stale frames, cleanup | Platform 001.25, 001.26, 001.27, 001.28, 001.31, 001.35, 001.36, 001.37, 001.38 |
| `use-session-git-refresh.test.tsx`: eligibility, visibility/focus, release, reconnect, reactivation | Platform 001.29, 001.36, 001.37 |
| `changes-panel-git-status.test.ts`: pending/warning priority, unavailable details, healthy sibling preservation | UI 001.4, Platform 001.25, 001.26, 001.38 |
| History model/row, working-tree, viewport/measurement, and commit-row suites: section/list/commit boundaries, collapse, measured shells, stable identity | Bounded Changes 001.4, 001.6, 001.8, 001.9 |

## E2E tests

Extend `e2e/tests/git/changes-panel-refresh-recovery.spec.ts` in `chromium`.
Keep the real held-enrichment fixture and replace its summary-row loading-text assertion with toolbar status.
Add hover/focus tooltip, status position after the complete left action group, stable header height, and pending/ready screenshots.
Assert that the spinner tooltip contains "Loading changes..." rather than relying on its accessible label alone.
Hover and focus the eye button, assert its "Review" tooltip, and confirm its existing action still opens Review.
Include narrow-panel placement, initial pending, completed empty, and preserved files during refresh.
Force failed reads with prior rows, allow recovery, and observe warning/spinner/ready without a Retry click.

Extend `e2e/tests/git/mobile-changes-panel-refresh-recovery.spec.ts` in `mobile-chrome`.
Assert warning and spinner before opening the diff sheet, automatic recovery, and zero document overflow.
Replace the manual Retry interaction with the scheduled recovery request.
Keep the diff sheet's own loading assertion and recovery evidence.
Add held commit-file requests on both viewports, including concurrency and context switching.
Measure adjacent commit-header spacing while files remain pending.
These scenarios cover AC-UI-CHANGES-LOADING-001.1 through 001.7 and Platform `.25` through `.29`, `.36` through `.38`.

Add small mixed PR/commit geometry cases to the existing desktop and phone
`large-changes-virtualization` suites, under test names containing `spacing`.
Measure inner content gaps and indentation without weakening existing assertions
that virtual wrappers are contiguous. Cover list/tree layout, collapse/expand,
repository groups, refresh, resizing, and panel reopening. Reuse PR #4145's
header regressions, adding only residual checks. Task 02 specifies the gates.

## Work orders

- [x] [Task 01: Consolidate Changes loading feedback](task-01-toolbar-loading.md)
- [ ] [Task 02: Verify and repair remaining section spacing after #4145](task-02-section-spacing.md)

## Verification results

Design validation passed after the PR #4145 overlap reconciliation:

- `python3 scripts/list-docs.py validate`: 339 decisions and 1286 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- Repository `validateCoverage` preflight: both work orders and their UI/Platform delivery references are complete.
- `git diff --check`: passed.

The disposable desktop comparison above confirmed the section regression.
Task 01 implementation and its desktop/phone regression tests now pass. Task 02
production implementation and post-#4145 browser verification remain pending
while PR #4145 is open at head `c5038084dc3d2a3f05294c0797e48905d5fb8953`
(checked 2026-10-02 at 04:01 WEST).
Exact commands are in Tasks 01 and 02.

Task 01 validation passed: 13 targeted Vitest files (88 tests), web typecheck,
scoped ESLint, Prettier, locale generation and checks, public-doc validation,
spec validation, and `git diff --check`. Desktop refresh-recovery E2E passed
3/3 tests for initial pending-to-settled refresh with commit history visible,
tooltips and Review action, held enrichment, and concurrent inline commit reads.
Phone E2E passed 2/2 tests,
including prior-file preservation across a failed refresh, automatic retry, a
held selected diff, concurrent commit reads, full-height diff-sheet geometry,
and no document overflow. `prCapture` recorded the pending and ready states.

## Risks

- Lifting the detail owner must preserve task/session/environment retirement and bounded caching.
- Overlapping requests must not clear the spinner when only one settles.
- Removing JSX without removing its row descriptor leaves blank virtualizer space.
- Narrow headers must retain the spinner when review actions are hidden.
- Retry delay must not reset during same-state rerenders or duplicate consumer attachment.
- A healthy repository or rejected notification must not hide an unresolved sibling failure.
- Moving footer padding must account for collapsed/empty sections and the final expanded commit child.
- Gaps outside measured wrappers would recreate anchor and blank-space defects.
- PR #4145 changes the header baseline. Old measurements cannot justify undoing its compact header design.

## Documentation impact

This package updates internal intent. During implementation, add a short recovery explanation to `docs/public/git-operations.md`.
Explain the toolbar status, preserved data, and automatic read retry in its reference content.
Do not publish planned behavior before implementation.

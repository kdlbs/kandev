---
created: 2026-11-10
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001
system_design:
  - ../../specs/integrations/system-design/github-pr-ci-bubble.md
legacy_specs: []
---

# Implementation Plan: GitHub PR CI bubble

## Overview

Expose CI failures and waits on draft PR status indicators. Passing and skipped-only CI add no bubble.
First retain the missing detailed check facts and project independent CI state.
Then update the existing indicators and disclosures. Finally, prove the desktop and phone flows and update public guidance.

The integrations system owns the package because GitHub evidence determines every visible CI state.
The work orders completed sequentially on 2026-11-10.

Historical layout note, 2026-11-11: this package records the inline conflict layout delivered by PR #5244.
Its Task 02/03 scope, previews, and results describe that delivery.
The completed [corner-badge fix](../pr-conflict-corner-badge/plan.md) records the corrected priority and fresh verification separately.
Keep these completed work orders and counts intact.

## Inputs and settled choices

- [Requirements](../../specs/integrations/requirements/github-pr-ci-bubble.md): `REQ-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001`, criteria .1-.17.
- [System design](../../specs/integrations/system-design/github-pr-ci-bubble.md).
- [Workflow attention](../../specs/integrations/requirements/github-workflow-attention.md) and its [design](../../specs/integrations/system-design/github-workflow-attention.md).
- [Check results](../../specs/integrations/requirements/github-pr-check-results.md) and their [design](../../specs/integrations/system-design/github-pr-check-results.md).
- [Replayable schema migrations](../../decisions/0027-replayable-schema-migrations.md).

Confirmed choices: upper-right CI bubble, no green success bubble, skipped checks are neutral, and all-skipped details do not claim success.
The agreed proposal keeps conflicts separate, uses shared status rendering, and retains desktop hover/focus plus phone tap disclosures.
Implementation details selected here: use a 10px wrapper, an inline conflict triangle, additive bounded metadata, and no placeholder bubble for missing evidence.

The completed [approval badge](../github-workflow-approval-badge/plan.md), [check results](../github-pr-check-results/plan.md),
and [negative projection](../pr-task-disclosure-negative-projection/plan.md) packages are prerequisites and historical records.
Their scenarios remain regression coverage. Do not reopen their completed work orders or rewrite their recorded counts.
Tasks 02 and 03 preserve the results of this original delivery. The later corner-badge behavior is recorded in its linked repair package.

## Scope

### In scope

- CI bubbles on task PR status glyphs, including drafts and aggregate multi-PR indicators.
- Compact summary delivery before hover, freshness ordering, reload, and lifecycle clearing.
- Separate merge conflicts and existing automation dots.
- All-skipped and mixed-skipped explanations in existing summaries and check popovers.
- An additive check-summary migration, existing full-sync/feedback paths, localization, accessibility, and phone behavior.
- Targeted unit, persistence, projection, component, and browser evidence.

### Out of scope

- GitHub mutation actions, polling/cache changes, credentials, and new provider reads.
- Changes to check selection, automatic merge or repair predicates, review rules, and base PR color precedence.
- GitLab, plugin providers, generic PR links, and code-host dashboard type icons.
- New disclosure surfaces, runtime flags, implementation delegation, commits, or publication during planning.

## Technical approach

### 1. Additive data and display projection

Add nullable bounded `check_summary` metadata to GitHub `PRStatus` and `TaskPR`.
It carries the selected snapshot's head SHA, observed count, and completed-skipped count.
One idempotent JSON column follows the existing `workflow_attention` storage pattern.
Populate it only from already selected detailed results. Preserve same-head metadata on lightweight sync and clear it on head changes.

Extend `statussummary.PullRequestSummary` with `ci_state`, `ci_pr_number`, `ci_repository`, and `ci_stale`.
The CI rank is independent of the muted draft aggregate state.
Existing reader and event inputs already carry the check and workflow facts needed for this reduction.
Explicit `none` clears an old bubble. Missing legacy fields do not imply success or a wait.
Keep event, authoritative rebuild, and serialized output equivalent.

Map the fields through `TaskStatusSummary`, `taskPRInfoFromSummary`, and `TaskPRInfo`.
Introduce the pure full-record classifier in `components/github/pr-ci-state.ts`.
Cover the same state matrix in Go and TypeScript. Do not use pass-count subtraction to infer pending checks.

### 2. Shared rendering and disclosure

Extend `PRStatusGlyph`, `PRTaskIcon`'s view model and disclosure props, `TaskContributionIcons`, `PRTopbarButton`, and `SubtaskCIGlyph`.
The composer draft glyph uses the same CI classification while its other status composition remains intact.
The shared wrapper places conflicts inline after the PR icon, before any aggregate PR count.
Keep existing automation dots in their corners.

Generalize compact/full CI authority alongside the current workflow freshness selection.
Preserve explicit negatives, per-PR attribution, hydration failure explanations, and terminal details.
Do not derive CI from aggregate icon colors or review states.

Extend `derivePRTaskStatusSummary` and `PRChecksSection` with skipped explanations.
Loaded selected feedback takes priority over persisted metadata.
Fallback skip text identifies the last detailed observation. Failure, progress, and workflow attention remain primary when present.
Use the shared row vocabulary for a muted all-skipped row and separate skipped-count detail.
New copy belongs in all required catalogs, with Traditional Chinese generated by the existing script.

### Compatibility matrix

| Provider and source | Identity/capability | Behavior | Verification | Missing-data fallback |
| --- | --- | --- | --- | --- |
| GitHub detailed REST/CLI feedback | Selected checks, current head, `ChecksPopulated` | Retain skipped metadata and existing CI reduction | Sync, storage, selected-check tests | Null metadata remains unknown |
| GitHub batched rollup | Current-head check rollup and workflow evidence | Bubble works without check arrays | Projection/adapter tests and background refresh E2E | Retain only same-head last detailed metadata |
| Compact task row | Explicit CI state, one identity, summary time | Bubble before hover or hydration | Mapping, freshness, reload E2E | Legacy omission cannot invent CI |
| Cached full task PRs | Scoped task/PR identity, sync and workflow times | Use newest eligible coherent observation | Component negative/freshness tests | Invalid timestamps keep full authority |
| GitLab/plugin providers | Existing provider models | Existing rendering | No changed provider behavior | No GitHub state inference |

## ASCII UI preview

The notation below is structural. `[PR]` denotes the existing PR glyph, and `(x)`, `(o)`, `(L)`, and `(!)` denote upper-right markers.
Real rendering uses the existing Tabler glyphs, colors, and a 10px wrapper. Spacing and sample titles are illustrative.

UI-01: Desktop task row before disclosure. Draft state remains muted.

```text
Draft CI passed or all skipped:
[done] Fix documentation checks                 [PR]       [...]

Draft CI failed:
                                                 (x)
[done] Fix documentation checks                 [PR]       [...]

CI running: (o)    Workflow approval: (L)    Other attention: (!)

Conflict plus approval, both automation options enabled:
                                                 (L)
[done] Fix documentation checks                .[PR]. [!] 2 [...]
                                              ^    ^  ^
                                       auto-fix    |  inline conflict
                                             auto-merge
```

The conflict triangle never occupies the CI position. The PR count follows the whole group.
The glyph and inline conflict fit inside their row. Badge absence does not certify a CI pass.
UI-01 maps to criteria .1-.4 and .7-.14.

UI-02: Desktop hover/focus details. These are alternative observed states.

```text
PR #4082  Fix documentation checks
CI       Failed
CI       Awaiting maintainer approval   (when also present)
Merge    Conflicts                     (when present)

All-skipped alternative:
CI       All checks skipped
         Last detailed observation      (persisted fallback only)
Merge    Draft

Mixed alternative:
CI       Passed
         2 checks skipped               (separate count)
Merge    Draft
```

Unavailable observations keep the existing unavailable/loading explanation. They do not become passed or skipped claims.
UI-02 maps to criteria .5-.8 and .13-.17.

UI-03: Phone entry and existing PR drawer.

```text
Tasks
[done] Fix documentation checks          [PR+L] [!]
                                          | tap whole PR control
                                          v
       +-------------------------------------+
       | Pull request #4082                  | fixed header
       | CI     Awaiting maintainer approval |
       | Merge  Conflicts                    | one scrolling body
       | Existing PR and automation details  |
       +-------------------------------------+
```

Reuse `PRTaskIconDrawer`, `PRStatusChipDrawer`, and `useTouchDrawer`.
The short status explanation uses a temporary drawer. It does not need another route or another drawer.
The task menu retains its current scroller. The PR drawer retains its bounded height and safe-area clearance.
The full trigger has a phone/coarse-pointer target of at least 44px, including the wider conflict group.
Tap does not select the task. Dismissal returns focus to the opener.
Do not change saved navigation state or add a separate bubble target.
UI-03 maps to criteria .9-.16. Rendered checks cover the canonical phone viewport and both sides of 768px.

## Tests

All criterion suffixes reference `AC-INTEGRATIONS-GITHUB-PR-CI-BUBBLE-001`.
The named new cases are planned evidence, not current results.

| Criteria | Test file and proposed case |
| --- | --- |
| .5-.6, .13, .17 | New `github/check_summary_test.go`: `TestCheckSummarySelectedSkipped`, `TestCheckSummaryAllSkippedRequiresEveryRecord` |
| .5, .13, .17 | New `github/store_check_summary_test.go`: fresh/read/write/legacy/replay; extend `service_test.go` with same-head preservation, zero replacement, and head clearing |
| .2-.4, .7-.8, .12-.14 | New `statussummary/projector_ci_state_test.go`: `TestCIStateProjectionMatrix`, `TestCIStateEventRebuildParity`, `TestCIStateTerminalCleanup` |
| .12-.14 | `backendapp/status_summary_adapter_test.go` and `lib/task-pr-info.test.ts`: identity, explicit none, legacy omission, serialized shape |
| .1-.4, .6-.8, .14, .17 | New `components/github/pr-ci-state.test.ts`: failure/approval/attention/running precedence, skipped neutrality, review independence, terminal filtering |
| .1-.4, .7-.16 | Existing `pr-task-icon` render, workflow approval, conflict, automation, hover, and negative-projection suites; new `pr-status-glyph.test.tsx` and `pr-topbar-button.ci-state.test.tsx` |
| .5-.6, .14-.17 | `pr-task-status-summary.test.ts`, `pr-ci-popover.checks.test.tsx`, `pr-status-chip.test.tsx`, and `lib/github/check-buckets.test.ts` |

## E2E tests

Extend existing feature suites rather than replacing their action and mutation scenarios.
Use provider-backed selected-check/workflow fixtures and wait for persisted API convergence before UI assertions.
Do not inject the desired result into browser state. Capture the request baseline before rendering compact target rows.

| Project and files | Added or updated scenarios | Criteria |
| --- | --- | --- |
| `chromium`: `tests/pr/pr-status-badge.spec.ts` | Draft failure, progress, approval, and no bubble for pass/skips before hover; conflict and automation coexistence; multi-PR attribution; light/dark and reduced motion | .1-.16 |
| `chromium`: `tests/pr/pr-sidebar-background-refresh.spec.ts`, `pr-sidebar-hover-hydration.spec.ts` | Inactive row update, no extra reads to obtain the bubble, refresh resolution, reload, head/terminal cleanup, newer negative projection | .12-.14 |
| `chromium`: `tests/pr/pr-topbar-popover.spec.ts` | Same draft CI state in top bar and composer; skipped-only and mixed-skip explanations | .1-.6, .11, .15-.16 |
| `mobile-chrome`: `tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts` | Bubble before tap in Tasks; separate conflict; 44px target and row containment; open actual drawer, attributed details, dismiss/focus and route preservation | .1-.16 |
| `mobile-chrome`: `tests/pr/mobile-pr-ci-chip.spec.ts`, `mobile-pr-sidebar-background-refresh.spec.ts` | Draft composer parity, all-skipped copy, no false pending state, clearing/reload and current-head ordering | .1-.6, .11-.16 |

Include passed+skipped, all-skipped, failed+skipped, queued/running+skipped, and approval with zero checks.
Also include skipped+cancelled and skipped+neutral, which must not be labelled all-skipped.
Keep actual failure+approval+conflict visible in details while the upper-right cross has priority.
Test a terminal failing sibling beside an open passed/skipped PR.
Retain existing unlink, merge-queue, automation, and provider-refresh scenarios.
Use the existing browser capture fixture for light/dark desktop and phone proof. Inspect the images against UI-01 through UI-03.

## Work orders

- [x] [Task 01: Retain check summaries and project CI state](task-01-project-ci-state.md), wave 1.
- [x] [Task 02: Render shared CI bubbles and skipped details](task-02-render-ci-bubbles.md), wave 2, depends on Task 01.
- [x] [Task 03: Prove desktop and phone behavior](task-03-browser-coverage-and-docs.md), wave 3, depends on Task 02.

Every work order is sequential. No subagents are authorized.

## Verification results

Implemented on 2026-11-10. Tasks 01-03 are complete.

- Backend: `go test ./internal/github -count=1`, `go test ./internal/task/statussummary -count=1`, `go test ./internal/backendapp -run 'StatusSummary|CIState' -count=1`, and `go build ./...` passed. `bash scripts/lint-go-changed` reported 0 issues.
- Frontend: 23 targeted test files and 316 tests passed, including the review-remediation and PR-fixup regressions for retained all-skipped history, superseded compact outcomes, same-number repository attribution, compact CI disclosure during loading/unavailable hydration, sibling outcome clearing, approval-row deduplication, and explicit success precedence. `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed.
- Browser: the new desktop spec passed 5 tests, the new phone spec passed 1 test, and the affected desktop and phone regression suites passed after their placement assertions were updated. Both new specs were rerun after review remediation and passed again.
- Documentation: public-doc validation, specification validation, specification lint, and the PR documentation coverage preflight passed.
- PostgreSQL replay coverage remains environment-gated and was skipped without `KANDEV_TEST_POSTGRES_DSN`.

Planning validation on 2026-11-10:

Planning validation on 2026-11-10:

- `python3 scripts/list-docs.py validate`: passed, including the new requirement/design pair.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Task 03's local `validateCoverage` preflight: passed for all three work orders, including requirement, acceptance, plan, and design references.
- Catalog discovery: both new integration specifications appear under `github-pr-ci-bubble`.
- Local links and whitespace checks: passed across all six new files.
- Prose sentence-length check: no prose line exceeds 25 words after technical identifiers are treated as single terms.
- `git diff --check -- docs/specs docs/plans/github-pr-ci-bubble`: passed.
- `git status --short -- docs/specs docs/plans/github-pr-ci-bubble`: only the six new package files are present, all unstaged.

No product unit tests or browser tests run during this planning turn.

## Risks

- All-skipped cannot be inferred from zero counted checks. Nullable selected metadata must distinguish unknown, empty, and all-skipped observations.
- Rollup-only polling can retain last detailed same-head counts. Qualify fallback text and never let it override an active failure, wait, or newer head.
- Legacy GitHub table rebuilds and qualified reads must retain the new JSON field. Cover both database dialects through their supported tests.
- A 14px glyph plus CI, conflict, count, and automation indicators can clip fixed-width wrappers. Measure the complete group on desktop and phones.
- Existing approval tests expect conflict priority in the upper-right corner. Update assertions to prove both separate indicators without removing the older scenarios.
- Compact/full timestamp handling already has negative-projection regressions. Extend the coherent authority decision instead of adding independent flag merges.

## Public documentation and lifecycle

Task 03 updated `docs/public/integrations.md` with the CI marker meanings, skipped neutrality, all-skipped copy, and the separate merge-conflict slot.
The section is an explanation, not a new setup procedure. No existing status illustration became inaccurate.
The requirement is `active`, the design is `current`, and this plan is `implemented`.

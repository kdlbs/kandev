---
created: 2026-09-30
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION-003
system_design:
  - ../../specs/integrations/system-design/github-workflow-attention.md
legacy_specs: []
---

# Implementation Plan: GitHub workflow approval badge

## Overview

Add an amber padlock to task PR icons when GitHub workflows await approval.
First deliver the bounded projection and icon together. Then prove desktop and phone behavior with provider-backed browser fixtures.
The integration system owns this capability because GitHub evidence defines approval eligibility.

## Inputs and decisions

- [Requirements](../../specs/integrations/requirements/github-workflow-attention.md), requirement 003 and its eight criteria.
- [System design](../../specs/integrations/system-design/github-workflow-attention.md#task-approval-badge).
- [Completed workflow-attention package](../github-workflow-attention/plan.md) and [polling package](../watch-task-cleanup/plan.md).
- [Reviewed-head merge boundary](../../decisions/2026-08-28-bind-github-auto-merge-attempts-to-reviewed-head.md).
- [Bounded task delivery](../../specs/platform/system-design/bounded-task-status-delivery.md).

The user accepted an amber filled padlock, the upper-right warning position, and red conflict priority.
Source inspection confirms stored workflow evidence and existing touch drawers.
The compact projection currently omits approval. Add bounded approval/conflict identity and freshness metadata beside the aggregate flag without new reads.
Reuse existing localized "Awaiting maintainer approval" text instead of introducing a second label.
These reversible presentation choices do not require a new ADR or runtime flag.

## Scope

### In scope

- Task PR glyphs shared by sidebar, Kanban, and task navigation.
- Full and compact PR data, multi-PR aggregation, accessible names, and existing disclosures.
- Current-head stale evidence, lifecycle cleanup, reload, and refresh convergence.
- Desktop/light/dark and phone rendering, targeted tests, and the public integration explanation.

### Out of scope

- Approval, cancellation, rerun, or merge actions on GitHub.
- Topbar badge expansion, GitLab/other providers, and unrelated status-color changes.
- Provider classification, polling/cache redesign, database migrations, and new overlays.
- Implementation subagents, commits, push, or PR publication during this design turn.

## Technical approach

`statussummary.PullRequestInput` carries bounded repository identity, head/classification, and staleness facts.
`githubTaskStatusSummaryPRReader` and `Projector.applyPREventLocked` populate the same facts.
`derivePullRequestSummary` adds `workflow_approval_required`, one attributed approval identity with its stale flag, and one conflict identity without expanding PR arrays.
`taskPRInfoFromSummary` maps the fields and summary timestamp to `TaskPRInfo`.

`PRTaskIcon` compares the compact summary time with every full PR sync/workflow observation time. It uses the compact workflow projection only when all cached full records are older; missing or malformed times keep full current-head evidence authoritative.
`PRTaskIconGlyph` passes it to `PRStatusGlyph`, which selects conflict triangle, approval padlock, or no warning.
Keep automation dots and status colors independent. Add approval text to both compact and full accessible names.
Compact-authoritative disclosures use the projected reason and per-PR identity instead of older cached workflow rows, including localized stale feedback. Preserve per-PR conflicts when both conditions apply.

| Source | Shape | Behavior | Evidence | Missing/unsupported fallback |
| --- | --- | --- | --- | --- |
| GitHub full record | Open PR, matching head, `approval_required` | Eligible badge and per-PR explanation | Component and provider-fixture E2E | No approval inference |
| GitHub compact row | Approval/conflict aggregates, one bounded identity per active reason, summary timestamp | Badge and disclosure when newer than cached PR evidence | Adapter, projector, mapping, desktop/phone E2E | Missing timestamps keep full-data authority |
| Legacy payload | No observation or approval field | Existing glyph behavior | Unit regressions | No approval badge |
| Other providers | Existing provider-specific controls | Existing behavior | No shared-provider logic changes | No GitHub approval claim |

## ASCII UI preview

UI-01: Desktop task row, current-head approval, before disclosure.

```text
[done] Review Contributor PR #4082   [PR+lock]   [...]
                                      ^
                        amber upper-right padlock
```

The base PR color remains its current status color. The badge uses an 8px filled lock in a 10px background wrapper.
For conflicts plus approval, `[PR+!]` contains the red triangle in the same position.
Existing top-left auto-fix and bottom-right auto-merge dots remain visible.

UI-02: Desktop hover/focus disclosure and the same phone drawer content after hydration.

```text
PR #4082
CI     Awaiting maintainer approval
Merge  Conflicts                    (when present)

PR #4083                            (when linked)
CI     Passed
```

Compact disclosure before hydration:

```text
Awaiting maintainer approval
Loading PR details...               (existing status)
```

If hydration fails, retain the first line and the existing unavailable-details message.
Same-head stale evidence uses the existing last-known explanation after hydration.
Unknown, generic action-required, head-mismatched, and terminal states have no lock.

UI-03: Phone task picker entry and existing touch disclosure.

```text
Tasks
[done] Review Contributor PR #4082 [PR+lock]
                                    | tap
                                    v
+----------------------------------------+
| Pull request #4082                      |
| CI    Awaiting maintainer approval      |
| Merge Conflicts        (when present)   |
| Existing PR/automation details          |
+----------------------------------------+
```

The picker retains its scroller. The drawer reuses its existing internal scroll region and safe-area behavior.
Tap opens details without selecting the task. Dismissal returns focus; desktop hover/focus remains available.
Placement, warning priority, and disclosure access are structural requirements. ASCII spacing and sample identities are illustrative.
UI-01 maps to 003.1-003.7; UI-02 and UI-03 map to 003.3, 003.5, 003.6, and 003.8.

## Tests

All suffixes refer to `AC-INTEGRATIONS-GITHUB-WORKFLOW-ATTENTION`.
Named regressions below are proposed, not existing results.

| Criteria | Targeted evidence |
| --- | --- |
| 003.1, 003.4-003.7 | New `projector_workflow_approval_test.go`: `TestWorkflowApprovalProjectionEligibility`, `TestWorkflowApprovalProjectionClearsOnLifecycle`, `TestWorkflowApprovalProjectionAggregatesOpenPRs`, `TestWorkflowApprovalProjectionEventRebuildParity` |
| 003.7 | `status_summary_adapter_test.go`: `TestGitHubStatusSummaryApprovalIdentity`; new `lib/task-pr-info.test.ts`: `maps only the explicit workflow approval flag` |
| 003.1-003.6, 003.8 | `pr-task-icon.render.test.tsx`: badge eligibility, conflict priority, automation dots, accessible labels, compact hydration failure, full negative overriding compact positive, terminal siblings |
| 003.4, 003.5 | Existing `pr-workflow-attention.test.ts` and `pr-workflow-attention-icon.test.ts` remain the observation/readiness regression gates |

## E2E tests

Extend existing feature files; do not create a separate file for each state.

| Project/file | Scenario | Criteria |
| --- | --- | --- |
| `chromium`, `tests/pr/pr-status-badge.spec.ts` | Seed a jobless fork approval through mock provider feedback. Assert sidebar and Kanban lock before hover, keyboard/hover explanation, reload, authoritative clearing, conflict priority, and light/dark colors. | 003.1-003.5, 003.7, 003.8 |
| `mobile-chrome`, `tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts` | Seed a same-head fork approval without conflicts. Open Tasks and assert the amber badge, row containment, and automation dots before tap. Verify per-PR drawer details, route and focus restoration, then update provider state to dirty on the same head and assert the conflict triangle wins while both reasons remain. Cover the ready sibling and terminal cleanup. | 003.1-003.8 |
| `mobile-chrome`, `tests/pr/mobile-pr-ci-chip.spec.ts` | Retain existing approval reason, provider link, and drawer behavior. | 003.8 and prerequisite contract |

Use `mockGitHubSeedPRFeedback` and API state convergence before UI assertions.
Do not inject approval into browser state. Verify the compact boolean before opening details.
Measure badge/row containment and document overflow on phone and narrow fine-pointer viewports.
Capture rendered light/dark desktop and phone states with `prCapture`; compare them with UI-01 through UI-03.
Restore every changed setting and shared fixture record.

## Work orders

- [x] [Task 01: Project and render the approval badge](task-01-project-and-render-approval-badge.md), wave 1, complete.
- [x] [Task 02: Prove desktop and phone behavior](task-02-browser-coverage-and-docs.md), wave 2, complete; depends on Task 01.

Both sequential work orders are complete.

## Verification results

Implementation completed on 2026-09-30. Both work orders passed, and the shared workflow-attention contract is now accepted/current: requirements 001 and 002 are covered by their completed plans, and requirement 003 is covered by this package.

- Desktop E2E: `(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-status-badge.spec.ts -- --retries=0)`; 11 tests passed.
- Phone E2E: `(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts tests/pr/mobile-pr-ci-chip.spec.ts -- --retries=0)`; 9 tests passed.
- Focused phone capture E2E: `(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts -- --retries=0 --grep "shows touch indicators")`; 1 test passed. The approval-only image was visually inspected with the amber lock visible.
- Component and projection regression: `(cd apps/web && pnpm exec vitest run lib/task-pr-info.test.ts components/github/pr-task-icon.render.test.tsx components/github/pr-task-icon.workflow-approval.test.tsx components/github/pr-task-icon-conflicts.test.ts components/github/pr-task-icon.automation.test.ts components/github/pr-task-status-summary.test.ts components/github/pr-workflow-attention-summary.test.ts components/github/pr-workflow-attention.test.ts components/github/pr-workflow-attention-icon.test.ts --reporter=dot)`; 73 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm run build:vite`: passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed.
- `node --test scripts/validate-public-docs.test.mjs`: 62 tests passed; `node scripts/validate-public-docs.mjs`: 47 published pages validated.
- `python3 scripts/list-docs.py validate`: 337 decisions and 1,271 specifications validated.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Focused capture E2E runs passed with `CAPTURE_PR_ASSETS=1`; the seven PNGs are listed in `apps/web/.pr-assets/manifest.json`. The directory is ignored by Git.

## Review remediation verification

On 2026-09-30, fixup corrected compact/full PR freshness precedence and added current review regressions. A task summary supersedes cached full PR records only when its timestamp is strictly newer than every cached PR's required sync timestamp and any later workflow observation; missing or malformed timestamps retain full-record authority. Compact summaries keep approval and conflict identity by repository/PR and include the localized last-known explanation for same-head stale approval. Explicit negative and changed-head evidence clear the approval badge.

- `go test ./internal/task/statussummary -count=1`: passed.
- `go test ./internal/backendapp -run 'StatusSummary|WorkflowApproval' -count=1`: passed, including JSON serialization and case-insensitive SHA comparison.
- Changed and neighboring component regression suites: 73 tests passed across nine files, including stale per-PR disclosure, compact freshness ordering, negative/changed-head clearing, conflict priority, and automation coexistence.
- `pnpm run typecheck`, targeted ESLint, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- `pnpm run build:vite`: passed.
- Desktop E2E: 11 tests passed; phone E2E: 9 tests passed. Both suites passed after remediation.
- Focused desktop and phone capture E2Es: 1 test each passed. Fresh captures were merged into a seven-image manifest, and the approval-only phone capture was visually inspected with the amber lock visible.
- `python3 scripts/list-docs.py validate`: 337 decisions and 1,271 specifications passed. `python3 scripts/lint-spec-files.test.py`: 36 tests passed; `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.

Focused capture commands:

```bash
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project chromium tests/pr/pr-status-badge.spec.ts -- --retries=0 --grep "explains a jobless fork workflow approval")
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --shards 1 --project mobile-chrome tests/pr/mobile-pr-sidebar-automation-indicators.spec.ts -- --retries=0 --grep "shows touch indicators")
```

Captures: desktop light and dark task icons, desktop workflow explanation, phone approval-only task-picker badge and details, and phone conflict-priority badge and details. The approval-only task-picker capture shows the amber padlock; the conflict-priority capture shows the red triangle. The desktop scenario measured light `rgb(217, 119, 6)` and dark `rgb(251, 191, 36)` colors and passed 900px fine-pointer containment with no document overflow. The phone scenario passed lock containment, document overflow, automation-dot coexistence, conflict-priority, multi-PR attribution, dismissal focus-return, and no-navigation checks.

## Risks

- A frontend-only badge would miss inactive task rows; cover both projection writers and reload.
- A stale compact positive must not override newer full-data clearing; cached full rows must not hide a newer compact approval.
- The small glyph needs rendered proof at normal density; conflict priority must preserve the hidden approval text.
- Approval is a workflow gate, not PR review approval. The badge does not assert that the current user has approval permission.
- Provider observations retain their existing refresh latency; this package adds no faster poller.

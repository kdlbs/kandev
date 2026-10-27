---
id: "05-preview-conformance"
title: "Verify rendered previews and update user guidance"
status: in_progress
wave: 5
depends_on:
  - "04-header-and-menu"
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-LIST-VIEWS-001
  - REQ-UI-MOBILE-LIST-VIEWS-002
  - REQ-UI-MOBILE-LIST-VIEWS-003
  - REQ-UI-MOBILE-LIST-VIEWS-004
acceptance_criteria:
  - AC-UI-MOBILE-LIST-VIEWS-001.1
  - AC-UI-MOBILE-LIST-VIEWS-001.3
  - AC-UI-MOBILE-LIST-VIEWS-001.6
  - AC-UI-MOBILE-LIST-VIEWS-001.7
  - AC-UI-MOBILE-LIST-VIEWS-002.1
  - AC-UI-MOBILE-LIST-VIEWS-002.5
  - AC-UI-MOBILE-LIST-VIEWS-002.7
  - AC-UI-MOBILE-LIST-VIEWS-002.8
  - AC-UI-MOBILE-LIST-VIEWS-003.1
  - AC-UI-MOBILE-LIST-VIEWS-003.2
  - AC-UI-MOBILE-LIST-VIEWS-003.3
  - AC-UI-MOBILE-LIST-VIEWS-003.4
  - AC-UI-MOBILE-LIST-VIEWS-004.1
  - AC-UI-MOBILE-LIST-VIEWS-004.2
  - AC-UI-MOBILE-LIST-VIEWS-004.3
  - AC-UI-MOBILE-LIST-VIEWS-004.4
  - AC-UI-MOBILE-LIST-VIEWS-004.5
  - AC-UI-MOBILE-LIST-VIEWS-004.6
system_design:
  - ../../specs/ui/system-design/mobile-list-saved-views.md
---

# Task 05: Verify rendered previews and update user guidance

## Summary

Perform the user-requested rendered comparison against the agreed ASCII previews.
Record screenshots, measured geometry, visual observations, and real-device evidence.
This work order does not add a general repository audit.

## In scope

- Add a focused `mobile-list-preview-conformance.spec.ts` using existing test fixtures.
- Seed coherent disposable data with the four-rule view and distinct activity/color states.
- Capture every applicable preview in a fresh production build, with settled animations.
- Open each resulting image and compare its actual appearance with the named preview.
- Record control heights against the pre-change baseline, insets, scroll ownership,
  containment, keyboard/focus behavior, and any mismatch.
- Include long names, many tabs, empty/loading/error states, links, and plugin overrides.
- Verify wider List/sidebar/Threads compatibility and narrow fine-pointer behavior.
- Attach Task 04's real-iPhone Safari/PWA evidence and repeat the final fixed state.
- Update public how-to guidance only after the rendered flow exists.
- Reconcile touched companion test paths, spec revision notes, statuses, and results.

## Out of scope

Broad `/verify`, generic QA, unrelated snapshots, a new screenshot framework,
product-video production, and approving unexplained preview drift.

## Acceptance

1. Every applicable UI-01 through UI-09 row in `verification.md` has an actual
   screenshot path, measured geometry, visual match decision, and explained differences.
2. The ordinary phone screen has only header, view tabs, and task body. Search
   reveals one row; saved editing opens only from the left. Baseline heights remain.
3. Real-iPhone blur evidence and all targeted behavior checks pass. Unresolved
   visual differences or unavailable device evidence leave acceptance incomplete.

## ASCII UI preview

The complete reference is the [plan's ASCII preview](plan.md#ascii-ui-preview).
These same labels are mandatory in the evidence record.

### UI-01: Phone List, search closed

Entry: ordinary `/tasks`, selected saved view, no result refinements.
AC: 001.1-.7, 002.1, 004.1-.2, 004.6.

```text
+--------------------------------------+
| Default                              |
| List v                       [S] [=] |
+--------------------------------------+
| [All tasks] [Archived] [My view]      |
+--------------------------------------+
|                                      |
| | Running task title       2m  [...] |
| | Another task title      12m  [...] |
| | Task title               1h  [...] |
|                                      |
| [Previous]             [Next]        |
|                                [+]   |
+--------------------------------------+
```

Header and tab row do not scroll vertically. Tabs own contained horizontal
scrolling when needed. The task body owns vertical scrolling. Pagination appears
only when required; the FAB clears the last row and bottom safe area.

### UI-04: View options, sections collapsed

Entry: top-left List v. AC: 003.1-.3, 003.5-.7, 004.2-.3.

```text
+--------------------------------------+
| View options                     [x] |
+--------------------------------------+
| [Kanban] [Threads] [List]             |
|                                      |
| Saved view: All tasks          [...] |
| Filters                           > |
| Sort                              > |
| Group by                          > |
| Task row                          > |
| Automatic colors                  > |
|                                      |
+--------------------------------------+
| [Discard] [Save as new]       [Save]  |
|              safe area               |
+--------------------------------------+
```

The title is fixed; the settings body owns one vertical scroller. The draft
footer is fixed and appears only for a view draft. Section summaries can wrap.
No empty action wrapper creates a gap above the layout selector. View creation,
rename, and delete remain inside the view actions, with existing confirmation rules.

Also compare UI-02, UI-03, UI-05, UI-06, UI-07, UI-08, and UI-09 from the plan.
Do not replace a rendered comparison with a DOM visibility assertion.

## Verification

```bash
pnpm --dir apps/web e2e:run --project mobile-chrome tests/task/mobile-list-preview-conformance.spec.ts tests/task/mobile-list-saved-views.spec.ts
pnpm --dir apps/web e2e:run --project chromium tests/task/workspace-sidebar-views.spec.ts tests/task/task-list-filters.spec.ts tests/task/task-listing-view-preferences.spec.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the package-reference preflight documented in the plan's planning results.
Require nonzero discovery counts for each intended project/spec.

The new conformance spec must capture settled screenshots with
`test.info().outputPath(...)` and attach them to the Playwright report.
Use deterministic IDs, dates, colors, and task states. Restore changed shared
settings/fixtures in cleanup. Do not use arbitrary sleeps to settle geometry.

Required visual procedure:

1. Use the plan's exact viewport/theme matrix.
2. Compare screenshots for UI-01 through UI-09 with their matching preview.
3. Inspect each image with the available image-viewing tool.
4. Record measured height/min-height, relevant bounding boxes, scroll owner,
   hit-test containment, safe-area clearance, and document overflow.
5. Record each structural difference and its correction or explicit accepted rationale.
6. Recheck corrected screens after changes. Keep failed or missing evidence open.

A generated screenshot or passing `toBeVisible` is not a visual match decision.
A new pixel baseline cannot approve itself. Record the human-readable comparison.
No blanket screenshot update command substitutes for this process.

Use Task 04's isolated real-device setup for Safari/PWA validation. Capture
cold load, scroll, close options/menu, and orientation recovery. Record the
actual browser/OS; do not label emulated WebKit as an iPhone pass.

## Files likely touched

- New `apps/web/e2e/tests/task/mobile-list-preview-conformance.spec.ts`.
- A focused shared fixture helper beside the mobile List specs if needed.
- `docs/plans/mobile-list-layout/verification.md` and work-order Results sections.
- `docs/public/mobile-remote-access.md`.
- `docs/public/tasks-and-workflows.md`, specifically List and archive guidance.
- Requirement/design revision notes linked by this plan after successful implementation.

## Dependencies

Tasks 01 through 04. Do not mark this work order done before their required evidence is complete.

## Risks

A small empty fixture can hide clipping and scroll problems. A selector-only
check can pass with wrong padding or hierarchy. Headless browser chrome cannot
prove the original iPhone blur correction. Screenshot paths must identify
actual retained artifacts, not planned filenames.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/mobile-list-saved-views.md), all criteria.
- [Design](../../specs/ui/system-design/mobile-list-saved-views.md), Verification and Geometry.
- [Visual acceptance matrix](plan.md#required-visual-acceptance).
- [Evidence record](verification.md).
- Task 02's pre-change baseline and Task 04's device evidence.

## Results

In progress on 2026-10-27. A fresh mobile run captured and visually inspected
UI-01 through UI-09 in the required light/dark and compatibility variants.
The 393x852 phone, 360x800 long-name phone, 767px fine-pointer phone, 768px
coarse-pointer tablet, and 1024px desktop List/Threads screenshots are retained
under [evidence](evidence/); every measured viewport has no document overflow.
The desktop list, sidebar, filter, and independent Threads consumers passed all
14 responsive compatibility tests. The affected mobile suite passed all 62
tests. Geometry and per-preview decisions are in [verification.md](verification.md).

The rendered browser compositions match the preview hierarchy, density, and
scroll ownership. Two existing details omitted from the schematic are recorded
as accepted differences: the app menu retains its existing Search tasks quick
action, and the result-filter sheet uses the shorter localized Filters title.
Actual iPhone Safari/PWA evidence for the Default label remains a completion
gate. Keep this work order in progress until that device evidence is recorded.

Review remediation on 2026-10-27 added and passed 12 focused phone E2E checks
for archive scope, effective grouping/sorting, orientation dialog retention,
and the settled clear-button hitbox. The clear target stays inside the existing
input bounds. These behavior/geometry checks do not replace the recorded
UI-01 through UI-09 screenshot comparison or the real-device Safari/PWA gate.
Normal Playwright runs attach screenshots and geometry from each test's output
directory. They copy files into committed evidence only when
`CAPTURE_PR_ASSETS=true`, and resolve that evidence path from the spec location.

The next PR E2E run (`40472095546`, head `66c0d4c01c36d202be225f68366a3fe7ea820c70`)
reported three retry-only flakes in unrelated shared tests. Their triggers are
now covered by a longer, explicitly running pause fixture, a retried Monaco
hover action, and subpixel-tolerant 44px geometry checks. Each scenario passed
three times locally with Playwright retries disabled. The fresh PR run remains
required. This does not clear the actual iPhone Safari/PWA Default-label gate.

PR run `40476223800` on head `a43fd70cd4c16767d3b6765a518c32babbeee13d`
reported two E2E failures that passed on retry. The workflow import now waits
for its route refresh response, and the turn-changes geometry test reloads
after backend finalization and waits for the page history response. Each test
passed three targeted runs with retries disabled. A fresh PR run is required;
the real-iPhone Safari/PWA Default-label gate remains pending.

PR run `40480462861` on head `4ba96f070ec2ae2d5a297798752ae7f28ebb182b`
reached the 45-minute wait deadline with three E2E shard leaves failed and the
backend and aggregate checks pending. The failed scenarios were mobile Stats
recovery, mobile terminal shell setup, and active-task storage maintenance.
Their fixtures now wait for an explicit Retry action or use an available
`mock-agent` / `mock-fast` profile. All three affected scenarios passed twice
in targeted managed E2E runs with retries disabled, as did both terminal shell
scroll cases. The PR branch merged current `main` locally and
needs a fresh GitHub run after the fixup commit is pushed. Keep this work order
in progress until that CI run and the real-iPhone Safari/PWA blur evidence are
complete.

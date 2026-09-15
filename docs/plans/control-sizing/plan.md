---
created: 2026-09-09
status: complete
requirements:
  - REQ-UI-CONTROL-SIZING-001
system_design:
  - ../../specs/ui/system-design/control-sizing.md
legacy_specs: []
---

# Implementation plan: Consistent control sizing

## Overview

Standardize ordinary desktop controls around the Start Task dialog.
Establish shared size classes first, then migrate task, settings, and remaining
application surfaces. Each migration preserves touch targets and interactions.

The user accepted the 28px standard, the 24px compact exception, and a broader sweep.
The UI system owns the reusable geometry contract.
Production implementation and focused browser verification are complete.

## Scope

### In scope

- Shared buttons, single-line inputs, select triggers, combobox triggers, and attached icon actions.
- Task details, task creation, settings, integrations, navigation, Office, and shared application wrappers.
- The [candidate inventory](sweep-inventory.md), plus helper, CSS, size-prop, and padding-only follow-up searches.
- Skill guidance and scoped frontend instructions for future control changes.
- Rendered desktop dimensions, touch minimums, and representative interaction checks.

### Out of scope

- Changes to task or provider state, APIs, permissions, or persistence.
- Uniform heights for selection cards, multiline fields, menu options, and third-party editors.
- A new user density preference or a global CSS override for every button.

## Root cause and evidence

The completed-session action combines small Button size with unconditional
`min-h-11`. CSS minimum height forces 44px on desktop.
Recovery actions repeat the same pattern. Other pages use unrelated 24px,
32px, and 36px sizes for equivalent actions.

Settings helpers only set a desktop minimum. Explicit larger heights still win.
The existing desktop typography test checks only a lower bound, so it accepts
oversized controls.

The initial AST scan examined 1,526 TSX source files and found 681 candidate
attributes across 315 files. These counts include correct responsive controls.
The completed inventory records each candidate's source-backed disposition.
The implementation adds representative unit and rendered geometry coverage.

## Technical approach

The [system design](../../specs/ui/system-design/control-sizing.md) defines shared class ownership and responsive behavior.
The existing primitive APIs retain their specialized variants.
Ordinary call sites use the default size.
Application wrappers consume one shared adaptive sizing source.

Each sweep work order owns every file in its inventory section.
Every candidate receives a disposition: changed, already conforming,
touch-only, content-sized, or documented exception.
Implementation must include additional files found through helper and CSS searches.
No candidate remains silently excluded at completion.

## Tests

- Shared size composition: `apps/web/lib/ui/control-sizing.test.tsx`.
- Task behavior: existing `session-stopped-banner.test.tsx` and
  `task-create-dialog-footer.test.ts`.
- Settings helper behavior: existing `settings-typography.test.ts`.
- Browser geometry remains authoritative. Unit class assertions cannot prove rendered heights.

## E2E tests

| Work order | Evidence | Criteria |
| --- | --- | --- |
| 01 | Shared baseline in `layout/control-sizing.spec.ts` and `layout/mobile-control-sizing.spec.ts` | .1-.5, .9 |
| 02 | `task/control-sizing.spec.ts` and `task/mobile-control-sizing.spec.ts` | .1-.10 |
| 03 | `settings/control-sizing.spec.ts`, mobile pair, and existing Layouts/typography specs | .1-.10 |
| 04 | Extend layout pair across integration, Office, navigation, and retained exceptions | .1-.10 |

All criterion suffixes refer to `AC-UI-CONTROL-SIZING-001`.
Use existing managed fixtures and rebuild through the managed E2E runner.
Include 390px and 700px phones, 900px coarse-pointer tablet, and fine-pointer desktop.
Assertions cover exact desktop size, same-row equality, touch minimums, and action results.

## Work orders

- [x] [Task 01: Shared control sizes](task-01-shared-sizes.md)
- [x] [Task 02: Task control sweep](task-02-task-controls.md)
- [x] [Task 03: Settings control sweep](task-03-settings-controls.md)
- [x] [Task 04: Application control sweep](task-04-application-controls.md)

Work proceeds sequentially. No delegation is authorized.

## Instruction changes

The planning turn updates mobile-parity, its mobile reference, the E2E skill,
and the scoped frontend engineering guide. The new sizing reference records
the sweep procedure and regression expectations.

## Verification results

Artifact checks passed on 2026-09-09:

- `python3 scripts/lint-harness-files.test.py`: 19 tests passed.
- `python3 .github/scripts/lint-harness-files.py --all`: all 187 harness files passed.
- `python3 scripts/lint-spec-files.test.py`: 30 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- `git diff --check`: passed.

Production and browser verification completed on 2026-09-10.
Each work order records its own red/green commands and results.

Final verification summary:

- `pnpm --dir apps install --frozen-lockfile`: passed.
- Focused Vitest: 9 files, 111 tests passed.
- Managed E2E: layout 3 desktop and 2 mobile, task 4 desktop and 1 mobile,
  settings 10 desktop and 8 mobile, plus the task workflow-stepper pair, all passed.
- Exact changed-file ESLint covered 200 `apps/web` TypeScript files with 0 errors
  and 7 non-blocking warnings.
- Harness and specification validation passed: 19 harness tests, 187 harness
  files, 30 specification tests, all specifications, targeted pre-commit harness
  lint, and `git diff --check`.

## Risks

- A minimum height can override a smaller explicit height.
- SelectTrigger data-size variants can override ordinary height classes.
- Attached clear/reveal controls can overlap after only the field shrinks.
- Shared wrappers can affect compact chrome or mobile-only surfaces.
- Long translations and coarse-pointer input fonts can expose clipping.
- The AST candidate list does not resolve named constants or runtime branches.

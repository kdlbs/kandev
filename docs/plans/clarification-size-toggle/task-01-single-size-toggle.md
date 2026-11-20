---
id: "01-single-size-toggle"
title: "Replace clarification size actions with one toggle"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CLARIFICATION-CONTEXT-001
acceptance_criteria:
  - AC-UI-CLARIFICATION-CONTEXT-001.9
  - AC-UI-CLARIFICATION-CONTEXT-001.10
  - AC-UI-CLARIFICATION-CONTEXT-001.11
  - AC-UI-CLARIFICATION-CONTEXT-001.13
  - AC-UI-CLARIFICATION-CONTEXT-001.14
  - AC-UI-CLARIFICATION-CONTEXT-001.15
  - AC-UI-CLARIFICATION-CONTEXT-001.16
system_design:
  - ../../specs/ui/system-design/clarification-context.md
---

# Task 01: Replace clarification size actions with one toggle

## Summary

Deliver the single Expand/Restore control across every shared pending-question host.
Retain the previous sizing mode through host changes and responsive remounts, with focused unit and browser evidence.

## In scope

- Extend the existing provider and hook sizing records with atomic auto/manual/expanded state and Restore targets for desktop and phone.
- Replace the paired header props with one toggle action and explicit state. Retain the existing grip, collapse, skip, and Submit behavior.
- Add localized Expand/Restore labels and tooltips across all catalogs, using repository generators for Traditional Chinese and pseudo values.
- Migrate every old expand/reset selector, including Coordinator and desktop/mobile Threads tests.
- Update the sizing paragraph in `docs/public/sessions-and-review.md` with the new control and previous-height behavior.
- Complete the [plan's scenario matrix](plan.md#e2e-tests) and record exact validation results.

## Out of scope

- Backend, answer-cache ownership, question schemas, response delivery, or barrier changes.
- Cross-task sizing persistence, new menus/settings, touch dragging, or unrelated header restyling.
- Rewriting completed companion plans or treating their recorded results as evidence for this work order.

## Acceptance

- One icon-only button replaces both size actions, with localized action-specific names/tooltips, stable position, preserved focus, and correct desktop/touch dimensions.
- Auto/manual restoration, repeated cycles, host changes, presentation isolation, remounts, drag/reset, and bundle handover pass the design's transition cases.
- All affected hosts preserve drafts and answer reachability. Targeted checks pass and public instructions describe the shipped single-control behavior.

## ASCII UI preview

Excerpts from [the full preview](plan.md#ascii-ui-preview). These retain its stable view labels.

### UI-01: Desktop pending question, normal and expanded

```text
Normal:   [Question]                   [E] [x] [v]
Expanded: [Question]                   [R] [x] [v]
```

### UI-02: Phone pending question, same inline surface

```text
[Question]          [E/R] [x] [v]
[context and question text     ]
[choices and answer input      ]

Multiple questions retain the existing two header rows:
[Question] [< 2/3 >] [answered]
[E/R]          [Submit] [x] [v]
```

### UI-03: Constrained host

```text
Normal, no room: [Question] [E disabled] [x] [v]
Expanded:       [Question]          [R] [x] [v]
```

E/R represents one maximize/minimize icon button with Expand/Restore names. The header has no persistent size-action text.
The existing conditional Submit retains its position. The existing scroll owner and header wrapping remain unchanged.
Phone and coarse-pointer size targets measure at least 44px square. Ordinary fine-pointer desktop buttons measure 28px square.
Grouping, a single button, and state behavior are required. Spacing and ASCII icon letters are illustrative.
These views cover .9-.11 and .13-.16. Compare focused browser screenshots with the full plan previews.

## Verification

Bootstrap once from the repository root if workspace dependencies are absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

After locale edits, generate the derived catalogs from the repository root:

```bash
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:pseudo)
```

Run these commands sequentially from the repository root. Managed E2E commands build current code before each project run.

```bash
(cd apps/web && pnpm exec vitest run hooks/use-resizable-clarification-overlay.test.ts components/task/chat/clarification-draft-context.test.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/clarification-input-overlay.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 hooks/use-resizable-clarification-overlay.ts hooks/use-resizable-clarification-overlay.test.ts components/task/chat/clarification-draft-context.tsx components/task/chat/clarification-draft-context.test.tsx components/task/chat/clarification-panel-section.tsx components/task/chat/clarification-panel-section.test.tsx components/task/chat/clarification-input-overlay.tsx components/task/chat/clarification-input-overlay.test.tsx components/task/chat/clarification-overlay-header.tsx e2e/tests/chat/clarification-resize.spec.ts e2e/tests/chat/clarification-context-hosts.spec.ts e2e/tests/chat/mobile-clarification.spec.ts e2e/tests/coordinator/needs-you.spec.ts e2e/tests/task/threads-composer-disclosure.spec.ts e2e/tests/task/mobile-threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/clarification-resize.spec.ts tests/chat/clarification-context-hosts.spec.ts tests/coordinator/needs-you.spec.ts tests/task/threads-composer-disclosure.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/chat/mobile-clarification.spec.ts tests/task/mobile-threads-composer-disclosure.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Search all `apps/web` source and E2E files for `clarification-panel-expand` and `clarification-panel-reset` after migration.
Neither removed selector can remain as an active test or UI dependency.
Run the documentation coverage preflight from the repository root:

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/ui/requirements/clarification-context.md',
  'docs/specs/ui/system-design/clarification-context.md',
  'docs/plans/clarification-size-toggle/plan.md',
  'docs/plans/clarification-size-toggle/task-01-single-size-toggle.md',
];
const fileContents = Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only'], { encoding: 'utf8' }).split('\n'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard'], { encoding: 'utf8' }).split('\n'),
].filter(Boolean))];
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
```

The plan records the design-turn preflight separately from implementation results.
After every check passes, promote the paired requirement/design to active/current and mark the plan implemented.

## Files likely touched

- `apps/web/hooks/use-resizable-clarification-overlay.ts` and its test.
- `apps/web/components/task/chat/clarification-draft-context.tsx` and its test.
- `apps/web/components/task/chat/clarification-panel-section.tsx` and its test.
- `apps/web/components/task/chat/clarification-input-overlay.tsx` and its test.
- `apps/web/components/task/chat/clarification-overlay-header.tsx`.
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/chat.json`.
- The six browser spec files named in the commands above.
- `docs/public/sessions-and-review.md`.
- The paired clarification-context requirements/design and this package's status/results.

## Dependencies

None. Use existing host measurement, localization, draft ownership, and mock-agent fixtures.

## Risks

- A callback-only implementation can lose the Restore target across responsive remounts.
- Null pixel heights do not imply empty state for expanded-auto records.
- A shrinking host must not disable Restore or erase the saved desired height.
- Existing multi-question and Threads fallback geometry must retain answer access and separate touch targets.

## Parallelism

`sequential`. This work order changes shared state and header contracts together. No subagents are authorized.

## Inputs

- [Requirements](../../specs/ui/requirements/clarification-context.md), .9-.11 and .13-.16.
- [System design](../../specs/ui/system-design/clarification-context.md), sizing transitions and accessible interaction.
- [Plan](plan.md), including the test matrix and full UI previews.
- `apps/web/AGENTS.md`, `/tdd`, `/mobile-parity`, `/e2e`, and `/docs-maintainer`.
- Existing hook/provider/component tests and the named browser specs.
- `apps/web/components/task/dockview-group-actions.tsx` for the existing maximize/minimize icon pair.

## Results

Implemented and verified on 2026-11-03.

- The hook stores `auto`, `manual`, and `expanded` records with an atomic normal-state Restore target per presentation.
- The header renders one 28px icon-only toggle (44px on phone and coarse pointers) with action-specific labels and tooltips.
- Focus remains on the same toggle through Expand and Restore, including while measurements catch up and after host contraction; `aria-disabled` with guarded activation keeps the DOM identity stable. Expand accepts only fresh panel and host measurements at click time. The grip and double-click reset remain unchanged.
- Drag updates keep applying after the pointer returns to the starting Y; a no-movement click still leaves auto sizing unchanged.
- Provider cache tests cover expanded-auto retention, presentation separation, task isolation, and the eight-bundle bound.
- Focused Vitest: 4 files and 105 tests passed. Typecheck, the production build, changed-file ESLint, i18n checks, and the i18n ratchet passed.
- Managed browser runs passed: 27 Chromium tests and 24 Mobile Chrome tests across task chat, Quick Chat, Needs You, Coordinator, Threads, and phone hosts.
- Public instructions now describe Expand, Restore, dragging, and double-click content sizing.

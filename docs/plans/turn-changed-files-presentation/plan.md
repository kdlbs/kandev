---
created: 2026-10-26
status: implemented
requirements:
  - REQ-TASKS-TURN-CHANGES-005
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
legacy_specs: []
---

# Implementation plan: Turn changed-files presentation

## Overview

Align terminal file summaries with the Changes panel and separate the next message.
Tasks owns this refinement because the card belongs to the captured turn.
One sequential work order covers the card and its rendered regression checks.

## Scope

Include file icons, file-kind colors, file/folder count colors, and message spacing.
Exclude capture changes, live staging actions, full tree replacement, and new copy.
The existing [feature plan](../turn-changed-files/plan.md) retains its capture and
executor qualification scope. This package owns only the presentation refinement.

## Technical approach

Use `FileIcon` from `components/ui/file-icon.tsx` with the historical file path.
Use `LineStat` from `components/diff-stat.tsx` for known file and folder counts.
Use it for header totals too so the card uses one count palette.
Preserve binary and unknown-count labels and existing accessible descriptions.
Match `FileStatusIcon` colors: added emerald-600, modified amber-600,
deleted rose-600, renamed purple-600. Keep copied/type/mode labels accurate;
use neutral text for kinds without a matching Changes status.
Do not coerce historical kinds to live Git status values.
Reuse these small presentation components, not the full file browser with its
selection, drag, context-menu, and live checkout dependencies.

Add non-collapsing 16px space after the bordered card. Inspect both
`TurnChangedFilesAfterMessage` and fallback placement in `message-list-native.tsx`.
Prefer card-owned outer padding so both anchors receive the same spacing.
Preserve the existing ResizeObserver and bottom-follow behavior.

## ASCII UI preview

UI-01: Expanded terminal card in task chat, desktop and phone.

```text
[3 changed files  +6 -3] [Expand all] [Open diff]
  v master
    v .agents/
      [MD icon] SKILL.md       Modified +2 -1
[card border]

             [Next user message]
```

Required: shared file icons, amber Modified, green additions, red deletions,
and at least 16px between card border and next message. Copy remains localized.
Phone retains the existing stacked header, 44px touch targets, and inline tree.
Allow row metadata to wrap below the filename when needed. Keep the transcript
as scroll owner. File taps retain the existing full-height `MobileDiffSheet`.
The existing mobile turn-changes card is the closest shipped exemplar.
Preview covers AC-TASKS-TURN-CHANGES-005.10 through 005.12.

## Tests

Extend `turn-changed-files-card.test.tsx` before implementation for file icons,
kind colors, and separate known counts. Cover binary/unknown counts and renames.
Keep navigation and folder expansion assertions.

## E2E tests

Extend desktop/mobile `turn-changed-files.spec.ts` for all three new criteria.
Compare computed colors against shared primitives in light and dark themes.
Verify icon glyphs and measure card-border to next-message distance after a
second turn. Cover anchored and fallback cards, expanded folders, long paths,
and page reload. Check 390px containment and 44px targets with fine and coarse
pointers. Use Pixel 5 plus widths 767px and 768px around the breakpoint.
Retain existing scroll suites to catch spacing-related bottom-follow regressions.

## Work orders

- [x] [Task 01: Align summary presentation](task-01-summary-presentation.md)

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm test components/task/chat/turn-changed-files-card.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/turn-changed-files-card.tsx --max-warnings 0)
make build-web
make build-backend
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/git/turn-changed-files.spec.ts tests/git/turn-changed-files-scroll.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/git/mobile-turn-changed-files.spec.ts tests/git/mobile-turn-changed-files-scroll.spec.ts
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Verification results

Implementation verification passed:

- Focused card test: 7 passed; web typecheck, focused ESLint, and both builds passed.
- Desktop changed-files E2E: 6 passed, including the 390px fine-pointer case.
- Desktop scroll E2E: 1 passed.
- Mobile changed-files and scroll E2E: 4 passed, including 390px, 767px, and 768px checks.
- PR fixup: the phone regression for long unbroken metadata failed before the
  wrapping change and passed after it. The unavailable-capture case now reloads
  persisted history before checking its baseline card. The final mobile suite
  passed all 4 cases, and the desktop changed-files plus scroll suite passed all
  7 cases.
- Unknown-file icon comparisons now require both rendered fallback icons.
- After the fixup, the focused card test passed 7 cases; web typecheck, focused
  ESLint, the E2E web build, and `git diff --check` passed.
- Catalog validation covered 403 decisions and 1,740 specifications; specification lint and `git diff --check` passed.
- Current PR fixup: the Chromium changed-files E2E rebuilt the backend and web assets and passed all 6 cases, including the narrow fine-pointer phone case. Mobile agent-profile deletion passed 3 cases; the targeted Chromium restart, branch-selector, and session-recency cases passed 3 cases with retries disabled. Focused ESLint and `git diff --check` passed.
- Profile-order cleanup skips the API write when the original order is empty; its mobile E2E passed 1 case with retries disabled.
- PR CI run 40482001180 completed on merge SHA `3663a7ca` with four flaky E2E
  failures across shards 6, 13, and 16. All passed on retry, but
  `E2E_FAIL_ON_FLAKY=1` failed the shards and aggregate. Follow-up now waits for
  conversation indexing, mock prompt acceptance before cancellation, and
  runtime replacement before sibling startup; the branch-policy tooltip check
  uses keyboard focus. Retries-disabled local E2E passed 1 branch-policy case
  and 3 mobile cases. Focused ESLint, web typecheck, Prettier, and
  `git diff --check` passed.

## Risks

- A generic tree replacement would introduce live workspace behavior into history.
- Margin collapse can hide the intended gap; measure actual message geometry.
- Long translated status labels can crowd phone filenames; allow metadata wrapping.

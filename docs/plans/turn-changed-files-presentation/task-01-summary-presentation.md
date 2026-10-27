---
id: "01-summary-presentation"
title: "Align summary presentation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-TURN-CHANGES-005
acceptance_criteria:
  - AC-TASKS-TURN-CHANGES-005.10
  - AC-TASKS-TURN-CHANGES-005.11
  - AC-TASKS-TURN-CHANGES-005.12
system_design:
  - ../../specs/tasks/system-design/turn-changed-files.md
---

# Task 01: Align summary presentation

## Summary

Reuse file icons and line statistics in the historical summary.
Add a measured gap before the next message on desktop and phone.

## In scope

Card presentation, shared primitive reuse, and focused rendered regressions.

## Out of scope

Capture, persistence, live Git actions, and file-tree interaction replacement.

## Acceptance

- File and folder counts match Changes colors; kinds retain accurate colored labels.
- File icons match the file tree, including unknown extensions.
- Both anchors leave at least 16px before the next message without overflow or scroll regressions.

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

See the [full plan](plan.md) for component mappings and the E2E scenario matrix.

## Verification

Use TDD for added logic and regressions. Run these commands from the repository root.

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

## Files likely touched

- `apps/web/components/task/chat/turn-changed-files-card.tsx`
- `apps/web/components/task/chat/turn-changed-files-card.test.tsx`
- `apps/web/components/task/chat/message-list-native.tsx` only if spacing needs parent ownership
- `apps/web/e2e/tests/git/turn-changed-files.spec.ts`
- `apps/web/e2e/tests/git/mobile-turn-changed-files.spec.ts`
- `apps/web/e2e/tests/git/turn-changed-files-helpers.ts`

## Dependencies

None. Existing `FileIcon` and `LineStat` APIs already support this use.

## Risks

Preserve binary/unknown semantics, historical targets, and scroll ownership.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/turn-changed-files.md), transcript card criteria.
- [Design](../../specs/tasks/system-design/turn-changed-files.md), transcript projection.
- Existing card tests, mobile historical-diff tests, and Changes file rows.

## Results

Implemented shared file icons, status colors, and line statistics in the
historical summary. Added 16px card spacing and responsive checks for anchored
and fallback cards, long renamed paths, and touch targets.

Verification passed:

- `pnpm install --frozen-lockfile` from `apps/`.
- Focused card test: 7 passed.
- Web typecheck with `NODE_OPTIONS=--max-old-space-size=8192` and focused ESLint.
- `make build-web` and `make build-backend`.
- Desktop changed-files E2E: 6 passed, including the 390px fine-pointer case.
- Desktop scroll E2E: 1 passed.
- Mobile changed-files and scroll E2E: 4 passed, including 390px, 767px, and 768px widths.
- PR fixup: the phone regression for long unbroken metadata failed before the
  wrapping change and passed after it. The unavailable-capture case now reloads
  persisted history before checking its baseline card. The final mobile suite
  passed all 4 cases, and the desktop changed-files plus scroll suite passed all
  7 cases.
- Unknown-file icon comparisons now require both rendered fallback icons.
- After the fixup, the focused card test passed 7 cases; web typecheck, focused
  ESLint, the E2E web build, and `git diff --check` passed.
- `python3 scripts/list-docs.py validate`: 403 decisions and 1,740 specifications validated.
- `python3 scripts/lint-spec-files.py --all` and `git diff --check`.
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

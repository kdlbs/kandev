---
id: "01-render-queue-timestamps"
title: "Render queue timestamps"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUEUED-MESSAGE-TIMESTAMPS-001
acceptance_criteria:
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.4
  - AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
system_design:
  - ../../specs/ui/system-design/queued-message-timestamps.md
---

# Task 01: Render Queue Timestamps

## Summary

Reuse the transcript timestamp component in every displayed queue row.
Prove that the rendered timestamp tracks the backend value after merges.

## In scope

Shared timestamp extraction, queue integration, focused component and browser
tests, timestamp-drawer Escape ownership, and the queue documentation sentence described in the plan.

## Out of scope

Backend behavior changes, new settings, periodic refresh, and historical repair.

## Acceptance

1. Queue rows render valid server timestamps in the selected format and omit invalid values.
2. Manual, repeated, reordered, and automatic merge scenarios display the newest contributing time after refresh and reload.
3. Phone users can open timestamp details with a contained 44-pixel target. Transcript behavior and queue actions pass regression checks.

## UI-01: Expanded queue row

See the [complete UI description](plan.md#ui-01-expanded-queue-row).
Keep position and sender on the left, preview and attachments in the center,
and actions on the right. Place the timestamp below the preview and attachments.
Phone labels wrap within that column. Tapping the timestamp opens a details
drawer. Invalid timestamps omit the line. This covers AC .1, .3, .4, and .5.
Use the task's Kandev rich-output preview instead of an ASCII drawing.

## Verification

Run from the repository root. Install dependencies once if this worktree lacks them.
Use TDD for the new component behavior before extracting production code.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/queued-ghost-message-timestamp.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/chat/queued-ghost-message-overflow.test.tsx components/task/chat/messages/message-actions.test.tsx lib/i18n/message-time.test.ts components/task/chat/use-queue-panel-escape.test.tsx components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-pin.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/chat/queued-ghost-message.tsx components/task/chat/queued-ghost-message-timestamp.test.tsx components/task/chat/queued-ghost-message.test.tsx components/task/chat/queued-ghost-message-overflow.test.tsx components/task/chat/messages/message-timestamp.tsx components/task/chat/messages/message-actions.tsx components/task/chat/use-queue-panel-escape.ts components/task/chat/use-queue-panel-escape.test.tsx components/task/chat/queued-ghost-list.test.tsx components/task/chat/queued-ghost-pin.test.tsx e2e/tests/chat/queued-message-timestamps.helpers.ts e2e/tests/chat/queued-message-timestamps.spec.ts e2e/tests/chat/mobile-queued-message-timestamps.spec.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/queued-message-timestamps.spec.ts tests/chat/message-timestamp-tooltip.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-queued-message-timestamps.spec.ts tests/chat/mobile-message-timestamp-tooltip.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Managed E2E commands build fresh assets. Run the two browser commands sequentially.
If implementation changes another test file, add its focused command here.

## Files likely touched

- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-timestamp.tsx` (new)
- `apps/web/components/task/chat/queued-ghost-message.tsx`
- `apps/web/components/task/chat/queued-ghost-message-timestamp.test.tsx` (new)
- `apps/web/e2e/tests/chat/queued-message-timestamps.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-queued-message-timestamps.spec.ts` (new)
- `apps/web/e2e/tests/chat/queued-message-timestamps.helpers.ts` (new fixture helper)
- Existing queue-row, overflow, queue-panel, and pin test fixtures gain `StateProvider`.
- `apps/web/components/task/chat/use-queue-panel-escape.ts` and its new focused test.
- `docs/public/operations.md`

## Dependencies

None. Existing backend merge persistence supplies the required timestamp.

## Risks

The shared extraction must preserve transcript selectors and touch behavior.
Browser assertions must scope timestamps to the intended queue row.
Long dates must not reduce action targets or interfere with dragging.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/queued-message-timestamps.md)
- [System design](../../specs/ui/system-design/queued-message-timestamps.md)
- Existing `message-actions.test.tsx` and `queued-ghost-message.test.tsx`.
- Existing desktop queue and mobile timestamp Playwright scenarios.

## Results

Completed on 2026-10-28.

- Dependency installation passed with the frozen lockfile.
- RED: 10 of the 15 new timestamp component cases failed because no timestamp rendered.
- RED: both phone scenarios exposed Escape collapsing the queue. The new Escape hook test also failed before its fix.
- GREEN: 181 tests passed across eight focused files. The five timestamp/row/formatter suites passed 120 tests. The queue panel, pin, and Escape suites passed 61 tests.
- The documented typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`. The initial default 4 GiB Node heap exhausted memory.
- The documented ESLint command passed with `--max-warnings 0`, including the added fixture and Escape files.
- `pnpm run i18n:check` passed. Existing unused-catalog warnings remain informational.
- Desktop browser command passed all four cases on the final build. Log: `/tmp/kandev-run.e2e.OSgx2XJ4.log`.
- Mobile browser command passed all three cases, including both 393px and 390px queue cases. Log: `/tmp/kandev-run.e2e.QerNPWxo.log`.
- Browser runs used the managed runner. A fresh build preceded final verification. Later test/capture-only reruns used `--no-build` against those same assets.
- Host setup required `KANDEV_SERVER_HOST=127.0.0.1`, installed Nix browser libraries through `LD_LIBRARY_PATH`, and removal of inherited `KANDEV_INTERNAL_CONFIG_FILE`, `KANDEV_INTERNAL_CONFIG_HOME_FILE`, and `KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`. These were process-only overrides in `/tmp/queued-timestamps-e2e-env.sh`. Repository runtime configuration was not changed. Initial fixture startup and missing-library failures occurred before test behavior ran.
- `CAPTURE_PR_ASSETS=1` captured the desktop row, phone rows, and phone drawers. Captures wait for finite entrance animations. Visual inspection confirmed the intended structure and contained phone drawer. Phone captures are retained under `/tmp/queued-timestamps-screenshots/`.
- Public-doc validator tests passed all 63 cases. Public-doc validation passed for 54 pages.
- Catalog validation, full specification lint, PR-documentation coverage preflight, and `git diff --check` passed.

Public documentation: `docs/public/operations.md` (reference).
No commits, pushes, or external publication were performed.

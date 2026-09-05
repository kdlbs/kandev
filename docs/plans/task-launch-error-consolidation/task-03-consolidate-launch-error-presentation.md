---
id: "03-consolidate-launch-error-presentation"
title: "Consolidate launch-error presentation"
status: completed
wave: 3
depends_on:
  - "02-classify-launch-recovery-actions"
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001
acceptance_criteria:
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.7
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 03: Consolidate launch-error presentation

## Summary

Make the task launch card the only primary presentation for its matching error.
Give desktop and phone users the same cause, details, and valid recovery.

## In scope

- Derive one launch-error owner from session identity and error stamp.
- Add clear category text, a no-change statement, and a details disclosure.
- Suppress matching duplicate notices, rows, banners, empty-turn copy, and toasts.
- Preserve unrelated historical and runtime errors.
- Add all required locale values.
- Add desktop and mobile Playwright coverage.

## Out of scope

- Redesign completed-session UI.
- Change non-launch runtime-error recovery.
- Change the mobile branch picker.

## Acceptance

- One failed start renders one error card after all events converge.
- Recovery failure updates the same card without another error surface.
- Phone actions remain reachable, at least 44 pixels high, and free of horizontal overflow.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run components/task/simple/components/task-launch-error-entry.test.tsx components/task/chat/chat-input-container.test.tsx components/task/chat/message-list-footer.test.tsx components/task/chat/session-stopped-banner.test.tsx components/task/chat/stopped-banner-props.test.ts components/task/chat/types.test.ts lib/ws/handlers/empty-turn-notice.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/task/launch-failure-recovery.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts
```

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/task-launch-error-context.tsx`
- `apps/web/components/task/simple/components/task-launch-error-entry.tsx`
- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-footer.tsx`
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/lib/ws/handlers/empty-turn-notice.ts`
- Focused tests beside these files.
- `apps/web/src/locales/*/task.json`
- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`

## Dependencies

Task 02.

## Risks

- Event order can briefly show a derived error before the typed record arrives.
- Broad suppression can hide a later agent runtime error.

## Parallelism

`sequential`

## Inputs

- Task launch failure recovery design, presentation ownership and responsive sections.
- Mobile exemplar `apps/web/components/task/mobile/mobile-picker-sheet.tsx`.
- Existing desktop and mobile launch-failure tests.

## Results

- The typed task launch card is the single owner for its matching session and
  error stamp. Matching preparation, empty-turn, agent-error, failed-status,
  stopped-session, and toast surfaces are suppressed without hiding unrelated
  runtime errors.
- The card now shows category-specific copy, bounded technical details, a
  no-change statement, inline recovery failures, and mobile-sized stacked
  actions.
- Added complete locale values and generated the pseudo-locale catalog.
- Focused frontend coverage passed: 10 Vitest files and 152 tests.
- `pnpm run typecheck`, `pnpm run lint`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Managed Playwright coverage passed: desktop launch-failure recovery 3/3 and
  mobile launch-failure recovery 2/2.
- Review-remediation coverage passed: 7 focused Vitest files and 75 tests,
  including composer ownership and launch-error recovery-state reset.

---
id: "01-resolve-notices"
title: "Resolve resumed-turn notices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-STALL-RECOVERY-001
acceptance_criteria:
  - AC-AGENTS-AGENT-STALL-RECOVERY-001.6
system_design:
  - ../../specs/agents/system-design/agent-stall-recovery.md
---

# Task 01: Resolve resumed-turn notices

## Summary

Derive running advisory notice resolution from later agent activity in the
same session and turn. Preserve terminal diagnostics and existing controls.

## In scope

- A pure timestamp/activity predicate and a boolean store selector.
- Live-update regression coverage and desktop/mobile hydration evidence.
- Synthetic before/after screenshot assets outside the PR branch.

## Out of scope

Changing watchdog thresholds, cancelling quiet work, or new backend events.

## Acceptance

1. An existing compaction tool row updated after the notice hides it while
   the session and active turn remain running, including after reload.
2. Agent text, reasoning, tools, plans, and permission rows resolve it;
   user/system rows and other sessions/turns do not.
3. Terminal error diagnostics remain available.

## ASCII UI preview

UI-01 (desktop and phone), from [the plan](plan.md#ascii-ui-preview):

```text
Quiet:    Still waiting on Compact conversation.  [Cancel turn]
Resumed:  <completed tool or latest agent content>
```

AC-AGENTS-AGENT-STALL-RECOVERY-001.6 owns visibility. Existing phone control
geometry remains in place.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/messages/action-message.test.tsx components/task/chat/messages/running-notice-activity.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/task/chat/messages/action-message.tsx components/task/chat/messages/action-message-state.ts components/task/chat/messages/running-notice-activity.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --project chromium e2e/tests/session/stall-notice-recovery.spec.ts)
(cd apps/web && CAPTURE_PR_ASSETS=1 pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-stall-notice-recovery.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Preserve the desktop capture outside `.pr-assets` before the second runner,
then merge and validate manifests before publication.

## Files likely touched

- `apps/web/components/task/chat/messages/action-message.tsx`
- `apps/web/components/task/chat/messages/action-message-state.ts`
- `apps/web/components/task/chat/messages/action-message.test.tsx`
- `apps/web/components/task/chat/messages/running-notice-activity.ts`
- `apps/web/components/task/chat/messages/running-notice-activity.test.ts`
- `apps/web/e2e/tests/session/stall-notice-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-stall-notice-recovery.spec.ts`

## Dependencies and parallelism

None. Sequential, primary session.

## Inputs

[Requirement](../../specs/agents/requirements/agent-stall-recovery.md) and
[design](../../specs/agents/system-design/agent-stall-recovery.md).

## Risks

Keep timestamp precision and distinguish agent activity from status traffic.

## Results

- RED: the compaction-row live-update assertion failed in the existing
  action-message harness and in Chromium E2E before the production change.
- GREEN: targeted Vitest, 57 tests passed across two files.
- TypeScript typecheck and targeted ESLint passed with no errors or warnings.
- Desktop Chromium and phone mobile-chrome E2E each passed, including
  the same running turn before/after activity and after reload.
- Specification catalog validation, specification lint, and diff whitespace
  checks passed.
- Fresh synthetic quiet/resumed screenshots captured for both viewports;
  screenshot binaries are published only on a separate media ref.
- PR CI, automated review, and merge are tracked in the platform task plan.


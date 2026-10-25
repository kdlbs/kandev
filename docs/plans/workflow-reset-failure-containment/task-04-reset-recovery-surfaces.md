---
id: "04-reset-recovery-surfaces"
title: "Verify reset recovery surfaces"
status: done
wave: 4
depends_on:
  - "03-workflow-reset-feedback"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-AGENT-START-OWNERSHIP-002.9
system_design:
  - ../../specs/tasks/system-design/workflow-step-agent-start-ownership.md
---

# Task 04: Verify reset recovery surfaces

## Summary

Prove that the existing error notice shows reset failure on desktop and phone.
Document recovery after the implementation exists.

## In scope

- Add desktop reload coverage to the existing error-indicator spec.
- Add a phone spec for the same reset notice, reload, dismissal, and viewport containment.
- Reuse the existing metadata seeding pattern after Task 03 proves production emission.
- Preserve the current notice layout and localized heading.
- Make its dismiss target at least 44 pixels on coarse pointers. Keep desktop dimensions unchanged.
- Update the reset paragraph in the existing public workflow how-to guide.

## Out of scope

A new recovery control, new navigation, automatic restart, broad UI redesign, and changing desktop control size.

## Acceptance

1. Desktop and phone show the reset cause and blocked-step explanation after reload. Phone text wraps without document overflow.
2. Phone dismissal has a target of at least 44 pixels and needs no hover. Dismissal remains effective after reload.
3. Public docs explain that failed reset blocks the step prompt and leaves the error visible. Recovery guidance uses existing session deletion and creation.

## ASCII UI preview

UI-01: Existing conversation error notice after failed reset.
See [combined preview](plan.md#ascii-ui-preview).

```text
Conversation history
+--------------------------------------+
| Previous agent error          [Hide] |
| Context reset failed.                |
| The workflow step prompt did not     |
| start.                               |
+--------------------------------------+
[Composer]
```

The same notice appears in the desktop chat and focused phone conversation.
The phone's existing conversation owns scrolling.
Retain the notice's bounded internal error-text scroll for long details.
Keep its alert role and localized dismissal label.
The existing `LastAgentErrorNotice` is the nearest shipped content exemplar.
Use the mobile control-sizing rule for its currently 32-pixel dismiss target.

The example wording is illustrative.
No new frontend literal bypasses localization.
Any new frontend key requires all five languages and the Traditional Chinese generator.

## Verification

Run from the repository root:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-error-indicator.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-workflow-reset-error.spec.ts)
(cd apps/web && pnpm exec eslint components/task/chat/message-list-shared.tsx)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the desktop and phone E2E commands sequentially with managed production builds.
The backend unit and integration RED gates prove the missing error emission.
Seeded E2E proves rendering and persistence; do not describe it as an end-to-end provider-hang reproduction.
For the touch target, record a geometry RED assertion before changing the control.
Assert the active notice rather than selecting a hidden desktop mount.
Use the existing dismissal API and restore any changed worker-scoped fixture state.

## Files likely touched

- `apps/web/e2e/tests/session/agent-error-indicator.spec.ts`
- `apps/web/e2e/tests/session/mobile-workflow-reset-error.spec.ts` (new)
- `apps/web/e2e/tests/session/workflow-reset-error-helpers.ts` (new, only for shared setup)
- `apps/web/components/task/chat/message-list-shared.tsx`
- `docs/public/tasks-and-workflows.md`
- `docs/plans/workflow-reset-failure-containment/plan.md` and work-order results

## Dependencies

Task 03.

## Risks

A seeded notice does not prove backend workflow emission.
Keep the Task 03 integration evidence with the rendering results.
Do not copy the legacy test's hidden-sidebar `.first()` workaround.
Preserve desktop dimensions when adding coarse-pointer target sizing.

## Parallelism

`sequential`

## Inputs

- [System design](../../specs/tasks/system-design/workflow-step-agent-start-ownership.md#persist-a-visible-failure).
- `apps/web/AGENTS.md`, mobile-parity, e2e, and docs-maintainer skills.
- `LastAgentErrorNotice` in `message-list-shared.tsx`.
- Existing `agent-error-indicator.spec.ts` and `mobile-provider-remediation-link.spec.ts`.

## Results

Verified the existing last-agent-error notice on desktop and phone after a
reload. The coarse-pointer dismiss control is 44 pixels while desktop sizing
is unchanged. Phone coverage verifies touch dismissal, persisted dismissal,
and zero document horizontal overflow.

Updated `docs/public/tasks-and-workflows.md` with the failed-reset behavior
and recovery path through session deletion and new session creation.

The desktop and phone coverage expands the localized technical-details
disclosure after reload and asserts the exact reset cause. The phone test keeps
the disclosure open while measuring horizontal overflow and checks the coarse
pointer dismiss target at 44 by 44 pixels.

Verification passed:

```text
(cd apps/web && pnpm exec vitest run components/task/chat/message-list-shared.test.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/session/agent-error-indicator.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-workflow-reset-error.spec.ts)
(cd apps/web && pnpm exec eslint components/task/chat/message-list-shared.tsx components/task/chat/message-list-shared.test.tsx e2e/tests/session/agent-error-indicator.spec.ts e2e/tests/session/mobile-workflow-reset-error.spec.ts e2e/tests/session/workflow-reset-error-helpers.ts)
(cd apps/web && pnpm run typecheck)
```

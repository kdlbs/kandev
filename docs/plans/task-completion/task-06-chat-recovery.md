---
id: "06-chat-recovery"
title: "Restore completed-chat interaction"
status: done
wave: 6
depends_on: 
  - 05-workflow-editor
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-002
acceptance_criteria:
  - AC-TASKS-COMPLETION-002.1
  - AC-TASKS-COMPLETION-002.2
  - AC-TASKS-COMPLETION-002.3
  - AC-TASKS-COMPLETION-002.4
  - AC-TASKS-COMPLETION-002.5
  - AC-TASKS-COMPLETION-002.7
  - AC-TASKS-COMPLETION-002.9
  - AC-TASKS-COMPLETION-002.10
  - AC-TASKS-COMPLETION-002.11
  - AC-TASKS-COMPLETION-002.12
system_design:
  - ../../specs/tasks/system-design/task-completion.md
---

# Task 06: Restore completed-chat interaction

## Summary

Show Resume beside New Agent and complete follow-up flows in the same chat. Prove the result on desktop and mobile against the guarded backend.

## In scope

- Reuse recovery actions and error feedback in the completed banner. Keep selected session identity through busy/success/failure responses and restore composer only from authoritative state.
- Preserve completed-chat passive-open gates and startup queue behavior. Show missing-profile and workspace-recovery errors without creating unrelated sessions.
- Use primary Resume, secondary New Agent, stacked on phones and compact on desktop. Keep touch targets at 44 px, safe-area access, keyboard focus, and a single transcript scroll owner.
- Add root/child completed history scenarios, live sibling preference, retired non-primary resume, passive reload, failed resume feedback, and a second response in the same conversation.
- Update relevant recovery/help text, public workflow follow-up guidance, and any scoped architecture notes affected by the final implementation.

## Out of scope

No changes to terminal-provider input, Office scheduler authority, New Agent semantics, or FAILED/CANCELLED eligibility.

## Acceptance

- A completed chat shows both actions; Resume followed by Send produces one new turn in the same session with previous context, unchanged task state/step/primary, and no new session.
- Reload/open does not resume; failed actions retain retry feedback and content. Existing recovery and clarification behavior still works.
- Desktop/mobile E2E pass against fresh builds; mobile checks prove touch interaction, containment and no horizontal overflow.

## TDD entry

Add use-session-recovery-actions.test.ts for identity-safe success/failure/busy cleanup, and resumption tests for completed no-auto-start. Add completed-session-resume.spec.ts and mobile-completed-session-resume.spec.ts; observe missing Resume or current backend rejection before implementation.

## Verification

Use the worktree dependency setup recorded below.

```bash
rtk pnpm --filter @kandev/web exec vitest run hooks/domains/session/use-session-recovery-actions.test.ts hooks/domains/session/use-session-resumption.test.ts hooks/domains/session/use-session-resumption.navigation.test.ts hooks/domains/session/session-input-mode.test.ts
rtk pnpm --filter @kandev/web typecheck
rtk pnpm --filter @kandev/web i18n:zh-hant
rtk pnpm --filter @kandev/web i18n:check
```

Run the commands above from apps/. From apps/web/, run sequentially:

```bash
rtk pnpm e2e:run --project chromium tests/session/completed-session-resume.spec.ts tests/session/session-resume-recovery.spec.ts tests/session/session-resume-prompt-queue.spec.ts tests/workflow/workflow-agent-switch.spec.ts -- --retries=0
rtk pnpm e2e:run --project mobile-chrome tests/session/mobile-completed-session-resume.spec.ts tests/session/mobile-session-resume-recovery.spec.ts tests/session/mobile-session-resume-prompt-queue.spec.ts tests/workflow/mobile-workflow-agent-switch.spec.ts -- --retries=0
```

Disable any in-spec retry override in reproductions. Confirm project discovery and test count. Inspect the rendered phone result and record screenshot/trace paths. Rerun changed backend regression tests after any integration correction.

From repository root:

```bash
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```


## Files likely touched

- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/session-stopped-banner.tsx`
- `apps/web/hooks/domains/session/use-session-recovery-actions.ts`
- `apps/web/hooks/domains/session/use-session-recovery-actions.test.ts (new)`
- `apps/web/hooks/domains/session/use-session-resumption.ts`
- `apps/web/hooks/domains/session/use-session-resumption.navigation.test.ts`
- `apps/web/lib/services/session-recovery-service.ts`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/session/completed-session-resume.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-completed-session-resume.spec.ts (new)`
- `docs/public/workflow-tips.md`

## Dependencies

Complete 05-workflow-editor first.

## Risks

Completed root and child conversations may reach this UI through different runtime paths; seed historical COMPLETED rows as well as actual workflow completion. A second text response alone is insufficient: assert session IDs/count, previous messages, provider context, task state, step, primary and no duplicate turn.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/task-completion.md), criteria in frontmatter.
- [System design](../../specs/tasks/system-design/task-completion.md).
- [Plan](plan.md), confirmed source trace and existing reproduction tests.
- Nearby workflow settings and session recovery tests supply fixture conventions.
- Follow `/tdd`, `/mobile-parity`, and `/e2e` in the primary session.

## Results

Completed chats now show Resume beside New Agent. Resume keeps the selected
session and provider conversation, restores the composer only after
authoritative readiness, and leaves task completion, workflow position, and
primary ownership unchanged. Passive open/reload does not start the agent, and
failure feedback keeps the recovery action available.

Final verification passed:

```text
recovery/resumption/input-mode unit gate: 51 tests in 4 files
focused editor/banner/session frontend gate: 75 tests in 8 files
desktop completed-chat E2E: 1 test
mobile completed-chat E2E: 1 test
desktop recovery-neighbor E2E: 23 tests
mobile recovery-neighbor E2E: 4 tests
web typecheck: passed
i18n catalog and pseudo-locale gate: passed
public-doc validation: passed
specification lint: passed
```

The desktop and mobile flows verify touch-sized controls, containment, safe
passive reload behavior, same-session follow-up, prior transcript preservation,
and unchanged completed task state.

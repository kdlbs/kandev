---
id: "02-resume-composer"
title: "Enable Send during resume"
status: done
wave: 2
depends_on:
  - "01-admission-readiness"
plan: "plan.md"
requirements:
  - REQ-TASKS-RESUME-PROMPT-QUEUE-001
acceptance_criteria:
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.1
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.2
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.3
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.4
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.5
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.6
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.7
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.8
  - AC-TASKS-RESUME-PROMPT-QUEUE-001.9
system_design:
  - ../../specs/tasks/system-design/resume-prompt-queue.md
---

# Task 02: Enable Send During Resume

## Summary

Permit submission for an existing queue-capable session during startup. Verify
that the accepted prompt runs after resume on desktop and mobile.

## In scope

- Thread selected-session queue eligibility into the shared composer.
- Keep button, shortcut, and plugin capability gates consistent.
- Preserve drafts and attachments after unsuccessful admission.
- Add unit and rendered E2E regressions with TDD.
- Update the sessions-and-review how-to section during implementation.

## Out of scope

- New layouts, terminal emulation, queue policy, or session lifecycle states.

## Acceptance

1. Send during resume persists the complete payload and clears only an accepted draft.
2. Desktop and mobile prove pending visibility, readiness dispatch, reload persistence, and paused-queue behavior.
3. Missing identity, queue-full, upload, recovery, and other existing blockers preserve content and show applicable feedback.

## Verification

For a fresh worktree, run once from `apps`:

```bash
rtk pnpm install --frozen-lockfile
```

Run from `apps/web`:

```bash
rtk pnpm exec vitest run components/task/chat/use-chat-input-container.test.ts components/task/chat/use-chat-input-state.test.ts components/task/chat/chat-input-body.test.tsx hooks/use-message-handler.test.ts hooks/domains/session/session-input-mode.test.ts hooks/domains/session/use-queue.test.ts
rtk pnpm run typecheck
rtk pnpm run i18n:check
rtk pnpm e2e:run --project chromium e2e/tests/session/session-resume-prompt-queue.spec.ts
rtk pnpm e2e:run --project mobile-chrome e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts
rtk pnpm e2e:run --project chromium e2e/tests/session/session-recovery.spec.ts -- --grep 'session startup keeps the composer' --retries=0
```

Run from the repository root after the public documentation update:

```bash
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
```

The managed E2E runner rebuilds production artifacts. Use its single-project
commands sequentially. Confirm that every requested scenario is discovered.

## Files likely touched

- `apps/web/components/task/chat/use-chat-input-container.ts` and its tests
- `apps/web/components/task/chat/chat-input-container.tsx`
- `apps/web/components/task/chat/chat-input-area.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/hooks/domains/session/use-session-state.ts`
- `apps/web/hooks/use-message-handler.ts` and its tests
- `apps/web/hooks/domains/session/use-queue-admission.ts`
- `apps/web/hooks/domains/session/use-queue.test.ts`
- `apps/web/components/task/chat/use-chat-input-state.test.ts`
- Task and Quick Chat callers of `ChatInputContainer`
- `apps/web/e2e/tests/session/session-recovery.spec.ts`
- `apps/web/e2e/tests/session/session-resume-prompt-queue.spec.ts` (new)
- `apps/web/e2e/tests/session/mobile-session-resume-prompt-queue.spec.ts` (new)
- `apps/web/e2e/helpers/session-resume-prompt-queue.ts` (new)
- Existing mock-agent readiness fixture, only if resume needs a deterministic barrier
- `docs/public/sessions-and-review.md`
- Locale catalogs if existing queue copy does not cover the state

## Dependencies

Task 01.

## Risks

The broad startup flag includes preparation without a usable queue identity.
Admission currently has silent no-op paths. Resume can finish before a queued
request arrives. E2E must hold actual backend readiness to expose this sequence.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/tasks/requirements/resume-prompt-queue.md)
- [Design](../../specs/tasks/system-design/resume-prompt-queue.md), Composer admission and Responsive behavior
- `apps/web/AGENTS.md`
- `e2e/helpers/session-resume-recovery.ts`
- `e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `/tdd`, `/e2e`, `/mobile-parity`, and `/docs-maintainer`

## Results

Implemented after Task 01 completed.

The shared composer now enables Send only when the selected STARTING session has
a complete queue identity. Admission returns an explicit success value, so
failed or unavailable queue operations retain drafts and attachments. Resume
projects STARTING locally while the request is in flight, which keeps the
composer on the queue path during a delayed resume. The backend admission
recheck and existing boot-ready drain provide the two readiness orderings.

The mock agent advertises native session resume and accepts a deterministic
resume delay for the desktop and mobile E2E fixtures. The public sessions guide
documents queued prompts, Auto-run OFF, and resume recovery.

Verification on 2026-09-09:

- Frontend focused Vitest suite: 150 tests passed across 10 files.
- `rtk pnpm run typecheck`: passed.
- `rtk pnpm run i18n:check`: passed; the repository's existing 138 orphan
  catalog warning remains informational.
- Desktop resume/Quick Chat/layout E2E: 3 passed.
- Mobile resume/touch/overflow E2E: 1 passed.
- Existing session-recovery startup E2E: 1 passed.
- Backend focused, mock-agent, and race suites: all passed.
- Public documentation validation: 61 tests and 46 pages passed.

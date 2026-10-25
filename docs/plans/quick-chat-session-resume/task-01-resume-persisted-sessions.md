---
id: "01-resume-persisted-sessions"
title: "Resume Persisted Quick Chat Sessions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-QUICK-CHAT-EXPIRATION-001
acceptance_criteria:
  - AC-TASKS-QUICK-CHAT-EXPIRATION-001.2
system_design:
  - ../../specs/tasks/system-design/quick-chat-session-resumption.md
---

# Task 01: Resume Persisted Quick Chat Sessions

## Summary

Connect the visible persisted Quick Chat conversation to the shared task-session resumption hook.
Add focused component and browser regression evidence for identity fallback, backend-restart
recovery, and restored session models.

## In scope

- Resolve the backing task ID for a persisted Quick Chat from its descriptor or hydrated session
  row.
- Wait for the task-session row before starting resumption, preserve authoritative hydration when a
  placeholder arrives first, and retry hydration after an interrupted tab switch.
- Mount `useSessionResumption` for ordinary, configuration, and passthrough conversation tabs.
- Add component coverage for direct and fallback task identity.
- Add a backend-restart Quick Chat Playwright scenario that proves agent and model recovery.

## Out of scope

- Backend, API, store-shape, persistence, and resumption-policy changes.
- Client-only setup tabs and Quick Terminal PTY lifecycle.
- Visual, responsive, touch, focus, or copy changes.

## Acceptance

- Opening a persisted Quick Chat with a known backing task invokes shared resumption exactly for
  that task/session pair, including when the task ID arrives through session hydration.
- Ordinary, configuration, and passthrough presentations retain their current rendering while the
  shared automatic-start preference controls whether a stopped resumable agent launches.
- After a backend restart, reopening a restored Quick Chat resumes the agent and exposes its dynamic
  session model controls without sending a prompt first.

## Verification

```bash
cd apps/web
pnpm exec vitest run components/quick-chat/quick-chat-session-view.test.tsx hooks/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts
pnpm run typecheck
pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "resumes a restored session after backend restart" --retries=0
```

All verification commands pass. The component and hydration suites cover descriptor precedence,
hydrated-row task identity, placeholder merging, and tab-switch retry. The typecheck passes, and the
production-build restart E2E confirms resumed runtime evidence and restored dynamic model controls.

## Files likely touched

- `apps/web/components/quick-chat/quick-chat-session-view.tsx`
- `apps/web/components/quick-chat/quick-chat-session-view.test.tsx`
- `apps/web/e2e/tests/chat/quick-chat.spec.ts`

## Dependencies

None.

## Risks

- Mount the hook before presentation selection. A later placement violates React hook ordering and
  omits passthrough recovery.
- Do not start resumption from a descriptor task ID until the task-session row is present; otherwise
  the status response can replace the authoritative hydration request with a placeholder row.
- A test that waits only for transcript text can race the runtime state transition. Use backend
  session state and resumed boot or capability evidence.

## Parallelism

`sequential`

## Inputs

- `AC-TASKS-QUICK-CHAT-EXPIRATION-001.2`
- `docs/specs/tasks/system-design/quick-chat-session-resumption.md`
- Existing task-page and kanban-preview `useSessionResumption` integrations.
- Existing Quick Chat dynamic-model and session-resume E2E patterns.

## Results

- `pnpm exec vitest run components/quick-chat/quick-chat-session-view.test.tsx`: 4 tests passed.
- `pnpm exec vitest run components/quick-chat/quick-chat-session-view.test.tsx hooks/use-ensure-task-session.test.ts hooks/domains/session/use-session-resumption.test.ts`: 28 tests passed.
- `pnpm run typecheck`: passed.
- `pnpm e2e:run tests/chat/quick-chat.spec.ts -- --grep "resumes a restored session after backend restart" --retries=0`: 1 test passed against the production Vite build.

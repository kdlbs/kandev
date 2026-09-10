---
id: 09-render-workflow-scripts-in-chat
title: Render workflow scripts in chat
status: done
wave: 7
depends_on:
  - 04-integrate-workflow-triggers
plan: plan.md
requirements:
  - REQ-TASKS-WORKFLOW-STEP-SCRIPT-005
acceptance_criteria:
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.2
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.3
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.4
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.5
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.6
  - AC-TASKS-WORKFLOW-STEP-SCRIPT-005.7
system_design:
  - ../../specs/tasks/system-design/workflow-step-script-actions.md
---

# Task 09: Render workflow scripts in chat

## Summary

Extend the existing script execution message to show workflow lifecycle context,
live output, and durable terminal evidence in normal agent chat.

## In scope

- Workflow step/trigger header, command, policy, timeout, output, status,
  duration, exit code, interruption, and truncation states.
- In-place WebSocket updates, reload/reconnect behavior, and missing-metadata
  fallback.
- Normal transcript filtering, turn-count exclusion, and lifecycle-only message
  persistence for workflow scripts.
- Recovery lookup by stable process-request identity without starting a
  replacement command.
- Desktop/mobile expansion, wrapping, accessibility, and localization.

## Out of scope

- Editor authoring and environment preparation presentation changes.

## Acceptance

1. One chronological message streams from starting to a correct terminal state
   and remains identical after reload or reconnect.
2. Workflow scripts stay out of preparation UI and never count as prompts,
   replies, completed turns, or workflow completion signals. A replay or
   recovery path updates the existing lifecycle message rather than creating a
   completed turn.
3. Every status and missing-field case is accessible on desktop/mobile without
   horizontal document overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps && pnpm --filter @kandev/web test -- --run components/task/chat/messages/script-execution-message hooks/processed-message-filtering)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web lint)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
git diff --check
```

## Files likely touched

- `apps/web/components/task/chat/messages/script-execution-message.tsx`
- `apps/web/components/task/chat/messages/script-execution-message.test.tsx`
- `apps/web/components/task/chat/message-renderer.tsx`
- `apps/web/hooks/processed-message-filtering.ts`
- `apps/web/lib/types/http.ts`
- Task locale catalogs.

## Dependencies

- Task 04 supplies persistent workflow script message metadata and updates.

## Risks

- Existing metadata assumptions may classify interruption as success when exit
  code is absent.
- Output updates can create duplicate rows if identity is not stable.

## Parallelism

`parallel-safe` with Task 08. This task owns task transcript and task locale
keys; Task 08 owns workflow editor mobile files and workflow locale keys.

## Inputs

- Existing setup, cleanup, and agent-boot `ScriptExecutionMessage` variants.
- Existing task message update and hydration behavior.

## Results

Implemented workflow lifecycle context, live and terminal output projection,
duration and interruption metadata, truncation display, normal transcript
filtering, lifecycle-only message persistence, request-identity recovery, and
terminal success handling. Focused chat and filtering tests pass.

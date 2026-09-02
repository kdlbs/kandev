---
id: "03-record-successful-launches"
title: "Record successful profile launches"
status: done
wave: 3
depends_on:
  - "02-apply-selector-ordering"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-002
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-001.1
  - AC-AGENTS-PROFILE-RECENT-USE-002.1
  - AC-AGENTS-PROFILE-RECENT-USE-002.2
  - AC-AGENTS-PROFILE-RECENT-USE-002.3
  - AC-AGENTS-PROFILE-RECENT-USE-002.4
  - AC-AGENTS-PROFILE-RECENT-USE-002.5
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 03: Record Successful Profile Launches

## Summary

Connect successful operational launch boundaries to the focused recency
mutation. Record the effective profile only after success and supersession
checks, without delaying or changing the primary launch outcome.

## In scope

- One best-effort frontend recording helper that applies authoritative
  responses to the store.
- Successful task, subtask, task-session, handoff, quick-chat, and
  configuration-chat integration points.
- Effective-profile fallback where backend responses can override submitted
  IDs.
- Tests for success, failure, cancellation, no-selection side effects, and
  quick/config-chat supersession.

## Out of scope

- Retrying through browser storage or surfacing recency-save errors to users.
- Changing task/session/chat API success semantics.
- Recording selector changes before launch completion.

## Acceptance

- Each completed operational launch records exactly one effective profile in
  its context.
- Cancelled, failed, or superseded operations issue no record mutation.
- Recency I/O is not awaited by navigation, dialog close, or successful launch
  completion, and its failure has no user-visible launch error.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-submit.test.tsx components/task/use-subtask-submit.test.ts components/task/new-session-form-actions.test.ts components/quick-chat/use-quick-chat-modal.test.ts components/config-chat/use-config-chat.test.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/agent-profile-recent-use.ts`
- `apps/web/components/task-create-dialog-submit.tsx`
- `apps/web/components/task/use-subtask-submit.ts`
- `apps/web/components/task/new-session-form-actions.ts`
- `apps/web/components/quick-chat/use-quick-chat-modal.ts`
- `apps/web/components/config-chat/use-config-chat.ts`
- Corresponding focused test files.

## Dependencies

- Task 02 provides the API helper, typed contexts, and frontend state action.

## Risks

- Task creation has multiple success paths; every path must record once without
  treating edit mode as new use.
- Quick-chat and configuration-chat can complete requests that are later
  discarded as superseded; recording must remain after winner selection.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-PROFILE-RECENT-USE-001`
- `REQ-AGENTS-PROFILE-RECENT-USE-002`
- Recording flow and failure sections in the system design.
- Existing task-create last-used and ephemeral-chat supersession patterns.

## Results

Connected task creation, subtask creation, task sessions, handoff/session
launches, quick chat, and configuration chat to best-effort success-only
recording with effective profile IDs and supersession checks. Deferred
task-create attribution is explicit and protected from MCP profile input and
generic task metadata replacement. Verified with:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task-create-dialog-submit.test.tsx components/task/use-subtask-submit.test.ts components/task/new-session-form-actions.test.ts components/quick-chat/use-quick-chat-modal.test.ts components/config-chat/use-config-chat.test.ts
cd apps/web && pnpm run typecheck
```

Result: 73 frontend tests passed and typecheck passed.

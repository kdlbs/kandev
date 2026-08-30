---
id: "02-apply-selector-ordering"
title: "Apply contextual selector ordering"
status: done
wave: 2
depends_on:
  - "01-persist-profile-recency"
plan: "plan.md"
requirements:
  - REQ-AGENTS-PROFILE-RECENT-USE-001
  - REQ-AGENTS-PROFILE-RECENT-USE-003
acceptance_criteria:
  - AC-AGENTS-PROFILE-RECENT-USE-001.2
  - AC-AGENTS-PROFILE-RECENT-USE-001.3
  - AC-AGENTS-PROFILE-RECENT-USE-001.4
  - AC-AGENTS-PROFILE-RECENT-USE-001.5
  - AC-AGENTS-PROFILE-RECENT-USE-003.3
  - AC-AGENTS-PROFILE-RECENT-USE-003.5
system_design:
  - ../../specs/agents/system-design/profile-recent-use.md
---

# Task 02: Apply Contextual Selector Ordering

## Summary

Hydrate recent-use records into independent frontend state and apply one stable
ordering helper to operational profile selectors. Keep defaults, eligibility,
search, selected-first behavior, and context-free selectors unchanged.

## In scope

- Frontend wire types, API client, store state/actions, boot hydration, and
  revision-aware WebSocket handling.
- Pure stable ordering helper with at most ten ranked IDs.
- Explicit context wiring for task create, subtask, task-session, handoff,
  quick-chat, and configuration-chat selectors.
- Unit/component coverage for hydration, stale revisions, stable ordering,
  eligibility, selected-first composition, and context-free callers.

## Out of scope

- Recording successful launches.
- Selector markup, copy, overlay type, layout, touch behavior, or breakpoints.
- Reordering automation, defaults, settings, or Office assignment selectors.

## Acceptance

- Every operational selector uses its declared context and preserves source
  order for eligible unseen profiles.
- Stale, unknown, and ineligible remembered IDs never render, while selected
  options and existing default behavior remain unchanged.
- Boot, API, and WebSocket records update only newer context revisions, with no
  use of browser persistence or `userSettings` state.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/agent-profile-recent-use.test.ts lib/state/slices/settings/agent-profile-recent-use.test.ts lib/ws/handlers/users.test.ts components/task-create-dialog-options.test.tsx components/config-chat/config-chat-setup.test.tsx components/quick-chat/quick-chat-setup.test.tsx components/task/new-session-dialog.test.tsx
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/types/backend.ts`
- `apps/web/lib/types/http-agent-profile-recent-use.ts`
- `apps/web/lib/api/domains/settings-api.ts`
- `apps/web/lib/state/slices/settings/types.ts`
- `apps/web/lib/state/slices/settings/settings-slice.ts`
- `apps/web/lib/state/default-state.ts`
- `apps/web/lib/state/hydration/hydrator.ts`
- `apps/web/lib/ws/handlers/users.ts`
- `apps/web/lib/agent-profile-recent-use.ts`
- `apps/web/components/task-create-dialog-options.tsx`
- `apps/web/components/task-create-dialog-computed.ts`
- `apps/web/components/task/new-subtask-dialog.tsx`
- `apps/web/components/task/new-session-dialog.tsx`
- `apps/web/components/quick-chat/quick-chat-setup.tsx`
- `apps/web/components/config-chat/config-chat-setup.tsx`

## Dependencies

- Task 01 provides the boot, HTTP, and WebSocket contracts.

## Risks

- `useAgentProfileOptions` has both operational and configuration consumers;
  the optional context must not become an implicit global order.
- The existing combobox performs selected-first ordering after it receives
  options; recent-use tests must assert the composed order rather than replace
  that behavior.

## Parallelism

`sequential`

## Inputs

- `REQ-AGENTS-PROFILE-RECENT-USE-001`
- `REQ-AGENTS-PROFILE-RECENT-USE-003`
- Frontend ordering and synchronization sections in the system design.
- Existing selector-options and user-settings hydration/event patterns.

## Results

Implemented the independent frontend recency state, boot and WebSocket
hydration, revision-aware merging, stable bounded ordering helper, and explicit
contexts for operational selectors. Verified with:

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/agent-profile-recent-use.test.ts lib/state/slices/settings/agent-profile-recent-use.test.ts lib/ws/handlers/users.test.ts components/task-create-dialog-options.test.tsx components/config-chat/config-chat-setup.test.tsx components/quick-chat/quick-chat-setup.test.tsx components/task/new-session-dialog.test.tsx
cd apps/web && pnpm run typecheck
```

Result: 70 frontend tests passed and typecheck passed.

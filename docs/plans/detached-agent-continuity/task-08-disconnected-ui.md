---
id: "08-disconnected-ui"
title: "Disconnected UI and notices"
status: pending
wave: 3
depends_on: ["03-disconnected-link-state", "04-reconnect-coordinator"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-006
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-001
  - REQ-PLATFORM-DETACHED-AGENT-CONTINUITY-002
acceptance_criteria:
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.1
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.2
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.3
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-006.6
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-001.5
  - AC-PLATFORM-DETACHED-AGENT-CONTINUITY-002.3
system_design:
  - ../../specs/platform/system-design/detached-agent-continuity-01.md
---

# Task 08: Disconnected UI and notices

## Summary

The chat shows a Disconnected banner with Reconnect and Stop on desktop and
phone. The task card and remote status show the disconnection. The state
survives a reload, and the reconnect notices render in the conversation.

## In scope

- **`SessionAgentctlStatus.status`** gains `"disconnected"` and
  `"stopped_pending_cleanup"`, with `since`, `host`, `budgetDeadline`,
  `nextAttemptAt`, `lastError`, `linkGeneration`, and `linkRevision`. It is
  set by `session.agentctl_disconnected` and the link payload on
  `session.agentctl_ready` in `lib/ws/handlers/agent-session.ts`, and seeded
  from `agent_link` metadata on load. A payload whose `linkRevision` is not
  newer than the stored one is dropped.
- **Banner selection** follows the design table: `disconnected` shows
  `DisconnectedSessionBanner`; `stopped_pending_cleanup` shows the existing
  `SessionStoppedBanner` with a cleanup line (test ID
  `stopped-pending-cleanup-line`) and no Reconnect.
- **`DisconnectedSessionBanner`:**
  - rendered from `chat-input-container.tsx`;
  - the pause time is worded as approximate;
  - `lastError` renders through the `agentLinkError*` keys;
  - the phone gets it through the shared `ChatInputArea`;
  - test IDs are `disconnected-session-banner`,
    `disconnected-reconnect-button`, and `disconnected-stop-button`;
  - Reconnect sends `session.reconnect`.
- **Indicators:** `RemoteCloudTooltip` and the kanban status icon show the
  disconnected state.
- **Copy:** the `task:` keys named in the system design, in all six locales.
  Run `pnpm run i18n:zh-hant` for the Traditional pair. Use no em dash.
- **Notices:** conversation notices render for the three status message kinds
  from task 04.

## Out of scope

- The backend event and metadata (task 03).
- The notices' creation (task 04).

## Acceptance

1. A seeded Disconnected session shows UI-01 on desktop and UI-02 on a phone
   without horizontal overflow. Reconnect sends `session.reconnect`, and Stop
   stops the session. After Stop, the stopped banner shows the cleanup line
   and no Reconnect action.
2. The card and tooltip show UI-04. A reload keeps the banner.
3. The UI-03 notices render. The banner clears within 5 s of
   `session.agentctl_ready`. An older link payload arriving after a newer
   one changes nothing.

## ASCII UI preview

The previews are UI-01, UI-02, UI-03 and UI-04 in [plan.md](plan.md#ascii-ui-preview).

```text
UI-01 excerpt
| [!] Disconnected from neo since 02:00                                |
|     It pauses at 02:15 if still disconnected.                        |
|                                          [ Reconnect ]  [ Stop ]     |
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
(cd apps/web && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm exec vitest run lib/ws/handlers/agent-session.test.ts components/task/chat)
(cd apps/web && pnpm e2e:run --project chromium tests/session/detached-agent-continuity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-detached-agent-continuity.spec.ts)
```

## Files likely touched

- `apps/web/lib/state/slices/session/types.ts`, `session-slice.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- New `apps/web/components/task/chat/disconnected-session-banner.tsx`, plus
  `chat-input-container.tsx`
- `apps/web/components/task/remote-cloud-tooltip.tsx`,
  `apps/web/components/kanban-card-status-icon.tsx`
- `apps/web/src/locales/*/task.json`
- New `apps/web/e2e/tests/session/detached-agent-continuity.spec.ts` and the
  `mobile-` variant; a seeding helper in `e2e/helpers/api-client.ts`

## Dependencies

- Tasks 03 and 04.

## Risks

- **Banner precedence.** The banner can collide with #3598's uncertain
  banner. Disconnected wins while the link is down; uncertain applies only
  after a reconnect outcome.

## Parallelism

`parallel-safe` with tasks 05 and 06.

## Inputs

- System design section: Frontend.
- The plan's ASCII previews.
- `apps/web/AGENTS.md`.

## Results

Pending.

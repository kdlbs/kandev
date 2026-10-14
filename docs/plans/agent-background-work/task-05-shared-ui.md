---
id: "05-shared-ui"
title: "Shared desktop and phone background-work UI"
status: completed
wave: 5
depends_on:
  - "04-controls-and-usage"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-003
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-001.1
  - AC-AGENTS-BACKGROUND-WORK-001.2
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-002.5
  - AC-AGENTS-BACKGROUND-WORK-003.1
  - AC-AGENTS-BACKGROUND-WORK-003.2
  - AC-AGENTS-BACKGROUND-WORK-003.3
  - AC-AGENTS-BACKGROUND-WORK-003.4
  - AC-AGENTS-BACKGROUND-WORK-004.1
  - AC-AGENTS-BACKGROUND-WORK-004.2
  - AC-AGENTS-BACKGROUND-WORK-004.3
  - AC-AGENTS-BACKGROUND-WORK-004.4
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-005.2
  - AC-AGENTS-BACKGROUND-WORK-005.3
  - AC-AGENTS-BACKGROUND-WORK-005.4
  - AC-AGENTS-BACKGROUND-WORK-005.5
  - AC-AGENTS-BACKGROUND-WORK-006.2
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 05: Shared desktop and phone background-work UI

## Summary

Build one capability-driven composer chip, workload browser and detail view for all adapters. Add normalized API/store/event handling and preserve existing transcript, draft, prompt-admission and terminal behavior.

## In scope

- Domain API, shared hook, normalized store slice, session-scoped subscriptions and revision/request-epoch reconciliation. Test late list responses, gaps, reset/delete, route switching and unknown fields.
- Composer chip and persistent history entry; central-group overview and dedicated job/agent Dockview tabs, plus phone summary Drawer to full-height list/detail transitions. Reuse message and ANSI-safe shell output presentation; no provider-name branches.
- Conditional controls and pending/uncertain/rejected feedback. Input is sent only on explicit submission, never on reconnect; drafts survive action errors. Connect child permission references to existing flows.
- Dockview renderer/type/title registration, explicit center-group targeting, one overview per session/incarnation and one detail tab per workload, independent view state, same-workload deduplication, layout restore and task/environment/reset/deletion cleanup. Quick Chat opens the owning task panel and preserves its draft; no desktop side sheet. Add `dockview-background-work-panel.test.ts` for placement, two simultaneous workloads, duplicate-name isolation, same-workload focus, moved-panel reuse, independent close, stale identity and close-without-stop.
- Read-only ledger usage with provenance, empty/loading/unknown/unsupported/truncated states, output auto-scroll/search/local-clear/reset, focus return and phone keyboard clearance.
- Integrate Task chat and shared Quick Chat composer where supported; preserve passthrough terminals. Do not change Send/queue/steer admission based on visible work.
- All six language catalogs plus pseudo; generate Traditional Chinese. Component, API, hook/store and desktop/phone E2E tests use normalized backend fixtures for both Codex and ACP shapes.

## Out of scope

New native providers, generic scheduling, host PID control, and production changes owned by later work orders. Preserve the design's explicit exclusions.

## Acceptance

1. Store/API tests prove versioned merge, deletion fences and receipt reuse; capability permutations work without agent-ID branches.
2. Desktop and phone E2E verify compact pill placement, central-group overview plus two independent workload tabs, repeat-click focus, duplicate-name isolation, close-without-stop, preserved drafts, phone direct-detail navigation, controls, completed history and backend-to-browser events.
3. Phone details use one scroll owner, at least 44px action targets even with a narrow fine pointer, safe-area/keyboard clearance, focus return and zero document overflow; i18n gates pass.

## ASCII UI preview


### UI-01: Compact composer pill and dedicated central tabs

Entry: the small pill sits immediately above the chat input. Click or keyboard
activation opens a Popover. Select an agent/job to open its detail directly in
its own named tab in the central group; View all opens the separate overview.

```text
   ( 2 Background Jobs )
+----------------------------------------------------------+
| Message draft...                                         |
|                                             [Send/Queue] |
+----------------------------------------------------------+

+ Background jobs ----------------------------+
| Build watcher       Running          [Open] |
| Review auth         Running          [Open] |
| [View all work]                             |
+--------------------------------------------+

Central group after opening the overview and two workloads:
+------+-----------------+-------------------+-----------------+
| Chat | Background Work | Build Watcher [x] | Review Auth [x] |
+------+-----------------+-------------------+-----------------+
| [All work]  Build watcher     Running             [Stop] |
| [Search retained output...]                              |
|                                                          |
| PASS auth.test.ts                                        |
| Watching for changes...                                  |
|                                                          |
| One scrolling output area                                |
+----------------------------------------------------------+
| [Auto-scroll: on] [Clear view] [Reset view]                |
| [Input ...] [Send input]   only if supported               |
+----------------------------------------------------------+
```

The pill is content-width, not a banner; it displays one count including running,
waiting and unknown jobs, with the breakdown in the summary. A Background work
chat action opens completed history when the active pill disappears. Initially
open each panel in the central group. One overview per session/incarnation and
one detail tab per session/incarnation/workload. Reopening a workload focuses its
existing tab in its user-chosen position. Distinct workloads keep independent
scroll/search/input draft and inspected-run state, even when titles match.
Closing any tab only closes that view; other tabs and jobs stay open/running.
Completed detail tabs remain inspectable. Returning to Chat preserves its draft.
Ordinary desktop controls remain 28px; the pill is deliberately compact.

### UI-02: Phone summary and full-height detail

Entry: the same small pill has a touch-sized hit area. Tap opens an inset summary
Drawer. Tap a job to open full-height detail directly, or View all for the list;
close the summary before transitioning. Back in detail returns to the list.

```text
 ( 2 Background Jobs )
+-----------------------------------+
| Message draft...           [Send] |
+-----------------------------------+

+ Background jobs ------------------+
| Build watcher       Running   [>] |
| Review auth         Running   [>] |
| [View all work]           [Close] |
+-----------------------------------+

+ [Back] Build watcher ---- [Close] +
| Running                          |
| [Search output...]               |
|                                  |
| One scrolling content body       |
|                                  |
| [Earlier output truncated]       |
+----------------------------------+
| [Stop] [Auto-scroll]              |
| [Input...] [Send] if supported    |
+------ keyboard / safe area -------+
```

Header and conditional action/input region stay fixed; content owns vertical
scroll. Use dynamic viewport height and keyboard-aware clearance, not stacked
sheets. Phone/coarse-pointer hit targets are at least 44px even though the pill
looks small. Close returns to chat/invoker; Back returns to the selected list
row. Existing inline transcript cards remain available on both viewports.

### UI-03: Honest failure and capability states

```text
Loading:       [Loading background work...]
Empty:         [No background work observed]
Disconnected:  build  Unknown [Stop disabled: reconnecting]
Unsupported:   [Live output unavailable] [Retained output]
Uncertain:     [Input delivery unknown. Check output before sending again.]
Usage:         [Tokens: 12k estimated] [Cost unavailable]
History:       build  Ended (outcome unavailable) [Open]
```

Structure/navigation/capability gating, one scroll owner and responsive targets
are required; spacing and example text are illustrative. No Restart/Re-run/Steer
control is included. These previews cover AC-AGENTS-BACKGROUND-WORK-001.2,
002.5, 003.1-.4, 004.1-.4 and 005.1-.5 through Task 05's rendered tests.

Full preview: [plan](plan.md#ascii-ui-preview).

## TDD and verification

Write the named behavioral tests first and observe the relevant failure before implementation. Proposed file/test names below are implementation targets, not existing coverage. Run from repository root; fresh worktrees first install with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/web && pnpm test -- lib/state/dockview-background-work-panel.test.ts lib/state/slices/session-runtime/background-work.test.ts lib/api/domains/background-work-api.test.ts hooks/domains/session/use-background-work.test.ts components/task/chat/background-work/background-work.test.tsx)
(cd apps/web && pnpm test -- components/task/chat/messages/tool-subagent-message.test.tsx)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/background-work.spec.ts tests/chat/subagent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-background-work.spec.ts tests/chat/mobile-subagent.spec.ts)
git diff --check
```

## Files likely touched

- Proposed: `apps/web/lib/types/background-work.ts`; `lib/api/domains/background-work-api.ts`; `lib/state/slices/session-runtime/background-work.ts`; `lib/ws/handlers/background-work.ts`; `hooks/domains/session/use-background-work.ts`, with focused tests.
- Proposed: `apps/web/components/task/chat/background-work/` shared compact pill, summary, Dockview overview/detail panels, phone surface and views; `background-work.test.tsx`.
- Existing: chat composer integration, actions, normalized message renderers, usage API/DTO consumers, session reset/removal and Quick Chat shared entry points.
- Existing: `apps/web/lib/state/dockview-extra-panel-actions.ts`, `dockview-store.ts`, layout-manager persistence/reconciliation, `apps/web/components/task/dockview-shared.tsx`, `dockview-panel-content.tsx`; proposed `apps/web/lib/state/dockview-background-work-panel.test.ts`.
- Existing locale catalogs; proposed `apps/web/e2e/tests/chat/background-work.spec.ts`, `mobile-background-work.spec.ts` and fixture helpers.

## Dependencies

Task 04: Authorized controls and attributed usage.

## Risks

UI count changes must not alter admission. Interactive input can be obscured by phone keyboards; snapshot replacement can erase newer events.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/background-work.md), criteria listed in frontmatter.
- [System design](../../specs/agents/system-design/background-work.md), corresponding mapped sections.
- [ADR](../../decisions/2026-09-27-provider-neutral-background-work.md).
- [Plan baseline and existing test patterns](plan.md#baseline-and-integration-points).

## Results

Completed on 2026-09-27. Implemented BackgroundWorkChip, BackgroundWorkPanel, useBackgroundWork hook, background work WebSocket handlers, and dockview panel integration. Verified with focused vitest suites and full i18n checks.

---
id: "04-scope-visible-session-demand"
title: "Activate detail data only for visible chat panels"
status: complete
wave: 4
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.3
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.4
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.5
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.12
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.13
  - AC-PLATFORM-BOUNDED-TASK-STATUS-DELIVERY-001.14
system_design:
  - ../../specs/platform/system-design/journey-data-loading.md
---

# Task 04: Activate detail data only for visible chat panels

## Summary

Eight sibling tabs and one visible desktop chat own one rich session stream. Two visible split chats own two streams; phone owns one; sidebar-only tasks own none.

## In scope

Separate `detailActive` from read acknowledgement and keyboard focus. Use actual group visibility for Dockview chat panels.
Gate rich subscriptions, session polling, transcript/turn loading, MCP/model/usage reads, and their timers while retaining drafts and tab state. Explicitly visible previews remain active even when they suppress read acknowledgement.
Use existing compact lifecycle and pending-action events for hidden tabs. Preserve shared subscription reference counts, split views, restore, task switches, and phone behavior.

## Out of scope

Other work orders, live installation mutation, unrelated refactors, pool increases, new runtime flags, and deployment.

## Acceptance

- Eight sibling tabs and one visible desktop chat own one rich session stream. Two visible split chats own two streams; phone owns one; sidebar-only tasks own none.
- Hidden tabs keep compact badges and drafts. Reveal/reconnect reconciles missed detail once. Obsolete task/panel demand releases without cancelling another visible consumer.
- Rendered tests cover visible previews, keyboard tab selection, split/merge, task navigation, permission events, and desktop-phone-desktop draft preservation.

## ASCII UI preview

[UI-02: full composition and states](plan.md#ascii-ui-preview). Applies to the acceptance IDs in this work order.

```text
UI-02 desktop: [Agent A | Agent B | Agent C] [optional visible split]
               [Visible chat + draft]        [Visible chat + draft]
UI-02 phone:   [Task / session picker]
               [One chat scroll region]
               [Composer + retained draft]
```

Existing controls and scroll ownership remain. Detail demand follows visibility, not mounting or keyboard focus. Preview spacing is illustrative.

## Verification

Use TDD. New test filenames and named methods below are planned deliverables, not existing passing evidence.
Run from the repository root. Install `apps/` dependencies first only if absent. Each command is independently rooted.

```bash
(cd apps/web && pnpm exec vitest run components/task/visible-session-demand.test.tsx components/task/dockview-session-tabs.test.ts components/task/dockview-session-tabs.hook.test.tsx hooks/domains/session/use-session-messages.test.ts lib/ws/handlers/session-pending-action.test.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/session/visible-session-demand.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-visible-session-demand.spec.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/task/dockview-panel-content.tsx components/task/dockview-session-tabs.ts components/task/task-chat-panel.tsx components/task/chat/use-chat-panel-state.ts hooks/use-panel-active.ts hooks/domains/session/use-session.ts hooks/domains/session/use-session-messages.ts components/task/visible-session-demand.test.tsx e2e/tests/session/visible-session-demand.spec.ts e2e/tests/session/mobile-visible-session-demand.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the web typecheck and targeted ESLint for every changed TS/TSX file after the listed tests. If localized copy changes, run `pnpm run i18n:check` and `pnpm run i18n:ratchet` from `apps/web`. Managed E2E commands build fresh assets and enforce resource limits.

## Files likely touched

- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-session-tabs.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/chat/use-chat-panel-state.ts`
- `apps/web/hooks/use-panel-active.ts (reuse existing visibility signal)`
- `apps/web/hooks/domains/session/use-session.ts`
- `apps/web/hooks/domains/session/use-session-messages.ts`
- `apps/web/components/task/visible-session-demand.test.tsx (new)`
- `apps/web/e2e/tests/session/visible-session-demand.spec.ts (new)`
- `apps/web/e2e/tests/session/mobile-visible-session-demand.spec.ts (new)`

## Dependencies

None. Execute in plan order by default.

## Risks

Active keyboard focus differs from visible split panels. Unmounting the whole chat can discard local drafts. Preview visibility currently has unread semantics.

## Parallelism

`sequential`

## Inputs

- [Plan, contract inventory, and test mapping](plan.md).
- [Journey loading design](../../specs/platform/system-design/journey-data-loading.md).
- [Measured baseline](evidence.md) and `evidence/` artifacts.
- Read the owned source and nearby tests before the first edit. Preserve existing user changes.

## Results

Complete. Hidden Dockview chats now release all detail consumers, including the utility-agent git-status hook; visible previews/splits remain demand-driven. Session-tab selection accepts a session present in the current compact task membership before its environment mapping arrives, while cross-task and removed-session guards remain in place.

The new compact-membership regression failed before the fix (`null` instead of the active-task/session pair) and passed after it. The prescribed five-file Vitest suite passed (81 tests), focused visibility/utility/git-status tests passed (49 tests), and targeted ESLint passed. Managed production-build Chromium and mobile-Chrome tests both passed. The Chromium journey observed one rich stream across eight sibling tabs, switched streams with the keyboard, and retained its draft through desktop-phone-desktop resizing. Full web typecheck and `git diff --check` passed. Full verification is repeated after the remaining work orders.

### Review correction

The final review suite repeats desktop split-tab visibility and phone session-switcher tests with the corrected route/read owners. Rich subscriptions remain bounded by visible chats. See the [review correction results](implementation-evidence.md#review-remediation).

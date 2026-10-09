---
id: "05-show-activity"
title: "Show automation activity on shortcut icons"
status: done
wave: 5
depends_on:
  - "04-render-navigation"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-CUSTOMIZATION-003
  - REQ-UI-SIDEBAR-CUSTOMIZATION-005
acceptance_criteria:
  - AC-UI-SIDEBAR-CUSTOMIZATION-003.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-003.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-003.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-003.4
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.1
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.2
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.3
  - AC-UI-SIDEBAR-CUSTOMIZATION-005.4
system_design:
  - ../../specs/ui/system-design/sidebar-customization.md
---

# Task 05: Show automation activity on shortcut icons

## Summary

Add current automation state bubbles to header icons and expanded entries. Keep state fresh for folded groups, overflow menus, and the collapsed rail.

## In scope

- Shared workspace-scoped summary controller, readiness/error state, idle polling, reconnect/visibility refresh, and late-response protection.
- Bubble geometry, accessible state labels, aggregate hidden-running markers, and documentation of status meanings.

## Out of scope

- Quick Chat unread changes, invented automation lifecycle states, and direct Run automation actions.

## Acceptance

- Running, idle, and paused match automationState; loading/failure never renders idle.
- One shared refresh loop updates visible pins every ten seconds, including folded sections, and stops for hidden consumers.
- Desktop and phone tests prove start/finish transitions, error recovery, aggregation, and workspace isolation.

## ASCII UI preview

See the [combined previews](plan.md#ascii-ui-preview). These views implement the
acceptance criteria listed above.

### UI-07: Activity in folded, expanded, and overflow states

```text
> Shortcuts                 [GH][S][C][A*]

v Shortcuts                 [GH][S][C][A*]
    GitHub
    Slack
    My canvas
    My automation                  Running *

> Shortcuts                 [GH][S][...*]
Rail: [Group*]
Phone:
> Shortcuts
  [GH]    [S]    [C]    [A*]
```

`*` represents an overlaid bubble, not a literal glyph. Running is blue,
idle green, and paused muted. Unknown/loading/error has a distinct neutral
indicator and explanatory text. Overflow and rail launchers aggregate hidden
running activity. No bubble click target is added; the parent opens the shortcut.

## Verification

Run from the repository root. Install dependencies once with
`(cd apps && pnpm install --frozen-lockfile)` if this worktree has no install.
Use TDD for new logic and the named E2E scenarios. Managed E2E commands rebuild
assets; run desktop and phone commands sequentially without worker overrides.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/sidebar/use-shortcut-activity.test.ts components/app-sidebar/shortcut-section.test.tsx components/runs/automation-rows.test.ts)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/layout/sidebar-shortcut-activity.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/layout/mobile-sidebar-shortcut-activity.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/hooks/domains/sidebar/use-shortcut-activity.ts (new)`
- `apps/web/components/runs/use-automation-summaries.ts`
- `apps/web/components/runs/use-live-refresh.ts`
- `apps/web/components/runs/automation-rows.ts (reuse semantics)`
- `apps/web/components/quick-chat/quick-chat-activity-indicator.tsx (presentation reuse only)`
- `apps/web/components/app-sidebar/shortcut-section.tsx`
- `apps/web/e2e/tests/layout/*sidebar-shortcut-activity.spec.ts (new)`
- `docs/public/use-kandev.md`

## Dependencies

04-render-navigation.

## Risks

An unloaded empty summary looks like idle in existing derivation. Gate by readiness and do not use unread-chat semantics for automation activity.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-customization.md), IDs above.
- [System design](../../specs/ui/system-design/sidebar-customization.md), corresponding sections.
- [Plan](plan.md), test matrix and previews.
- Scoped `apps/web/AGENTS.md` and `apps/backend/AGENTS.md` where applicable.

## Results

Implemented shared automation summary activity for shortcut headers, expanded
rows, overflow, and phone surfaces. Running, idle, paused, loading, and error
states use the existing automation semantics; folded groups retain activity
markers and refreshes are gated by visible consumers. Activity, shortcut
section, automation-row, canvas-readiness, and inactive-surface tests passed, as
did typecheck and the i18n checks. The desktop customization E2E also verified
an idle pinned automation bubble changing to running after a seeded run.

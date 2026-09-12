---
id: "03-extend-task-panel-capabilities"
title: "Extend generic task panel capabilities"
status: done
wave: 2
depends_on:
  - "01-publish-browser-conversation-contract"
plan: "plan.md"
spec: "../../specs/plugins/requirements/prompt-history-extraction-host.md"
requirements:
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-001
  - REQ-PLUGINS-PROMPT-HISTORY-HOST-003
acceptance_criteria:
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.4
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.5
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.6
  - AC-PLUGINS-PROMPT-HISTORY-HOST-001.7
  - AC-PLUGINS-PROMPT-HISTORY-HOST-003.1
  - AC-PLUGINS-PROMPT-HISTORY-HOST-003.2
  - AC-PLUGINS-PROMPT-HISTORY-HOST-003.3
  - AC-PLUGINS-PROMPT-HISTORY-HOST-003.4
system_design:
  - "../../specs/plugins/system-design/prompt-history-extraction-host.md"
---

# Task 03: Extend Generic Task Panel Capabilities

## Scope

Extend the existing generic task-panel contribution rather than adding prompt-history-specific host branches. Add localized title resolution, typed visibility context, session kind, and a generation-bound native message-navigation capability on desktop and mobile.

## Acceptance

- Desktop add-panel menus, mobile Panels picker, live tabs, restored layouts, and layout previews consume the single reactive host resolver `resolveTaskPanelTitle(registration, i18n)` with the literal title fallback; locale changes update open dockview `api.title` without changing panel identity.
- `visible(context)` receives no private state and is applied consistently to desktop and mobile choices; omission preserves existing visibility. Predicate exceptions are caught by the contribution boundary, logged with plugin/panel identity, and treated as hidden without affecting other entries.
- `PluginTaskPanel` passes current context, injects the Host-created panel-scoped conversation history handle, and revokes the prior navigation/history capabilities on any plugin/panel/task/session/presentation identity change or unmount.
- Desktop uses `scrollTranscriptToMessage`; mobile retains `MobileSessionLayout`'s local scroll target and switches to Chat. Neither path moves mobile state into global Zustand.
- Existing plugin panel registration, persistence, lifecycle failure handling, and mobile full-height presentation stay green.
- Saved-layout validation, serializer/restore, layout-editor registration, and plugin-panel removal/drop handling accept `plugin:<pluginId>:<panelKey>` when the plugin is registered, preserve unknown plugin panels as droppable records when it is absent, and refresh titles through the same resolver after late registration or locale changes.

## TDD sequence

1. RED: add registry/component/menu/mobile/layout-editor tests for title fallback/reactivity, saved-layout round-trip, late registration/removal, visibility success/exception, context propagation, accepted/unavailable outcomes, and stale capability revocation.
2. GREEN: add the optional registration fields and inject the scoped adapters through existing renderers.
3. REFACTOR: centralize context/title resolution without adding a prompt-history condition to generic plugin code.

## Likely files

- `apps/web/lib/plugins/registry.ts`
- `apps/web/components/task/plugin-task-panel.tsx`
- `apps/web/components/task/dockview-add-panel-items.tsx`
- `apps/web/components/task/dockview-panel-content.tsx`
- `apps/web/components/task/dockview-shared.tsx`
- `apps/web/components/task/mobile/plugin-panel-picker.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/lib/state/layout-manager/serializer.ts`
- `apps/web/lib/state/layout-manager/panel-titles.ts`
- `apps/web/lib/state/layout-manager/plugin-panels.test.ts`
- `apps/web/lib/state/layout-manager/serializer.test.ts`
- `apps/web/lib/state/layout-manager/panel-titles.test.ts`
- `apps/web/lib/layout/layout-profiles.ts`
- `apps/web/lib/layout/layout-profiles.test.ts`
- focused registry, task-panel, layout, and mobile tests

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/plugins components/task/plugin-task-panel.test.tsx components/task/dockview-add-panel-items.test.tsx components/task/mobile lib/state/layout-manager/serializer.test.ts lib/state/layout-manager/panel-titles.test.ts lib/state/layout-manager/plugin-panels.test.ts lib/layout/layout-profiles.test.ts
cd apps/web && pnpm run typecheck
```

Do not register a prompt-history plugin or remove the built-in panel in this task.

## Result

Extended generic task panels with reactive localized titles, typed visibility
context, session kind, independent conversation scope, and revocable native
message navigation. Desktop, restored-layout, layout-editor, mobile picker, and
full-height mobile rendering paths use the shared contracts without a
prompt-history-specific branch.

Verified:

- Focused browser Host and task-panel suite (49 files, 442 tests)
- `cd apps/web && pnpm run typecheck`

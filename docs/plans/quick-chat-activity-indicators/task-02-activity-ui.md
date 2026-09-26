---
id: "02-activity-ui"
title: "Quick Chat activity UI"
status: done
wave: 2
depends_on: ["01-activity-selectors"]
plan: "plan.md"
spec: "../../specs/quick-chat-idle-dot/spec.md"
---

# Task 02: Quick Chat Activity UI

- **Acceptance:**
  1. Each working conversation tab shows `GridSpinner`. Settled, setup, and terminal tabs do not.
  2. All five Quick Chat entry points show a blue running bubble or emerald finished bubble from the aggregate selector.
  3. Entry labels describe the state in every supported locale, and opening the dialog clears the finished state.

- **Verification:**
  ```sh
  cd apps && pnpm install --frozen-lockfile \
    && pnpm --filter @kandev/web run i18n:pseudo \
    && pnpm --filter @kandev/web run i18n:zh-hant \
    && pnpm --filter @kandev/web test -- --run \
      components/quick-chat/quick-chat-tab-item.test.tsx \
      components/quick-chat/quick-chat-modal.test.tsx \
      components/quick-chat/quick-chat-activity-indicator.test.tsx \
      components/app-sidebar/app-sidebar-primary-nav.test.tsx \
      components/app-sidebar/app-sidebar-new-task-item.test.tsx \
      components/kanban/kanban-header-mobile.test.tsx \
      components/task/mobile/session-task-switcher-sheet.test.tsx \
    && pnpm --filter @kandev/web run i18n:check \
    && pnpm --filter @kandev/web run typecheck
  ```

- **Files likely touched:**
  - `apps/web/components/quick-chat/quick-chat-activity-indicator.tsx`
  - `apps/web/components/quick-chat/quick-chat-activity-indicator.test.tsx`
  - `apps/web/components/quick-chat/use-quick-chat-activity.ts`
  - `apps/web/components/quick-chat/quick-chat-tab-item.tsx`
  - `apps/web/components/quick-chat/quick-chat-tab-item.test.tsx`
  - `apps/web/components/quick-chat/quick-chat-modal.tsx`
  - `apps/web/components/quick-chat/quick-chat-modal.test.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-nav-item.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-primary-nav.test.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-new-task-item.tsx`
  - `apps/web/components/app-sidebar/app-sidebar-new-task-item.test.tsx`
  - `apps/web/components/kanban/kanban-header.tsx`
  - `apps/web/components/kanban/kanban-header-mobile.tsx`
  - `apps/web/components/kanban/kanban-header-mobile.test.tsx`
  - `apps/web/components/task/mobile/quick-chat-sheet-button.tsx`
  - `apps/web/components/task/mobile/session-task-switcher-sheet.test.tsx`
  - `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/sidebar.json`

- **Dependencies:** Task 01.
- **Parallelism:** sequential.
- **Inputs:** Spec `What`, plus plan sections `Tab status`, `Entry activity bubble`, and `Mobile parity`.
- **Risks:** Repeated indicator markup can drift across breakpoints. Use one shared indicator component and keep the existing touch surfaces.
- **Output contract:** Report changed files, exact command results, blockers, risks, locale-generation results, and synchronized task and plan status.

## Results

- Added the shared activity indicator and hook for all desktop, tablet, and mobile Quick Chat entries.
- Added the conversation-tab spinner while preserving setup and terminal tab behavior.
- Added running/finished state coverage for the shared indicator, tab mapping, sidebar, and mobile entry points.
- `pnpm --filter @kandev/web lint` passed with zero warnings.
- `pnpm --filter @kandev/web run typecheck` passed.
- `pnpm --filter @kandev/web run i18n:check` and `pnpm --filter @kandev/web run i18n:ratchet` passed.
- The full Traditional Chinese generator was blocked by the pre-existing `agents:dynamicProfileSettings` residual keys. The targeted `sidebar` conversion generated the required `zh-hk` and `zh-tw` key successfully.
- No implementation blockers remain for this task.

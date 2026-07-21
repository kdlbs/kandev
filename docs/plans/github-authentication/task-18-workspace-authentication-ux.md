---
id: "18-workspace-authentication-ux"
title: "Workspace authentication UX"
status: done
wave: 6
depends_on: ["17-frontend-registration-client"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 18: Workspace Authentication UX

## Acceptance

1. Workspace settings provide PAT, matched-height named CLI, known App selection, guided existing-App
   import, and guided manifest creation without segmented tabs or a System Settings detour.
2. Method guidance, private/public meaning, registration sharing, actor attribution, permission
   dialog, and conditional personal identity match the spec and expose no secrets after submission.
3. Component tests cover all source/states and the desktop/mobile layouts meet the 44px, one-scroll,
   wrapping, no-overlap contract.

## Verification

```bash
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test -- components/github/github-connection-dialog.test.tsx components/settings/system/github-app-settings.test.tsx
cd apps && rtk pnpm --filter @kandev/web lint
```

Rename/move the singleton settings tests to workspace component names as part of the task and run
their resulting paths if the old paths no longer exist.

## Files Likely Touched

- `apps/web/components/github/github-settings.tsx`
- `apps/web/components/github/github-status.tsx`
- `apps/web/components/github/github-connection-dialog.tsx`
- `apps/web/components/github/github-connection-dialog.test.tsx`
- `apps/web/components/github/github-permissions-dialog.tsx`
- `apps/web/components/settings/system/github-app-settings.tsx` (move/refactor)
- `apps/web/components/settings/system/github-app-setup-form.tsx` (move/refactor)
- `apps/web/components/settings/system/github-app-permissions-dialog.tsx` (remove/reuse shared dialog)
- `apps/web/components/settings/system/github-app-settings-model.ts` (move/refactor)
- corresponding component/model tests
- `apps/web/app/settings/integrations/github/page.tsx`

## Dependencies

Task 17.

## Inputs

- Spec: **Choosing A Method**, **UX And Mobile Contract**, and scenarios.
- Existing workspace connection dialog and settings shell patterns.
- Invoke mobile-parity and use existing icon components.

## Output Contract

Report user flows and responsive behavior, tests run, files touched, screenshots inspected,
blockers/risks, and update this task plus `plan.md` to done.

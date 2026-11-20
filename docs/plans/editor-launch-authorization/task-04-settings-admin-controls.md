---
id: "04-settings-admin-controls"
title: "Hide custom editor management from non-admins"
status: done
wave: 2
depends_on:
  - "01-editor-management-scope"
plan: "plan.md"
requirements:
  - REQ-AUTH-ROLES-AND-SCOPES-001
acceptance_criteria:
  - AC-AUTH-ROLES-AND-SCOPES-001.7
system_design:
  - ../../specs/auth/system-design/editor-authorization-boundary.md
---

# Task 04: Hide custom editor management from non-admins

## Summary

The Editors settings section shows custom editors read-only to callers
without admin rights, mirroring the server-side rule from task 01.

## In scope

- `apps/web/components/settings/editors-settings.tsx`: `CustomEditorsList`
  reads `useIsAdmin()` (`apps/web/hooks/domains/auth/use-is-admin.ts`). When
  false, do not render the Add button, the new-editor form, or the Edit and
  Remove buttons in `CustomEditorRow`. Rows and the empty state stay.
- Component test next to the existing
  `apps/web/components/settings/editors-settings-state.test.tsx`, mocking
  `useIsAdmin` as in
  `apps/web/components/settings/agents/agent-admin-gating.test.tsx`.
- `docs/public/developer-tools.md` (line 415 area): one sentence stating that
  with authentication enabled, only admins can add custom editors. Translate
  nothing; public docs are English.

## Out of scope

Default-editor selection, supported-editor list, LSP settings, new copy,
and backend behavior.

## ASCII UI preview

`UI-01: Settings > Editors, custom editors block` (full view in
[plan.md](plan.md#ascii-ui-preview)).

```text
Non-admin with auth on
+---------------------------------------+
| Custom editors                        |
| | Zed (command)                     | |
+---------------------------------------+
```

Same composition on phone. Admin and authentication-off views are unchanged.

## Acceptance

1. With `useIsAdmin` false, the Add, Edit and Remove controls are absent and
   existing custom editors are listed. With it true, all controls render.
2. No new user-facing strings; `pnpm run i18n:check` passes.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/editors-settings && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
```

## Likely files

- `apps/web/components/settings/editors-settings.tsx`
- `apps/web/components/settings/editors-settings.test.tsx` (new)
- `docs/public/developer-tools.md`

## Results

Non-admins can read custom editor rows and the empty state, but cannot add,
edit, remove, or enter edit mode. Admin controls remain available. The shared
settings composition remains unchanged on desktop and phone. Public developer
tools docs now state the administrator requirement.

Verification passed:

- `pnpm exec vitest run components/settings/editors-settings` (2 files, 14 tests)
- `pnpm run typecheck`
- `pnpm run i18n:check`
- `pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/editors-settings.tsx components/settings/editors-settings-admin-gating.test.tsx`
- `pnpm run build:vite`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

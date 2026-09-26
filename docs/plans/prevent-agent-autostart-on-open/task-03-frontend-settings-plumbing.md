---
id: "03-frontend-settings-plumbing"
title: "Frontend settings plumbing"
status: done
wave: 1
parallelism: sequential
depends_on: ["01-backend-settings-field"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/prevent-agent-autostart-on-open.md"
---

# Task 03: Frontend settings plumbing

## Acceptance

- `UserSettings` / `UserSettingsUpdatePayload` carry
  `prevent_auto_start_agent_on_open?: boolean`; `UserSettingsState` carries
  `preventAutoStartAgentOnOpen: boolean` defaulting to `false`.
- SSR hydration maps the snake_case API field into the store field, keeping the
  default when the field is absent.
- `ensureTaskSession` accepts `{ autoStart?: boolean }` and includes
  `auto_start` in the `session.ensure` WS payload only when explicitly set
  (absent otherwise, so existing behavior is untouched).

## Verification

```bash
(cd apps/web && pnpm run typecheck)
```

```bash
(cd apps/web && pnpm vitest run lib/ssr/user-settings.test.ts lib/services/session-launch-service.test.ts hooks/domains/session/use-ensure-task-session.test.ts)
```

## Files Likely Touched

- `apps/web/lib/types/http-user-settings.ts` (`UserSettings`, `UserSettingsUpdatePayload`)
- `apps/web/lib/ssr/user-settings.ts` (`createDefaultUserSettings` at `:26`, `buildBehaviorFields` at `:243`)
- `apps/web/lib/ssr/user-settings.test.ts`
- `apps/web/lib/state/slices/settings/types.ts` (`UserSettingsState` at `:207`)
- `apps/web/lib/services/session-launch-service.ts` (`ensureTaskSession` at `:76`)
- `apps/web/hooks/domains/session/use-ensure-task-session.test.ts` (mock signature keeps matching)

## Dependencies

Task 01 (backend contract the payload maps to).

## Inputs

- Spec "Data model" and "API surface".
- Existing patterns: `confirm_task_archive` / `confirmTaskArchive` across the
  same frontend files; `ensure_execution` payload precedent in
  `session-launch-service.ts`.

## Output Contract

The setting is readable from `state.userSettings.preventAutoStartAgentOnOpen`
after boot hydration and writable via `updateUserSettings`. `ensureTaskSession`
can request prepare-only on the wire. Tests pin defaults, hydration, and the
`auto_start` payload.

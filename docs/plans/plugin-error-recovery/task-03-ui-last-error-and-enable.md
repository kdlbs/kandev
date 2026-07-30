---
id: "03-ui-last-error-and-enable"
title: "Show last_error in Plugins UI and Enable on error"
status: done
wave: 3
depends_on: ["01-store-last-error"]
plan: "plan.md"
spec: "../../specs/plugins/spec.md"
---

# Task 03: Show last_error in Plugins UI and Enable on error

## Acceptance

1. `PluginRecord` TypeScript type includes optional `last_error` and `last_error_at`.
2. Settings → Plugins list/detail: when `status === "error"` and `last_error` is set, the message is visible (`data-testid` e.g. `plugin-last-error-${id}`).
3. Enable is available for `status === "error"` (same handler as disabled), not only disabled/registered.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/plugins/plugin-row.test.tsx
```

If detail tests exist and change:

```bash
cd apps && pnpm --filter @kandev/web test -- components/settings/plugins/plugin-row.test.tsx components/settings/plugins/plugin-detail
```

## Files likely touched

- `apps/web/lib/types/plugins.ts`
- `apps/web/components/settings/plugins/plugin-row.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/components/settings/plugins/plugin-row.test.tsx`
- Possibly a small shared snippet if list/detail would duplicate markup

## Dependencies

task-01 for field names (frontend can land after 01 without 02 if mock data supplies last_error).

## Parallelism

parallel-safe with task-02 once task-01 field names are known (frontend-only files).

## Inputs

- Spec: Last failure reason; scenario "operator lists plugins… Enable control available".
- Plan: Frontend section.
- Existing: `canEnable` / `canDisable` in plugin-row and plugin-detail; `PluginStatusBadge`; plugin-row.test.tsx patterns.

## Implementation notes

- TDD: extend/add row test for error status with last_error text and Enable button; implement types + canEnable + display.
- Keep copy plain (show backend string as-is); truncate in CSS if very long, do not invent messages.
- mobile-parity: row is already div-based stack; ensure error text wraps (`text-xs`, break-words) and Enable remains in the action row.
- No new API client; list/get already return full record.

## Output contract

- Summary of UI/types changes + test names
- Files changed
- Test command + results
- Update task status `done` and plan checkbox
- Blockers / risks

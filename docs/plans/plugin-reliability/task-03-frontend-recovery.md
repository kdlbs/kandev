---
status: done
---

# Task 03: Expose plugin recovery in Settings

## Objective

Make an errored plugin actionable and show the persisted backend diagnostic in
responsive settings UI.

## Scope

- `apps/web/lib/types/plugins.ts`
- `apps/web/components/settings/plugins/plugin-row.tsx`
- `apps/web/components/settings/plugins/plugin-detail.tsx`
- `apps/web/components/settings/plugins/plugin-manifest-card.tsx`
- `apps/web/components/settings/plugins/use-plugin-actions.ts`
- Component/action tests and plugin Playwright coverage.

## Requirements

- Add nullable `last_error` and `last_error_at` client fields.
- Include `error` in the enable-action states for row and detail views.
- Show a compact `role=alert` diagnostic only when a message is present.
- Clear stale diagnostics in local state after a successful enable.
- After a failed Enable, refetch `GET /api/plugins/{id}` with `no-store` and
  upsert the authoritative record before showing the toast.
- Keep action controls visible and usable at phone widths; recovery targets are
  at least 44px high on phone layouts. Add a phone recovery E2E assertion using
  the existing native mobile settings path.
- Add wrapping/overflow protection for long diagnostics.
- Ensure runtime fields are not shown as authored manifest fields.
- Route any newly introduced user-facing copy through i18n.

## Test-first acceptance

- Error rows/details render Enable and the diagnostic.
- Enable action clears diagnostic fields on success and refetches/upserts a
  replacement error on failure.
- Manifest raw-field display excludes both new runtime fields.
- Desktop and phone Playwright flows can retry a genuinely errored fixture
  plugin, observe a changed diagnostic after consecutive failed retries, measure
  recovery targets at least 44px, and prove a long diagnostic does not create
  horizontal overflow.

## Dependencies

Task 01 API fields and lifecycle behavior.

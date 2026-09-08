---
spec: docs/specs/office/requirements/unread-divider.md
related_specs:
  - docs/specs/ui/requirements/transcript-navigation-settings.md
created: 2026-07-31
status: completed
---

# Implementation Plan: Centralize portable user-setting defaults

## Overview

Make the backend the authority for missing portable user-setting values and use
one frontend mapper for boot, HTTP-save, and WebSocket delivery. Change the
missing-value defaults for the unread divider and transcript auto-scroll control
to disabled while preserving every explicit stored value.

Confirmed root cause: the backend has two default branches and the frontend
repeats the same defaults in its store, HTTP mapper, WebSocket handler,
save-response mapper, and carry-forward helper. E2E setup also treats production
defaults as a global test baseline, creating collateral changes when a default
moves.

## Backend

- Add one backend default constructor and overlay stored JSON values onto it.
- Resolve missing `unread_divider` and
  `show_transcript_auto_scroll_control` fields to `false`.
- Preserve explicit `true` and `false` values without a schema migration.

## Frontend

- Export one frontend default-state factory and one wire-to-store mapper.
- Reuse the mapper for boot/HTTP hydration, WebSocket updates, and editor-save
  responses; partial live payloads preserve the current in-memory value.
- Carry forward the required settings state directly instead of reconstructing
  it with per-field fallbacks.
- Keep the settings controls and save behavior unchanged.

## E2E Tests

- Keep the shared E2E fixture as an explicit, stable test baseline rather than
  mirroring production defaults.
- Explicitly enable the unread divider in chat behavior specs.
- Keep the existing desktop/mobile preference specs deterministic by seeding
  the explicit enabled value they exercise; default-off behavior stays at the
  backend/frontend contract-test level.

## Validation

- `cd apps/backend && go test ./internal/user/store -run 'TestScanUserSettingsUnreadDividerDefault'`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ssr/user-settings.test.ts`
- `cd apps && pnpm --filter @kandev/web test -- --run lib/ws/handlers/users.test.ts`
- `cd apps && pnpm --filter @kandev/web test -- --run hooks/use-user-display-settings.test.ts`
- `cd apps/web && pnpm e2e:run tests/task/unread-divider-preference.spec.ts -- --project=chromium --project=mobile-chrome`
- `cd apps/web && pnpm e2e:run tests/chat/unread-divider.spec.ts tests/chat/mobile-unread-divider.spec.ts -- --project=chromium --project=mobile-chrome`

## Implementation Waves

Sequential: one task changes shared backend/frontend defaults and the related
regression coverage. No subagent delegation is authorized by this plan.

- [completed] [task-01-disable-default](task-01-disable-default.md)

## Mobile Parity Note

This is a data/default change inside the existing General > Task Actions
surface. Desktop and mobile use the same setting control and no layout,
navigation, touch, or scrolling composition changes. Existing mobile preference
coverage remains the appropriate parity check.

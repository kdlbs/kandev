---
id: "03-e2e-docs-and-verification"
title: "Verify notification settings across viewports and update contracts"
status: done
wave: 3
depends_on:
  - "01-backend-events-and-persistence"
  - "02-frontend-and-desktop"
plan: "plan.md"
spec: "../../specs/platform/requirements/notifications.md"
---

# Task 03: Verify Notification Settings and Update Contracts

## Acceptance

- A browser E2E proves a seeded provider appears on a cold settings load and
  both semantic event controls can be selected independently and saved.
- A mobile E2E at 390px proves both event rows are readable, touch-operable,
  and do not cause horizontal overflow.
- Public WebSocket/desktop docs and existing durable specs/ADRs use the semantic
  notification names and accurately describe compatibility behavior.
- Targeted checks and the repository verification suite pass.

## Verification

```bash
cd apps/web
pnpm e2e:run --project chromium \
  tests/settings/settings-manual-save.spec.ts
pnpm e2e:run --project mobile-chrome \
  tests/settings/mobile-notification-events.spec.ts
cd ../../
make fmt
make typecheck
make test
make lint
```

## Files likely touched

- `apps/web/e2e/tests/settings/settings-manual-save.spec.ts`
- `apps/web/e2e/tests/settings/mobile-notification-events.spec.ts`
- `docs/public/websocket-api.md`
- `docs/public/desktop-app.md`
- `docs/specs/desktop/requirements/desktop-tauri-app.md`
- `docs/specs/office/requirements/inbox.md`
- `docs/decisions/0039-native-desktop-integration-boundary.md`

## Dependencies

- Tasks 01 and 02 complete the behavior exercised by E2E.

## Inputs

- Entire semantic notifications spec.
- Mobile-parity canonical viewport and interaction requirements.
- Docs-maintainer terminology and contract checks.

## Output contract

Report E2E fixture setup and cleanup, desktop/mobile evidence, documentation
files reconciled, full verification results, and any remaining gaps.

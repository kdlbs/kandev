# Agent-message inline comments — implementation plan

Status: complete

Spec: [`docs/specs/ui/agent-message-comments.md`](../../specs/ui/agent-message-comments.md)

## Constraints

- Frontend-only. Do not change backend/API/schema/task_comments unless implementation evidence disproves the approved boundary.
- Use existing frontend comment store and browser sessionStorage persistence.
- Ordinary settled agent prose only: `message`/`content`, inactive turn, no raw view. Sibling rich blocks remain outside the selectable prose surface.
- Preserve normal Task Chat, Quick Chat, passthrough, queue, attachments, and existing comment types.

## Waves and tasks

### Wave 1 — comment foundation

1. [`task-01-comment-foundation.md`](task-01-comment-foundation.md) — Extend the comment union, message-local anchor helpers, sessionStorage hydration, Markdown formatting, and focused tests.

### Wave 2 — selection and overlays

2. [`task-02-selection-and-overlay.md`](task-02-selection-and-overlay.md) — Capture local text selections, restore plan-style highlights and badges after virtualization, and provide the plan comment affordance plus shared desktop Popover/mobile Drawer actions.

### Wave 3 — composer and delivery wiring

3. [`task-03-chat-wiring.md`](task-03-chat-wiring.md) — Add message-comment context chips, composer inclusion/cleanup, immediate/queued Run, Task Chat/Quick Chat integration, and passthrough preservation.

### Wave 4 — browser verification

4. [`task-04-e2e-and-verification.md`](task-04-e2e-and-verification.md) — Add render-cost/component coverage plus Chromium and mobile-chrome E2E for Add, restore, Drawer, and Run paths.

## Mobile design contract

- Entry point: select settled agent prose in Task Chat or Quick Chat, then activate the plan-style comment affordance.
- Exemplar: existing inset `Drawer` context surfaces (`mobile-menu-sheet.tsx`) and `useTouchDrawer` coarse-pointer branching.
- Hierarchy: selected excerpt → feedback input → attached Add/Run actions, with update/delete for pending comments.
- Surface: the shared plan-comment Popover on desktop; an equivalent phone inset bottom Drawer. Drawer owns its content scroll and clears safe area.
- Shared: comment model, anchor resolver, store, formatter, context chips, and delivery callbacks shared across compositions.
- Verification: desktop Chromium and `mobile-chrome` selection/Add/Run tests, containment, and no horizontal document overflow.

## Verification commands

- `cd apps && pnpm --filter @kandev/web test -- --run <focused tests>`
- `cd apps/web && pnpm e2e:run --host tests/chat/agent-message-comments.spec.ts --project=chromium`
- `cd apps/web && pnpm e2e:run --host --no-build tests/chat/mobile-agent-message-comments.spec.ts --project=mobile-chrome`
- `make fmt`
- `make typecheck test lint`
- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`

---
id: "02-e2e-pin"
title: "E2E pin persistence across navigation"
status: done
wave: 2
depends_on: ["01-frontend-pin"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-pin.md"
---

# Task 02: E2E pin persistence across navigation

- **Acceptance:**
  1. A queued-message scenario: after expanding the queue panel and clicking
     `queue-pin`, navigating away from the task and back shows the expanded
     panel (`queued-ghost-list` visible) without clicking `queue-chip`.
  2. A regression scenario: without pinning, the same navigation leaves the
     panel collapsed with `queue-chip` visible.
- **Verification** (rooted at `apps/web`; requires the e2e fixture backend per
  `apps/web/e2e/README.md`):
  ```bash
  cd apps/web && pnpm e2e:raw -- tests/chat/message-queue-pin.spec.ts
  ```
- **Files likely touched:**
  - `apps/web/e2e/tests/chat/message-queue-pin.spec.ts` (new)
  - Possibly `apps/web/e2e/tests/chat/message-queue.spec.ts` helpers reuse
- **Dependencies:** `01-frontend-pin` must land first (the pin button must
  exist in the header).
- **Parallelism:** sequential.
- **Inputs:** spec Scenarios; plan E2E Tests section. Pattern:
  `apps/web/e2e/tests/chat/message-queue.spec.ts` (queue a message, open via
  `queue-chip`, navigate with `page.goto`).
- **Output contract:** summary, files changed, exact command + outcome,
  task/plan status update.

## Results

- `cd apps/web && pnpm e2e:run tests/chat/message-queue-pin.spec.ts` (managed
  runner, fresh production build) → 2 passed (pinned stays expanded across
  navigation; unpinned stays collapsed).
- `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/chat/mobile-message-queue-management.spec.ts` → 3 passed, including the new
  "mobile pin keeps the queue panel open across navigation" test (touch target
  ≥44px asserted via `expectTouchTarget`).
- Files changed beyond the planned list: mobile coverage added to the existing
  `mobile-message-queue-management.spec.ts` (mobile-parity requirement);
  `seedBusyQueueTask` now returns `{ session, taskId }`.
- Security/trust boundaries: `None`. External side effects: `None`.

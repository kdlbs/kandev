---
id: "08-frontend-failure-surface-and-recovery"
title: "Frontend launch-error data"
status: done
wave: 5
depends_on: ["07-status-summary-projection"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/task-launch-failure-recovery.md"
---

# Task 08: Frontend launch-error data

Carry the bounded launch-error contract into task-detail data and chat view models.

- **Acceptance:**
  1. Frontend status, office-task, `LastAgentError`, and `RunError` types carry the bounded fields.
  2. Task-detail mapping preserves `status_summary`.
  3. A tested selector chooses the newest summary from detail data and live task caches.
  4. Session metadata parsing keeps exact row identity, action order, and explicit error stamp.
  5. A chat helper suppresses a summary error when a matching failed-session error exists.
  6. Task-owned errors with no session remain eligible for standalone rendering.

- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- lib/session-last-agent-error.test.ts lib/task-status-summary.test.ts components/task/simple/chat-entries.test.ts && cd web && pnpm run typecheck`

- **Files likely touched:**
  `apps/web/lib/types/task-status-summary.ts`,
  `apps/web/lib/session-last-agent-error.ts`,
  `apps/web/lib/task-status-summary.ts`,
  `apps/web/lib/state/slices/office/types.ts`,
  `apps/web/app/office/tasks/[id]/types.ts`,
  `apps/web/app/office/tasks/[id]/page.tsx`,
  `apps/web/components/task/simple/chat-entries.ts`,
  focused tests beside those files.

- **Dependencies:** Task 07.
- **Parallelism:** sequential.
- **Inputs:** spec "Bounded task projection" and plan "Types and live summary selection".

## Results
Implemented the bounded task summary types, task-scoped error context, and the standalone recovery card.
The card preserves exact task-repository identity and coalesces duplicate recovery requests.
It also renders typed errors without actions, guards refreshes by summary
revision, and shows pending state in duplicate task surfaces.

Verification:

- `cd apps/web && pnpm exec vitest run components/task/simple/components/task-launch-error-entry.test.tsx components/task/simple/task-chat.test.tsx lib/task-status-summary.test.ts lib/session-last-agent-error.test.ts`: 4 files and 50 tests passed.
- `cd apps/web && pnpm run typecheck`: passed.
- `cd apps/web && pnpm run lint`: passed.
- PR fixup verification: focused tests passed in 3 files with 64 tests.

Second PR fixup hardening requires equal session error stamps before suppressing
a summary card and keeps non-typed status errors on the normal responsive
layout path.

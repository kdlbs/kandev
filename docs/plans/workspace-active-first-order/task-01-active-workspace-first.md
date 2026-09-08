---
id: "01-active-workspace-first"
title: "Order active workspace first"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/workspace-active-first-order.md"
---

# Task 01: Order active workspace first

- **Acceptance:**
  - The shared display-order helper puts the workspace matching
    `workspaces.activeId` first and preserves the relative order of all other
    workspaces, including null and unknown active ids.
  - The Settings sidebar workspace branch and `/settings/workspaces` cards both
    use that order without changing active badges, selection, or persistence.
  - Desktop and phone E2E coverage proves the active workspace is first in both
    Settings surfaces.
- **Verification:**

  ```bash
  (cd apps && pnpm install --frozen-lockfile)
  (cd apps && pnpm --filter @kandev/web test -- --run lib/settings/workspace-display-order.test.ts components/app-sidebar/sections/settings/settings-menu-branches.test.ts)
  (cd apps/web && pnpm run typecheck)
  (cd apps/web && pnpm e2e:run --project chromium tests/settings/workspace-display-order.spec.ts)
  (cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-workspace-display-order.spec.ts)
  git diff --check
  ```

- **Files likely touched:**
  - `apps/web/lib/settings/workspace-display-order.ts`
  - `apps/web/lib/settings/workspace-display-order.test.ts`
  - `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
  - `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`
  - `apps/web/app/settings/workspace/workspaces-page-client.tsx`
  - `apps/web/e2e/tests/settings/workspace-display-order.spec.ts`
  - `apps/web/e2e/tests/settings/mobile-workspace-display-order.spec.ts`
- **Dependencies:** None.
- **Parallelism:** sequential. The sidebar and page must consume the same
  helper, and E2E should run after both consumers are wired.
- **Inputs:**
  - `docs/specs/ui/requirements/workspace-active-first-order.md`
  - `docs/plans/workspace-active-first-order/plan.md`
  - `apps/web/AGENTS.md`
  - `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
  - `apps/web/app/settings/workspace/workspaces-page-client.tsx`
  - `apps/web/e2e/helpers/settings-menu.ts`
  - `apps/web/e2e/tests/settings/mobile-settings-sidebar.spec.ts`
- **Output contract:** Summarize the helper and both consumers, list the exact
  files changed, record red/green unit and E2E results plus typecheck and diff
  checks, note blockers/risks, and update this task and `plan.md` status in the
  same conversation.

## Results

Implemented a shared `orderWorkspacesForDisplay` helper and applied it to the
Settings sidebar branch builder and the `/settings/workspaces` card list. The
helper returns a new array, moves only the matching active workspace, and
preserves input order for null, unknown, and already-first active ids.

Files changed:

- `apps/web/lib/settings/workspace-display-order.ts`
- `apps/web/lib/settings/workspace-display-order.test.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.ts`
- `apps/web/components/app-sidebar/sections/settings/settings-menu-branches.test.ts`
- `apps/web/app/settings/workspace/workspaces-page-client.tsx`
- `apps/web/e2e/tests/settings/workspace-display-order.spec.ts`
- `apps/web/e2e/tests/settings/mobile-workspace-display-order.spec.ts`
- `docs/specs/INDEX.md`
- `docs/specs/ui/requirements/workspace-active-first-order.md`
- `docs/plans/workspace-active-first-order/plan.md`
- `docs/plans/workspace-active-first-order/task-01-active-workspace-first.md`

Verification:

- Focused Vitest: 2 files, 35 tests passed.
- Web typecheck: passed.
- Web lint: passed.
- Fresh-build desktop E2E: 1 test passed.
- Fresh-build phone E2E: 1 test passed.
- PR capture: 1 managed test passed and produced inspected, compressed desktop
  (1280x720) and phone (390x844) screenshots in ignored `.pr-assets/`.
- `git diff --check`: passed.

The initial RED runs failed on the intended active-order assertions. No
blockers or known risks remain; the spec is marked `shipped` after review
triage.

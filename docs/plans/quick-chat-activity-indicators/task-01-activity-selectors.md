---
id: "01-activity-selectors"
title: "Quick Chat activity selectors"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/quick-chat-idle-dot/spec.md"
---

# Task 01: Quick Chat Activity Selectors

- **Acceptance:**
  1. One selector reports whether a Quick Chat session has live work, including background work and environment preparation.
  2. One aggregate selector returns `running`, `finished`, or `null` for one workspace.
  3. Running has priority over finished, and an open dialog returns `null`.

- **Verification:**
  ```sh
  cd apps && pnpm install --frozen-lockfile \
    && pnpm --filter @kandev/web test -- --run \
      lib/session-working.test.ts \
      hooks/domains/session/use-session-state.test.ts \
      lib/state/slices/ui/quick-chat-activity-selectors.test.ts
  ```

- **Files likely touched:**
  - `apps/web/lib/session-working.ts`
  - `apps/web/lib/session-working.test.ts`
  - `apps/web/hooks/domains/session/use-session-state.ts`
  - `apps/web/lib/state/slices/ui/quick-chat-activity-selectors.ts`
  - `apps/web/lib/state/slices/ui/quick-chat-activity-selectors.test.ts`
  - `apps/web/lib/state/slices/ui/quick-chat-unseen-selectors.ts`

- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:** Spec `What`, `State machine`, and aggregate scenarios. Preserve the `deriveSessionFlags()` behavior in `use-session-state.ts`.
- **Risks:** A second definition of working state can drift from the chat panel. Reuse the shared derivation and add background-work coverage.
- **Output contract:** Report changed files, exact command results, blockers, risks, and synchronized task and plan status.

## Results

- Added the shared `isSessionWorking` predicate and reused it in `deriveSessionFlags`.
- Added pure selectors for per-session work and workspace-level Quick Chat activity.
- Covered starting, running, background, preparation, settled, workspace, setup-tab, and open-dialog behavior.
- The focused selector/session tests passed as part of the final frontend suite: 10 files, 107 tests.
- No blockers remain for this task.

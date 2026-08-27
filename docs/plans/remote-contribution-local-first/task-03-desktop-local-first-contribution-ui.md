---
id: "03-desktop-local-first-contribution-ui"
title: "Desktop local-first contribution UI"
status: completed
wave: 3
depends_on: ["02-local-first-relation-and-shared-actions"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 03: Desktop Local-First Contribution UI

Replace the blocking desktop warning with a compact local-first status, a collapsed provider version,
and explicit version actions.

## Inputs

- Spec scenarios: local-first rewritten contribution, exact replacement, stale lease, provider
  adoption, and local file preservation.
- Plan section: Desktop Changes and VCS surfaces.
- Task 02 relation and resolution state.

## Acceptance

1. A diverged Changes panel keeps the task history primary and keeps the PR history collapsed behind
   **PR #<number> version**.
2. Changes and VCS controls keep local actions enabled and expose **Replace PR branch** and
   **Use PR version** with destructive confirmation.
3. Success and error feedback show the recovery branch or the actionable stale-lease and dirty-tree
   result.

## Files Likely Touched

- `apps/web/components/task/changes-panel-body.tsx`
- `apps/web/components/task/changes-panel-data.tsx`
- `apps/web/components/task/changes-panel-header.tsx`
- `apps/web/components/task/changes-panel-timeline.tsx`
- `apps/web/components/task/changes-panel-hooks.ts`
- `apps/web/components/task/remote-contribution-resolution-dialog.tsx` (new)
- `apps/web/components/task/remote-contribution-resolution-dialog.test.tsx` (new)
- `apps/web/components/task/changes-panel-header.test.tsx`
- `apps/web/components/vcs-split-button.tsx`
- `apps/web/components/vcs-split-button.test.ts`
- `apps/web/components/vcs-multi-repo-menu.tsx`
- `apps/web/components/vcs-multi-repo-menu.test.ts`

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/changes-panel-header.test.tsx components/task/remote-contribution-resolution-dialog.test.tsx components/vcs-split-button.test.ts components/vcs-multi-repo-menu.test.ts && cd web && pnpm run typecheck
```

## Dependencies

Task 02.

## Parallelism

Parallel-safe with Task 04 after Task 02. The two tasks own disjoint presentation files. The primary
conversation remains sequential unless the user authorizes subagents.

## Risks

- Do not hide local commit recovery actions while the provider disclosure is collapsed.
- Do not keep the old generic force-push entry for a diverged contribution.
- Keep the destructive action scoped to the selected repository.

## Output Contract

Report the desktop interaction, component files, focused rendered evidence, and exact test results.
Update this task and `plan.md` in the same conversation.

## Results

Completed 2026-08-12.

- The Changes panel now presents the local checkout first, keeps provider commits collapsed behind
  **PR #<number> version**, and preserves local edit, commit, and review actions during drift.
- The Changes header, split VCS button, and per-repository menu expose repository-scoped **Replace PR
  branch**, **Use PR version**, and **PR #<number> version** actions. Generic Push and force-push entries are
  removed only for the selected diverged contribution, while generic Pull remains unavailable.
- The desktop confirmation names the repository and exact provider head, explains the replacement or
  recovery effect, keeps failed confirmations open for retry, and shows translated result feedback.
- Verification passed: the shared focused desktop/VCS suite (26 tests in 7 files), `pnpm run typecheck`,
  and the desktop local-first Playwright scenario (1 passed, retries disabled).

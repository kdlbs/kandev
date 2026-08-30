---
id: "02-source-aware-commit-ui"
title: "Source-aware commit UI"
status: done
wave: 2
depends_on: ["01-github-commit-detail"]
plan: "plan.md"
spec: "../../specs/ui/requirements/pr-only-commit-details.md"
---

# Task 02: Source-aware commit UI

- **Acceptance:** Merged commit rows carry a discriminated local/GitHub target;
  matching SHAs prefer the local source; PR-only rows omit unknown stats and
  local mutation controls; opening a local target calls only
  `session.commit_diff`; opening a GitHub target calls only
  `github.pr_commit.get`, renders remote metadata/files on desktop and mobile,
  disables worktree-dependent actions/expansion, preserves repository identity,
  and never falls back locally on a GitHub error.
- **Verification:** Follow strict TDD. Add focused merge, request/hook, row,
  panel, dockview, and mobile-target regressions; run the smallest affected
  Vitest files and record failures caused by the current SHA-only target and
  local-only loader. Use
  `cd apps/web && pnpm test -- components/task/changes-panel.test.ts components/task/commit-detail-request.test.ts components/task/commit-detail-panel.test.tsx lib/state/dockview-panel-actions.test.ts`
  for the focused RED/GREEN cycle. After implementation, rerun it, then run
  `cd apps/web && pnpm run typecheck`,
  `cd apps && pnpm --filter @kandev/web lint`,
  `cd apps && pnpm run i18n:check && pnpm run i18n:ratchet`, and
  `git diff --check`.
- **Files likely touched:**
  `apps/web/lib/types/github.ts`;
  a source-aware target/request hook under
  `apps/web/components/task` or `apps/web/hooks/domains`;
  `apps/web/components/task/changes-panel-helpers.ts`;
  `apps/web/components/task/changes-panel-data.tsx`;
  `apps/web/components/task/changes-panel-body.tsx`;
  `apps/web/components/task/commit-row.tsx`;
  `apps/web/components/task/commit-detail-panel.tsx`;
  `apps/web/components/task/changes-diff-target.ts`;
  `apps/web/components/task/dockview-shared.tsx`;
  `apps/web/components/task/dockview-panel-content.tsx`;
  `apps/web/lib/state/dockview-store.ts`;
  `apps/web/lib/state/dockview-panel-actions.ts`;
  `apps/web/components/task/mobile/mobile-changes-panel.tsx`;
  `apps/web/components/task/mobile/mobile-diff-sheet.tsx`; and colocated unit
  tests for the changed helpers, state, hook, and components.
- **Dependencies:** Task 01's settled WebSocket action and response types.
- **Parallelism:** sequential — source identity crosses merge logic, serialized
  desktop panel state, mobile selection, and one shared detail renderer; split
  implementations would create an intermediate incompatible UI contract.
- **Inputs:** repair spec desired behavior; Task 01 output contract; current
  `mergeCommits`, `CommitRow`, `requestCommitDiff`, `useCommitDiff`,
  `CommitDetailPanel`, dockview commit panel actions, `MobileChangesPanel`, and
  `MobileDiffSheet` patterns.
- **Output contract:** Report the discriminated target shape, route selection,
  row/action behavior, desktop/mobile rendering behavior, exact files changed,
  RED/GREEN and lint/type/i18n results, remaining patch limitations, and
  synchronized task/plan status.

## Results

- RED: source-routing regressions demonstrated that SHA-only rows always used
  `session.commit_diff`, including PR-only commits, and that remote rows could
  inherit fabricated zero statistics and local actions.
- GREEN: added the discriminated local/GitHub target, repository-aware merge
  precedence, source-aware request/hook loading, remote metadata/file mapping,
  dockview serialization identity, and shared desktop/mobile rendering. Remote
  rows omit unmeasured statistics and all worktree mutation/file/context
  actions; GitHub errors do not fall back locally.
- Verification: focused Vitest coverage passed (7 files, 72 tests), followed
  by web typecheck, lint, i18n check, i18n ratchet, and `git diff --check`.
- Review remediation: keyed PR commit results by the complete workspace,
  repository, pull request, and refresh identity so rapid PR switches cannot
  reuse stale rows. Unavailable clients and malformed commit responses now
  remain errors with a retry state, and GitHub detail requests no longer
  refetch when local agent readiness changes. The remediation suite passed
  (8 files, 69 tests), with additional focused regressions covering the PR
  switch, protocol failures, retry rendering, and remote dependency boundary.
- Review follow-up: normalized missing repository names to the workspace-root
  identity, rejected malformed backend and frontend commit-detail payloads,
  bound loaded detail state to the selected target, hardened serialized GitHub
  target validation, and corrected the late-response regression. The rebased
  verification passed with 1,086 backend GitHub tests and 73 focused frontend
  tests.

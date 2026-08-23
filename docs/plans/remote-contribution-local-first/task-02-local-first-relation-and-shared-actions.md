---
id: "02-local-first-relation-and-shared-actions"
title: "Local-first relation and shared actions"
status: completed
wave: 2
depends_on: ["01-exact-lease-contribution-operations"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 02: Local-First Relation and Shared Actions

Change diverged history from a blocked state to a local-first state. Add typed frontend operations and
shared confirmation state for the exact-lease backend actions.

## Inputs

- Spec sections: What, API surface, Failure modes, and rewritten-history scenarios.
- Plan sections: Relation and action model, Shared copy and result handling, and Tests.
- Task 01 request and response contracts.

## Acceptance

1. The pure relation model distinguishes normal Push, provider-ahead Pull, remote replacement,
   provider adoption, and unavailable evidence.
2. Frontend callbacks send the exact provider head and one explicit repository scope. They never use
   destructive multi-repository fan-out.
3. Shared state and translated copy cover confirmation, stale leases, dirty worktrees, and recovery
   branch success feedback in all four task catalogs.

## Files Likely Touched

- `apps/web/hooks/domains/session/remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/remote-contribution-relation.test.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.ts`
- `apps/web/hooks/domains/session/use-remote-contribution-relation.test.tsx`
- `apps/web/hooks/use-git-operations.ts`
- `apps/web/hooks/use-git-operations.test.ts`
- `apps/web/components/task/use-remote-contribution-resolution.ts` (new)
- `apps/web/components/task/use-remote-contribution-resolution.test.tsx` (new)
- `apps/web/components/task/changes-panel-helpers.ts`
- `apps/web/components/task/changes-panel-remote.test.ts`
- `apps/web/src/locales/en/task.json`
- `apps/web/src/locales/pt-pt/task.json`
- `apps/web/src/locales/zh-cn/task.json`
- `apps/web/src/locales/pseudo/task.json`

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- hooks/domains/session/remote-contribution-relation.test.ts hooks/domains/session/use-remote-contribution-relation.test.tsx hooks/use-git-operations.test.ts components/task/use-remote-contribution-resolution.test.tsx components/task/changes-panel-remote.test.ts && pnpm --filter @kandev/web i18n:check && pnpm --filter @kandev/web i18n:ratchet && cd web && pnpm run typecheck
```

## Dependencies

Task 01.

## Parallelism

Sequential. The desktop and mobile tasks consume this shared model.

## Risks

- Do not treat incomplete provider commits as permission to replace a branch.
- Keep GitHub-specific selection in the current hook. Do not imply GitLab UI support without MR-head
  evidence.
- Keep translation keys at render time and preserve plural handling through `count`.

## Output Contract

Report the action truth table, request payloads, localized keys, files changed, and exact test results.
Update this task and `plan.md` in the same conversation.

## Results

Completed on 2026-08-12.

- Replaced the blocking divergence relation with an action model covering `normal_push`,
  `provider_ahead_pull`, `diverged_replace`, and `unavailable_evidence`. Diverged relations expose
  `canReplaceRemote` and `canUseRemote` only when provider evidence is complete and the provider
  head is a full commit ID. Incomplete evidence disables all version-changing actions.
- Added typed `replaceRemoteContribution` and `useRemoteContribution` callbacks. They send
  `expected_remote_head` and preserve an explicit empty `repo` scope for the workspace root.
- Added `useRemoteContributionResolution` for shared desktop/mobile confirmation state and result
  handling. It keeps the provider head and repository target in the pending confirmation, exposes
  recovery branch results, and maps lease, dirty-tree, and generic failures to translated keys.
- Added task-namespace copy for the status, version choices, confirmations, stable error guidance,
  and recovery success in English, Portuguese, Simplified Chinese, and pseudo locales.
- Verification: focused Vitest command from this task passed 42 tests in 5 files. `pnpm run
  i18n:check` and `pnpm run i18n:ratchet` passed. `pnpm run typecheck` passed.

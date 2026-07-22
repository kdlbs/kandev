---
id: "11-reconcile-main"
title: "Reconcile latest main"
status: done
wave: 0
depends_on: []
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 11: Reconcile Latest Main

## Acceptance

1. The branch contains current `origin/main` with no unmerged paths or conflict markers.
2. Upstream repository-scope UI/API helper behavior and this branch's workspace-keyed GitHub auth
   behavior are both retained.
3. Frontend typecheck and the existing GitHub connection-dialog test pass.

## Verification

```bash
test -z "$(git ls-files -u)"
git diff --check
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test -- components/github/github-connection-dialog.test.tsx
```

## Files Likely Touched

- `apps/web/components/github/github-settings.tsx`
- `apps/web/e2e/helpers/api-client.ts`
- `docs/specs/INDEX.md`

## Dependencies

None.

## Inputs

- Plan: **Conflict Resolution**.
- Use a merge of current `origin/main`; do not choose an entire side for files changed in both.

## Output Contract

Report merge commit/base, resolutions made, commands run, files touched, blockers/risks, and update
this task plus `plan.md` to done.

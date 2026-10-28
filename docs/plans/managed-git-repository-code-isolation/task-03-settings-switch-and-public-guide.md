---
id: "03-settings-switch-and-public-guide"
title: "Settings switch and public guide"
status: draft
wave: 3
depends_on:
  - "02-hooks-setting-persistence-and-propagation"
plan: "plan.md"
requirements:
  - REQ-AGENTS-GITISOLATION-003
acceptance_criteria:
  - AC-AGENTS-GITISOLATION-003.1
  - AC-AGENTS-GITISOLATION-003.2
  - AC-AGENTS-GITISOLATION-003.3
  - AC-AGENTS-GITISOLATION-003.4
system_design:
  - ../../specs/agents/system-design/managed-git-repository-code-isolation.md
---

# Task 03: Settings switch and public guide

## Summary

Add the "Run Git hooks" switch to the repository card with its help text, wire
save and dirty tracking, localize it, cover it with desktop and mobile
Playwright, and document the behavior in the public Git guide.

## In scope

- `repository-card.tsx` switch (preview `UI-01` in the system design), save and
  dirty plumbing, `run_git_hooks` in the web action types.
- Copy in all seven locales (`pnpm run i18n:zh-hant` for the Traditional pair).
- Vitest for dirty/save mapping; Playwright for toggle, persistence after reload,
  save failure, on desktop and the mobile project.
- `docs/public/git-operations.md` section and troubleshooting entry.
- Help text and guide statements of `003.1` and `003.4`: user-global and system hooks skipped, submodule recursion disabled when hooks are on, git-crypt files shown as binary changes, Kandev-run Git LFS `pre-push` step, Kubernetes and plugin-remote executors with hooks off.

## Out of scope

Backend behavior (task 02), any new notice for a skipped hook, and the disabled-while-flag-off state (task 04).

## Acceptance

`AC-AGENTS-GITISOLATION-003.1` through `003.4`.

## Verification

```bash
cd apps/web
pnpm run typecheck && pnpm run i18n:check
pnpm exec vitest run app/settings/workspace
pnpm e2e:run -- --grep "Run Git hooks"
cd ../.. && python3 scripts/lint-spec-files.py --all && python3 scripts/list-docs.py validate
```

## Likely files and risks

`apps/web/components/settings/repository-card.tsx`,
`apps/web/app/settings/workspace/workspace-repositories-{client,save,dirty}*`,
`apps/web/app/actions/workspaces.ts`, `apps/web/src/locales/*`,
`docs/public/git-operations.md`. Risk: the i18n ratchet and translation
completeness gate; mobile touch target.

## Results

Pending.

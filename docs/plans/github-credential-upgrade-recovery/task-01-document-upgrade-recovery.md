---
id: "01-document-upgrade-recovery"
title: "Document GitHub credential upgrade recovery"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.13
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-03.md
---

# Task 01: Document Upgrade Recovery

## Summary

Keep the public task-access guidance concise and consistent with preserved policies.
Record the upgrade recovery evidence internally for possible later use.

## In scope

- Qualify the managed opt-in description for historical workspaces.
- Keep the existing **Choose task Git credentials** section focused on durable user actions.
- Describe explicit policy selection, task-only executor inheritance, and a fresh terminal after launch or resume.
- Distinguish managed checkouts, user-managed checkouts, and remote executors in short bullets.
- Keep the upgrade investigation evidence in this plan instead of adding a temporary public runbook.
- Include the checklist in Acceptance as a manual documentation review.

## Out of scope

Production code, test code, UI copy, automatic migration, credential probes, a temporary public
upgrade runbook, and generated changelog edits.

## Acceptance

1. The guide explains historical managed defaults, current executor defaults, and preservation of existing workspace policies.
2. Task-access bullets describe explicit executor selection, a fresh terminal after launch or resume,
   and the managed, user-managed, and remote-executor boundaries.
3. The guide does not promise automatic policy conversion or comprehensive credential preflight,
   and it does not add a temporary upgrade-specific recovery runbook.

## Verification

Run from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Review the three acceptance conditions against the final document and current source.
No new test is required because the correction changes documentation only.

## Files likely touched

- `docs/public/integrations.md`
- `docs/public/use-kandev.md` (cross-page summary)
- `docs/plans/github-credential-upgrade-recovery/plan.md` (status and results)
- `docs/plans/github-credential-upgrade-recovery/task-01-document-upgrade-recovery.md` (status and results)

## Dependencies

None. The origin fixes already exist in current main and v0.94.0.

## Risks

Host `gh` authentication does not establish task Git transport access.
Managed clone worktrees share remote settings. A running agent retains its existing credential environment.
The procedure must not suggest broad manual rewrites, token disclosure, or automatic policy changes.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-authentication.md), criterion 001.13.
- [Design](../../specs/integrations/system-design/github-authentication-03.md#upgrade-recovery-guidance).
- [Investigation](plan.md#investigation-evidence).
- `apps/backend/internal/github/store.go`: `addTaskGitCredentialsMode`, `defaultWorkspaceSettings`.
- `apps/backend/internal/github/workspace_defaults.go`: existing-installation guard.
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`: managed admission scope.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`: prepared launch ordering.
- Existing **Choose task Git credentials** section in the public guide.

## Results

Implemented concise task-access guidance in
[`docs/public/integrations.md`](../../public/integrations.md). The guide now explains the
historical `managed` compatibility policy, current executor inheritance, preserved saved
policies, conditional **Connect GitHub** and **Change connection** entry points, task-only
executor selection, fresh-terminal requirements, and managed, user-managed, and remote-executor
boundaries. The cross-page summary in
[`docs/public/use-kandev.md`](../../public/use-kandev.md) no longer describes managed access as the
default for new workspaces.
The temporary upgrade-specific recovery section was removed from the public guide; the
investigation evidence remains in the plan for possible later documentation.

All three acceptance conditions pass by manual review. Public-doc tests (62), public-doc
validation (46 pages), catalog validation, specification lint, focused Go tests, both backend and
web builds, and `git diff --check` pass. The final documentation pass removed the temporary
upgrade-specific recovery section, retained concise task-access guidance, and aligned the plan
and work-order scope. Post-fixup focused documentation validation passed: public-doc validation
(46 pages), catalog validation (264 decisions and 818 specifications), specification lint, and
`git diff --check`.

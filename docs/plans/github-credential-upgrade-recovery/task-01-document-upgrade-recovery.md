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

Explain the historical managed default and the current recovery path in the existing integration guide.
Keep the instructions consistent with preserved policies and automatic managed-checkout reconciliation.

## In scope

- Qualify the managed opt-in description for historical workspaces.
- Add a focused subsection under **Upgrade and recovery** about the task credential policy refactor.
- Explain the healthy-host-login symptom without claiming that every authentication error has this cause.
- Link to the existing task-access section and describe explicit policy selection and subsequent launch or resume.
- Identify v0.92.0 as the first stable release with the prepared-origin and dynamic-protocol fixes.
- Include the checklist in Acceptance as a manual documentation review.

## Out of scope

Production code, test code, UI copy, automatic migration, credential probes, and generated changelog edits.

## Acceptance

1. The guide explains historical managed defaults, current executor defaults, and preservation of existing workspace policies.
2. Recovery describes explicit executor selection, the service-user boundary, origin inspection, and a later launch or resume. It distinguishes managed checkouts, user-managed checkouts, and remote executors.
3. The guide does not promise automatic policy conversion or comprehensive credential preflight. Its introductory mode description agrees with the upgrade subsection.

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
- Existing **Choose task Git credentials** and **Upgrade and recovery** sections in the public guide.

## Results

Implemented the focused upgrade and recovery guidance in
[`docs/public/integrations.md`](../../public/integrations.md). The guide now explains the
historical `managed` compatibility default, current executor inheritance, preserved saved
policies, conditional **Connect GitHub** and **Change connection** entry points, task-only
executor recovery, fresh-terminal requirements, Local and Worktree origin reconciliation from
v0.92.0, user-managed checkout ownership, service-user credentials, remote executor boundaries,
and the limits of managed preflight. The cross-page summary in
[`docs/public/use-kandev.md`](../../public/use-kandev.md) no longer describes managed access as the
default for new workspaces.

All three acceptance conditions pass by manual review. Public-doc tests (62), public-doc
validation (46 pages), catalog validation, specification lint, focused Go tests, both backend and
web builds, and `git diff --check` pass. A follow-up source review corrected the conditional
connection entry point and the requirement to create a fresh terminal process; no builds or tests
were rerun for that review. The PR fixup review then clarified preservation of either saved task
Git policy for historical workspaces, scoped the recovery list to managed connections, aligned the
plan's control labels with the UI, and completed the touched-file inventory. Post-fixup focused
documentation validation passed: public-doc validation (46 pages), catalog validation (264
decisions and 818 specifications), specification lint, and `git diff --check`.

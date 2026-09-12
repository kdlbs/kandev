---
id: "01-document-upgrade-recovery"
title: "Document GitHub credential upgrade recovery"
status: pending
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

Pending. This work order requires a later explicit implementation request.

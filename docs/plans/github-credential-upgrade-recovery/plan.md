---
created: 2026-09-12
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-AUTHENTICATION-001
system_design:
  - ../../specs/integrations/system-design/github-authentication-01.md
  - ../../specs/integrations/system-design/github-authentication-03.md
legacy_specs: []
---

# Fix Plan: Explain GitHub Credential Upgrade Recovery

Issue: [#3071](https://github.com/kdlbs/kandev/issues/3071).

## Overview

The issue remains useful as a documentation repair. Its original runtime proposals need separate treatment.
This package records the historical default change and keeps the public task-access guidance concise.
The upgrade-specific recovery runbook is deferred because it is temporary migration guidance.
It preserves the current policy contract and uses one documentation work order.

## Investigation evidence

Investigation date: 2026-09-12. Local HEAD and remote main both equal
`49df3794bc15d610988acc674637384ce8f1d4be`. Latest stable: v0.94.0, published 2026-09-09.
The issue was opened on 2026-08-26 against v0.91.0+ and contains no attachments or comments.

| Concern | Current evidence | Disposition |
| --- | --- | --- |
| Historical managed default | `github.Store.addTaskGitCredentialsMode` still adds the column with `DEFAULT 'managed'`, including in v0.94.0. | The historical upgrade behavior remains. |
| New workspace default | PR #2156, commit `190eeddbc`, persists executor access. First containing stable tag: v0.84.1. | Already fixed before this issue. |
| Existing workspace policy | `InitializeFreshWorkspaceDefaults` skips existing stores. Missing settings still resolve to managed. | Preserve intentional managed choices. |
| Stale prepared checkout | PR #3077, commit `c2e2b3a43`, reconciles origins before prepared launches. | Fixed in v0.92.0. |
| Host clone protocol | PR #3078, commit `e4d17b3925`, resolves host-specific protocol dynamically. | Fixed in v0.92.0. |
| Preflight | `PreflightManagedGitCredentials` validates persisted repository identity and skips executor mode. | It does not prove SSH or HTTPS authentication. |
| Public recovery guide | The **Choose task Git credentials** section explains the current modes and entry points. | Keep durable task-access guidance concise; defer the temporary upgrade runbook. |

## Confirmed root cause and reproduction

The original column migration supplied managed mode to existing settings rows.
The current new-workspace initializer deliberately excludes existing installations.
The database stores the resulting policy without evidence of whether the user selected it.
A missing settings row also uses the managed compatibility fallback.

The smallest current reproduction uses an existing GitHub store with no workspace settings row.
Run `InitializeFreshWorkspaceDefaults`, then read the workspace settings. The policy remains managed.
`TestInitializeFreshWorkspaceDefaultsSkipsExistingGitHubStore` proves this behavior through the real SQLite store.
The public guide does not connect this behavior to the reported healthy-host-login symptom.

There is no production regression to correct within this package.
The documentation acceptance review currently fails because the historical explanation and recovery procedure are absent.
A code test that expects automatic policy conversion would contradict the current design.

## Scope

### In scope

- Explain the historical managed default and the current new-workspace default.
- Summarize task-access selection and the managed, user-managed, and remote-executor boundaries.
- Keep the public guidance concise and scan-friendly.
- Qualify the existing description of managed access as an opt-in policy for historical workspaces.

### Out of scope

- Automatic conversion of saved or missing policies to executor mode.
- Conditional policy selection based on App presence or temporary credential health.
- A new transport-authentication probe, launch warning, settings UI, or schema migration.
- A temporary upgrade-specific recovery runbook in the public guide.
- Reimplementation of #3069 or #3070.
- Editing generated changelog history or closing the entire issue automatically.

## Technical approach

Update the existing task-access section in `docs/public/integrations.md` with short bullets.
Keep workspace API authentication separate from credentials inside task processes.
Use **Workspace GitHub access**, **Change connection** when an automation connection exists, **Connect GitHub** otherwise, and **Inherit executor Git credentials** as the existing control labels.
Explain saved policy preservation, task-only executor selection, and the need for a new terminal after a launch or resume.
Do not add a temporary upgrade-specific recovery section; keep its investigation evidence in this plan.

Keep the origin, service-user, and preflight findings in this plan as evidence for a possible future
runbook. Do not publish those migration details in the current public guide.

The current specifications intentionally preserve existing policies.
This package follows that boundary and adds only documentation criterion `AC-INTEGRATIONS-GITHUB-AUTHENTICATION-001.13`.
No new ADR is necessary because the runtime boundary does not change.

## Tests

Existing source-backed evidence covers the preserved behavior:

- `workspace_defaults_test.go`: existing-store fallback, explicit managed preservation, and new executor defaults.
- `executor_resume_clone_transport_test.go`: policy-driven origins and prepared launch reconciliation.
- `executor_credentials_preflight_test.go`: executor mode skips managed identity admission.

All paths are under `apps/backend/internal/github` or `apps/backend/internal/orchestrator/executor` respectively.
The focused command passed in both packages during this investigation:

```bash
(cd apps/backend && go test -tags fts5 ./internal/github ./internal/orchestrator/executor -run 'TestInitializeFreshWorkspaceDefaultsSkipsExistingGitHubStore|TestInitializeWorkspaceDefaultsPersistsExecutorAndBindsActiveCLI|TestInitializeWorkspaceDefaultsPreservesExistingWorkspaceState|TestLaunchPreparedSession_ExistingWorkspace_ReconcilesGitHubOriginsBeforeAgentStart|TestEnsureRepoLocalPath_ReconcilesGitHubOriginForCredentialPolicy|TestPreflightManagedGitCredentialsSkipsWhenExecutorModePolicy' -count=1)
```

No new unit or browser test is needed for a documentation-only correction.
Task 01 owns the public document validation and manual acceptance checklist.

## Companion packages

The completed packages remain unchanged because this repair changes none of their implementation scope:

- [New workspace defaults](../new-workspace-github-access-defaults/plan.md).
- [Prepared workspace origins](../prepared-workspace-origin-reconciliation/plan.md).
- [Executor clone transport](../github-executor-clone-transport/plan.md).
- [Managed admission](../managed-git-credential-admission-repair/plan.md).

## Work orders

- [x] [Task 01: Document upgrade recovery](task-01-document-upgrade-recovery.md).

## Verification results

Investigation: the focused Go command passed in both packages.
Package validation passed: catalog validation (264 decisions, 818 specifications), specification lint, and `git diff --check`.
Public documentation implementation completed in `docs/public/integrations.md` and
`docs/public/use-kandev.md`. The integration guide now uses short bullets for historical saved
policies, conditional task-access entry points, managed and executor credentials, task-only
executor selection, fresh terminals, and managed, user-managed, and remote-executor boundaries.
The temporary upgrade-specific recovery section was removed from the public guide; its
investigation evidence remains here for a later runbook if needed.
Documentation validation passed: public-doc tests (62), public-doc validation (46 pages), and
the three work-order acceptance conditions.
Build validation passed: `make -C apps/backend build` and
`pnpm --filter @kandev/web build`.
Follow-up source review corrected the disconnected-workspace entry point and the fresh-terminal
recovery instruction; the public upgrade/recovery section was later removed as temporary guidance.
Review follow-up also clarified that historical workspaces may preserve either saved task Git
policy, aligned the plan's control labels with the UI, and completed the work-order touched-file
inventory. Focused post-fixup documentation validation passed: public-doc validation (46 pages),
catalog validation (264 decisions and 818 specifications), specification lint, and
`git diff --check`.
No temporary tests, instances, or database mutations were needed.

## Risks

Saved managed policies cannot distinguish migration defaults from deliberate user choices.
A blanket conversion can change credentials for working automation.
An App-only heuristic also excludes valid PAT and named CLI managed connections.
A host credential check cannot establish credential availability inside a remote executor.
The broader transport preflight proposal remains deferred and requires its own executor-aware design.
Completing this package addresses the documentation gap, not every proposal in #3071.

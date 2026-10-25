---
spec: docs/specs/integrations/requirements/github-authentication.md
created: 2026-07-28
status: complete
---

# Fix Plan: Honor Executor-Inherited GitHub Clone Transport

## Overview

PR #1810 made host materialization of provider-backed GitHub repositories use a workspace-scoped
credential and canonical HTTPS URL. PR #1985 later separated managed task credentials from
executor inheritance, but only changed the environment injected into task processes. The shared
managed checkout's `origin` therefore remains HTTPS and prevents SSH-oriented
`includeIf hasconfig:remote.*.url` rules from matching.

The repair keeps backend materialization authenticated by the workspace automation connection,
then reconciles only Kandev-managed GitHub checkout remotes to the selected task policy before
Local/Worktree preparation. User-managed local repositories and remote-executor clone contracts
remain unchanged.

## Confirmed Root Cause

`repoclone.Cloner.workspaceCloneAuth` intentionally converts every workspace GitHub clone to
canonical HTTPS so the backend can use the workspace automation credential.
`executor.configureGitHubCredentialBrokerForRepositories` honors
`task_git_credentials_mode=executor` by removing the broker helper, but
`executor.ensureRepoLocalPath` returns immediately for an existing valid managed checkout and
never reconciles its `origin`. A newly materialized checkout also retains the HTTPS URL used for
the authenticated clone.

The smallest reproduction is a provider-backed repository whose managed local path has
`origin=https://github.com/acme/widgets.git`, a host clone protocol of `ssh`, and workspace task
credential mode `executor`. Calling `ensureRepoLocalPath` leaves the HTTPS origin unchanged, so a
conditional include matching `git@github.com:acme/**` does not activate.

---

## Backend

### Managed checkout origin boundary

- Add an origin-update operation to `apps/backend/internal/repoclone/clone.go` and the
  `executor.RepoCloner` contract. It must invoke Git without a shell, terminate no user-controlled
  option position ambiguously, and return a credential-safe error.
- In `apps/backend/internal/orchestrator/executor/executor_resume.go`, resolve the workspace task
  Git credential policy before returning an existing provider-backed checkout and after a fresh
  materialization.
- For GitHub-managed checkouts, derive canonical HTTPS in `managed` mode and use
  `RepoCloner.BuildCloneURLWithHost` in `executor` mode so the host's startup-detected `gh` clone
  protocol selects SSH or HTTPS.
- Reconcile only repositories with provider-backed ownership. Preserve the existing
  `source_type=local` early return so user-owned repository remotes are never mutated.
- Treat a policy-resolution or remote-update error as repository preparation failure. Do not fall
  back to the stale transport.

## Tests

- **What:** an existing Kandev-managed GitHub checkout changes from HTTPS to the host-selected SSH
  URL in executor-inherited mode before `ensureRepoLocalPath` returns.
  **File:** `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`.
  **How:** filesystem-backed Git repository plus the real cloner origin updater and a fake
  non-secret policy resolver.
- **What:** switching back to managed mode changes an existing managed checkout from SSH to
  canonical HTTPS.
  **File:** `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`.
  **How:** table-driven companion case against the same real Git state boundary.
- **What:** a `source_type=local` repository keeps its original remote in either policy.
  **File:** `apps/backend/internal/orchestrator/executor/executor_resume_clone_transport_test.go`.
  **How:** real Git repository assertion after `ensureRepoLocalPath`.
- **What:** the repoclone origin updater changes `origin` and reports a missing/invalid repository
  error without leaking credentials.
  **File:** `apps/backend/internal/repoclone/clone_test.go`.
  **How:** focused real Git subprocess test.

## Documentation

- Update `docs/public/integrations.md` to explain that Local/Worktree executor inheritance uses the
  host-detected clone protocol for Kandev-managed checkout origins, allowing remote-URL conditional
  includes to match.
- Keep the existing setting and terminology. No frontend, API, persistence, or migration change is
  required.

## Implementation Task

- [x] [task-00-reconcile-managed-checkout-origin](task-00-reconcile-managed-checkout-origin.md) — done

Execution is sequential in the primary conversation. There are no parallel candidates.

## Risks And Out Of Scope

- Existing active worktrees share the managed repository's Git configuration. Reconciliation is
  workspace-policy-wide and occurs during later preparation; it does not rewrite user-managed
  local checkouts.
- Custom SSH host aliases are not inferred from conditional includes. Executor mode follows the
  existing `gh config get git_protocol` preference and canonical provider host construction.
- Remote Docker, SSH, and cloud executor clone URLs remain executor-specific and are not changed by
  this repair.

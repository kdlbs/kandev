---
id: "04-credential-and-push-routing"
title: "Credential and push routing"
status: completed
wave: 4
depends_on: ["03-runtime-contribution-materialization"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/remote-contribution-tasks.md"
---

# Task 04: Credential and Push Routing

## Acceptance

- Managed GitHub sessions receive a source-repository credential scope only when an exact valid fork
  binding belongs to the same task, session, and target attachment; unrelated or malformed scopes fail.
- Agentctl push uses the configured contribution remote and provider head branch, rejects force for a
  contribution, and leaves normal repositories on `origin` plus the local branch.
- A non-mutating push preflight runs with final managed/executor credentials before agent startup, with
  credential-safe errors, and existing create-PR/MR operations reuse the associated change.

## Verification

```bash
cd apps/backend
rtk go test ./internal/backendapp ./internal/github ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agentctl/server/process -run 'Test.*(BrokerScope|RemoteContribution|ContributionPush|PushPreflight|ExistingPR|ExistingMR)'
```

## Files likely touched

- `apps/backend/internal/backendapp/services.go`
- `apps/backend/internal/backendapp/services_github_broker_test.go`
- `apps/backend/internal/github/credential_broker.go`
- `apps/backend/internal/github/credential_broker_test.go`
- `apps/backend/internal/orchestrator/executor/executor_credentials.go`
- focused executor credential tests
- lifecycle launch/preparation files and tests
- `apps/backend/internal/agentctl/server/process/workspace_tracker.go`
- `apps/backend/internal/agentctl/server/process/git.go`
- `apps/backend/internal/agentctl/server/process/git_test.go`
- `apps/backend/internal/agentctl/server/process/git_pr_providers_test.go`

## Dependencies

Task 03's runtime contribution and push-target projection.

## Parallelism

Sequential security boundary. Implement credential authorization before enabling source-routed writes.

## Inputs

- Spec: **Permissions**, credential/preflight failures, no-force boundary.
- ADR: exact binding-authorized source scope and executor-owned credential separation.
- Existing patterns: GitHub broker scope authorization, executor scope JSON, `GitOperator.Push`, PR/MR
  association lookup, and credential-safe Git error sanitization.

## Risks

- Never authorize a source repository from caller input or an unvalidated metadata map.
- A GitHub source fork can use another owner; the target task-repository link alone is insufficient.
- `--dry-run` must use the exact eventual refspec and environment without mutating refs or logging secrets.

## Output contract

Report authorization proofs, push/preflight behavior, files changed, exact tests, blockers/risks,
divergence, and task/plan status updates.

## Completion

Implemented exact binding-authorized GitHub source scopes, executor credential separation, source-remote
and provider-head-branch push routing, force-push rejection, credential-safe dry-run preflight, and
existing PR/MR reuse. Ordinary repository pushes retain their `origin` and current-branch behavior.

The affected credential, lifecycle, agentctl, provider, and executor packages passed in the 17-package
backend suite: 5,603 tests. Temporary Git coverage confirms preflight does not mutate the source branch,
successful pushes advance only the contribution branch, and both GitHub and GitLab binding shapes reuse
the existing change URL.

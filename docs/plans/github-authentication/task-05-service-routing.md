---
id: "05-service-routing"
title: "Workspace-aware GitHub service routing"
status: completed
wave: 4
depends_on: ["02-app-token-primitives", "03-workspace-credential-resolver", "04-app-oauth-webhooks"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 05: Workspace-Aware GitHub Service Routing

## Inputs

- Spec: Identity And Routing, repository boundary, failure modes, and background/personal scenarios.
- ADR 0047: no process-global operational client and principal-scoped cache/rate state.
- Resolver contract from Tasks 02-04.

## Acceptance

- All GitHub domain operations resolve an explicit workspace/purpose; direct production use of the
  singleton `Service.client` and `Service.authMethod` is removed.
- Task, PR, issue, watch, cleanup, and CI paths derive and verify workspace ownership and fail closed
  when absent or contradictory; concurrent workspaces cannot share client, cache, rate, or
  singleflight state.
- Personal reads/mutations follow the specified fallback/attribution rules and enforce the
  automation-visible plus configured repository intersection before provider calls.
- App-backed review watches use a persisted verified target login rather than `@me`; migrated
  watches without one are disabled without a provider call.

## Files Likely Touched

- `apps/backend/internal/github/service.go`
- `apps/backend/internal/github/service_pr.go`
- `apps/backend/internal/github/service_pr_status.go`
- `apps/backend/internal/github/service_reviews.go`
- `apps/backend/internal/github/service_issues.go`
- `apps/backend/internal/github/service_accessible_repos.go`
- `apps/backend/internal/github/workspace_settings_service.go`
- `apps/backend/internal/github/service_pr_watch.go`
- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_cleanup.go`
- `apps/backend/internal/github/poller.go`
- `apps/backend/internal/github/service_ci_automation.go`
- `apps/backend/internal/github/service_task_issue.go`
- `apps/backend/internal/orchestrator/event_handlers_github*.go`
- `apps/backend/internal/automation/evaluator.go`
- `apps/backend/internal/workflowsync/service.go`
- `apps/backend/internal/task/share/` GitHub Gist call sites

## Verification

```bash
cd apps/backend && rtk go test ./internal/github
cd apps/backend && rtk go test ./internal/orchestrator ./internal/automation -run 'Test.*GitHub'
cd apps/backend && rtk rg 's\.client|s\.authMethod' internal/github
```

The final search may find only constructor/test compatibility code with an explicit justification;
it must find no operational service path.

## Output Contract

Report migrated call families, ownership derivation, actor routing, cache/rate keys, tests run, files
touched, blockers, and remaining compatibility references.

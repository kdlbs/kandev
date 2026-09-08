---
id: "03-system-identity-background-work"
title: "Per-org background work and system identity"
status: todo
wave: 2
depends_on: ["02-org-entity-and-identity"]
plan: "plan.md"
spec: "../../specs/multi-tenancy/spec.md"
---

# Task 03: Per-Org Background Work and System Identity

Critical path. This task changes what an identity-free context *means* across
the whole backend. Review it as a security change, not a refactor.

## Acceptance

- `authn.SystemIdentity(orgID)` exists, is the only constructor for background
  contexts, and is unreachable from any HTTP or WS code path (enforced by a
  test that greps the import graph or by package placement).
- With `features.multiTenancy` on, a context with no identity reaching any
  tenant-scoped service entry point is **denied** with the existing `*NotFound`
  sentinels and a warn-level log naming the caller. With the flag off, the
  existing "unscoped internal caller" behavior is byte-identical.
- Every background producer iterates active orgs and carries that org's system
  identity for the whole cycle: the orchestrator and its scheduler/queue/
  watchers, office schedulers and routines, the Jira / Linear / GitHub / GitLab
  / Azure DevOps / Sentry pollers and `healthpoll`, automation runners, workflow
  sync, PR status sync, and storage maintenance.
- A suspended org is skipped by every cycle without stopping the loop.
- One org's cycle failure does not abort another org's cycle.

## Verification

- `go test ./internal/... -run 'TestIdentityFree|TestPerOrgCycle'` from `apps/backend`
- `go test -race ./internal/orchestrator/... ./internal/office/... ./internal/jira/... ./internal/linear/... ./internal/github/... ./internal/gitlab/...`

## Files Likely Touched

- `apps/backend/internal/auth/authn/identity.go`
- `apps/backend/internal/task/service/service_access.go`
- `apps/backend/internal/orchestrator/` (scheduler, queue, watchers, watcher_dispatch)
- `apps/backend/internal/office/{scheduler,routines,runtime,service}/`
- `apps/backend/internal/{jira,linear,github,gitlab,azuredevops,sentry}/` pollers
- `apps/backend/internal/integrations/healthpoll/`
- `apps/backend/internal/workflowsync/`, `internal/automation/`

## Inputs

- Spec: What (background work runs per-org), Failure modes (identity-free
  denial, suspended org).
- Patterns: the poller `Start`/`Stop` + `goleak` conventions in
  `apps/backend/AGENTS.md`; `internal/mcp/scope`'s "never return an
  identity-free context from this path" rule, which this task generalizes.

## Output Contract

Enumerate every background entry point migrated, with a per-entry-point test
that runs it identity-free and asserts denial. Report which entry points were
found by grep versus by test failure, RED/GREEN commands, and set this task plus
its plan checkbox to done.

---
id: "02-read-capabilities"
title: "Read contributions and capabilities"
status: done
wave: 2
depends_on: ['01-management']
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002
acceptance_criteria:
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.1
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.2
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.3
  - AC-INTEGRATIONS-TASK-CHANGE-LINK-MCP-002.4
system_design:
  - ../../specs/integrations/system-design/task-change-link-mcp.md
---

# Task 02: Read contributions and capabilities

## Summary

Add the bound read tool and a small provider adapter at the composition boundary.
Return status, switches, prompts, and actual capabilities without altering task-list projections.

## In scope

- Add shared response DTOs and provider adapters using existing listers and automation read services.
- Expose provider-level prompt settings separately from association switches.
- Represent incomplete reads, unavailable providers, and unresolved/ambiguous historical identity explicitly.
- Resolve GitLab canonical identity only from verified unique persisted evidence.
- Keep discovery profile-gated; preserve existing list_tasks and list_related_tasks fields.

## Out of scope

Provider-store redesign, new UI, cross-provider replacement, and GitLab outcome tracking.
Do not change unrelated provider algorithms or historical transcripts.

## Acceptance

- GitHub-only, GitLab-only, mixed, empty, unavailable, and failed-provider fixtures return accurate status and capability values.
- GitLab outcome capability is false; no aggregate prompt conceals provider differences.
- Read calls reject public task/session fields, enforce backend principal binding, and preserve list projections.

## Verification

Run from the repository root. Add failing tests first, then implement and run these checks.
Use existing package fixtures; keep all new tests inside this work order's listed suites.

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp ./internal/task/dto ./pkg/api/v1 -count=1)
```

## Files likely touched

- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers.go`
- `apps/backend/internal/mcp/handlers/handlers.go`
- `apps/backend/internal/mcp/handlers/task_change_request.go (new)`
- `apps/backend/internal/backendapp/task_change_request_read.go (new)`
- `apps/backend/internal/backendapp/task_change_request_read_test.go (new)`
- `apps/backend/internal/mcp/handlers/task_pr_enrich_test.go`
- `apps/backend/pkg/websocket/actions.go`

## Dependencies

Task 01. Use its DTOs and validated backend boundary.

## Risks

Historical MR rows may have empty repository identity or multiple project paths for one canonical triple.

## Parallelism

`sequential`. Shared schemas and registration files prevent parallel ownership.

## Inputs

- [Requirements](../../specs/integrations/requirements/task-change-link-mcp.md), frontmatter IDs.
- [System design](../../specs/integrations/system-design/task-change-link-mcp.md), relevant contract sections.
- [Plan](plan.md), baseline, test matrix, and agent requests.
- Existing adjacent tests named in the plan and `apps/backend/AGENTS.md`.
- `.agents/skills/tdd/SKILL.md` and its backend testing reference before implementation.

## Results

Implemented the bound read contract and provider adapters behind the existing
task-list lister and automation service seams. Provider availability, read
failures, prompt settings, per-association switches, capability reasons, and
verified GitLab identity resolution are represented independently.

Validation passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/backendapp ./internal/task/dto ./pkg/api/v1 -count=1`
- `go test -race ./internal/backendapp -run 'TaskChange|ChangeRequest' -count=1`
- Read regressions cover GitLab connection failures, failed repository identity
  lookups, unresolved legacy identities, and principal binding before task
  existence lookup.

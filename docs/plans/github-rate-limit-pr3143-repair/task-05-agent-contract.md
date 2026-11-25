---
id: "05-agent-contract"
title: "Restore the operation-local contract"
status: done
wave: 5
depends_on: ["04-continuations"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-RATE-004
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-RATE-004.1
  - AC-INTEGRATIONS-GITHUB-RATE-004.2
  - AC-INTEGRATIONS-GITHUB-RATE-004.3
  - AC-INTEGRATIONS-GITHUB-RATE-004.4
system_design:
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
---

# Task 05: Restore the operation-local contract

## Summary

Remove the restored agent-facing snapshot tool. Retain safe rate details on failed managed operations and internal coordinator observations.

## In scope

- Remove tool registration, handler wiring, websocket action, and Office prompt entry.
- Remove orphaned snapshot DTOs and accessors only after a caller inventory.
- Replace tool-presence tests with negative surface assertions for both Kanban and Office.
- Keep sanitization coverage in the internal tracker and operation-response tests.
- Update automation-and-mcp.md, workflow-sync.md, and coverage.json with the implemented boundary.
- Preserve historical result receipts in the original package. Append supersession notes instead of rewriting old passing counts.

## Out of scope

Other work orders, external GitHub writes, and unrelated refactoring.

## Regression evidence

Add TestTaskProfilesDoNotExposeGitHubRateSnapshot before removing registration. Retain TestHTTPForceSyncReturnsRateLimitDetailsWithFailedOperation and TestHTTPForceSyncSuccessOmitsRateLimitDetails.

## Acceptance

- Neither agent profile advertises or dispatches get_github_rate_limit_kandev.
- Failed operations retain classified retry details. Success omits quota details.
- Public docs, specifications, prompts, and live tool registries describe one consistent boundary.

## Verification

Run from the repository root after implementation:

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers ./internal/github ./internal/workflowsync -count=1)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- apps/backend/config/prompts/office-context.md
- apps/backend/internal/backendapp/helpers.go
- apps/backend/internal/mcp/server/server.go
- apps/backend/internal/mcp/server/handlers.go
- apps/backend/internal/mcp/server/server_test.go
- apps/backend/internal/mcp/server/sysprompt_sync_test.go
- apps/backend/internal/mcp/server/github_rate_limit_test.go
- apps/backend/internal/mcp/handlers/handlers.go
- apps/backend/internal/mcp/handlers/github_rate_limit.go
- apps/backend/internal/mcp/handlers/github_rate_limit_test.go
- apps/backend/pkg/websocket/actions.go
- apps/backend/internal/github/service_rate_limit.go
- apps/backend/internal/github/service_rate_limit_test.go
- docs/public/automation-and-mcp.md
- docs/public/workflow-sync.md
- docs/public/coverage.json
- docs/plans/github-rate-limit-coordination/

## Dependencies

04-continuations. Execute sequentially.

## Risks

Do not remove internal coordinator state or provider admission. Historical references in this repair package do not imply a live tool.

## Parallelism

`sequential`

## Inputs

RATE-004. System design: Operation-local failure contract. Maintainer comment 5468064001 and original removal commit 03ed91826.

## Results

Removed the GitHub rate snapshot MCP registration, profile tool group, task-bound
handler wiring, WebSocket action, Office prompt entry, snapshot service DTOs and
accessors, and obsolete surface tests. Added
`TestTaskProfilesDoNotExposeGitHubRateSnapshot` for both Kanban and Office.
The coordinator remains internal. Added tracker coverage proving secondary
provider response text is reduced to the bounded classification. Existing
Workflow Sync response tests continue to cover rate details on failure and
their absence on success.

Updated the task and Office MCP catalog in `automation-and-mcp.md`, documented
the operation-local contract in `workflow-sync.md`, and removed the deleted
tool from `coverage.json`. The requirement and system design already specified
this boundary. Appended supersession notices to the original package without
changing its historical results.

Validation passed:

- `go test ./internal/mcp/server ./internal/mcp/handlers ./internal/github ./internal/workflowsync -count=1`
- `node --test scripts/validate-public-docs.test.mjs` (62 tests)
- `node scripts/validate-public-docs.mjs` (47 pages)
- `python3 scripts/list-docs.py validate` (316 decisions and 1203 specifications)
- `python3 scripts/lint-spec-files.py --all`
- `git diff --check`

The new Kanban/Office absence test first failed on both profiles before removal.

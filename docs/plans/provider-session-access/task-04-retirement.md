---
id: "04-retirement"
title: "Retire the old proxy and document the direct path"
status: pending
wave: 4
depends_on:
  - 02-host-lease
  - 03-plugin-adapter
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
acceptance_criteria:
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.4
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.7
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.8
system_design:
  - ../../specs/integrations/system-design/provider-session-access.md
---

# Task 04: Retire the old proxy and document the direct path

## Summary

Remove the branch-only server CI rerun service, grant/request tables and task
MCP tool after the direct plugin adapter passes its contract. Publish the
new Host grant/lease semantics and clear obsolete feature claims.

## In scope

- Remove `request_fresh_ci_run_kandev` registration, handler, provider
  mutation service, stale grant/request schema, and action-specific tests.
- Update public integration/automation docs, coverage, feature status,
  specifications, decisions, and plan status to the final behavior.
- Verify the old tool is absent in every MCP mode and no live consumer CI was
  triggered by tests.

## Out of scope

GitLab PAT exposure, merge, deployment, and duplicate Carlos notification.

## Acceptance

- No action-specific Kandev rerun/dispatch proxy remains in the final diff.
- Published docs describe exact scope, approval/revocation, provider limits,
  direct plugin calls, and GitLab fail-closure without claiming PR-scoped
  GitHub tokens.
- Fresh target tests, spec/doc validation, independent Review, distinct QA,
  and exact-head CI use the final normal-pushed branch head.

## Verification

```bash
(cd apps/backend && go test -race -count=1 ./internal/provideraccess ./internal/plugins ./internal/github ./internal/mcp/handlers ./internal/mcp/server ./internal/backendapp ./pkg/pluginsdk)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check upstream/main...HEAD
```

## Files likely touched

- `apps/backend/internal/github/service_ci_run_*` and `store_ci_run_*`
- `apps/backend/internal/mcp/handlers/ci_run_request.go`
- `apps/backend/internal/mcp/server/`
- `docs/public/automation-and-mcp.md`
- `docs/public/integrations.md`
- `docs/public/coverage.json`
- `docs/public/feature-status.md`
- `docs/specs/integrations/`

## Dependencies

Tasks 02 and 03; do not claim integrated behavior from Host tests alone.

## Risks

Deleting the old branch feature must preserve unrelated mobile E2E and
current-main changes. Use explicit paths, review the final diff, and do not
rewrite published history.

## Parallelism

`sequential`

## Inputs

- Old proxy diff against `upstream/main`.
- Plugin adapter exact-head contract/test receipt.

## Results

Pending.

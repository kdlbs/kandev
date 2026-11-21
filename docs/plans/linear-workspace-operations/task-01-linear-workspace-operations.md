---
id: "01-linear-workspace-operations"
title: "Resolve and authorize Linear workspace operations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WORKSPACE-RESOLUTION-001
  - REQ-INTEGRATIONS-LINEAR-WRITE-SCOPE-001
acceptance_criteria:
  - AC-INTEGRATIONS-WORKSPACE-RESOLUTION-001.1
  - AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.1
  - AC-INTEGRATIONS-LINEAR-WRITE-SCOPE-001.2
system_design:
  - ../../specs/integrations/system-design/linear-workspace-operations.md
---

# Task 01: Resolve and Authorize Linear Workspace Operations

## Summary

Resolve integration workspaces through the active workspace setting and the
oldest-workspace fallback. Require `task.write` before a user changes a Linear
issue state. Keep the plugin broker path unchanged.

## In scope

- Check PostgreSQL table visibility through the active `search_path`.
- Strengthen the workspace resolver regression test.
- Apply the Linear write check to HTTP and legacy WebSocket state changes.
- Map write denial to HTTP 403 and WebSocket `FORBIDDEN`.
- Add focused regression tests and document the user-visible permission.

## Out of scope

- Other provider write paths.
- Plugin broker authorization changes.
- UI changes.

## Acceptance

- A valid active workspace wins over an older fallback, including when the
  first schema in PostgreSQL `search_path` has no workspace tables.
- A user without `task.write` cannot send a Linear state mutation through HTTP
  or WebSocket; a user with `task.write` can use the selected workspace.
- The plugin broker continues to use its existing writeback method.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/linear ./internal/integrations/workspacescope ./internal/backendapp -run 'TestHTTPIssueDetailAndStateUseRequestedWorkspace|TestWSIssueStateWriteRequiresWorkspaceWriteScope|TestDefaultResolverPostgres' -count=1)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --input-type=commonjs -e 'const fs=require("fs"); const d=require("./.github/scripts/pr-docs.cjs"); const paths=["apps/backend/internal/backendapp/helpers.go","apps/backend/internal/integrations/workspacescope/workspacescope.go","apps/backend/internal/integrations/workspacescope/workspacescope_postgres_test.go","apps/backend/internal/linear/handlers.go","apps/backend/internal/linear/handlers_test.go","apps/backend/internal/linear/service.go","docs/specs/integrations/requirements/linear-workspace-operations.md","docs/specs/integrations/system-design/linear-workspace-operations.md","docs/plans/linear-workspace-operations/plan.md","docs/plans/linear-workspace-operations/task-01-linear-workspace-operations.md","docs/public/integrations.md"]; const fileContents=Object.fromEntries(paths.map(p=>[p,fs.readFileSync(p,"utf8")])); const result=d.validateCoverage({changedFiles:paths,fileContents}); console.log(JSON.stringify(result,null,2)); process.exitCode=result.ok?0:1;'
git diff --check
```

## Results

- Focused Go tests passed. The PostgreSQL test was skipped locally because
  `KANDEV_TEST_POSTGRES_DSN` was not set.
- Changed-package Go lint and repository commit hooks passed.
- `node --test scripts/validate-public-docs.test.mjs` passed all 63 tests.
- `node scripts/validate-public-docs.mjs`, `python3 scripts/list-docs.py
  validate`, and `python3 scripts/lint-spec-files.py --all` passed.
- The local PR documentation coverage preflight returned `covered` with no
  errors. `git diff --check` passed.

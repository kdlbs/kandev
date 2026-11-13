---
id: "02-document-executor-transition"
title: "Document the executor transition"
status: completed
wave: 2
depends_on: ["01-elect-detached-environments"]
plan: "plan.md"
requirements:
  - REQ-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003
  - REQ-TASKS-MCP-WORKSPACE-MODE-004
acceptance_criteria:
  - AC-TASKS-ADDITIONAL-SESSION-WORKSPACE-REUSE-003.5
  - AC-TASKS-MCP-WORKSPACE-MODE-004.2
system_design:
  - ../../specs/tasks/system-design/additional-session-workspace-reuse.md
  - ../../specs/tasks/system-design/mcp-workspace-mode.md
---

# Task 02: Document the executor transition

## Summary

Update the existing public workspace-mode guidance and MCP field description
after Task 01 establishes the final runtime behavior. Explain that an eligible
executor transition creates a separate child workspace and does not copy
uncommitted parent changes.

## In scope

- Update the inheritance guidance in `docs/public/automation-and-mcp.md` and
  `docs/public/coordination.md`.
- Update the `workspace_mode` description in
  `apps/backend/internal/mcp/server/server.go`.
- Keep the existing workspace-mode enum and same-executor inheritance contract.
- Confirm `docs/public/coverage.json` already maps the updated pages, tool, and
  changed source/test evidence; update it only if coverage ownership is missing.
- Ensure the changed work order and plan satisfy the PR documentation-coverage
  artifact requirements.

## Out of scope

- Adding a new public documentation page or changing `docs/public/meta.json`.
- Claiming that uncommitted parent files transfer to the fresh workspace.
- Changing executor or workspace behavior in this documentation work order.

## Acceptance

- Public pages and the MCP schema say that `inherit_parent` normally reuses the
  parent's materialized workspace, with a separate child-owned workspace for a
  new launch on a different non-Kubernetes executor.
- The copy explains that uncommitted parent changes are not copied and retains
  the Kubernetes and resume restrictions.
- The MCP `workspace_mode` schema description states the same transition
  contract, and its server test asserts the executor, isolation, and
  Kubernetes wording.
- Documentation coverage maps the updated public pages, MCP tool, implementation
  sources, and regression tests.
- Public-doc validation passes, and the current PR documentation-coverage status
  recognizes the changed plan/work order after the push.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
cd apps/backend && go test ./internal/mcp/server -run '^TestCreateTask_ToolSchema_HasParentID$' -count=1
```

## Verification results

- Passed `node --test scripts/validate-public-docs.test.mjs` (63 tests).
- Passed `node scripts/validate-public-docs.mjs` (55 published pages; coverage
  evidence and ownership validated).
- Passed `python3 scripts/list-docs.py validate` and
  `python3 scripts/lint-spec-files.py --all`.
- Passed the focused MCP schema regression test; it asserts the executor
  transition, child-owned workspace, uncommitted-file, and Kubernetes copy.
- Passed `gofmt -l` for all changed Go files and
  `git status --short | awk '{print $NF}' | rg '\\.go$' | xargs -r bash scripts/lint-go-changed`
  (`0 issues`).
- Passed `git diff --check`.
- The PR documentation-coverage status is revision-specific and remains to be
  checked after the parent pushes the reviewed head.

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/coordination.md`
- `docs/public/coverage.json` only if needed
- `apps/backend/internal/mcp/server/server.go`
- `apps/backend/internal/mcp/server/handlers_test.go`

## Dependencies

Task 01 establishes the behavior and regression tests before copy is published.

## Risks

- The docs must distinguish normal workspace inheritance from the executor
  transition without suggesting that the workspace mode changes.
- The PR documentation-coverage workflow is revision-specific; confirm the
  status against the exact pushed head.

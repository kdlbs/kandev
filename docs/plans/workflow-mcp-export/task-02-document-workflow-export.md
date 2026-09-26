---
id: "02-document-workflow-export"
title: "Document MCP workflow export"
status: done
wave: 2
depends_on: ["01-expose-workflow-export"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/external-mcp.md"
---

# Task 02: Document MCP Workflow Export

## Intent

Update the public External MCP reference and coverage metadata for `export_workflow_kandev`.

## Acceptance

- The public reference lists workflow export and states the correct live external tool count.
- The static-preview caveat identifies the new tool as omitted and non-authoritative.
- Public coverage metadata includes `export_workflow_kandev`, and both document validation commands pass.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs && node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/coverage.json`

## Dependencies

Task 01 must fix the final tool name, surface, and result contract.

## Parallelism

Sequential. This task follows the backend contract and changes only public documentation.

## Inputs

- `docs/specs/integrations/requirements/external-mcp.md`, API surface and scenarios.
- `docs/plans/workflow-mcp-export/plan.md`, Public documentation section.
- The final registered tool counts from Task 01.

## Risks

- Do not present the static settings preview as the live tool contract.
- Do not document a workspace-level batch tool.

## Output contract

Report the files changed, exact document validation results, remaining risks, and task status. Update this task and `plan.md` in the same conversation.

## Results

- Updated `docs/public/automation-and-mcp.md` with the live external MCP count of 36, the workflow export contract, and the non-authoritative static-preview omission list.
- Added `export_workflow_kandev` to `docs/public/coverage.json`.
- Verification: `node --test scripts/validate-public-docs.test.mjs` passed 61 tests with 0 failures.
- Verification: `node scripts/validate-public-docs.mjs` validated 41 published docs pages.

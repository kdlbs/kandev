---
id: "07-document-provider-scoped-task-mcp"
title: "Document provider-scoped task MCP"
status: done
wave: 4
depends_on: ["01-scope-task-mcp-tools-by-provider", "02-propagate-providers-through-agent-launch", "03-refresh-tools-after-source-attachment", "06-derive-mcp-handler-inventory"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/provider-aware-review-automation.md"
---

# Task 07: Document provider-scoped task MCP

## Acceptance

- Public automation/MCP documentation explains the GitHub-only, GitLab-only,
  mixed, and unsupported provider discovery behavior.
- Documentation states that successful live source additions can update the task
  MCP tool list and that backend validation remains authoritative.
- Spec and ADR indexes link the approved artifacts, and public-doc validation
  passes without describing unimplemented public payload changes.

## Verification

- `node --test scripts/validate-public-docs.test.mjs`
- `node scripts/validate-public-docs.mjs`
- `rtk rg -n 'get_task_(pr|mr)_automation_kandev|provider' docs/public/automation-and-mcp.md docs/specs/integrations/requirements/provider-aware-review-automation.md docs/decisions/2026-08-03-provider-scoped-task-mcp-tools.md`

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/specs/INDEX.md`
- `docs/decisions/INDEX.md`

## Dependencies

Tasks 01-03 must settle implemented runtime behavior. Task 06 settles the
observability language, although it does not add a public contract.

## Parallelism

Sequential final documentation pass after implementation behavior is verified.

## Inputs

- Approved focused spec and ADR
- Implemented provider mapping and live refresh semantics from Tasks 01-03
- Existing public automation/MCP terminology

## Risks

- Do not imply that hidden tools replace backend authorization.
- Do not document GitLab auto-fix/auto-merge or any public payload change.

## Output contract

Updated `docs/public/automation-and-mcp.md` with a provider-scoped review
automation section covering GitHub-only, GitLab-only, mixed, and empty or
unsupported provider sets. It documents live tool-list refresh after a
successful source attachment, best-effort failure/reconciliation semantics,
backend authorization as authoritative, and unchanged automation payloads.
The approved spec and ADR were already linked from their indexes during the
planning wave.

Behavior was cross-checked against the MCP membership tests, launch/resume
propagation tests, live refresh tests, and lifecycle endpoint tests completed
in Tasks 01-03.

Verification:

```text
node --test scripts/validate-public-docs.test.mjs
58 passed, 0 failed

node scripts/validate-public-docs.mjs
Validated 41 published docs pages.

rtk rg -n 'get_task_(pr|mr)_automation_kandev|provider' docs/public/automation-and-mcp.md docs/specs/integrations/requirements/provider-aware-review-automation.md docs/decisions/2026-08-03-provider-scoped-task-mcp-tools.md
60 matches in 3 files
```

Terminology risk: the provider-specific tools are discovery-scoped only;
documentation explicitly retains backend authorization and validation as the
runtime authority and does not describe GitLab auto-fix/auto-merge or payload
changes.

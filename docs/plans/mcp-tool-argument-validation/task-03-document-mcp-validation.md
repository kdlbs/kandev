---
id: "03-document-mcp-validation"
title: "Document fail-closed MCP arguments"
status: done
wave: 3
depends_on: ["01-enforce-registered-schemas", "02-create-task-prompt-compatibility"]
plan: "plan.md"
spec: "../../specs/integrations/requirements/mcp-tool-argument-validation.md"
---

# Task 03: Document fail-closed MCP arguments

## Acceptance

- Public MCP documentation states that required fields, types, constraints, and unknown top-level fields are validated before tool execution.
- Public documentation identifies `prompt` as canonical and explains the legacy `description` migration without adding the compatibility alias or repeated text to tool schemas.
- Public-doc validation passes without navigation changes.

## Verification

Run separately from the repository root:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/automation-and-mcp.md`

## Dependencies

- Tasks 01 and 02 establish the final shipped behavior.

## Parallelism

`sequential` — public guidance must match the completed protocol boundary.

## Inputs

- Spec API surface and failure modes.
- ADR decision and consequences.
- Existing MCP mode and troubleshooting guidance in `docs/public/automation-and-mcp.md`.

## Risks

- Keep the guidance concise and do not expand every tool description.

## Output contract

Report the file changed, both validation results, remaining documentation risks, and task/plan status updates in the primary conversation.

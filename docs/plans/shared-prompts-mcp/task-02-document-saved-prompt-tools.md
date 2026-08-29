---
id: "02-document-saved-prompt-tools"
title: "Document saved prompt tools"
status: done
wave: 2
depends_on:
  - "01-expose-saved-prompt-reads"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-MCP-002
acceptance_criteria:
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.1
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.2
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.3
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.4
  - AC-INTEGRATIONS-EXTERNAL-MCP-002.7
system_design:
  - ../../specs/integrations/system-design/external-mcp-shared-prompts.md
---

# Task 02: Document saved prompt tools

## Summary

Update the public MCP reference after the live tool contract is complete. Add
the two tools to the public coverage map.

## In scope

- Update the External MCP tool count and tool groups.
- Document list and get inputs and outputs.
- Update Configuration Chat saved prompt capabilities.
- Add both tools to public coverage data.

## Out of scope

- New public pages or navigation changes.
- UI screenshots.
- Saved prompt mutation documentation.

## Acceptance

- The public reference matches the live tool catalog and response contract.
- The Configuration Chat guide states that saved prompt reads are available.
- Public documentation validation succeeds.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```

Run these commands from the repository root.

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/developer-tools.md`
- `docs/public/coverage.json`

## Dependencies

- Task 01 must define the final live tool contract.

## Risks

- An incorrect exact tool count can make the public reference stale.

## Parallelism

`sequential`

## Inputs

- `REQ-INTEGRATIONS-EXTERNAL-MCP-002`
- `docs/specs/integrations/system-design/external-mcp-shared-prompts.md`
- The implemented tool schemas and result shapes from Task 01.

## Results

Updated the External MCP reference with the 42-tool catalog, saved prompt
request and response examples, exact-name behavior, and read-only limits.
Updated Configuration Chat capability text and assigned both tools in the
public coverage map.

- `node --test scripts/validate-public-docs.test.mjs` passed with 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published docs pages.
- `git diff --check -- docs/public` passed.

---
id: "04-document-explorer"
title: "Document the explorer"
status: done
wave: 3
depends_on: ["02-build-responsive-explorer"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
---

# Task 04: Document the Explorer

## Acceptance

- The MCP explanation tells users how to open the server explorer and inspect
  the Kandev tool catalog.
- The docs explain that third-party catalogs are unavailable because those
  servers connect directly to the agent.
- The profile troubleshooting steps use the final dialog and drawer labels.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 02 supplies the final user-facing labels and behavior.

## Parallelism

Parallel-safe with Task 03 after Task 02. This task owns only public docs.

## Inputs

- Spec sections `Kandev tool catalog` and `User experience`.
- ADR `2026-08-16-session-mcp-tool-catalog`.
- Final UI labels from Task 02.

## Output contract

Report the updated explanation sections, public-doc validation, files changed,
blockers, and risks. Update this task and the plan status in the same session.

## Results

Updated the public MCP guidance to name the **MCP servers** explorer and its
wide desktop dialog and full-height touch drawer. The guidance explains that
Kandev lists its own tool names and descriptions after `tools/list`, while
third-party catalogs remain unavailable because those servers connect directly
to the agent.

Verification:

- `node --test scripts/validate-public-docs.test.mjs`: 61 passed.
- `node scripts/validate-public-docs.mjs`: validated 41 published docs pages.

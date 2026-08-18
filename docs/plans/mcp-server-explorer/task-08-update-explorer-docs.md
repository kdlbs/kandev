---
id: "08-update-explorer-docs"
title: "Update explorer documentation"
status: pending
wave: 3
depends_on: ["06-refine-explorer-ux"]
plan: "plan.md"
spec: "../../specs/mcp-session-observability/spec.md"
---

# Task 08: Update Explorer Documentation

## Acceptance

- Public MCP guidance describes the server, tool-list, and tool-detail levels.
- The guidance explains that token values are `o200k_base` estimates, not
  provider or billing counts.
- The guidance explains schema limits and the unchanged third-party catalog
  boundary.
- Troubleshooting uses the final labels and Back navigation.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 06 supplies the final labels and behavior.

## Parallelism

Parallel-safe with Task 07. This task owns public documentation files.

## Inputs

- Spec sections `Kandev tool catalog` and `User experience`.
- ADR `2026-08-18-session-mcp-tool-definition-details`.
- Final UI labels from Task 06.

## Output contract

Report the changed guidance, validation results, files, blockers, and risks.
Update this task and the plan status in the same session.

## Results

Pending.

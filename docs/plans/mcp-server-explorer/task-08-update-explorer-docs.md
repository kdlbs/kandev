---
id: "08-update-explorer-docs"
title: "Update explorer documentation"
status: done
wave: 3
depends_on: ["06-refine-explorer-ux"]
plan: "plan.md"
spec: "../../specs/platform/requirements/mcp-session-observability.md"
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

Updated both planned public guides. **Automation and MCP** now explains the
server, tool-list, and tool-detail levels on desktop and touch devices. It
documents both Back labels, independent list scrolling, argument display,
schema states, storage limits, and `~N tokens`.

The token guidance identifies `o200k_base` and states that the value is not a
provider context count or billing count. The third-party guidance keeps the
reviewed boundary: Kandev shows owned status metadata but cannot inspect a
direct server's tools, schemas, descriptions, or estimates.

The **Agents and Profiles** troubleshooting step now follows the final labels
and navigation. It also directs users to inspect the affected session and
distinguishes Connected, Active, Delivered, gray omissions, and red errors.

Verification passed from the repository root:

```text
node --test scripts/validate-public-docs.test.mjs
61 passed

node scripts/validate-public-docs.mjs
Validated 41 published docs pages.
```

No blockers or new documentation risks remain.

---
id: "06-authoring-docs"
title: "Plugin agent-tool documentation"
status: done
wave: 5
depends_on: ["05-end-to-end-fixture"]
plan: "plan.md"
spec: "../../specs/plugins/requirements/agent-tools.md"
adr: "../../decisions/2026-08-11-plugin-tools-through-kandev-mcp.md"
---

# Task 06: Plugin agent-tool documentation

## Acceptance

- Public manifest and authoring guides document declarations, SDK method,
  derived names, surfaces, defaults, limits, bound context, capabilities,
  timeout/no-retry behavior, and a complete minimal example.
- Automation/MCP and feature-status docs explain live discovery, client
  list-changed compatibility, reconnect fallback, and that plugins use Kandev's
  MCP server rather than a separate profile server.
- The core plugin spec no longer presents all plugin agent tools as unsupported;
  links and terminology agree across public docs, spec, ADR, and implementation.

## Verification

```bash
git diff --check && make -C apps/backend build
```

## Files likely touched

- `docs/public/plugins-manifest.md`
- `docs/public/plugins-authoring.md`
- `docs/public/automation-and-mcp.md`
- `docs/public/feature-status.md`
- `docs/specs/plugins/requirements/plugins.md`
- `docs/specs/plugins/requirements/agent-tools.md`
- `apps/backend/AGENTS.md`

## Dependencies

Task 05.

## Parallelism

Sequential. Documentation must reflect the verified final contract.

## Inputs

- Completed Tasks 01-05 and their recorded verification results
- `/docs-maintainer` guidance
- Canonical plugin authoring guide and manifest reference

## Risks

- Examples that omit conservative annotations, cancellation, or Host capability
  requirements could encourage unsafe plugins.
- Do not describe hot reload as universal client behavior; distinguish server
  replacement from client refresh support and plugin binary upgrades.

## Output contract

Report updated public contracts and examples, links checked, commands/results,
remaining client-compatibility caveats, and task/plan status updates.

## Results

- Documented the agent-tool manifest fields, SDK handler, derived MCP names,
  supported task surfaces, conservative annotations, bound context, limits,
  timeout/no-retry behavior, and a complete echo-tool example.
- Updated automation/MCP and feature-status documentation with live catalog
  replacement, `tools/list_changed` client compatibility, and reconnect
  fallback behavior.
- Updated the core plugin spec, feature spec, ADR index, and backend authoring
  guidance so terminology and contract links agree.
- Verification: `git diff --check && make -C apps/backend build` passed. The
  cross-platform build emitted only the expected unsigned Darwin binary
  warnings in this Linux environment.

---
id: "04-document-negotiated-mcp-support"
title: "Document negotiated MCP support"
status: done
wave: 3
depends_on:
  - "02-preserve-stateless-mcp-observability"
  - "03-protect-mcp-client-route-compatibility"
plan: "plan.md"
spec: "../../specs/agents/requirements/mcp-protocol-compatibility.md"
---

# Task 04: Document Negotiated MCP Support

## Inputs

- The completed behavior from Tasks 01 through 03.
- `REQ-AGENTS-MCP-PROTOCOL-001`.
- `docs/public/automation-and-mcp.md`.
- `docs/public/feature-status.md`.
- The `docs-maintainer` and `simple-english` skills.

## Change

1. Update the public MCP guide with the exact supported protocol versions and
   existing routes.
2. Explain that compatible clients can negotiate `2026-07-28` on `/mcp` and
   that legacy clients keep their current behavior.
3. State that client-side automatic negotiation can require an explicit client
   option. Do not claim that every v2 SDK client enables it by default.
4. Explain that configured third-party MCP servers negotiate directly with
   the agent. Kandev does not upgrade or proxy them.
5. Keep the feature-status table consistent with the shipped behavior.
6. List the exclusions: MCP Tasks, new OAuth behavior, and third-party proxying.

## Acceptance

- Public docs identify `2026-07-28` and the legacy compatibility path.
- The docs describe one `/mcp` endpoint and retained SSE routes.
- The docs do not call the protocol simply “MCP v2” without the date version.
- The docs distinguish Kandev-owned servers from configured third-party
  servers.
- The feature-status page matches the implemented and tested state.

## Verification

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check -- docs/public
```

## Files likely touched

- `docs/public/automation-and-mcp.md`
- `docs/public/feature-status.md`

## Dependencies

Tasks 02 and 03. Documentation must describe the verified final behavior.

## Parallelism

Sequential. This task uses the results from every implementation work order.

## Output contract

Report the documented versions, routes, compatibility notes, exclusions,
files changed, commands run, failures, and any user-facing caveats. Update this
work order and `plan.md` with the result.

## Result

- Updated `docs/public/automation-and-mcp.md` with modern `2026-07-28`, legacy `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05` support.
- Documented `/mcp`, agentctl `/sse` and `/message`, external `/mcp/sse` and `/mcp/message`, automatic-negotiation limits, per-request authentication, and direct third-party delivery.
- Listed the exclusions for MCP Tasks, new OAuth behavior, and third-party proxying.
- Updated the feature-status row to match modern negotiation, legacy SSE, and authentication behavior.
- Verification passed: all 61 public-doc tests, live validation of 41 published pages, and `git diff --check -- docs/public`.

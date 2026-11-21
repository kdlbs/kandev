---
id: "06-manifest-author-urls"
title: "Keep manifest author URLs out of MCP endpoints"
status: done
wave: 6
depends_on: ['05-native-identity']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.2
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Task 06: Keep manifest author URLs out of MCP endpoints

## Summary

Plugin manifests publish an author homepage. Discovery was treating that URL as an MCP server and never reading the plugin's `mcp.json`. Use the declared MCP endpoints instead.

## In scope

- Skip bare server parsing for a plugin manifest that has no `mcpServers` declaration.
- Continue to `.mcp.json` and `mcp.json` in the same plugin root.
- Cover Stripe and Resend layouts: `author.url` in `.cursor-plugin/plugin.json`, real endpoint in `mcp.json`.

## Out of scope

Installing plugins, OAuth token storage, and changing native `plugin-<plugin>-<server>` names.

## Acceptance

1. An enabled plugin whose manifest author URL is `https://stripe.com` and whose `mcp.json` declares `https://mcp.stripe.com` imports the `mcp.json` endpoint.
2. The same rule applies to Resend (`https://resend.com` versus `https://mcp.resend.com/mcp`).
3. A manifest that declares `mcpServers` still supplies those servers. A bare server map that is not a plugin manifest still parses.

## Verification

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig/ -count=1 -run 'TestDiscoverCursorPluginMCPServers|TestCursorPluginCacheWithoutEnabledState')
```

Result: passed.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/cursor_plugins.go`
- `apps/backend/internal/agent/mcpconfig/cursor_plugins_test.go`
- `docs/specs/agents/requirements/cursor-plugin-mcp-import.md`
- `docs/specs/agents/system-design/cursor-plugin-mcp-import.md`

---
id: "02-native-inventory"
title: "Discover enabled native plugins and source-repository disables"
status: done
wave: 2
depends_on: ['01-authenticated-source']
plan: "plan.md"
requirements:
  - REQ-AGENTS-CURSOR-PLUGIN-MCP-001
acceptance_criteria:
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.1
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.6
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.11
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.12
  - AC-AGENTS-CURSOR-PLUGIN-MCP-001.13
system_design:
  - ../../specs/agents/system-design/cursor-plugin-mcp-import.md
---

# Discover enabled native plugins and source-repository disables

## Scope and files

Own new cursor_inventory.go, cursor_inventory_test.go,
cursor_inventory_credentials_darwin.go, cursor_inventory_credentials_other.go,
cursor_disabled.go and cursor_disabled_test.go in internal/agent/mcpconfig;
update cursor_plugins.go and its tests to accept an explicit native inventory.
Use the existing Go standard HTTP/exec boundaries; no Node helper or UI scraper.

Implement the versioned service/credential contract in the design. Remove the
invented installed-registry fallback for native marketplace roots. Keep local
explicit and global paths separate. Select exact contained immutable revisions;
unsupported or missing revisions never select another cached directory.

Read only the explicitly supplied canonical source-repository and target
workspace disable stores. No scan of unrelated disabled files. The caller owns
trusted source-repository resolution. Bound reads to one MiB per disable file.

## Acceptance

1. Enabled exact cached Atlassian revision is discovered while disabled plugins,
   wrong marketplace/version, path escapes and unsupported shapes are skipped.
2. Source/task native disables filter automatic imports; an unrelated workspace
   disable has no effect. Missing files are empty; unsafe/unreadable/invalid
   selected stores make automatic discovery unavailable.
3. Import-disabled/ineligible launches never read account credentials or request
   inventory. Service authentication, redirect, timeout, oversize or schema
   failure expose no secrets and do not activate stale cache entries.

## TDD and validation

Use synthetic Connect JSON fixtures shaped like EffectivePlugin responses and
an injected credential reader/HTTP transport. RED tests:
TestCursorInventorySelectsEnabledExactRevision,
TestCursorInventoryRejectsRedirectAndSecretErrors,
TestCursorMCPDisablesUseSourceRepositoryOnly.
Include disabled=false/omitted flags, empty successful inventory vs errors,
pinned revision precedence, source symlinks, two marketplaces with one plugin
name, cancellation, concurrent callers and no-auth negative cases with positive
controls. Assert exact destination origin and no Authorization on redirects.

From apps/backend:

- go test ./internal/agent/mcpconfig -run 'CursorInventory|CursorMCPDisables|CursorPlugin' -count=1
- go test -race ./internal/agent/mcpconfig -count=1
- golangci-lint run ./internal/agent/mcpconfig/...

Unsupported OS credential readers must compile and return unsupported; they
must not scan an unrelated credential store. No live API calls in tests.

## Dependencies and exclusions

Task 01 supplies credential behavior, but its OAuth tokens are never used for
the inventory RPC. Exclude login/refresh, settings UI, downloads, plaintext
account-token persistence and editing Cursor disable/approval files.

## Results

Inventory and native disable readers are implemented. Synthetic EffectivePlugin
fixtures verify exact enabled revisions, fixed-origin requests, bounded and
sanitized failures, no-auth behavior, marketplace ambiguity, exact native
disable stores, and fail-closed selected-store errors. A conflicting repeated
marketplace-ID regression failed before the parser was hardened and now passes.
Focused and full tests, race tests, and lint all pass:

- `go test ./internal/agent/mcpconfig -run 'CursorInventory|CursorMCPDisables|CursorPlugin' -count=1`
- `go test ./internal/agent/mcpconfig -count=1`
- `go test -race ./internal/agent/mcpconfig -count=1`
- `golangci-lint run ./internal/agent/mcpconfig/...`

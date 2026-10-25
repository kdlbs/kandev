---
created: 2026-08-28
status: done
requirements:
  - REQ-INTEGRATIONS-EXTERNAL-MCP-002
system_design:
  - ../../specs/integrations/system-design/external-mcp-shared-prompts.md
legacy_specs: []
---

# Implementation Plan: Shared Prompts MCP

## Overview

Add two read-only saved prompt tools to configuration and external MCP. The
backend tool work comes first because the public reference must describe the
final live contract.

## Scope

### In scope

- List saved prompt summaries without content.
- Read one saved prompt by its exact name.
- Register both tools only on configuration and external MCP surfaces.
- Use the current prompt service, repository order, authentication, and backend
  bridge.
- Update the configuration-agent context and public MCP reference.

### Out of scope

- Saved prompt create, update, or delete tools.
- Automatic expansion in workflow-step list results.
- Prompt storage, ownership, or reference-expansion changes.
- Frontend behavior or UI changes.

## Technical approach

### Prompt read boundary

Add `GetPromptByName` to `internal/prompts/service`. The method trims the name
and maps an absent row to `ErrPromptNotFound`.

Add a narrow prompt-reader dependency to `internal/mcp/handlers`. Wire the
existing prompt service through `internal/backendapp/helpers.go`.

### Backend actions and handlers

Add `ActionMCPListSharedPrompts` and `ActionMCPGetSharedPrompt` in
`pkg/websocket/actions.go`. Register both actions only when the prompt reader is
available.

Place the backend handlers in `internal/mcp/handlers/config_prompt_handlers.go`.
The list handler returns name, built-in status, and UTF-8 byte size. The get
handler returns the full read result without the internal prompt ID.

### MCP tool surface

Add a saved prompt tool group in `internal/mcp/server/config_handlers.go`.
Register it for the same configuration and external predicates as other
configuration groups.

Use the existing backend forwarding path. Add all four read-only MCP
annotations to both tools.

Update `config/prompts/config-context.md` with the exact names, inputs, and
read-only behavior.

### Public documentation

Update the External MCP reference in `docs/public/automation-and-mcp.md`. Add
the saved prompt tools and change the external tool count from 40 to 42.

Update the Configuration Chat text in `docs/public/developer-tools.md`. State
that configuration agents can list and read saved prompts.

Add both tool names to the MCP coverage list in `docs/public/coverage.json`.
The primary content type remains reference.

## Tests

- `AC-INTEGRATIONS-EXTERNAL-MCP-002.1`, `.7`: MCP server surface and tool-count
  tests.
- `AC-INTEGRATIONS-EXTERNAL-MCP-002.2`, `.3`, `.4`: backend handler and server
  forwarding tests.
- `AC-INTEGRATIONS-EXTERNAL-MCP-002.5`: service, backend, and server error tests.
- `AC-INTEGRATIONS-EXTERNAL-MCP-002.6`: handler wiring and existing transport
  context propagation.
- `AC-INTEGRATIONS-EXTERNAL-MCP-002.8`: focused prompt service and resolver tests
  remain green without resolver changes.

## E2E tests

No browser E2E test is necessary. The change adds a backend MCP contract and
documentation. It does not change rendered UI behavior.

## Work orders

- [x] [Task 01: Expose saved prompt reads](task-01-expose-saved-prompt-reads.md)
- [x] [Task 02: Document saved prompt tools](task-02-document-saved-prompt-tools.md)

## Verification results

Implemented the two read-only saved prompt tools on configuration and external
MCP surfaces. The service, backend bridge, server catalog, configuration-agent
context, public MCP reference, and coverage map now share the same contract.

- `go test ./internal/prompts/service ./internal/mcp/handlers ./internal/mcp/server ./internal/sysprompt ./pkg/websocket` passed with 979 tests.
- `go test -race ./internal/prompts/service ./internal/mcp/handlers ./internal/mcp/server` passed with 914 tests.
- `python3 scripts/lint-spec-files.py --all` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed with 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published docs pages.
- `git diff --check -- docs/public` passed.

## Risks

- Saved prompt content can contain operator conventions. The tool must remain
  on authenticated configuration surfaces.
- The list result can expose content if a handler reuses the existing full
  prompt DTO. A dedicated summary type prevents that error.
- Tool-count tests and public docs use exact counts. Both must change with the
  two new tools.

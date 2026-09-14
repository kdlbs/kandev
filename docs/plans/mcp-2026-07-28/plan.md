---
spec: docs/specs/agents/requirements/mcp-protocol-compatibility.md
design: docs/specs/agents/system-design/mcp-protocol-compatibility.md
decision: docs/decisions/2026-08-30-dual-era-mcp-protocol.md
created: 2026-08-30
status: complete
---

# Implementation Plan: MCP 2026-07-28 Compatibility

## Objective

Serve MCP `2026-07-28` and legacy MCP from each existing Kandev endpoint. Let
clients select their supported era without changing tool behavior,
authorization, third-party server delivery, or legacy SSE support.

## Current state

- Kandev uses `github.com/mark3labs/mcp-go v1.0.0-beta.1` for modern `2026-07-28` and legacy protocol support.
- The dependency supports legacy `2025-11-25`, `2025-06-18`, `2025-03-26`, and `2024-11-05` negotiation.
- All Kandev tool profiles share `apps/backend/internal/mcp/server`.
- Agentctl and passthrough injection already prefer `/mcp` and retain SSE.
- The external MCP endpoint authenticates each request with a personal access
  token.
- The Jira MCP client pins `2025-06-18`. The mock agent uses legacy `2024-11-05` over SSE.
- Third-party configured MCP servers connect directly to agents.

## Decision summary

Use one negotiated `/mcp` endpoint. Keep mark3labs and upgrade to its v1 line.
Retain legacy initialize and SSE. Do not add a runtime feature flag, a second
endpoint, the MCP Tasks extension, or a Kandev proxy for third-party servers.

The current implementation target is `mcp-go v1.0.0-beta.1`. Task 01 must
check for a newer compatible v1 release before it changes the dependency. If
no acceptable v1 release exists, stop and revise the dependency decision. Do
not implement the modern wire contract inside Kandev.

## Work packages

| Order | Work order | Result | Depends on |
| --- | --- | --- | --- |
| 1 | [Serve dual-era MCP](task-01-serve-dual-era-mcp.md) | The shared server negotiates modern and legacy requests. | None |
| 2 | [Preserve stateless MCP observability](task-02-preserve-stateless-mcp-observability.md) | Modern evidence is attempt-owned and legacy evidence stays connection-owned. | Task 01 |
| 2 | [Protect MCP client and route compatibility](task-03-protect-mcp-client-route-compatibility.md) | Existing clients, routes, auth, and passthrough behavior have regression coverage. | Task 01 |
| 3 | [Document negotiated MCP support](task-04-document-negotiated-mcp-support.md) | Public docs explain negotiation, compatibility, and exclusions. | Tasks 02 and 03 |

Tasks 02 and 03 can run in parallel if their owners do not edit the same MCP
server test file. Task 02 owns observability hooks and evidence. Task 03 owns
route, client, and injection regression tests.

## Compatibility matrix

The completed change must prove these cases:

| Case | Expected result |
| --- | --- |
| Modern automatic client to Kandev | Discovery selects `2026-07-28`; list and call succeed. |
| Modern direct client to Kandev | A valid request succeeds without discovery. |
| Legacy HTTP client to Kandev | Initialize, list, and call keep legacy behavior. |
| Legacy SSE client to Kandev | Initialize, list, and call keep working. |
| Modern and legacy clients at the same time | Each request uses its selected era. |
| Modern automatic client to a legacy-only fixture | The client selects a shared legacy version or uses its documented fallback. |
| Authenticated external modern requests | Each stateless request resolves the same caller and tool permissions. |
| Dynamic plugin tools | Modern tool-list metadata prevents shared or stale caching. |

## Requirement and acceptance mapping

| Requirement or criterion | Work orders |
| --- | --- |
| `REQ-AGENTS-MCP-PROTOCOL-001` | 01, 02, 03, 04 |
| `AC-AGENTS-MCP-PROTOCOL-001.1` through `.4` | 01 |
| `AC-AGENTS-MCP-PROTOCOL-001.5` | 01, 03 |
| `AC-AGENTS-MCP-PROTOCOL-001.6` and `.7` | 03 |
| `AC-AGENTS-MCP-PROTOCOL-001.8` | 01 |
| `AC-PLATFORM-MCP-SESSION-OBSERVABILITY-001.4` and `.9` | 02 |

## Integrated verification

After all work orders finish, run:

```bash
make -C apps/backend test
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Risks and controls

- **Prerelease SDK:** Focused protocol tests cover discovery, direct modern
  requests, legacy HTTP, SSE, hooks, and concurrent eras. Implementation must
  prefer a compatible stable v1 if one exists.
- **SDK API changes:** The v1 hook API returns a general tool result. Task 01
  adapts the hook without weakening result or error handling.
- **Stateless authorization:** External MCP resolves auth on each request.
  Task 03 proves that no session-local identity is required.
- **False observability:** Task 02 does not create a connection ID or close
  event for modern HTTP requests.
- **Dynamic tool caching:** The modern server advertises private, zero-lifetime
  tool-list caching so tool permissions and plugin tools stay current.

## Exclusions

- MCP Tasks extension support.
- New MCP OAuth or authorization behavior.
- Migration to `github.com/modelcontextprotocol/go-sdk`.
- A `/mcp/v2` route or runtime feature flag.
- Protocol proxying for third-party MCP servers.
- Modernizing the Jira MCP client or mock-agent SSE client.

## Result

- Implemented one negotiated `/mcp` endpoint for modern `2026-07-28` and legacy initialize clients, while retaining legacy SSE routes.
- Added attempt-owned modern observability, connection-owned legacy observability, per-request external PAT coverage, route regressions, cache-hint coverage, and the legacy mock-agent SSE pin.
- Updated the public MCP guide and feature-status boundary with protocol versions, routes, authentication, direct third-party delivery, and exclusions.
- Integrated verification passed: `make -C apps/backend test` with the task-workspace bootstrap configuration unset and a temporary home directory, 363 MCP server race tests, 61 public-doc tests, 41 published-doc validation checks, 20 specification-lint tests, full specification lint, and `git diff --check`. An ambient run first failed because the task process forced `/root/.kandev/config.yaml` into config and launcher discovery; the clean rerun passed without code changes.
- The implementation uses `github.com/mark3labs/mcp-go v1.0.0-beta.1`; the prerelease SDK remains the primary release risk.

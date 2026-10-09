---
id: "01-operator-registered-acp-agents"
title: "Let an operator-registered agent declare and run ACP"
status: in_progress
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CUSTOM-ACP-001
acceptance_criteria:
  - AC-AGENTS-CUSTOM-ACP-001.1
  - AC-AGENTS-CUSTOM-ACP-001.2
  - AC-AGENTS-CUSTOM-ACP-001.3
  - AC-AGENTS-CUSTOM-ACP-001.4
  - AC-AGENTS-CUSTOM-ACP-001.5
  - AC-AGENTS-CUSTOM-ACP-001.6
  - AC-AGENTS-CUSTOM-ACP-001.7
system_design:
  - ../../specs/agents/system-design/custom-acp-agents.md
---

# Task 01: Operator-registered ACP agents

## Summary

Make the discovery sweep read the live agent registry, add a protocol to the operator-registered
definition, and let the capability probe spawn the command that definition names.

## In scope

- `internal/agent/discovery`: resolve agents per sweep, generation-fence the result cache, drop the
  dead `KnownAgent`/`Definitions()` capture.
- `internal/agent/settings/controller`: invalidate after each registry mutation; persist the protocol;
  seed an ACP profile as non-passthrough and probe it; share one spec builder.
- `internal/agent/agents`: add `CustomACPAgent`; mark its command operator-defined.
- `internal/agent/registry`: build the agent the protocol names; reject unknown protocols and an
  ACP-plus-strategy pair.
- `internal/agentctl/server/utility`: `resolveSpawnCommand` accepts an operator-defined command.
- `apps/web`: protocol selector; hide the model and MCP fields for ACP; stop calling the dialog a TUI
  agent.

## Acceptance

- **AC-AGENTS-CUSTOM-ACP-001.1** Creating an agent makes it appear in Installed Agents and deleting it
  makes it disappear, with no backend restart and without Rescan re-detecting the deleted binary.
- **AC-AGENTS-CUSTOM-ACP-001.2** A membership change is visible before the cache TTL, and a sweep that
  started before the invalidation does not publish its results.
- **AC-AGENTS-CUSTOM-ACP-001.3** A definition with no protocol still registers a `*TUIAgent` and is
  still classified passthrough-only.
- **AC-AGENTS-CUSTOM-ACP-001.4** An ACP definition registers an agent with `Runtime().Protocol = ACP`
  that implements `InferenceAgent`, is not passthrough-only, and offers no passthrough config.
- **AC-AGENTS-CUSTOM-ACP-001.5** Creating an ACP agent seeds one profile with `CLIPassthrough = false`
  and an empty model, and starts a capability probe for that agent only.
- **AC-AGENTS-CUSTOM-ACP-001.6** An unknown protocol and an ACP-plus-MCP-strategy pair are both
  refused, at creation and on the MCP-strategy route, with no row written.
- **AC-AGENTS-CUSTOM-ACP-001.7** The probe spawns an operator-defined command and still refuses an
  unlisted command that is not operator-defined.

## Verification

- RED/GREEN Go: `internal/agent/discovery`, `internal/agent/registry`, `internal/agent/agents`,
  `internal/agent/settings/controller`, `internal/agentctl/server/utility`.
- Web: `add-tui-agent-dialog.test.tsx`, `custom-tui-mcp-mount.test.tsx`.
- E2E: `e2e/tests/settings/custom-acp-agent.spec.ts`.
- `make lint-backend`, web `typecheck`, web `i18n:check`.

## Not verified

No real ACP session has completed end to end. The CLI used during development fails `session/new`
against its own model-catalogue service in every mode, reproduced outside Kandev with `acpdbg` and with
a hand-written JSON-RPC client, so the spawn, `initialize`, `authenticate`, and `session/new` path is
confirmed correct but the last mile waits on that service.

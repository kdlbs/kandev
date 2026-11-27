---
id: "01-hide-unconfigured-plugin-tools"
title: "Hide tools of plugins with unset required config"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-AGENT-TOOLS-001
acceptance_criteria:
  - AC-PLUGINS-AGENT-TOOLS-001.2
  - AC-PLUGINS-AGENT-TOOLS-001.6
  - AC-PLUGINS-AGENT-TOOLS-001.9
  - AC-PLUGINS-AGENT-TOOLS-001.10
  - AC-PLUGINS-AGENT-TOOLS-001.11
  - AC-PLUGINS-AGENT-TOOLS-001.12
system_design:
  - ../../specs/plugins/system-design/agent-tool-config-readiness.md
---

# Task 01: Hide tools of plugins with unset required config

## Summary

Build the agent tool catalog only from active plugins whose required config is set,
recompute it when config is saved, and keep one plugin's unreadable config from
affecting other plugins.

## In scope and owned files

- `apps/backend/internal/plugins/agent_tools.go`: filter records by status and config
  readiness before `buildAgentToolDefinitions`; readiness helpers.
- `apps/backend/internal/plugins/service_config.go`: signal a catalog recompute after a
  committed config save, before the restart attempt.
- `apps/backend/internal/plugins/agent_tools_test.go`: focused unit tests.
- `apps/backend/internal/plugins/agent_tools_catalog_order_test.go`: deterministic
  configure/clear revision-order regressions.
- `docs/public/plugins-manifest.md` and `docs/public/plugins-authoring.md`: state that a
  plugin's tools are listed only once its required config is set.

## Out of scope

No change to manifest or config validation, secret storage, invocation payloads, plugin
status, HTTP or WebSocket APIs, the MCP server, agentctl, or the web settings UI.

## Acceptance

1. An active plugin with required fields and no stored config contributes no tools; the
   same plugin contributes its tools after `UpdateConfig` stores every required field,
   and the save signals the catalog listener (AC 001.9, 001.10, 001.6).
2. A stored `false` or `0` satisfies a required field; an absent key, null, or blank
   string does not (AC 001.9).
3. A plugin without required fields, and a configured plugin, keep their tools; inactive
   plugins still contribute none (AC 001.2, 001.9).
4. A plugin whose config file cannot be parsed contributes no tools while another
   plugin's tools stay listed and the catalog call succeeds (AC 001.10).

## TDD and exact verification

Write the tests first and run them against the unchanged catalog. Acceptance 1, 2 (blank
string), and 4 must fail because the tools are still listed or no signal is sent, not
because of fixture setup.

From `apps/backend`:

```bash
go test ./internal/plugins/ -run 'TestAgentToolCatalog|TestServiceUpdateConfig|TestRestartForConfigChange|TestInvokeAgentToolWaitsForNewlyAdvertisedConfigRestart' -count=1
go build ./...
go vet ./internal/plugins/
```

Fixtures use placeholder values only; no real credentials.

## Review remediation coverage

The catalog regression also clears a required string after a successful save and
asserts that tools disappear, the effective revision advances once, and the
service generation remains stable. The unreadable-config regression includes a
plugin without required fields to verify that its config is never needed for
catalog membership. Production behavior is unchanged.

The maintainer review uses bounded exact-source proofs with explicit storage,
runtime, and vault boundary fakes. Genuine package tests, builds, static checks,
and full application suites are verified by hosted CI; the bounded proofs do not
replace those checks. Exact-head hosted receipts are kept in the persistent
review task's evidence directory.

The restart-failure regression lives in `service_config_restart_test.go` so the
change respects the existing service-test file length limit.

A real-store failure regression covers catalog invalidation and deliverer refresh
even when saving the restart failure status fails. The existing registry rollback
contract is preserved; public copy distinguishes agent/task-session refresh from
the running plugin subprocess restart.

## Invocation ordering remediation

Ordinary `InvokeAgentTool` holds the existing per-plugin dispatch read lease
from installation lookup through the RPC, serializing with the config restart
write lease (AC 001.11). `agent_tools_config_restart_test.go` blocks a real
service config restart at the runtime boundary, observes the newly advertised
catalog, starts invocation, and verifies that it waits until the restart failure
has settled. The bounded exact-source proof first failed with the premature
missing-runtime error. Full package and race verification remain hosted checks.

## Catalog revision ordering remediation

`AgentToolCatalog` holds its existing snapshot mutex from required-config reads
through revision assignment (AC 001.12). The separate permanent
`agent_tools_catalog_order_test.go` regression pauses a pre-commit read and checks
both configure and clear commits: their post-commit snapshots must have the newer
revision. Standard-library synctest barriers avoid sleeps and preserve unchanged
revision/generation behavior. Bounded exact-source RED established revision3 for
old readiness after revision2 for committed readiness. The corrected final proof
passes both cases and the race detector with the mutex fix. Genuine new-head package,
race/static and full application acceptance remain hosted-CI gates.

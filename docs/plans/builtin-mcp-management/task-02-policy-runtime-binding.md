---
id: "02-policy-runtime-binding"
title: "Policy, task grants, and runtime delivery"
status: in_progress
wave: 2
depends_on:
  - "01-tool-group-contract"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-001.3
  - AC-AGENTS-BUILTIN-MCP-001.4
  - AC-AGENTS-BUILTIN-MCP-002.1
  - AC-AGENTS-BUILTIN-MCP-002.3
  - AC-AGENTS-BUILTIN-MCP-002.4
  - AC-AGENTS-BUILTIN-MCP-002.5
  - AC-AGENTS-BUILTIN-MCP-003.1
  - AC-AGENTS-BUILTIN-MCP-003.2
  - AC-AGENTS-BUILTIN-MCP-003.3
  - AC-AGENTS-BUILTIN-MCP-003.4
  - AC-AGENTS-BUILTIN-MCP-003.5
  - AC-AGENTS-BUILTIN-MCP-004.1
  - AC-AGENTS-BUILTIN-MCP-004.2
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 02: Policy, task grants, and runtime delivery

## Summary

Deliver the complete backend package-selection path before exposing choices.
Human workspace policy and task preferences resolve to one execution manifest
used by injection, registration, dispatch, permissions, prompts, and evidence.

## In scope

Workspace policy store/service/HTTP and task preference store/service/HTTP with
atomic revision checks; human-only writers; context eligibility; no implicit
Configuration grant inheritance; fixed Configuration chat defaults; manifest
preparation/acceptance; every launch/control/config carrier; helper negotiation;
all supported injection builders; exact reserved identities and collision handling;
backend raw-action authorization; recovery; package attachment DTOs; generated
prompt parity; typed queued Walkthrough intent admission/dispatch.

Keep the regular task principal when Configuration is selected. Protect package
policy from generic settings, profile, task-metadata, legacy MCP config, and agent
launch mutation paths. Audit shared configuration resource authority and secret
redaction. Use one registry-backed boundary rather than per-handler ad hoc grants.

## Out of scope

UI editors, independent session/profile package overrides, immediate revocation,
new roles/probes/health states, runtime flags, sibling catalog selection behavior,
and loosening existing resource permissions. Never expose a save runtime ignores.

## Acceptance

1. Missing rows preserve defaults; policy precedence, reset, stale two-revision
   writes, denied human/agent/foreign requests, corrupt data, SQLite/PostgreSQL
   upgrade/cleanup, and explicit-grant non-inheritance pass. A regular task with
   Configuration retains its surface and cannot transfer tasks or grant packages.
2. Exact package entries and routes reach each supported adapter. Disabled tools,
   raw action bypass, forged manifests, lookalike endpoints, collisions, and
   permission-prefix spoofing fail without effects. Restricted surfaces and
   external clients retain their catalogs. Required capabilities survive pruning.
3. One accepted manifest governs launch and guidance. Save during start, failed
   start, legacy adoption, corrupt recovery, old helper, workspace switch, and
   dynamic capability changes behave as designed. Desired/bound/observed state
   remain distinct; queued walkthrough intent rechecks the dispatch execution.

## Verification

Run from repository root. Use a disposable PostgreSQL fixture; never a developer
instance. The required DSN names that owned fixture. Test names in the plan are
proposed and must be created through TDD.

```bash
(cd apps/backend && go test -trimpath ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/agent/settings/handlers ./internal/mcp/profile ./internal/mcp/server ./internal/mcp/scope ./internal/mcp/handlers ./internal/sysprompt -count=1)
(cd apps/backend && go test -trimpath ./internal/task/service ./internal/task/repository/sqlite ./internal/orchestrator ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/agent/runtime/agentctl -run 'Builtin|MCP|Mcp|CanvasGuidance|Walkthrough' -count=1)
(cd apps/backend && go test -trimpath ./internal/agentctl/server/config ./internal/agentctl/server/instance ./internal/agentctl/server/api ./internal/agentctl/server/process ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/types ./internal/agentctl/types/streams -run 'Builtin|MCP|Mcp|Permission' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/mcpconfig ./internal/agent/settings/store ./internal/task/service ./internal/orchestrator/executor ./internal/agent/runtime/lifecycle ./internal/mcp/handlers -run 'Builtin' -count=1)
: "${KANDEV_TEST_POSTGRES_DSN:?Set the DSN of an owned disposable PostgreSQL test fixture}"
(cd apps/backend && go test -trimpath ./internal/agent/settings/store ./internal/task/repository/sqlite -run 'Builtin|MCP|Mcp' -count=1)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
git diff --check
```

Run any changed adapter/executor suites outside those package paths explicitly.
Use all-suite tests for new tests not matched by the focused expressions. Update
schema upgrade fixtures/manifests where required. Run changed-file Go lint and
record exact paths/commands. Report wire/mock versus installed-provider evidence.

## Files likely touched

- `apps/backend/internal/agent/mcpconfig/builtin_policy*.go`, reserved-name
  validation/import/composition and passthrough/Cursor builders; settings store
  migrations, `builtin_mcp_policy*.go`, handlers and backendapp wiring.
- `apps/backend/internal/task/` service, repository, HTTP handlers and
  `builtin_mcp_preferences*.go`; task creation/duplication authorization seams.
- `apps/backend/internal/mcp/{profile,scope,handlers,server}/` manifest, principal,
  dispatch and package tests; protected generic configuration mutation paths.
- `apps/backend/internal/orchestrator/executor/executor_execute.go`,
  `executor_builtin_mcp_test.go`; runtime lifecycle launch/recovery/metadata;
  agentctl config/instance/API/process and each supported adapter injection path.
- `apps/backend/internal/agentctl/types/streams/mcp_attachment.go`, session DTOs,
  persisted attachment projection and ordering/sanitization tests.
- `apps/backend/internal/sysprompt/`, `apps/backend/config/prompts/{kandev-context,office-context,config-context}.md`,
  prompt producers and typed queued built-in intent path.

## Dependencies

Task 01. Package-mode starts activate only after all listed enforcement is wired.

## Inputs

Full revised system design. Parent MCP resolution-delivery and pending-change
contracts. Existing managed/coordinator recovery and execution identity patterns;
existing permission provenance and transport fallback tests. Read all scoped
AGENTS.md. Runtime consumers use `runtime.Runtime` or narrow interfaces.

## Risks

The current injection and permission path assumes one `kandev` server. Prefix
matching would trust lookalikes. A raw MCP action can bypass discovery unless the
backend enforces the bound manifest. A new lookup during retained execution can
silently widen it. Shared configuration resources must retain domain authorization.

## Parallelism

`sequential`

## Results

In progress. Package-mode selection is not yet enabled for normal launches.
The management writers and selection resolver remain unwired until runtime and
prompt enforcement are complete. No UI controls have shipped.

Completed TDD slices include policy resolution; human-only workspace/task writers
with two-revision CAS; SQLite/PostgreSQL policy persistence; Configuration grant
removal on workspace transfer; package injection for session request families,
passthrough, and Cursor; raw dispatch and permission guards; helper capability
negotiation; host-derived attachment provenance; and conditional task/Office
prompt formatting. An unused workspace helper now supports one authenticated
initial binding before agent configuration, with an identical retry permitted.
Runtime binding updates use an execution-ID CAS and an unavailable marker around
remote changes. The status-snapshot preservation check passes on SQLite and PostgreSQL.
Authenticated helper reads confirm the canonical binding before process startup.
Focused recovery tests reject corrupt bindings before streams resume and persist
a recovery block. Retired execution streams cannot dispatch package tools.

The complete SQLite/PostgreSQL store-conformance suite, including upgrades,
passed with `-race` (618.162 seconds) against the task-owned socket-only fixture.
Focused transfer, HTTP/SSE/catalog, adapter permission, SSH/helper identity,
Cursor preservation, policy, and prompt tests passed. These are controlled wire,
registry, configuration-generation, and store tests; they do not certify an
installed provider's connection. Detailed transient logs are under
`/tmp/kandev-builtin-*.log`. The publication checkpoint records the final lint run and post-rebase checks.

Initial new-task and prepared-session launches now share one prepared policy
snapshot with prompt formatting. Normal dependency injection still does not
activate the policy reader or management writers.

Remaining: policy-reader and management writer activation; broader recovery/retained
binding verification; accepted-state/session projection; remaining prompt producer
paths; queued Walkthrough intent; final backend checks. WO-03 stays pending.

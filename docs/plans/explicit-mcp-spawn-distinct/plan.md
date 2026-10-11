# Explicit MCP Spawn Distinct Session Repair

Status: implementation complete; independent Review and QA pending. Sequential execution by MAIN.

Spec: [Spawn session requirements](../../specs/agents/requirements/spawn-session-effective-profile.md).
Approved platform plan: task 6cd04ffd-2351-4b45-9247-583200666444, version 2a1df183.
Base: upstream/main e8c1761cb5b7006b905c0aa995925372ce8664e1.

## Problem and approach

The MCP handler leaves ProfileExplicit false, allowing workflow SessionTarget reuse to redirect an additional-session request to an existing session. Set the existing flag for every explicit spawn. Preserve a non-empty resolved profile; an empty profile keeps normal defaults/errors without workflow session-target reuse. Ordinary launches and Office guards retain their behavior. No new API, persistence schema, executor policy, plugin or deployment change.

## Acceptance

Successful spawn returns and launches a persisted new session N != existing S under reuse, including when profiles match. Explicit and inherited profiles beat workflow overrides. Missing profiles may use normal defaults or fail without reusing S. Ordinary workflow reuse remains valid. Existing authorization, naming, errors, effective response and Office guards remain valid. Specs, tool descriptions and public docs agree.

## Tasks

- [x] [Task 01: real integration regression](task-01-regression.md)
- [x] [Task 02: minimal launch intent](task-02-launch-intent.md)
- [x] [Task 03: contract alignment](task-03-contract.md)

## Tests and verification results

Real handler -> orchestrator -> SQLite session repository, with existing fake agent runtime only. Red: distinct-session regression fails on unpatched base with existing ID `sess-1`; ordinary reuse control passes. Green: final handler regression passes (2.931s), focused handler/server checks pass, focused orchestrator explicit-profile/reuse checks pass (0.488s). Affected packages all pass: handlers 98.343s, MCP server 7.759s, orchestrator 153.084s. `make -C apps/backend lint` reports zero issues. Public docs tests pass 63/63, validation covers 47 pages; specification lint and catalog validation pass. `git diff --check` passes.

Empty-profile refinement: a step with SessionTarget intentionally supplies no default profile, so unresolved profiles return the existing launch error without reusing S. A separate ordinary-step case proves valid defaults still create a fresh session. Office scheduler-guard regression passes. Runtime is transient per-test SQLite; no live DB, Docker executor, server, UI or browser instance is required. No submodules. Independent Review and QA sessions inspect the committed final candidate before PR handoff.

## Delivery and overlay retirement

Source ownership is core backend. Historical canonical 627b39b3dc and deployed overlay 37b8f8a744 protect only non-empty profiles and have a mocked flag assertion. This repair strengthens identity coverage and the empty-profile gate. Do not import the overlay's unrelated v0.96 prepared-retry port or workspace-adoption patch.

PR creation, CI and merge belong to the later PR phase. Record actual merge SHA, first release containing it and isolated release acceptance (distinct ID enters a guarded running turn with requested profile, ordinary workflow reuse still valid) in task metadata before retiring spawn-specific overlay hunks. Source tests alone do not establish release runtime acceptance. The live deployment remains unchanged.

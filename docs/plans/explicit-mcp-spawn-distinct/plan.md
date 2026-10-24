---
status: complete
created: 2026-10-11
requirements:
  - REQ-AGENTS-SPAWN-SESSION-EFFECTIVE-PROFILE-001
system_design:
  - ../../specs/agents/system-design/spawn-session-effective-profile.md
---

# Explicit MCP Spawn Distinct Session Repair

Status: implementation complete. Independent Review and QA passed the initial implementation and documentation corrections. The pinned-source binding correction below requires final-candidate independent revalidation. Sequential execution by MAIN.

Spec: [Spawn session requirements](../../specs/agents/requirements/spawn-session-effective-profile.md).
Design: [Explicit spawn launch intent](../../specs/agents/system-design/spawn-session-effective-profile.md).
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

Draft publication and CI are separate from Human acceptance and release acceptance. Record actual merge SHA, first release containing it and isolated release acceptance (distinct ID enters a guarded running turn with requested profile, ordinary workflow reuse still valid) in task metadata before retiring spawn-specific overlay hunks. Source tests alone do not establish release runtime acceptance. The live deployment remains unchanged.

## Delivery documentation correction

The trusted PR documentation evaluator rejected the three work orders because their frontmatter omitted `wave` and the requirement, acceptance-criterion, and system-design references. The work orders now link the existing requirement and a design record of the implemented launch boundary; this plan links back to all three work orders. No runtime behavior, validator rule, or documentation exemption changes.

Local reproduction used `validateCoverage` from the trusted CI checkout against the exact PR file set and document contents: before the repair it reported `invalid` with three missing-`wave` errors; after the repair it reported `covered` with no errors and accepted all three requirement/design references. `python3 scripts/lint-spec-files.py --all` and `python3 scripts/list-docs.py validate` pass after using the supported system-design status. These local results do not represent a completed remote CI check. Independent Review and QA must account for the final documentation commit.

## Pinned-source binding correction

An additional spawn at a step pinned to profile A could replace that step's
original source binding with the new session, even when the new session used
profile B. The original reuse-target fixture did not exercise a pinned source
step. A separate server-only `PreserveWorkflowBinding` intent now prevents this
replacement without changing ordinary workflow or selector-backed recording.
The ceiling start payload persists the intent and restores it on replay.

Permanent handler regressions first failed for different/same supplied profiles
and caller/primary inheritance while ordinary recording controls passed. The
corrected cases pass. A real SQLite persisted-payload replay regression also
failed before flag transport and passes after it; it verifies downstream
source-target resolution and reuse. Legacy replay payloads retain ordinary
recording. Public JSON cannot set or expose the server-only flag.

Specification lint/catalog validation, public documentation tests (63) and
published-page validation (47) pass. Backend lint against the original merge
base reports zero issues. Exact final-candidate test, independent Review/QA and
consumer execution receipts remain in task metadata; remote CI and release
acceptance are separate gates.

---
id: "01-runtime-observations"
title: "Preserve and collect runtime observations"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-RUNTIME-UPDATES-004
acceptance_criteria:
  - AC-AGENTS-RUNTIME-UPDATES-004.1
  - AC-AGENTS-RUNTIME-UPDATES-004.2
  - AC-AGENTS-RUNTIME-UPDATES-004.3
  - AC-AGENTS-RUNTIME-UPDATES-004.8
system_design:
  - ../../specs/agents/system-design/runtime-model-discovery.md
---

# Task 01: Preserve and collect runtime observations

## Summary

Attach a typed host-runtime observation to profile discovery.
Collect verified component evidence under the same launch context, with bounded read-only inspection and explicit unknown fallbacks.

## In scope

- Trusted optional descriptors for Codex ACP and Claude ACP, with generic primary evidence for other agents.
- Additive internal probe and HTTP response fields, without client-controlled inspection recipes.
- Actual installed dependency manifests in the selected exact npm tree, including nested and hoisted layouts.
- Effective external Codex version inspection with fixed arguments, child environment, and owned cleanup.
- Existing profile generation fences, fresh explicit refresh, and sanitized version observations.
- Mixed verified/unknown components, unknown prefix contexts, and unsupported protocol fallback.

## Out of scope

Rendered controls, registry checks, activation changes, remote inspection, dependency replacement, and package installation are excluded.

## Acceptance

1. The model response preserves configured and observed bridge versions independently and distinguishes verified bundled and external components.
2. Inspection uses the captured child context, bounds time/output, and preserves successful catalogs when metadata cannot be verified.
3. Secret/path output never escapes, and context or activation changes reject stale models and observations together.

## TDD and regression evidence

First add `TestFetchProfileDynamicModelsPreservesRuntimeInfo` in `profile_discovery_test.go` using the existing recording utility and repository fixture.
Populate an observed bridge version and assert the approved serialized `runtime_info` contract. Record its expected RED failure before production edits.
The removed diagnostic test proved this response currently drops version evidence.

Add `runtime_observation_test.go` fixtures for Codex bundled, explicit external, inherited override, empty override, and failed external version.
Include Claude SDK evidence with a contradictory globally installed `claude` version.
Cover nested/hoisted manifests, wrong package/version, missing trees, invalid versions, replaced manifests, prefix ambiguity, and timeout cleanup.
Use fake executables with paths containing spaces. Add Windows command-shim coverage under the existing platform test conventions.
Output tests must include credential-shaped text and oversized stdout/stderr, proving that only parsed versions survive.
No fallback to bundled Codex is allowed after an external version failure.

Add hostutility tests for observation isolation, explicit-refresh replacement at the same external path, and activation during an in-flight probe.
`TestProfileRuntimeObservationRejectedAfterActivation` must reject the older snapshot, preserving the existing cache generation contract.
Confirm one valid component survives alongside another unknown component.
Code-review regressions also cover native OpenCode attribution from the captured resolved command, conservative custom-command handling, exact managed fallback attribution, and relative executable/PATH resolution against the probe work directory (including empty PATH entries).

## Verification

Run from the repository root. The regression test name can be narrowed during RED, then run the package commands after GREEN.

```bash
(cd apps/backend && go test -tags fts5 ./internal/agent/settings/controller -run '^TestFetchProfileDynamicModelsPreservesRuntimeInfo$' -count=1)
(cd apps/backend && go test -tags fts5 ./internal/agent/agents ./internal/agent/managedruntime ./internal/agent/hostutility ./internal/agent/settings/controller ./internal/agent/settings/dto ./internal/agentctl/server/utility -count=1)
(cd apps/backend && go test -tags fts5 -race ./internal/agent/hostutility -run 'Profile.*(Runtime|Activation|Generation)' -count=1)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the utility fixture suite natively on Windows through the repository's Windows test environment before claiming native Windows execution proof.
On Linux, `GOOS=windows` compilation is compatibility evidence only. Record any unavailable Windows environment explicitly.

## Files likely touched

- `apps/backend/internal/agent/agents/runtime_observation.go` and sibling tests (new)
- `apps/backend/internal/agent/agents/codex_acp.go`
- `apps/backend/internal/agent/agents/claude_acp.go`
- `apps/backend/pkg/agent/` only if shared descriptor types require that existing boundary
- `apps/backend/internal/agent/managedruntime/` for a narrow shared read-only cache helper if needed
- `apps/backend/internal/agentctl/server/utility/types.go`
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`
- `apps/backend/internal/agentctl/server/utility/runtime_observation.go` and sibling tests (new)
- `apps/backend/internal/agent/hostutility/types.go`
- `apps/backend/internal/agent/hostutility/manager.go`
- `apps/backend/internal/agent/hostutility/profile_probe.go` and sibling tests
- `apps/backend/internal/agent/settings/controller/profile_discovery.go` and sibling tests
- `apps/backend/internal/agent/settings/dto/dto.go` and contract tests

## Dependencies

None. Reuse existing profile discovery and validated managed activation.

## Risks

Manifest lookup must match the actual npm execution tree and must not substitute a similarly named global package.
Prefix-wrapped contexts can remain unverified. A verified provider dependency is not proof of every advertised model.

## Parallelism

`sequential`

## Inputs

- [Runtime requirement](../../specs/agents/requirements/runtime-updates.md), REQ-AGENTS-RUNTIME-UPDATES-004.
- [Runtime model discovery design](../../specs/agents/system-design/runtime-model-discovery.md), Observation contract through Snapshot flow.
- Existing `profile_discovery_test.go`, `profile_probe_test.go`, and `profile_probe_context_test.go` fixtures.
- Root, backend, and agentctl `AGENTS.md` guidance.

## Results

Completed 2026-10-04.

- Added trusted runtime descriptors and bounded host-side observation for managed bridge packages, bundled provider dependencies, and external Codex executables. Preserved typed observations through profile discovery and the HTTP response.
- Added permanent controller, agent, hostutility, and agentctl utility tests for projection, cache isolation, context changes, exact package evidence, bounded output, and unknown fallbacks.
- The focused controller regression and all six Task 01 package suites passed. Hostutility race tests passed.
- Windows-native test execution was unavailable on Linux. `GOOS=windows GOARCH=amd64 go test -c` passed for both `internal/agentctl/server/utility` and `internal/agent/hostutility`.
- The managed E2E runner built the backend successfully. Specification catalog validation, spec lint, and `git diff --check` passed.
- Follow-up review regressions passed: native OpenCode reports external ownership with trusted CLI guidance, while its exact managed npm fallback remains Kandev-managed and custom wrappers remain unknown. Relative PATH and executable values resolve under the captured work directory; empty PATH entries retain work-directory semantics. The complete agentctl utility suite passed after these additions.
- Additional review fixes attribute trusted manual runtimes without managed npm fallback, retain conservative unknown results for custom commands, and correct the Task 01 commands to include the required `fts5` build tag.
- PR follow-up preserves trusted configured bridge ownership and package identity for a command-prefixed launch while clearing its unverified observed version; the provider remains unknown. Bundled dependency inspection now uses the npm executable beside the captured launched npx, ignores a conflicting child-PATH npm, and accepts the final nonempty cache output line. Utility and hostutility tests passed normally and with the race detector. Windows `.cmd` and `.bat` shim tests are included in the native Windows CI suite; the package cross-compiled locally on Linux.
- Review follow-up replaced the vacuous empty-Codex-path test with a fixture that supplies a configured `CODEX_PATH`, and made the Linux descendant-cleanup test wait for its child PID before starting the timeout.

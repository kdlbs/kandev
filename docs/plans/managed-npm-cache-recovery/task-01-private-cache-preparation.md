---
id: "01-private-cache-preparation"
title: "Verify and privately prepare incomplete caches"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-002
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-006
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.3
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-002.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.1
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.4
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-006.10
system_design:
  - ../../specs/agents/system-design/managed-npm-cache-recovery.md
---

# Task 01: Verify and privately prepare incomplete caches

## Summary

Add verified cache evidence, one executor-local private preparation owner,
and optional authenticated task/probe directives. Ordinary requests retain
their current behavior until Task 02 connects automatic recovery.

## In scope

- Exact managed command/spec validation and independent missing-root inventory.
- Raw diagnostic path comparison with the derived cache root before redaction.
- Typed evidence and capability-negotiated directives for task and utility paths.
- Private npm preparation, package/bin validation, bounded output, and owned cleanup.
- Common ownership for ACP and native Codex app-server executors.
- A local registry/package fixture that reproduces real npm root-manifest ENOENT.
- Prepared-file lifetime through child exit, descendant cleanup, and detach/adoption.

## Out of scope

- Backend retry routing, new UI copy, shared-tree mutation, and requirement 005's dependency validator.
- Persistent repaired-runtime promotion, new storage schema, or cache-wide cleanup.

## Acceptance

1. `TestManagedRuntimeCacheEvidence` fails before the change. It then accepts
   only matching generation/attempt, exact installed identity, and confirmed
   missing metadata. Hostile, incomplete, ambiguous, and permanent failures fail closed.
2. `TestPreparedRuntimeNPM` first reproduces npm exit 254 from the incomplete
   layout. Private preparation then initializes the same exact package while
   shared and healthy-sibling files remain usable.
3. `TestPreparedRuntimeLifetime` and wire regressions prove capability gating,
   authenticated revalidation, cancellation, contained bin links, Windows
   shims, descendant cleanup, and file availability through detach/adoption.

## Verification

Run from the repository root. Add the failing regression before production changes.
The real-npm test uses a loopback registry and local archives. It must not skip
when `KANDEV_TEST_REAL_NPM=1` is supplied; missing npm is a failed prerequisite.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/common/npmresolution ./internal/agent/agents ./internal/agent/managedruntime ./internal/agentctl/types ./internal/agentctl/server/process ./internal/agentctl/server/utility ./internal/agentctl/server/api ./internal/agentctl/server/instance ./internal/agent/runtime/agentctl -run 'ManagedRuntimeCache|ManagedRuntimePreparation|PreparedRuntime|ManagedStartup|RuntimeObservation|HandleStart|MissingExecutionRootManifest|BuiltinManagedNPMRuntimeRegistration' -count=1)
(cd apps/backend && KANDEV_TEST_REAL_NPM=1 go test -trimpath -tags fts5 ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'TestPreparedRuntimeNPM' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/managedruntime ./internal/agentctl/server/process ./internal/agentctl/server/utility -run 'ManagedRuntimeCache|ManagedRuntimePreparation|PreparedRuntime' -count=1)
```

## Files likely touched

- New `apps/backend/internal/agent/managedruntime/cache_inventory.go`, `prepared_runtime.go`, and matching tests.
- `apps/backend/internal/common/npmresolution/matcher.go` and matching diagnostics tests.
- `apps/backend/internal/agentctl/types/managed_startup.go` and a proposed preparation contract file.
- `apps/backend/internal/agentctl/server/process/managed_runtime.go`, `manager_command.go`, and new cache-preparation tests.
- `apps/backend/internal/agentctl/server/api/server.go`, initialize error projection, and capability advertisement owner.
- `apps/backend/internal/agentctl/server/utility/acp_executor.go`, `codex_app_server.go`, `types.go`, and `runtime_observation.go`.
- `apps/backend/internal/agentctl/server/instance/manager.go` for prepared-root ownership through instance cleanup.
- `apps/backend/internal/agent/runtime/agentctl/client.go`, `agent.go`, and matching wire tests.

## Dependencies

None. Read backend and agentctl scoped guidance before implementation.
This task supplies the generic owner for the pending Codex dependency package.

## Risks

- A legitimate npm bin symlink differs from an unsafe symlinked cache root.
- Diagnostic redaction can remove the evidence needed for exact path comparison.
- Cleanup must join installers and provider descendants before directory removal.

## Parallelism

`sequential`

## Inputs

- Design: Executor-local evidence, Private preparation, and Wire and compatibility.
- ADR `2026-11-07-isolated-managed-npm-cache-recovery`.
- Existing `managedruntime/cache_test.go`, `utility/runtime_observation_test.go`,
  `process/managed_runtime_test.go`, and utility process-lifecycle tests.
- Pending Codex dependency Task 01; reuse its lifetime constraints, not duplicate owners.

## Results

Completed on 2026-11-07 with one user-authorized implementation worker.

- The exact focused command above passed all nine package selections.
- The exact real-npm command passed `TestPreparedRuntimeNPM` against a loopback
  registry. It reproduced shared-tree ENOENT, prepared the same exact package
  privately, initialized the fixture protocol, and preserved shared sentinels.
- The exact race command passed managedruntime, process, and utility packages.
- Commands used `GOCACHE=/tmp/kandev-go-cache` because the default cache is
  outside the writable area.
- RED/GREEN regressions covered exact missing-manifest evidence, ambiguous bin
  rejection, private preparation, cancellation and owned lifetime, generation/
  attempt binding, old-peer refusal, and ordinary empty-body start compatibility.
- Utility regressions proved physical private-cache observation, provider-version
  attribution, package/agent argument separation, and canceled-probe rejection.
  ACP and managed Codex app-server use the same preparation owner.
- Utility proof combines the shared real-npm installer fixture with utility
  protocol/evidence/observation and process-lifetime tests. No duplicate real-npm
  installer fixture was added to the utility package. Task 03 still owns
  end-to-end restart and native discovery proof.
- The initial full run found an API cleanup registration compile error.
  Replacing its out-of-scope `procMgr` reference with `s.procMgr` fixed it before
  the passing rerun. No Git mutations or live-instance changes occurred.

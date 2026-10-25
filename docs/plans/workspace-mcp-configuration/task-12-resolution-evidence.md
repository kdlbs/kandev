---
id: "12-resolution-evidence"
title: "Carry resolution decisions through MCP attachment"
status: done
wave: 9
depends_on:
  - 11-complete-setup-inputs
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-004
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-005
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.3
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-004.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.5
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.6
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-005.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
system_design:
  - ../../specs/agents/system-design/workspace-mcp-resolution-delivery.md
---

# Task 12: Carry resolution decisions through MCP attachment

## Summary

Repair finding 3's lost host-resolution evidence and finding 7's unsupported
reconnect state. Reconcile new main runtime entrypoints with the feature's
existing effective-set and exact-operation acceptance contracts.

## In scope

Paired ephemeral prepared MCP operations, safe authenticated sideband and actual
per-call response identity, client pinning, complete ACP new/load/resume/reset
handoffs, CLI start/resume/fallback/reset, Cursor/SSH/rebind integration, evidence
projection and localization. Reuse existing process-generation and snapshot/CAS
guards, reducer, sanitizers and persistence ownership.

## Out of scope

New probes, scheduler or reconnect policy, neutral delivery styling, built-in
catalog management, inherited exclusions and publishing executable placeholders.

## Acceptance

1. Secret-resolution and policy exclusions reach the existing persisted/API
   attachment projection under the exact operation's attempt, with safe server
   identity/revision/origins. Healthy delivery remains independent. Excluded
   rows never enter provider executable configuration.
2. Every actual ACP/CLI start path uses matching immutable configuration,
   exclusions, setup and revision. Cursor recovery/rebind/rollback preserve
   matching context; remote CLI uses effective scope resolution. Adoption makes
   no new delivery claim. Late old results cannot alter a successor attempt.
3. Unsupported ACP reconnect and project-file MCP providers defer before resolution/materialization. Supported
   failure preserves prior acceptance. Recording uses the actual successful
   operation's returned ID, not mutable latest client state. Existing desktop/
   phone surfaces explain the typed reason without exposing secrets.

## ASCII UI preview

UI-07: Existing Connections list/detail and phone plug drawer, same hierarchy.

```text
Atlas documentation    Not available
A workspace secret could not be read. Check the secret binding.
Healthy server        Active / existing observed state
Saved changes         Existing separate pending/deferred notice
```

No connection success is invented. Phone retains repaired content-sized rows,
one scroller and back navigation. Map to AC-004.3/4/6 and existing touch criteria.

## Verification

From root, write failing producer/transport/reducer regressions first. Preserve
the current package's passing narrow no-capability regression.

```bash
(cd apps/backend && go test ./internal/agent/mcpconfig/... ./internal/agent/runtime/agentctl/... ./internal/agent/runtime/lifecycle/... ./internal/agentctl/server/api/... ./internal/agentctl/types/streams/... ./internal/orchestrator/... -run 'MCP|Passthrough|Rebind' -count=1)
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/task/chat/mcp-explorer/)
(cd apps/web && corepack pnpm@9.15.9 run i18n:check)
make build PNPM="corepack pnpm@9.15.9"
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/chat/mcp-status.spec.ts e2e/tests/chat/mcp-pending-changes.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-mcp-status.spec.ts e2e/tests/chat/mobile-mcp-pending-changes.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check origin/main
```

Use deterministic deferred calls for A/B operations; verify the actual provider
payload, request sideband, persisted history and accepted setup together. Table
test all ACP request families and four CLI Start sites. Cover supported resume
failure/load success, no capabilities, cancelled/late replies, client replacement,
remote idempotent lookup, no attempt on adoption and workspace rebind/rollback.
API omission remains compatible. Synthetic secret values cannot occur in safe
metadata/logs/history. Run managed browser suites sequentially.

## Files likely touched

Lifecycle prepared operation/launch/session/reset/reconfiguration, Cursor/remote
MCP producers and passthrough transport; agentctl session client/API request
types; stream evidence/reducer; existing orchestrator projection; shared status
presentation and locale catalogs. Relevant tests stay beside their owner.

## Dependencies

Task 11. Independent review remains read-only and receives final immutable head.

## Risks

Reading the execution's latest snapshot after a deferred call mismatches its
server list. A client lease and current-generation admission are distinct from
the MCP attachment ID. CLI publication must have one owner for both local and
SSH execution and must not include ordinary terminal shells.

## Parallelism

`sequential`

## Inputs

Independent findings 3/7 and read-only current-main integration recommendation;
owning design Freeze each delivery operation. Existing session-observability
contract and remote passthrough-generation ownership remain authoritative.

## Results

All four ACP request families and CLI startup producers carry a frozen operation
and actual acceptance identity. Registry exclusions and helper policy filtering
reach safe attachment evidence. Full lifecycle/client/API/streams suites and
scoped orchestration integration passed. Real lock-order, client-release,
unanswered reconnect and stalled-restoration regressions reproduced failures,
then passed with correct lease ordering, a two-minute host deadline and a
separate five-second failure budget preserving old acceptance.

Desktop and phone pending-change cases passed with retries disabled. Their
busy-turn fixture now ends promptly on cancellation and waits for observable
editable idle before inspecting real acceptance, matching current-main mock
semantics without invoking cancellation recovery. The complete browser sweep
covers 23 desktop/tablet and 20 phone cases; affected settings reruns are green.
Independent rounds through five have no unresolved findings. Final immutable
candidate/commit review, hooks, publication/CI and merge confirmation remain
separate parent gates. No additional management feature was implemented.

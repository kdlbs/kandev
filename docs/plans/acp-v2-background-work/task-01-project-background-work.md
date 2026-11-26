---
id: "01-project-background-work"
title: "Project ACP v2 background work"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-V2-002
  - REQ-AGENTS-ACP-V2-003
  - REQ-AGENTS-BACKGROUND-WORK-001
  - REQ-AGENTS-BACKGROUND-WORK-002
  - REQ-AGENTS-BACKGROUND-WORK-004
  - REQ-AGENTS-BACKGROUND-WORK-005
  - REQ-AGENTS-BACKGROUND-WORK-006
acceptance_criteria:
  - AC-AGENTS-ACP-V2-002.1
  - AC-AGENTS-ACP-V2-003.1
  - AC-AGENTS-BACKGROUND-WORK-001.3
  - AC-AGENTS-BACKGROUND-WORK-002.1
  - AC-AGENTS-BACKGROUND-WORK-002.2
  - AC-AGENTS-BACKGROUND-WORK-002.3
  - AC-AGENTS-BACKGROUND-WORK-002.5
  - AC-AGENTS-BACKGROUND-WORK-004.1
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-006.2
system_design:
  - ../../specs/agents/system-design/acp-v2-preview.md
  - ../../specs/agents/system-design/background-work.md
---

# Task 01 · Project ACP v2 background work

## Summary

Translate the attached root's ACP v2 background list into workload events.
Use the existing terminal and subagent state for the details in each event.

## In scope

- Use one map of tracked entities in the adapter and the existing ordered
  event batch. Apply the shared toggle and the current root replay boundary.
- Attach reported terminal exits and retained output to the same workload.
  Keep a child's identity when it leaves and returns to the root list.
- Verify the existing viewer on desktop and phone. Add the verified ACP v2
  inspection support to the existing provider list.
  [Agents and profiles](../../public/agents-and-profiles.md#inspect-agent-background-work)

## Acceptance

1. The root lists a running terminal and child. Each appears once while the
   root is idle. Repeated lists and terminal references preserve that count.
2. An empty list clears background membership. An omitted or null list makes
   activity unknown. A child's return uses the same record. A terminal exit
   records the outcome after idle, including after the terminal leaves the list.
3. The shared toggle gates observations. A resumed session uses its current
   list to reconcile live work. Foreground completion uses the existing path.

## Regression evidence

- Add `TestWireProjectsBackgroundSnapshot` to `acpv2/updates_test.go`. Enable
  the feature and count the workload updates before a message marker. The
  baseline emits 0 where the test expects 2.
  `AC-AGENTS-BACKGROUND-WORK-002.1`, `AC-AGENTS-BACKGROUND-WORK-006.2`
- Use the wire fixture for repeated lists, omitted and null lists, `[]`, and a
  child that leaves while a sibling is still listed. Check the child's ID
  when it returns. Verify a terminal exit and retained output after idle.
  Check the disabled feature in the same fixture.
  `AC-AGENTS-BACKGROUND-WORK-001.3`, `AC-AGENTS-BACKGROUND-WORK-002.2`,
  `AC-AGENTS-BACKGROUND-WORK-002.5`, `AC-AGENTS-BACKGROUND-WORK-004.1`
- Extend `TestWireResumeFencesReplayAndPreservesIdentity` with a current
  snapshot after the root replay marker. Check that replay is silent and
  the live list can reconcile the work. Run the existing foreground wire
  tests with the package.
  `AC-AGENTS-BACKGROUND-WORK-002.3`, `AC-AGENTS-ACP-V2-003.1`,
  `AC-AGENTS-ACP-V2-002.1`
- Run the existing desktop and phone viewer tests for the chip and History.
  These start with normalized workloads. Use the isolated Codex check
  for the provider-to-chip path. `AC-AGENTS-BACKGROUND-WORK-005.1`

## Verification

Run the focused regression before the production change. Then run the
adapter package and the existing viewer checks.

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acpv2 -run '^TestWireProjectsBackgroundSnapshot$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acpv2 -count=1)
(cd apps/backend && go test -trimpath ./internal/orchestrator -run '^TestBackgroundWorkProjectionReplayAndSuccessor$' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && GOWORK=off pnpm e2e:run --host --shards 1 --project chromium -- e2e/tests/task/background-work-viewer.spec.ts --grep 'running-only summary and Active/History separation' --retries=0)
(cd apps/web && GOWORK=off pnpm e2e:run --host --no-build --shards 1 --project mobile-chrome -- e2e/tests/task/mobile-background-work-viewer.spec.ts --grep 'running chip opens a summary' --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

When an isolated Codex ACP 2.8.0 profile is available, enable both existing
feature settings. Use a managed command that yields with its terminal
still alive and release a child that still has work. Check the provider's
list, the chip at root idle, and History after the entries leave the list.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acpv2/updates.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acpv2/tools.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acpv2/session.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acpv2/updates_test.go`
- `apps/backend/internal/agentctl/server/adapter/transport/acpv2/adapter_test.go`
- `docs/public/agents-and-profiles.md`

## Dependencies

The existing SDK exposes the optional list and typed entity references.

## Risks

An item can leave the list before its outcome is known. Keep its identity
for later evidence. A new root session uses its own map.

## Parallelism

`sequential`

## Inputs

- [ACP v2 wire mapping](../../specs/agents/system-design/acp-v2-preview.md#background-work-projection)
- [Background Work requirements](../../specs/agents/requirements/background-work.md)
- [ACP v2 preview requirements](../../specs/agents/requirements/acp-v2-preview.md)
- [Desktop viewer test](../../../apps/web/e2e/tests/task/background-work-viewer.spec.ts)
- [Phone viewer test](../../../apps/web/e2e/tests/task/mobile-background-work-viewer.spec.ts)

## Results

Implemented the root snapshot projection and identity-preserving terminal and
subagent updates. The wire regression failed before the change with 0 of 2
expected updates. The adapter race suite passed:
`go test -trimpath -race ./internal/agentctl/server/adapter/transport/acpv2 -count=1`.
The replay/successor test passed:
`go test -trimpath ./internal/orchestrator -run '^TestBackgroundWorkProjectionReplayAndSuccessor$' -count=1`.
The backend build (`make -C apps/backend build`) passed.

Desktop viewer E2E: 1 passed for running-only summary and Active/History.
Phone viewer E2E: 1 passed for opening and dismissing the running summary.
Frozen pnpm install passed. Public-doc tests: 63 passed; all 56 published pages
validated. Spec catalog validation passed for 464 decisions and 2114
specifications; full spec lint passed. `git diff --check` passed.

The changed-package Go lint helper could not start while concurrent
`golangci-lint` runs held its lock. The optional live-provider check was not run
because no isolated Codex ACP profile is configured in this workspace. Full
verification notes are in the [plan](plan.md#verification-results).

Review follow-up: the idle-child fixture now carries a nested terminal list;
terminal snapshots and two separate output chunks use base64 on the wire;
terminal and child workloads retain the launch generation and submission ID
when first listed or completed in a later prompt. The resume fixture sends the
same snapshot before and after `replay_complete`. A feature-off reference-first
fixture verifies that the first terminal frame remains a tool-call event, and
repeat assertions resend their snapshots. The ACP v2 adapter race suite and
full backend build pass after these changes. The ACP v2 system design records
the resulting rules.

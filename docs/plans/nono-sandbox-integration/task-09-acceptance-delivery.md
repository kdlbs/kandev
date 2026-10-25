---
id: "09-acceptance-delivery"
title: "Prove real Nono acceptance and prepare the follow-up PR"
status: pending
wave: 10
depends_on:
  - "02-bootstrap-recovery"
  - "08-operator-policy-learning"
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-001
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-004
  - REQ-AGENTS-SANDBOX-005
  - REQ-AGENTS-SANDBOX-006
  - REQ-AGENTS-SANDBOX-008
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005
  - REQ-PLUGINS-SANDBOX-001
acceptance_criteria:
  - AC-AGENTS-SANDBOX-001.1
  - AC-AGENTS-SANDBOX-001.2
  - AC-AGENTS-SANDBOX-001.3
  - AC-AGENTS-SANDBOX-001.4
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-AGENTS-SANDBOX-004.1
  - AC-AGENTS-SANDBOX-004.2
  - AC-AGENTS-SANDBOX-004.3
  - AC-AGENTS-SANDBOX-004.4
  - AC-AGENTS-SANDBOX-004.5
  - AC-AGENTS-SANDBOX-005.1
  - AC-AGENTS-SANDBOX-005.2
  - AC-AGENTS-SANDBOX-005.3
  - AC-AGENTS-SANDBOX-005.4
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
  - AC-AGENTS-SANDBOX-008.2
  - AC-AGENTS-SANDBOX-008.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-005.5
  - AC-PLUGINS-SANDBOX-001.1
  - AC-PLUGINS-SANDBOX-001.2
  - AC-PLUGINS-SANDBOX-001.3
  - AC-PLUGINS-SANDBOX-001.4
  - AC-PLUGINS-SANDBOX-001.5
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
  - ../../specs/tasks/system-design/bootstrap-attempt-recovery.md
  - ../../specs/plugins/system-design/sandbox-provider-capability.md
---

# Task 09: Prove real Nono acceptance and prepare the follow-up PR

## Summary

Run complete real-provider acceptance and document only proven support before the new draft PR.

## In scope

Use disposable authenticated test data, a packaged plugin, pinned Nono, fresh current-source binaries, explicit network policy, and negative enforcement controls.
Verify that real runtime evidence drives active status for the exact attempt and policy.
Document the protection scope and unsupported evidence shapes. A seeded mock reply cannot close this gate.

## Out of scope

No retained-instance mutation, replacement-run recovery claim, merge, parent-branch commit, or parent PR modification.

## Acceptance

- Discovery, ACP tool work, terminal input/tools, same-session restart, cancellation, cleanup, approvals, reports, learning, and MCP pass with positive receipts.
- Denied read/write and network controls fail while permitted work succeeds. Policy edits by agents, callback forgery, and credential disclosure checks fail safely.
- Delivery evidence identifies exact commits, plugin/runtime hashes, platform, commands, limitations, and the verified main base with #4538 as its merged prerequisite.
  The new draft PR targets `main`. Do not reopen or modify #4538.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
python3 scripts/check-nono-real.test.py
python3 scripts/check-nono-real.py --run --record docs/plans/nono-sandbox-integration/acceptance.md
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `docs/plans/nono-sandbox-integration/acceptance.md (new)`
- `scripts/check-nono-real.py`
- `docs/public/agents-and-profiles.md`
- `docs/public/executors.md`
- `docs/public/tasks-and-workflows.md`
- `docs/public/plugins-authoring.md`

## Dependencies

02-bootstrap-recovery, 08-operator-policy-learning

## Risks

Missing disposable login, unsupported kernel, or missing platform evidence blocks that acceptance item. Do not mark it passed or replace it with mocks.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).
- [Design](../../specs/tasks/system-design/bootstrap-attempt-recovery.md).
- [Design](../../specs/plugins/system-design/sandbox-provider-capability.md).

## Results

Pending. No production implementation is authorized during the design turn.

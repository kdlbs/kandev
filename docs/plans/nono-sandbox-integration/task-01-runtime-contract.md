---
id: "01-runtime-contract"
title: "Pin the runtime and build conformance evidence"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-SANDBOX-003
  - REQ-AGENTS-SANDBOX-006
acceptance_criteria:
  - AC-AGENTS-SANDBOX-003.1
  - AC-AGENTS-SANDBOX-003.2
  - AC-AGENTS-SANDBOX-003.3
  - AC-AGENTS-SANDBOX-003.4
  - AC-AGENTS-SANDBOX-003.5
  - AC-AGENTS-SANDBOX-003.6
  - AC-AGENTS-SANDBOX-006.1
  - AC-AGENTS-SANDBOX-006.2
  - AC-AGENTS-SANDBOX-006.3
  - AC-AGENTS-SANDBOX-006.4
system_design:
  - ../../specs/agents/system-design/sandbox-policies.md
---

# Task 01: Pin the runtime and build conformance evidence

## Summary

Create the opt-in Nono harness from the temporary binary experiments. Pin version, archive digest, source commit, and host capability evidence.

## In scope

Test pipe transparency, PTY transport, approval grant/deny/timeout, denied access, and owned cleanup with synthetic fixtures. Record unsupported shapes explicitly.

## Out of scope

No real credentials, retained-instance changes, plugin release, or product support claims.

## Acceptance

- The harness verifies the pinned binary and refuses unsupported hosts before spawning an agent.
- Actual subprocess tests cover pipe/PTY behavior, approval exchange, permitted work, and filesystem/network denial.
- Cleanup preserves unrelated listeners and processes, including PID reuse and TERM-time descendant creation.

## Verification

Commands run from the Kandev repository root unless a subshell selects the dedicated plugin checkout.
New test and runner paths below are implementation deliverables, not existing passing evidence.

```bash
python3 scripts/check-nono-real.test.py
python3 scripts/check-nono-real.py --check-prerequisites
python3 scripts/check-nono-real.py --conformance --record docs/plans/nono-sandbox-integration/acceptance.md
```

Use TDD for changed logic. Before the first pnpm command in this workspace, run `(cd apps && pnpm install --frozen-lockfile)`.
Run each listed suite after its files exist. Do not count a zero-test filter as a pass.

## Files likely touched

- `scripts/check-nono-real.py (new)`
- `scripts/check-nono-real.test.py (new)`
- `docs/plans/nono-sandbox-integration/acceptance.md (new)`

## Dependencies

None. Execute sequentially under the plan.

## Risks

Nono documentation contains contradictory webhook statements. Binary evidence must decide support.

## Parallelism

`sequential`

## Inputs

- [Evidence](investigation.md).
- [Design](../../specs/agents/system-design/sandbox-policies.md).

## Results

Pending. No production implementation is authorized during the design turn.

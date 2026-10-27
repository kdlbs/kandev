---
id: "01-runtime-contract"
title: "Pin the runtime and build conformance evidence"
status: done
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
Identify a protected runtime evidence source that binds the live sandbox mechanism to the child identity and policy digest.
If the pinned runtime cannot supply that evidence, record unknown status and keep the active-shield capability unsupported.

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
python3 scripts/check-generic-wrapper-real.test.py
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

Completed on 2026-10-23.

- Red: missing binary returned success in the initial skeleton. The real conformance test then refused the unimplemented runner.
- Red: missing Landlock still reported ready before adding the ABI gate.
- `python3 scripts/check-nono-real.test.py`: three safety tests passed, one explicit-runtime test skipped without its environment input.
- The same command with `NONO_ACCEPTANCE_BINARY` set to the verified private binary: four tests passed.
- `python3 scripts/check-generic-wrapper-real.test.py`: 14 tests passed, including PID reuse, unrelated listener preservation, and TERM-time descendants.
- `--check-prerequisites`: verified binary digest, Linux x86_64, and Landlock ABI 7.
- `--conformance --record ...`: ten actual-runtime checks passed. See [acceptance.md](acceptance.md).

Set `NONO_ACCEPTANCE_BINARY` to an explicit verified 0.80.0 binary for runtime commands.
The harness does not install a runtime or load credentials. Its process ownership/cleanup reuses the existing acceptance runner.
The session registry lacks a current policy digest. Its process identity alone cannot support an active-shield claim.
The evidence gate remains unsupported until a later provider implementation proves a stronger source.
Fresh-process restart is not Kandev same-session recovery. No full product acceptance is claimed.

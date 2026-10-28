---
id: "00-record-owner-decisions"
title: "Record the owner decisions as an ADR"
status: pending
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-NATIVE-WORKER-001
acceptance_criteria: []
system_design:
  - ../../specs/executors/system-design/remote-executor-plugin-native-workers.md
---

# Task 00: Record the owner decisions as an ADR

## Summary

Record D1, D2, D3, D6 and the medium-term open-question answers as one ADR; the owner answered D1, D2, D3, and D6 on 2026-10-28.

## Size

S: ~120 doc lines, 0 code.

## In scope

- Name, in the ADR, the clauses of the accepted [boundary ADR 2026-09-26](../../decisions/2026-09-26-remote-executor-plugin-boundary.md) that this package amends, each with its replacement: (1) "Core owns ... workspace materialization" is amended for `workspace_preparation: provider`, where the provider materializes and core verifies the repository inventory; (2) "Serializable connection leases cross gRPC" and "all agentctl clients use the same lease authority" are amended by the `host_tunnel` transport, still one lease authority and not a public relay; (3) "Keep built-in executors in place" and "migration of built-in executors remains separate work" are amended only by the coexistence mechanism of MIGRATION-001, which removes no built-in. The clause "one environment per session and rejects shared-workspace admission" stands unchanged in this package (shared environments are deferred, see the plan Out of scope). Every other clause stands.
- Mark the medium-term open questions as answered by the owner on 2026-10-28 (sharing: same principal only, recorded for the deferred shared-environments work, structured log line for the maintenance audit, hook seam with default allow-all, status label plus existing Stop), so no build order starts with an open question.
- Record D1 (host-owned dialer), D2 (both modes plus the extensible preparation contract), D3 (keep built-ins until the plugin passes its live scenarios; no host gate and no scenario count), D6 (pinned-reference SDK), and the answers to the medium-term open questions (sharing principal, audit sink, admission policy scope, pending-placement UI) as one ADR. The owner answered D1, D2, D3, and D6 on 2026-10-28.

## Out of scope

- No production code.

## Acceptance

- One ADR under docs/decisions/ with alternatives and the owner answer for each of D1, D2, D3, D6 and each medium-term open question.
- Design decision table updated; no requirement ID changes.

## Failing-first tests

- None (documentation).

## Verification

Run from the repository root:

```bash
python3 scripts/list-docs.py validate
```

```bash
python3 scripts/lint-spec-files.py --all
```

## Likely files and risks

- `docs/decisions/<date>-executor-plugin-native-worker-decisions.md`
- `docs/specs/executors/system-design/remote-executor-plugin-native-workers.md`
- Risk: concurrency on both dialects; add the PostgreSQL race run before marking done.

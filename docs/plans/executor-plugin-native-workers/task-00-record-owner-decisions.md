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

Record D1 (transport), D2 (workspace materialization), D3 (keep built-ins) and D6 (SDK pinning) as one ADR after the owner answers, and update the design table from pending to settled.

## Size

S: ~120 doc lines, 0 code.

## In scope

- Record D1 (transport), D2 (workspace materialization), D3 (keep built-ins) and D6 (SDK pinning) as one ADR after the owner answers, and update the design table from pending to settled.

## Out of scope

- No production code.

## Acceptance

- One ADR under docs/decisions/ with alternatives and the owner answer for each of D1, D2, D3, D6.
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

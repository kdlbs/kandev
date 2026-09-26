---
id: "03-plugin-adapter"
title: "Integrate the Coordinator provider adapter"
status: pending
wave: 3
depends_on:
  - 02-host-lease
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001
acceptance_criteria:
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.3
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.8
  - AC-INTEGRATIONS-PROVIDER-SESSION-ACCESS-001.9
system_design:
  - ../../specs/integrations/system-design/provider-session-access.md
---

# Task 03: Integrate the Coordinator provider adapter

## Summary

The existing plugin program owner admits one separately owned plugin slice
against `provider-access/v1`. The adapter redeems the exact-session lease and
calls GitHub directly; it persists idempotent non-secret action receipts.

## In scope

- Plugin-owned provider adapter, PR/fork/run checks, direct GitHub call,
  ambiguous-send readback, and audit correlation to Host lease.
- Versioned SDK/Host compatibility and contract fixtures.
- Typed GitLab unsupported result until a safe credential source exists.

## Out of scope

Editing the plugin repository from this host worktree, changing the existing
coordinator-policy `1.1.0` contract silently, and live consumer CI.

## Acceptance

- Fake provider tests prove exact-head rerun/readback and wrong
  fork/PR/head/run denial before mutation.
- Duplicate, concurrent and unknown-outcome paths never submit twice and
  have non-secret provider receipts correlated to the Host lease.
- The plugin returns unsupported for GitLab PAT-backed connections and does
  not persist bearer material.

## Verification

```bash
# Run the plugin repository's admitted provider-adapter test command in its
# dedicated task/worktree; record its exact command and immutable head here.
```

## Files likely touched

- Dedicated `kandev-plugin-coordinator` repository (owned by its admitted
  plugin task, not this worktree).

## Dependencies

Task 02, owner admission by plugin program task
`1e46d457-6869-4750-bf97-4640a8df3b68`, and a versioned contract fixture.

## Risks

No plugin implementation owner or branch has been admitted yet.

## Parallelism

`sequential`

## Inputs

- `provider-access/v1` Host/SDK fixture and minimum compatible version.
- Plugin program's current policy contract `1.1.0` digest
  `00bc80871b666c17abacb96382f3e01291421cc8fe4b726f539fc613cdfb84a4`.

## Results

Pending.

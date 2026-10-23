---
status: active
system: agents
created: 2026-10-13
owners:
  - kandev
---
# Retained checkout preparation reconciliation Requirements

## Overview

A new session can reach the assigned task checkout path without a matching
session/worktree registration while the previous checkout remains intact.
Preparation must reuse a valid same-task, same-repository checkout on the
requested branch without deleting the directory or rewriting Git history.

## Requirements

### REQ-AGENTS-RETAINED-CHECKOUT-001: Retained checkout preparation reconciliation

**Intent:** Recognize the exact assigned checkout during preparation and use
normal registration admission instead of colliding with it or retiring it.

#### Acceptance criteria

- **AC-AGENTS-RETAINED-CHECKOUT-001.1:** An absent assigned path keeps normal materialization behavior.
- **AC-AGENTS-RETAINED-CHECKOUT-001.2:** Before refreshing or creating branches, preparation shall inspect only the declared task checkout path and require a healthy linked worktree of the selected repository holding the exact expected branch.
- **AC-AGENTS-RETAINED-CHECKOUT-001.3:** Existing valid registration shall retain its worktree ID, repository/branch slot, path and history. Conflicting or ambiguous registration shall remain unchanged.
- **AC-AGENTS-RETAINED-CHECKOUT-001.4:** An unregistered checkout with an explicit matching checkout branch shall use the existing normal materialization registration path, retaining its directory, refs, index, tracked modifications and untracked files.
- **AC-AGENTS-RETAINED-CHECKOUT-001.5:** Another task's declared ownership, a different repository or branch, an invalid linked checkout, or a main checkout shall not be adopted. Unavailable ownership/repository/branch information shall be reported as unavailable rather than asserted foreign.
- **AC-AGENTS-RETAINED-CHECKOUT-001.6:** Existing target-path reservation and normal environment/claim/slot admission shall remain authoritative; failure to register shall preserve the checkout.
- **AC-AGENTS-RETAINED-CHECKOUT-001.7:** Reuse shall neither retire, remove, prune, reset, clean, relocate nor reseed the retained checkout.
- **AC-AGENTS-RETAINED-CHECKOUT-001.8:** Recorded reuse/recreate, attach-only reuse, immutable PR heads and remote contribution preparation retain their existing contracts.
- **AC-AGENTS-RETAINED-CHECKOUT-001.9:** This path adds no owner ledger, receipt, marker, hash/fingerprint comparison, broad filesystem discovery, scheduler or permanent verification process.

## Boundaries

This is normal checkout reuse/registration, separate from the metadata repair
and receipt contract in [workspace inventory repair](workspace-inventory-repair.md).
It does not change task-environment ownership or replace failure environments.

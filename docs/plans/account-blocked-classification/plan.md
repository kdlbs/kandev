---
status: done
created: 2026-10-13
requirements:
  - REQ-AGENTS-ACCOUNT-BLOCK-001
  - REQ-AGENTS-ACCOUNT-BLOCK-002
system_design:
  - ../../specs/agents/system-design/account-blocked-classification.md
---

# Provider account-block classification

## Overview

A restricted provider account keeps receiving requests because routing treats
its rejection as an ordinary failure. Classify the restriction, hold only the
failed execution profile, and let its owner release it.

## Technical approach

Add one provider-neutral rule and a hold-until-release code. Build the hold on
the existing circuit registry with a profile-scoped key, so no schema changes.
Add one owner-release route to the settings API. Each task is an independent
commit that a reviewer can take separately.

## Delivery order

1. [Task 01: Classify restricted accounts](task-01-classify-restricted-accounts.md)
2. [Task 02: Hold restricted profiles](task-02-hold-restricted-profiles.md)
3. [Task 03: Release a held profile](task-03-release-held-profile.md)

## Verification strategy

- Run the focused packages in `apps/backend`: `routingerr`, `routingpolicy`,
  `dynamic`, `orchestrator` (`-run AccountBlocked`), and
  `agent/settings/handlers`.
- Run backend lint with the PR base SHA and the specification checks.

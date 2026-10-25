---
id: "03-release-held-profile"
title: "Release a held profile"
status: done
wave: 3
depends_on:
  - "02-hold-restricted-profiles"
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACCOUNT-BLOCK-002
acceptance_criteria:
  - AC-AGENTS-ACCOUNT-BLOCK-002.1
  - AC-AGENTS-ACCOUNT-BLOCK-002.2
  - AC-AGENTS-ACCOUNT-BLOCK-002.3
  - AC-AGENTS-ACCOUNT-BLOCK-002.4
system_design:
  - ../../specs/agents/system-design/account-blocked-classification.md
---

# Task 03: Release a held profile

## Summary

Add the owner release route that closes one profile's hold and nothing else.

## In scope

- Settings controller seam, handler, route, and backend wiring.
- Handler tests for success, repeat, not found, unwired, and failure.

## Out of scope

A user interface, automatic release, and profile setting changes.

## Acceptance

`AC-AGENTS-ACCOUNT-BLOCK-002.1` through `002.4`.

## Verification

```bash
cd apps/backend && go test -trimpath ./internal/agent/settings/handlers -run ReleaseAccountBlock -count=1
```

## Results

- GREEN: `agent/settings/handlers` passed, including success, repeat, not
  found, unwired, and failed-save release.
- Locally on Windows, `agent/settings/controller` timed out while a host
  runtime updater test waited, and three SQLite ordering tests in
  `agent/settings/store` failed. This work order does not change either
  package; CI is the reference for them.

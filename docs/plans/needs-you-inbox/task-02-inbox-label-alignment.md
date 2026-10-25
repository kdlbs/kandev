---
id: "02-inbox-label-alignment"
title: "Align Inbox feature-toggle terminology"
status: done
wave: 2
depends_on:
  - "01-needs-you-inbox-delivery"
plan: "plan.md"
requirements:
  - REQ-UI-NEEDS-YOU-INBOX-001
acceptance_criteria:
  - AC-UI-NEEDS-YOU-INBOX-001.1
system_design:
  - ../../specs/ui/system-design/needs-you-inbox-03.md
---

# Task 02: Align Inbox feature-toggle terminology

## Summary

Align the runtime feature-toggle label and public configuration reference with
the Inbox destination name. Keep the runtime flag identity and behavior unchanged.

## Acceptance

- The Feature Toggles page shows `Inbox` for `features.needsYouInbox`.
- The label matches the Inbox destination copy contract outside the Office-mode
  collision case described by D4.
- The key, environment variable, route, defaults, persistence, restart behavior,
  and effective feature value remain unchanged.
- The public configuration reference uses `Inbox`.
- The registry test pins the display label and its existing metadata.

## Verification

- `go test -tags fts5 ./internal/runtimeflags/...` passed.
- `node --test .github/scripts/pr-docs.test.cjs` passed: 52 tests.
- The local documentation coverage evaluator reported `status: covered` with no
  errors for the changed runtime file and this delivery package.
- `node --test scripts/validate-public-docs.test.mjs` passed: 62 tests.
- `node scripts/validate-public-docs.mjs` passed: 46 published docs pages.
- `git diff --check` passed.

## Files

- `apps/backend/internal/runtimeflags/registry.go`
- `apps/backend/internal/runtimeflags/registry_test.go`
- `docs/public/configuration.md`

## Results

The registry label is `Inbox`. The metadata test protects the label and stable
runtime-flag metadata. The public configuration reference matches the label.
No runtime behavior or route identity changed.

Follow-up PR: https://github.com/kdlbs/kandev/pull/3673

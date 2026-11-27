---
status: done
created: 2026-10-08
requirements:
  - REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001
system_design:
  - ../../specs/architecture-lint/system-design/migrated-system-query-owner.md
---

# Implementation Plan: Guard migrated System Query ownership

## Overview

Add the bounded architecture-lint guard defined by
[REQ-ARCHITECTURE-LINT-MIGRATED-SYSTEM-QUERY-OWNER-001](../../specs/architecture-lint/requirements/migrated-system-query-owner.md)
and the [system design](../../specs/architecture-lint/system-design/migrated-system-query-owner.md).
The rule protects only the four migrated System snapshots and leaves Query,
Zustand, hydration, runtime, and UI behavior unchanged.

## Work package

The focused ESLint rule, owner-syntax fixtures, actual ESLint-config wiring
tests, narrow documentation reconciliation, and verification belong to one
vertical work order. It covers four separately testable outcomes: supported
mirrors fail with actionable diagnostics; unrelated state and lexical
lookalikes remain accepted; normal web lint checks the current production
owners; and wiring tests prove every owner path is active under the real config.

- [Task 01: System Query owner guard](task-01-system-query-owner-guard.md)

## Verification

The work order owns RuleTester and real-config `lintText` coverage for all four
owner filenames, targeted lint of all guarded production paths, the full web
lint/typecheck/test checks, specification validation, trusted-base
documentation-coverage preflight, and before/after lint-time measurements. No
browser or E2E test is required because the work changes enforcement only.

## Status

Implementation is complete under the approved bounded scope. Pull request
delivery checks and review evidence are tracked on [PR #4357](https://github.com/kdlbs/kandev/pull/4357).

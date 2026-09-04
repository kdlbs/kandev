---
created: 2026-09-03
status: done
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
legacy_specs: []
---

# Implementation Plan: Proxy Provider Error Classification

## Overview

Classify safe, exact ACP gateway failures as shared provider availability
failures. Classify a proxy's explicit all-credentials-refused envelope as a
hard credential failure. Dynamic routing keeps its existing policy evaluator,
generation fence, and pre-result effect-safety gate.

## Scope

### In scope

- Structured HTTP 500, 502, and 504 provider availability classification.
- Exact ACP `API Error` gateway-envelope classification.
- The explicit Claude ACP proxy credential-refusal envelope.
- Unit coverage for class, policy invariants, and false-positive boundaries.

### Out of scope

- Provider health polling, account-pool usage inspection, or TeamClaude API
  integration.
- Broad matching of generic `internal error` prose.
- Changes to retry policy values, candidate order, or effect-safety rules.

## Work orders

- [x] [Task 01: Classify proxy gateway failures](task-01-classify-proxy-gateway-failures.md)

## Verification results

- `cd apps/backend && go test -tags fts5 ./internal/agent/runtime/routingerr` passed.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

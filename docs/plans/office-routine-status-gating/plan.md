---
created: 2026-09-08
status: done
requirements:
  - REQ-OFFICE-ROUTINE-STATUS-001
  - REQ-OFFICE-ROUTINE-STATUS-002
  - REQ-OFFICE-ROUTINE-STATUS-003
  - REQ-OFFICE-ROUTINE-STATUS-004
  - REQ-OFFICE-ROUTINE-STATUS-006
system_design:
  - ../../specs/office/system-design/routine-status-gating.md
legacy_specs: []
---

# Implementation Plan: Office Routine Status Gating

## Overview

Routine status must control every routine fire path. This plan records the
delivery package for cron suppression, manual and webhook refusals, and the
frontend status surfaces.

## Scope

- Read routine status before a cron trigger claim.
- Advance suppressed cron cursors without creating fire evidence.
- Refuse manual and webhook fires for non-firing routines.
- Keep refusal responses and frontend messages localizable.
- Keep routine list and detail views consistent with the firing status.

## Work orders

- [Task 01: Enforce routine status gating](task-01-enforce-routine-status-gating.md)

## Verification

```bash
cd apps/backend
go test ./internal/office/routines
cd ../..
python3 scripts/lint-spec-files.py --all
```

---
id: 01-agent-notes
title: Manager-curated coordinator notes
status: pending
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-COORDINATOR-AGENT-NOTES-001
  - REQ-COORDINATOR-AGENT-NOTES-002
  - REQ-COORDINATOR-AGENT-NOTES-003
acceptance_criteria:
  - AC-COORDINATOR-AGENT-NOTES-001.1
  - AC-COORDINATOR-AGENT-NOTES-001.2
  - AC-COORDINATOR-AGENT-NOTES-001.3
  - AC-COORDINATOR-AGENT-NOTES-001.4
  - AC-COORDINATOR-AGENT-NOTES-001.5
  - AC-COORDINATOR-AGENT-NOTES-001.6
  - AC-COORDINATOR-AGENT-NOTES-001.7
  - AC-COORDINATOR-AGENT-NOTES-001.8
  - AC-COORDINATOR-AGENT-NOTES-002.1
  - AC-COORDINATOR-AGENT-NOTES-002.2
  - AC-COORDINATOR-AGENT-NOTES-002.3
  - AC-COORDINATOR-AGENT-NOTES-002.4
  - AC-COORDINATOR-AGENT-NOTES-002.5
  - AC-COORDINATOR-AGENT-NOTES-002.6
  - AC-COORDINATOR-AGENT-NOTES-003.1
  - AC-COORDINATOR-AGENT-NOTES-003.2
  - AC-COORDINATOR-AGENT-NOTES-003.3
system_design:
  - ../../specs/coordinator/system-design/agent-notes.md
---

# Task 01: Manager-curated coordinator notes

## Summary and scope

Implement the full note lifecycle, citations, safe per-turn injection, reference projection, flag and settings UI described by the linked contract. Integrate all proposal kinds after the answer-proposals slice, including answer. Cover both database dialects, retry and concurrent mutation boundaries. Do not implement the dream or automatic learning.

## Acceptance conditions

- Managers can curate versioned notes; access controls, credential rejection, flag cycle and conflicts follow the requirements.
- Validated notes reach each attended and unattended turn exactly as specified, including same-conversation turns, retries and citation failures.
- Proposal references and responsive localized settings represent the durable state; tests cover lifecycle and races.

## ASCII UI preview

UI-01: Notes in coordinator settings (see [plan](plan.md#ascii-ui-preview) for desktop, phone and state details):

```text
Notes                           [Add note]
Project  Release target         Valid  [View]
Referenced by: Proposal 42, Task 12
Versions                         [Edit] [Delete]
Version 1                        [Revert]
```

## Verification

- `cd apps/backend && go test ./internal/coordinator/... ./internal/orchestrator/... ./internal/backendapp/... ./internal/mcp/...`
- `cd apps/web && pnpm run typecheck && pnpm run i18n:check`
- `cd apps/web && pnpm e2e:run --project=chromium e2e/tests/coordinator/agent-notes.spec.ts`
- `python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all`

Create the named Playwright spec for the new feature, enabled with `KANDEV_FEATURES_COORDINATOR_PHASE35=true` (and the coordinator, phase 2, 3 and 3.1 flags). Exercise manager/reader controls, same conversation second turn, stale, unavailable and over-the-cap states, expired and deleted notes, conflict handling, desktop and phone viewports, Tab and Enter keyboard operation, and the English and one non-Latin-script locale. Verify SQLite and PostgreSQL migration/replay using existing coordinator test conventions.

## Likely files, dependencies and risks

`apps/backend/internal/coordinator/`, `internal/backendapp/coordinator.go`, `internal/orchestrator/`, MCP proposal tools, runtime flag registry, `profiles.yaml`, coordinator settings and locale catalogs under `apps/web/`. Resolve and integrate the answer-proposals slice before starting. Delivery identity and concurrency between note mutation and proposal creation require transactional tests. Backend tests must also pin: stale over unavailable precedence, the 5 second validation deadline, overlapping validations, receipt idempotency across recovery, expiry as the later of old and new, prompt_hash unchanged by note edits, no section in steers, emptiness checks and dream episodes, and flag-off rejection of `note_ids`.

## Results

Pending.

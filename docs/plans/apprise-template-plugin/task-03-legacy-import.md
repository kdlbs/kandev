---
id: 03-legacy-import
title: Legacy import and rendering parity
status: in_progress
wave: 2
depends_on:
  - 01-host-contract
  - 02-template-plugin
plan: plan.md
requirements:
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-003
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-004
  - REQ-PLATFORM-NOTIFICATION-FORMATTER-002
acceptance_criteria:
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.1
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.2
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-004.3
  - AC-PLATFORM-NOTIFICATION-FORMATTER-002.5
  - AC-PLATFORM-NOTIFICATION-FORMATTER-003.6
system_design:
  - ../../specs/platform/system-design/notification-formatter-plugins.md
---

# Task 03: Legacy import and rendering parity

## Summary

Implement a one-time offline legacy exporter and transactional owner-bound
import in the plugin repository. Prove byte-preserving source transfer and
render/fallback parity with a pinned legacy oracle before any live cutover.

## In scope

- Private offline snapshot reader with an explicit schema/version allowlist;
  minimal owner/provider validation and no credential/config export.
- All-or-none import, fresh live revision tokens, source-provenance receipt,
  identical retry no-op, collision rejection, count/digest readback.
- Import prepares a complete snapshot before any binding. First activation is
  separate and selects that exact revision; failed preparation cannot change
  live delivery. Cover owner/candidate installation races before activation.
- Add rollback export retaining all six event settings and a legacy-safe
  projection for the original four, rejecting unsupported grammar/schema rather
  than discarding edits or pretending older hosts support newer events.
- Synthetic oracle fixtures for all four legacy events, update variants,
  unavailable values, output limits, escaping, URL encoding, unknown events,
  missing origin, wrong owner/provider, interrupted import, and conflict.
- Document source snapshot creation, private permissions, edit freeze, backup,
  candidate-only import, and rollback; no real template text in tracked fixtures.

## Out of scope

Live host SQL, automatic production binding changes, deleting legacy records,
real-recipient sends, and host/plugin release.

## Acceptance

1. Synthetic export/import preserves source bytes and associations without
   provider/subscription/history/task/session mutation; failures roll back fully.
2. Identical imports are no-ops; owner or target revision conflicts cannot cause
   partial writes or silent overwrites.
3. Pinned old/new render results match across the plan's fixture matrix, and the
   private operator procedure exposes only counts/digests in reports.

## Verification

Run from the independent plugin task worktree root. These command/test paths are
proposed deliverables of this work order, not files already present. The export
smoke test creates and reads a disposable legacy database; do not pass a live DB.

```bash
go test -trimpath -race ./server/... -run 'TestImport|TestLegacyRenderingParity' -count=1
go test -trimpath -race ./cmd/legacy-export/... -count=1
node scripts/verify-legacy-migration.mjs --fixtures testdata/legacy-migration --output .build/migration
git diff --check
```

The new harness must assert source DB digest/row inventories are unchanged,
simulate interruption/ownership conflicts, capture count/digest-only receipts,
and exit nonzero on any mismatched render/fallback or partial import.

## Files likely touched

Plugin `cmd/legacy-export/`, `server/import.go`, `server/import_test.go`,
`server/parity_test.go`, `testdata/legacy-migration/`, proposed
`scripts/verify-legacy-migration.mjs`, and migration documentation.

## Dependencies

Tasks 01 and 02 provide owner-authorized management and the real renderer/store.
Read the paired design's legacy migration section before implementation.

## Risks

Source rows can change after export. Import provenance and private before/after
readback must detect drift; freeze template edits during an authorized cutover.

## Parallelism

Sequential after Tasks 01 and 02.

## Inputs

Pinned fork schema/renderer from the design and synthetic fixture inventory.
Do not copy operational credentials or personal template literals into fixtures.

## Results

Implemented the version/schema-allowlisted offline exporter, immutable owner/
provider import receipts, interrupted-receipt rollback test, and explicit
`legacy-v1` rollback export retaining the full six-event document plus a
four-event legacy projection. Source personal data and receivers are excluded
from tracked fixtures.

The pinned legacy source generated 38 render and six URL oracle cases; the
plugin parity test passes. It found and fixed an invalid-origin session URL
that incorrectly returned only a query string. Import, storage, and exporter
race tests pass in the private development SDK lane. The product smoke runner
also passed its synthetic exporter-to-live-import pipeline: four templates,
source-byte preservation, receiver exclusion, forged-owner rejection,
idempotent preparation without binding, explicit activation, four live
synthetic previews, and unchanged subscriptions. Exact artifact digests and
count/digest-only receipts are retained under ignored development output.

---
id: "03-canvas-receipts"
title: "Preserve canvas publisher receipts"
status: done
wave: 3
depends_on:
  - "02-native-installation"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-PUBLISHER-001
  - REQ-PLUGINS-PUBLISHER-002
  - REQ-PLUGINS-PUBLISHER-003
acceptance_criteria:
  - AC-PLUGINS-PUBLISHER-001.4
  - AC-PLUGINS-PUBLISHER-002.1
  - AC-PLUGINS-PUBLISHER-002.2
  - AC-PLUGINS-PUBLISHER-002.5
  - AC-PLUGINS-PUBLISHER-002.6
  - AC-PLUGINS-PUBLISHER-003.4
system_design:
  - ../../specs/plugins/system-design/publisher-identity.md
---

# Task 03: Preserve canvas publisher receipts

## Summary

Carry trusted catalog evidence through canvas preparation and installation receipts.
Project it only for the imported release to which the receipt applies.

## In scope

- Use the shared source resolver in the existing canvas registry path.
- Add provenance to preparation/review and the committed receipt with its release ID.
- Keep archive digest and normalized content digest separate.
- Add nullable fields through the existing receipt migration mechanism.
- Project unverified status after local edits and exclude provenance from exports.
- Preserve user/workspace bindings, consent, retry receipts, and restart behavior.

## Out of scope

Canvas authoring authority, new approval rules, new UI, or a new installation pipeline.

## Acceptance

- A registry import carries matching server evidence through review, commit, retry, and restart.
- Uploads and URLs cannot forge verification through source/repository fields or exported metadata.
- Local edits lose verification. Original-release selection uses only its own receipt, without transferring grants.

## Verification

Run from the repository root:

```bash
(cd apps/backend && go test -tags fts5 ./internal/canvas ./internal/plugins/instances)
(cd apps/backend && go test -tags fts5 ./internal/backendapp -run 'TestPublisherCanvas|TestCanvas.*Install|TestCanvas.*Distribution')
git diff --check
```

Add `canvas_publisher_test.go` and the backend-app route tests. Include upgrade
of an old receipt database, failed transaction cleanup, and lost-response replay.

## Files likely touched

- `apps/backend/internal/canvas/distribution_install.go`, `distribution_preparation.go`, `distribution_export.go`.
- `apps/backend/internal/canvas/repository.go`: receipt schema upgrades and transactional persistence.
- `apps/backend/internal/canvas/authoring.go`: `commitCanvasInstallation` receipt/release binding.
- `apps/backend/internal/backendapp/canvas_distribution_routes.go` and tests.
- Canvas install transaction and active-release DTO projection.
- `apps/backend/internal/plugins/instances/store.go` only where receipt/release association requires it.

## Dependencies

Task 02 shared resolver, provenance type, and trust tests.

## Risks

An archive SHA-256 is not the normalized package digest.
An instance-level badge must not outlive a locally edited release.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/plugins/requirements/publisher-identity.md).
- [System design](../../specs/plugins/system-design/publisher-identity.md).
- [Trust decision](../../decisions/2026-09-18-plugin-publisher-trust.md).

## Results

- RED: canvas preparation and receipt projections initially had no publisher
  provenance or release binding; the receipt and forged-input tests failed
  before those fields were carried through the transaction.
- `cd apps/backend && go test -tags fts5 ./internal/canvas ./internal/plugins/instances`: passed.
- `cd apps/backend && go test -tags fts5 ./internal/backendapp -run 'TestPublisherCanvas|TestCanvas.*Install|TestCanvas.*Distribution'`: passed.
- The focused canvas tests cover catalog receipt replay, upload/source
  impersonation rejection, receipt migration and release projection, and
  rollback cleanup.
- `git diff --check`: passed.

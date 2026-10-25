---
spec: docs/specs/system-page/requirements/storage-maintenance.md
created: 2026-07-29
status: complete
---

# Implementation Plan: Quarantine Lifecycle

## Overview

Complete the quarantine lifecycle by adding a typed best-effort purge operation, running eligible
purges during full maintenance, and exposing separate retention-respecting and retention-bypassing
bulk actions. Then wire the API through the existing System job flow, make eligibility and
automatic-cleanup timing visible on desktop and mobile, update operator documentation, and prove the
flows with focused backend, frontend, and Playwright coverage.

No schema migration is required: `storage_quarantine_entries.delete_after`, state, size, and error
fields already contain the durable data needed for eligibility and result accounting.

---

## Backend

### Typed purge result and deletion modes

- Add `apps/backend/internal/system/storage/quarantine_purge.go` with
  `QuarantinePurgeScope` (`eligible`, `all`), `QuarantinePurgeFailure`, and
  `QuarantinePurgeResult`.
- Extend the backend quarantine controller contract with a bulk purge method that returns considered,
  deleted, protected, and failed counts/bytes plus per-entry failures.
- Iterate all active `quarantined|failed` entries deterministically. Eligible-only purge skips future
  `delete_after` entries; force purge attempts every active entry.
- Continue after entry failures and return the complete result together with an aggregate error so
  successful deletions remain committed while the System job or maintenance run is visibly failed.

### Resource deletion safety

- Refactor `apps/backend/internal/system/storage/workspaces/provider.go` so normal and forced
  permanent deletion share path, state, pruning, and filesystem safety validation. The force path
  skips only the retention comparison.
- Refactor Go-cache deletion in
  `apps/backend/internal/backendapp/storage_maintenance.go` through the same retention-mode boundary.
  Preserve ownership metadata validation, managed-trash containment, replacement-path ambiguity
  checks, and state transitions for both modes.
- Require `DELETE ALL NOW` at the external force boundary; never use the generic maintenance
  `force` flag as a retention override.

### Maintenance provider composition

- Create the `workspaceQuarantineController` before assembling cleanup providers in
  `apps/backend/internal/backendapp/storage_maintenance.go`.
- Add a typed `quarantine` cleanup provider to `storageCleanupProviders`. Its ordinary `Cleanup`
  purges only eligible entries and returns `QuarantinePurgeResult` in the maintenance-run result.
- Include the provider in scheduled and full manual runs. Existing explicit selections such as
  `resources: ["go_cache"]` continue to select only the named provider and do not purge unrelated
  quarantine entries.
- Keep the provider inside the existing maintenance activity lease so task admission can preempt a
  long purge. Provider errors remain isolated by `Runner.runProviders`.

### Bulk HTTP operation

- Extend `apps/backend/internal/system/storage/operations.go` with
  `PurgeQuarantine(ctx, scope, confirmation)`, validate the scope/phrase pair before starting work,
  and run it as `storage-quarantine-delete`.
- Register `DELETE /api/v1/system/storage/quarantine` in
  `apps/backend/internal/system/storage/handler.go` with body
  `{scope, confirm}`. Preserve `DELETE /quarantine/:id` and its retention `409`.
- Return `400` for unknown scopes or mismatched confirmation phrases. A started bulk purge returns
  `202 {job_id}` and publishes its structured result through the existing System job tracker.
- Invalidate the cached storage overview after any successful deletion, including a partially
  successful job.

---

## Frontend

### Types, API client, and domain controller

- Add the bulk scope/result types to `apps/web/lib/types/system.ts`.
- Add `purgeStorageQuarantine(scope)` to
  `apps/web/lib/api/domains/system-api.ts`, mapping `eligible` to `DELETE ELIGIBLE` and `all` to
  `DELETE ALL NOW`.
- Extend `apps/web/hooks/domains/system/use-storage-maintenance.ts` with one shared bulk-delete job
  path, terminal refresh, success/error feedback, and actions for `clearEligible` and
  `forceClearAll`.
- Keep individual deletion wired to `DELETE`; the UI will only enable that action for entries whose
  deadline has elapsed.

### Quarantine card

- Update `apps/web/components/settings/system/storage/storage-quarantine-card.tsx` to derive and
  display `eligible now` versus `protected until <localized timestamp>` from each persisted
  `delete_after` value using semantic `<time dateTime>` markup.
- Show card-level eligible/protected counts and explain:
  - scheduling enabled: eligible entries are removed by the first successful maintenance run after
    their deadline, subject to the idle gate;
  - scheduling disabled: automatic cleanup is off and a full manual run or quarantine action is
    required.
- Disable the per-row Delete action before its deadline with a visible disabled reason.
- Add **Clear eligible** and **Force clear all** actions. Disable **Clear eligible** when no entry is
  eligible and disable both when the quarantine list is empty or another conflicting storage action
  is active.
- Extend `storage-confirmation-dialogs.tsx` with separate bulk confirmations:
  `DELETE ELIGIBLE` names how many eligible items will be removed and how many remain protected;
  `DELETE ALL NOW` warns that every restore window will be lost.
- Wire scheduling state and the two controller actions through
  `storage-maintenance-settings.tsx`.

### Mobile design contract

- **Desktop outcome and mobile entry:** both use **Settings → System → Storage** and provide row
  deadlines, eligible/protected totals, individual restore/delete, **Clear eligible**, and
  **Force clear all**.
- **Nearest shipped exemplar:** the current `StorageQuarantineCard`, responsive Storage page, typed
  `ConfirmationDialog`, and `mobile-storage-maintenance.spec.ts`.
- **Phone hierarchy and primary action:** keep the inline quarantine card as the single vertical
  scroll surface; show status immediately under each path, then full-width row actions. Put the safe
  **Clear eligible** action before the destructive force action.
- **Presentation choice:** use the existing viewport-contained `AlertDialog` for the short typed
  confirmations. A drawer would add navigation depth without improving this focused irreversible
  decision.
- **Geometry:** the page remains the only scroll owner, action controls are at least 44 pixels high,
  dialogs remain within the dynamic viewport, long paths wrap, and document horizontal overflow
  stays zero. No fixed mobile control is introduced, so additional safe-area positioning is not
  required.
- **Shared behavior:** eligibility derivation, counts, controller actions, API calls, job tracking,
  and confirmation semantics are shared across viewports; only responsive layout classes differ.

---

## Public Documentation

- Update `docs/public/operations.md` to explain the retention deadline, automatic purge dependency
  on scheduled/full manual maintenance, **Clear eligible**, and the irreversible
  **Force clear all** override.
- State that `delete_after` is an earliest safe deletion time, not a guaranteed exact deletion
  instant, because idle-gate admission and preemption can delay a run.

---

## Tests

- **Eligible versus forced resource deletion**
  - **Files:** `apps/backend/internal/system/storage/workspaces/provider_test.go`,
    `apps/backend/internal/backendapp/storage_maintenance_test.go`
  - **How:** table-driven tests prove normal deletion rejects future deadlines, force deletion
    bypasses only the deadline, and both modes retain path/ownership/pruner validation.
- **Best-effort bulk result**
  - **File:** `apps/backend/internal/backendapp/storage_maintenance_test.go`
  - **How:** real temporary workspace/Go-cache entries cover eligible, protected, and invalid
    payloads; assert counts/bytes, aggregate error, retained failures, and committed successes.
- **Automatic maintenance purge**
  - **Files:** `apps/backend/internal/backendapp/storage_maintenance_test.go`,
    `apps/backend/internal/system/storage/runner_test.go`
  - **How:** production composition and runner tests prove full runs invoke `quarantine`, explicit
    `go_cache` selection does not, and a purge failure does not prevent later providers.
- **Bulk API validation and job result**
  - **Files:** `apps/backend/internal/system/storage/handler_test.go`,
    `apps/backend/internal/system/storage/operations_test.go`
  - **How:** HTTP tests cover both scope/confirmation pairs and `400` mismatches; operations tests
    wait for the tracked job and assert result, overview invalidation, and partial failure state.
- **Frontend API/controller**
  - **Files:** `apps/web/lib/api/domains/system-api.test.ts`,
    `apps/web/hooks/domains/system/use-storage-maintenance.test.tsx`
  - **How:** assert exact request bodies, job tracking, terminal reload, and action-specific toasts.
- **Eligibility and responsive card**
  - **Files:** new
    `apps/web/components/settings/system/storage/storage-quarantine-card.test.tsx`,
    existing `storage-maintenance-settings.test.tsx`
  - **How:** fake current time around the deadline; assert semantic timestamps, counts, scheduling
    copy, disabled row deletion, both confirmation phrases, and controller wiring.

---

## E2E Tests

- **Desktop protected lifecycle**
  - **File:** `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
  - **Scenario:** quarantine a real orphan workspace, verify its protected deadline and disabled
    row delete, then confirm **Force clear all** and assert the quarantine payload and filesystem
    data are removed.
- **Desktop eligible bulk contract**
  - **File:** `apps/web/e2e/tests/system/storage-maintenance.spec.ts`
  - **Scenario:** present eligible and protected entries, confirm **Clear eligible**, and verify the
    completed-job feedback plus the protected remainder. Backend package tests provide the
    time-controlled filesystem integration when E2E setup cannot age a persisted entry safely.
- **Mobile capability and geometry**
  - **File:** `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts`
  - **Scenario:** review eligibility, complete both typed bulk-confirmation paths through touch
    targets, and assert no document horizontal overflow.

Final E2E verification rebuilds production artifacts through:

```bash
cd apps/web && pnpm e2e:run tests/system/storage-maintenance.spec.ts tests/system/mobile-storage-maintenance.spec.ts
```

## Validation Evidence

- Backend: `go test ./internal/system/... ./internal/backendapp` passed, including the storage
  system, job tracker, route, and maintenance packages.
- Frontend: 63 focused Vitest tests, `pnpm run typecheck`, and `pnpm run lint` passed.
- Documentation: public-doc validation tests and the validator passed.
- E2E: focused desktop Chromium and mobile-chrome quarantine flows passed against the production
  bundle; the full PR E2E workflow also passed.
- Review follow-up: cancellation, deadline refresh, shared confirmation constants, partial failed
  job result documentation, and force-clear failure wording are covered by the remediation commit.

---

## Implementation Waves and Task Status

All tasks are sequential because they extend the same purge contract and job/UI flow.

- [x] [Task 01 — Backend purge engine](task-01-backend-purge-engine.md)
- [x] [Task 02 — Bulk quarantine API](task-02-bulk-quarantine-api.md)
- [x] [Task 03 — Frontend quarantine domain](task-03-frontend-quarantine-domain.md)
- [x] [Task 04 — Responsive quarantine UI](task-04-responsive-quarantine-ui.md)
- [x] [Task 05 — Operator documentation](task-05-operator-documentation.md)
- [x] [Task 06 — Desktop and mobile E2E](task-06-storage-quarantine-e2e.md)

## Risks

- Force deletion must remain a retention-only override; sharing a boolean too low in the filesystem
  layer could accidentally skip unrelated validation.
- Bulk jobs can partially succeed. Job state, result details, cached overview invalidation, and UI
  reload must agree so deleted rows do not linger and failed rows remain actionable.
- Scheduled deletion time is conditional on the idle gate. UI copy must show the exact eligibility
  timestamp without promising a run time the scheduler cannot guarantee.
- A resource-specific maintenance selection must not silently acquire the new quarantine provider.

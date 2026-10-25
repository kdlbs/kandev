---
spec: docs/specs/agents/requirements/runtime-updates.md
created: 2026-07-31
status: complete
---

# Fix Plan: Handle Same-Version Agent Runtime Updates

## Overview

The update preview already reports both the installed and upstream versions,
but the shared approval predicate checks only that the current version is
present. Render an identical target as one version with an **Up to date**
status, tighten the predicate so that preview is non-actionable, then cover the
shared desktop dialog and mobile drawer and document the no-op state.

## Confirmed root cause

`canApproveUpdate` in
`apps/web/components/settings/agent-runtime-update-control.tsx` returns true for
any successful preview with a non-empty `current_version`. It never requires a
non-empty `target_version` or compares the two values, so a preview such as
`0.64.0 → 0.64.0` both suggests a transition that does not exist and enables
**Approve update**, which can send the update POST.

## Frontend

### Shared approval predicate

Update `canApproveUpdate` so approval requires non-empty current and target
versions whose reported values differ, in addition to the existing preview,
loading, maintenance, and request-state guards. Keep the normal upgrade and
failed-job retry paths unchanged when current and target versions differ.

Update the version summary in `UpdateBody` so identical reported values render
the version once with a visible **Up to date** status and no arrow. Differing
versions continue to render the existing `current → target` transition.

### Desktop and mobile behavior

The existing desktop `Dialog` and phone `Drawer` both render `UpdateFooter`, so
the state rule remains shared rather than duplicated. This fix does not change
composition, navigation, scrolling, safe-area behavior, or touch targets. The
nearest mobile exemplar remains the shipped agent runtime update drawer in
`mobile-agent-runtime-update.spec.ts`; focused mobile Playwright coverage will
prove that the same-version preview is visible and its shared approval action
is disabled without sending a POST.

### Public documentation

Update `docs/public/agents-and-profiles.md` to say that a matching target is
shown once as **Up to date** and **Approve update** is available only when the
upstream target differs from the reported current version.

## Tests

- **What:** A successful preview with identical non-empty current and target
  versions shows a single version with **Up to date** and cannot start an
  update.
  **File:** `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`.
  **How:** Configure the existing route fixture with `0.64.0` for both values,
  open the desktop dialog, assert the single-version summary, **Up to date**
  status, absence of the arrow transition, disabled confirmation button, and a
  zero POST count.
- **What:** Existing differing-version approval and failed-job retry behavior
  stays enabled.
  **File:** Existing scenarios in
  `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts` and
  `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`.
  **How:** Run both focused spec files after the predicate change.
- **What:** Public documentation remains structurally valid.
  **File:** `docs/public/agents-and-profiles.md`.
  **How:** Run the public-doc validators.

No new public function or API contract is introduced; the private version
resolution helper and approval predicate are covered by focused unit tests and
the rendered shared action is exercised through the mocked update boundary.

## E2E Tests

- **Desktop scenario:** **GIVEN** current and target are both `0.64.0`, **WHEN**
  the operator opens the update dialog, **THEN** it shows `0.64.0` once with
  **Up to date**, omits the arrow transition, disables **Approve update**, and
  sends no update request.
  **File:** `apps/web/e2e/tests/settings/agent-runtime-update.spec.ts`.
- **Mobile scenario:** **GIVEN** the same preview on a phone viewport, **WHEN**
  the operator opens the update drawer, **THEN** it uses the same single-version
  **Up to date** presentation, the shared approval action is disabled, and no
  update request is sent.
  **File:**
  `apps/web/e2e/tests/settings/mobile-agent-runtime-update.spec.ts`.

## Implementation Wave

- [x] [Task 01: Disable same-version approval](task-01-disable-same-version-approval.md) — done

Execution is sequential in the primary conversation. No subagents are
authorized.

## Risks and out of scope

- Equality is based on the two reported preview strings. Normalizing alternate
  SemVer spellings such as `v0.64.0` versus `0.64.0` is out of scope.
- The update icon remains available so operators can open a fresh read-only
  preview; this change disables only the confirmation action after equality is
  known.

## Stale preview guard

The backend resolves the target and then re-reads the current runtime version
before invoking the package command. If the versions now match, it completes a
successful no-op job with an explicit message and skips the update and refresh
commands.

---
created: 2026-09-08
status: done
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Secret reference protection

## Overview

Issue #3500 traces to unchecked secret deletion followed by strict runtime lookup of the retained secret ID.
The user authorized implementation after a confident diagnosis. The source trace confirms this path.

## Scope and technical approach

Protect user-facing secret deletion through `secrets.Service`, wire reference discovery in `backendapp`, and return structured HTTP/WS conflicts.
Add source-specific recovery guidance to runtime errors. Preserve scope checks and secret identity.
Automatic rebinding, scope-transfer changes, and the separate worktree recovery symptom are excluded.

## Tests

The work order owns targeted service/HTTP/WS, reference-discovery, and runtime regression tests for acceptance criteria .9 through .11.
Handler tests provide end-to-end API evidence without a browser or live instance.

## Work orders

- [x] [Task 01: Protect secret references](task-01-protect-references.md)

## Verification results

All work-order checks passed. The backend tests reported 154 passing secrets/environment tests, nine reference-checker tests, and three lifecycle tests.
The UI tests reported 13 passes, including the structured conflict toast. TypeScript, targeted Go/frontend lint, localization, specification, and public-documentation checks passed.
The first regression runs reproduced unsafe deletion and missing repair/profile information before the corresponding fixes.
Issue #3500 is assigned to `carlosflorencio`. The implementation is committed in PR #3503; review fixup is in progress.

## Risks

The pre-delete check does not serialize concurrent profile saves. Forced deletion retains broken references by design.

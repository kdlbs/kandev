---
created: 2026-10-21
status: implemented
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Implementation Plan: Workspace secret edit persistence

## Overview

Workspace-scoped secret edits always failed with "secret not found", and after the
persistence fix a revealed value could still show its stale pre-edit snapshot across an
edit. One work order fixes the scoped update routing and replaces the edited secret's
row with the edit form. Workspaces owns this change because it owns secret scope and
the workspace settings surface.

## Scope

### In scope

- Workspace-scoped update writes and read-back through a workspace-aware store surface.
- Hiding the edited secret's list row while its edit is open or in flight.
- Service, registered HTTP/WebSocket, component, and desktop/phone E2E coverage for the edited-secret flow.

### Out of scope

- Global secret updates, creation, deletion, copy/move, and reveal authorization.
- Secret encryption, binding, and reference-protection behavior.

## Technical approach

Follow the [system design](../../specs/workspaces/system-design/repository-secrets.md).
Route workspace writes through `WorkspaceSecretUpdater` and scope the post-update read
to the same workspace. Filter the edited row from the workspace and global settings
lists during an open edit so the row's local reveal state cannot survive the save.
Split REQ-WORKSPACES-REPOSITORY-SECRETS-002 into its own requirements file so the
repository-secrets requirements file stays within the specification size limit after
adding AC-WORKSPACES-REPOSITORY-SECRETS-001.15.

## Tests

- `go test ./internal/secrets/...` covers the scoped update service tests.
- `apps/backend/internal/backendapp/secret_workspace_update_integration_test.go` covers
  the HTTP workspace update path.
- `apps/web/components/settings/secrets-settings.lifetime.test.tsx` covers the
  row-replacement rendering contract.

## E2E tests

| File under `apps/web/e2e/tests/` | Project and evidence |
| --- | --- |
| `settings/secrets-edit.spec.ts` | chromium: workspace edit persists name and value; a reveal-then-edit no longer shows the stale value. AC-WORKSPACES-REPOSITORY-SECRETS-001.15. |
| `settings/mobile-secrets-edit.spec.ts` | mobile-chrome: Workspace and Global edits hide the old reveal, persist after reload, and reveal the rotated value; touch targets and no horizontal overflow. |

## Work orders

- [x] [Task 01: Persist workspace secret edits](task-01-persist-workspace-secret-edits.md)

## Verification results

Validation passed on 2026-10-21:

- `go test ./internal/secrets/...`: ok.
- `pnpm --filter @kandev/web exec vitest run components/settings/...`: 40/40 passed.
- Full web Vitest suite: green except four pre-existing locale-dependent failures in
  `routine-detail-view.test.tsx`, reproduced on a clean tree without this change.
- `pnpm e2e:run -- e2e/tests/settings/secrets-edit.spec.ts`: 2 passed.
- `make typecheck`, `make lint-web`, `pnpm run i18n:ratchet`, `pnpm run i18n:check`: clean.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`:
  passed after the AC.15 addition and the REQ-002 split.
- Local PR-documentation `validateCoverage` preflight: covered, no errors.
- Manual verification in a secondary dev instance by the reporting user.

## Independent PR validation

The follow-up adds registered HTTP/WebSocket regression coverage using the real
user-visible wrapper and encrypted SQLite store. It rejects a foreign owner,
wrong workspace, omitted workspace, and explicit workspace authorization denial
before proving a successful rotation and metadata-only response. A fresh store
and reloaded on-disk key can decrypt the committed value. This does not simulate
a process restart or the full authentication middleware.

Phone coverage follows the shipped Settings secret-create card/form pattern,
with the existing Settings content scroll owner and floating Save action.
Desktop and phone tests verify persistence after a page reload; phone tests also
cover Global edits and measure touch targets. No new production behavior or
authorization policy is introduced by the follow-up.

Public how-to guidance in `docs/public/agents-and-profiles.md` now explains
rotation, hidden reveal state during editing, and the existing running-process
environment snapshot boundary.

Independent follow-up checks on 2026-10-21:

- Complete secrets package with `-trimpath -race -count=1`: 82 top-level tests
  passed; eight PostgreSQL tests skipped because no test DSN was configured.
- Focused secrets/backendapp race checks passed. Registered HTTP and WebSocket
  update cases both passed with the real wrapper and encrypted SQLite store.
- Three focused web suites: 23 tests passed, including failed-save retention and
  cancellation. The original row-visibility case fails against the pre-PR UI.
- Managed Docker E2E, one worker and retries disabled: desktop 2/2 and phone
  2/2 passed. Phone built the production assets; desktop reused those identical
  assets. The backend/SQLite/key are disposable; the agent is mocked.
- Go overlays independently reproduce the scoped-write failure in the pre-PR
  service and the final-read failure when only the write is repaired.
- Scoped Go lint and ESLint, public-doc validators, specification catalog/lint,
  and localization checks passed. Broader contributor validation listed above
  was not independently repeated.

## Risks

- Revealed values are per-row local state, so a saved edit re-hides the value; a future
  refinement could re-fetch and keep it visible.

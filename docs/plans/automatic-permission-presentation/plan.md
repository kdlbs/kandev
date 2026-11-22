---
created: 2026-10-29
status: complete
requirements:
  - REQ-AGENTS-PERMISSION-CONTROL-INTEGRITY-003
system_design:
  - ../../specs/agents/system-design/automatic-permission-presentation.md
legacy_specs: []
---

# Fix Plan: Automatic Permission Presentation

## Overview

Keep automatic permission decisions out of the user-input presentation while
preserving their durable audit. Implement the complete presentation path first.
Then add browser evidence that observes the entire decision interval on desktop
and phone. Execute both work orders sequentially.

The [requirement](../../specs/agents/requirements/permission-control-integrity.md)
adds criteria 003.12-003.17 to the existing permission contract.
The [design](../../specs/agents/system-design/automatic-permission-presentation.md)
owns message metadata, fallback, replay, attention projections, and chat behavior.
Agents owns the contract because it owns permission policy and provider requests.

## Confirmed defect

Task `9e34f096-5637-4cce-be21-1ce74b367c19` used session
`474a1aa6-34e0-40bb-9903-b051d75bf5fa` and native ACP session
`7220690b-1c4c-4ecd-96ef-33225ec63159` on 2026-10-29.
Claude reported `acceptEdits`, and the profile enabled `auto_approve`.

| Command group | Record created (UTC) | Automatic approval finalized (UTC) | Elapsed (ms) |
| --- | --- | --- | --- |
| Git fetch/rebase | 21:36:06.524938 | 21:36:06.596430 | 71.49 |
| Git log/show | 21:36:19.186766 | 21:36:19.293891 | 107.13 |

These intervals measure database creation to finalization, not browser visibility.
Both decisions recorded `auto_approve`, `allow-once`, and `accepted`.
All five tools completed. A separate browser cancellation followed at
21:36:24.868 UTC.

`process.Manager.permissionRequestEvent` already identifies an automatic
candidate. `handlePermissionRequest` creates its transcript row before the
durable resolution claim. The initial message omits presentation intent.
`parsePermission` treats its absent status as pending, and
`isPermissionVisible` hides it only after approval. The resulting add/update
sequence explains the flash.

Evidence came from read-only SQLite, backend logs, and ACP session notifications.
The raw ACP logger does not capture permission request/response RPC frames.
The resolution audit supplies those decisions. No provider or launch defect
is inferred from the flash.

## Scope

### In scope

- Initial persisted presentation intent, before publication or hydration.
- Shared classification for standalone, ordinary tool, and Kandev tool rows.
- Permission attention projections that describe input owed by a person.
- Safe interactive fallback, unavailable feedback, replay, and stale updates.
- Existing audit, exact request identity, and provider response ordering.
- Desktop/phone browser evidence and the public auto-approval explanation.

### Out of scope

- Changing permission modes, profile defaults, or provider command classification.
- Adding an allowlist, approval timer, new audit viewer, or runtime flag.
- Changing external MCP live enumeration, authorization, or resolution options.
- Redesigning chat layout or implementing a general permission recovery engine.
- Office-owned permission policy and unrelated storage-outage recovery.

## Technical approach

1. Add typed `permission_presentation` message metadata with `automatic`,
   `interactive`, and `unavailable` values. Missing or unknown values retain
   legacy behavior. Replace the internal optional decision argument with
   `PermissionRequestMessageOptions` so intent reaches the initial create.
2. Preserve `message -> claim -> deliver -> finalize`. Promote a failed
   automatic attempt to interactive only for the same live, unclaimed request.
   Use an identity-qualified conditional update. Never reopen a dispatch claim.
3. Add a pure classifier under `apps/web/lib/permissions/`. Use it in transcript
   filtering, permission rendering, and attention selectors. Keep unavailable
   feedback non-actionable. Reuse existing localized labels.
4. Update SQL pending-action and pending-interaction projections consistently.
   Rank the current permission before classification. Do not resurrect an older
   manual row when the newest row is automatic. Keep live runtime enumeration
   available to `ResolveAgentPermission`.
5. Preserve persisted intent and message freshness across hydration, reconnect,
   duplicate events, and replay. No schema migration is required.

### Compatibility matrix

| Shape | Intended behavior | Evidence/fallback |
| --- | --- | --- |
| Claude ACP candidate with existing candidate fields | Hidden input presentation until resolution | Source incident, backend and browser tests |
| Other transport with the same normalized candidate fields | Same shared presentation | Provider-neutral event/component fixtures |
| Legacy already-approved automatic event | Approved history, no input prompt | Existing decision-event tests |
| Unmarked manual request or unknown metadata value | Existing interactive behavior | Manual browser control |
| No offered allow choice, including no options | Existing visible permission path | Existing selection tests, renderer cases |
| Stale generation or uncertain dispatch | Non-actionable failure/recovery feedback | Service identity/race tests |

### Companion package inventory

The original [integrity plan](../agent-permission-control-integrity/plan.md)
and [approval-audit remediation](../agent-permission-pr3886-remediation/task-04-auto-approval-audit.md)
record completed selection and audit work. The
[external-resolution plan](../external-agent-permission-resolution/plan.md)
owns live authority and claims. Preserve their completed records and guarantees.
This package adds presentation coverage. It does not reopen their work orders
or claim their historical tests observed transient DOM state.

## UI preview

Views use state tables to follow the session's Kandev rich-output instruction.
The labels remain stable across both work orders. Structure is required.
The example command and wording are illustrative and must use existing
localization at implementation.

| View | Entry and state | Required composition |
| --- | --- | --- |
| UI-01: Automatic approval | Task chat, automatic decision in progress | Ordinary running tool row. No approval row, buttons, amber input state, or added prompt space. |
| UI-02: Interactive fallback | Same chat, confirmed live request with no dispatch claim | Tool/request title, existing offered choices, existing reply behavior. |
| UI-03: Unavailable decision | Same chat, failed or uncertain dispatch | Existing localized unavailable/recovery feedback. No approval controls. |

The same composition serves desktop and phone. The phone exemplar is the
existing inline permission flow in `mobile-permission-offered-choices.spec.ts`.
The transcript keeps its single scroll owner. The composer retains its safe-area
behavior. Automatic approval does not move focus or add/remove a prompt region.
Fallback choices retain keyboard access and 44px phone touch targets.

## Tests

All AC suffixes below belong to `AC-AGENTS-PERMISSION-CONTROL-INTEGRITY-003`.

| Criteria | Targeted evidence |
| --- | --- |
| .4, .9, .11, .12 | `TestAutomaticPermissionPresentationIsStoredBeforePublication`, existing claim-before-delivery test, replay integration test |
| .13, .14 | `TestAutomaticPermissionFallbackRequiresLiveUnclaimedRequest`, `TestAutomaticPermissionFailureDoesNotReopenDispatch` |
| .12, .16 | SQLite/Postgres pending-projection cases, including a superseded manual row and a manual request in another session |
| .12, .14, .17 | Pure classifier, attached/standalone component tests, unavailable feedback |
| .15 | Message merge tests, duplicate candidate replay, reload/reconnect browser cases |
| .12, .13, .17 | Browser observation armed before prompt submission, plus a real manual response on each viewport |

The first RED must assert that an automatic candidate retains its presentation
before publication. A second RED must render that unresolved message and reject
any input prompt. Compilation or missing-selector failures are not behavioral RED.

## E2E tests

- Add `e2e/tests/chat/automatic-permission-presentation.spec.ts` on `chromium`.
- Add `e2e/tests/chat/mobile-automatic-permission-presentation.spec.ts` on
  `mobile-chrome` with its configured Pixel 5 device.
- Share request-scoped observation/gating in
  `e2e/helpers/automatic-permission-presentation.ts`.
- Open an idle task before submitting `/e2e:git-commit-permission`. Observe DOM
  changes from submission to completion, then verify the commit and audit.
- Hold only browser-bound completion frames for the exact request when a
  deterministic pending interval is needed. Gate matching snapshot reads too.
  Preserve unrelated frames and release every gate in `finally`.
- Use seeded message states for rendered fallback/unavailable cases. Backend
  tests prove real failure classification. Do not call a seeded state a real
  runtime failure. Include a real attended mock-agent request that completes
  after a desktop click or phone tap.
- Retain existing manual approval, offered-choice, and unattended Git coverage.

## Work orders

- [x] [Task 01: Carry automatic presentation through permission delivery](task-01-permission-presentation.md)
- [x] [Task 02: Prove desktop and phone behavior](task-02-browser-evidence.md)

## Documentation and lifecycle

The design turn changed internal requirements, design, and delivery documents.
Implementation in task 02 updated the public auto-approval explanation in
`docs/public/agents-and-profiles.md`, replacing the obsolete first-option and
empty-option cancellation description with the allow-or-prompt contract.
This plan and both work orders record the completed implementation results.
No new ADR is needed: presentation preserves the existing live-authority ADR.

## Verification results

Design-package checks passed on 2026-10-29:

- `python3 scripts/list-docs.py validate`: 420 decisions and 1849 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/automatic-permission-presentation`: passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: both work orders passed
  the local preflight with a representative planned runtime change.
- Catalog discovery includes the amended requirement and the presentation design.

Implementation checks passed on 2026-10-30:

- Backend targeted suites passed for `internal/backendapp`, `internal/orchestrator`,
  `internal/task/service`, `internal/task/repository/sqlite`, and
  `internal/integration` with the permission/presentation/pending filters.
- `scripts/lint-go-changed` equivalent per-package runs reported 0 issues for
  every changed Go package.
- Web `tsc --noEmit`, ESLint on all changed files, `i18n:check`, and 138 unit
  tests across eight changed suites passed.
- The new desktop spec passed all four cases. The new phone spec passed all four
  cases. Re-running the desktop permission group passed 9 tests, and the phone
  permission group passed 5 tests.
- Public docs validation passed for 54 pages, and the docs test file passed 63
  tests.
- `git diff --check` passed.

The full SQLite repository package and the full orchestrator package exceeded
their local 10- and 15-minute package budgets under concurrent workspace load.
The changed permission/presentation tests inside both packages passed in the
targeted runs above.

## Risks

- Filtering before newest-row selection can reactivate stale user prompts.
- A global profile-based suppression can hide a real interactive request.
- A failed final write does not prove that provider delivery failed.
- A terminal-state-only browser assertion can miss the entire defect.
- Old clients ignore additive metadata. The deployed backend and web bundle
  must include the repair together for the full presentation outcome.

---
created: 2026-09-14
status: implemented
requirements:
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-001
  - REQ-AGENTS-NO-SILENT-MODEL-FALLBACK-002
system_design:
  - ../../specs/agents/system-design/no-silent-model-fallback-01.md
  - ../../specs/agents/system-design/no-silent-model-fallback-02.md
legacy_specs: []
---
# Implementation Plan: Exact Profile Model Identity as an Explicit Opt-in

## Overview

Revise PR #3473 so existing remote-executor launches keep their pre-PR model
fallback behavior. Add Require exact model as a per-profile opt-in, default off.
Implement storage/API/runtime policy first, then profile editors, then end-to-end
acceptance and public documentation. All new work orders are sequential.

This package was requested on 2026-09-15 and is implemented as a maintainer
fixup on the contributor branch. The plan, work orders, requirements, design,
tests, and public guidance are included with the implementation.

## Baseline and completed work

Implementation baseline: `561d2a8cf0d5c30224696304839a550952331ebe`.
Reviewed PR head: `561d2a8cf0d5c30224696304839a550952331ebe`.
The two intervening commits only retrigger CI; their tree diff is empty.
Compatibility oracle: pre-PR merge base
`ba960f973205854733e0dcd9afa355bdda3dddf6`.
The implementation was based on this contributor head and preserves the
completed transport and reuse fixes.

Task 01 records the completed strict-by-default implementation. Preserve its
transport staging, replacement catalog event handling, single validated reuse
lookup, and durable warnings. Its old inferred strict predicate is superseded
by the new explicit field. Existing green tests do not prove upgrade compatibility.

## Scope

### In scope

- False-by-default persisted `require_exact_model` through every profile path.
- Strict mode precedence over dormant fallback values; no destructive migration.
- Compatible launch/error behavior, including runtime-only unique variations.
- Shared launch/reset/rebind/recovery policy and explicit strict workflow reuse.
- Desktop/mobile profile editors, settings discovery, copy, and public guides.
- Actual store-upgrade-to-launch evidence and strict/non-strict regressions.

### Out of scope

Global/workspace/executor policy, runtime feature flags, provider authentication,
Office post-start routing changes, a live-session model lock, new provider APIs,
and unrelated PR cleanup. Do not recreate completed transport fixes.

## Technical approach

The [requirement](../../specs/agents/requirements/no-silent-model-fallback.md)
and [policy design](../../specs/agents/system-design/no-silent-model-fallback-01.md)
own the behavior matrix, migration, precedence, and transport contracts.
[Recovery and UI design](../../specs/agents/system-design/no-silent-model-fallback-02.md)
owns replacement-catalog handling, workflow eligibility, and profile surfaces.
The [ADR](../../decisions/2026-09-15-explicit-profile-model-strictness.md)
records why strictness is per profile and why old auto-fallback fields remain.

Extend the existing profile store and DTO contract, `StartModelPolicy`, profile
resolvers, executor/backend adapters, and profile snapshots. Gate strict checks
on the new field. Preserve legacy omissions on partial update. Compatible mode
must not be implemented by setting `AutoFallback=true`: that changes fallback
priority and error tolerance. Extend existing React shared fallback components.
No new state-management or provider abstraction is needed.

## ASCII UI preview

### UI-01: Desktop profile, expanded Fallback settings

Entry: Settings > Agents > profile. Default state; strictness is off.

```text
Start model                      [ saved model             v ]
Fallback settings v              Executor default allowed
+------------------------------------------------------------+
| Require exact model                              [ OFF ]   |
| Stop before starting if the selected model cannot be used.  |
| Fallback settings do not apply while this is on.            |
+-----------------------------+------------------------------+
| Automatic fallback [ OFF ]  | Explicit fallback    [ OFF ] |
| Existing help and controls  | Existing help and controls   |
+-----------------------------+------------------------------+
                                      [ Cancel ] [ Save ]
```

Strict on: summary becomes "Exact model required". Both fallback cards remain
visible and disabled, including their switches. Existing values remain saved.
Strict off restores their prior states. The fallback selector appears only when
its existing enable switch is on.
A strict+empty-model validation error appears beside the model field; Save
keeps the draft open. Collapsed state retains the summary and dirty indicator.

### UI-02: Phone profile, expanded Fallback settings

```text
< Agent profile
Start model
[ saved model                    v ]
Fallback settings v
Executor default allowed
+----------------------------------+
| Require exact model      [ OFF ] |
| Stop before starting if the      |
| selected model cannot be used.   |
| Fallback settings do not apply  |
| while this is on.                |
+----------------------------------+
| Automatic fallback       [ OFF ] |
| Visible helper text        (i)   |
+----------------------------------+
| Explicit fallback        [ OFF ] |
| Visible helper text        (i)   |
+----------------------------------+
[ Cancel ]                 [ Save ]
```

The existing page/editor scrolls as one form. Help opens the shared inset
bottom drawer and returns focus on close. Switch rows and help targets have
at least 44px touch hit areas; desktop ordinary controls keep 28px sizing.
Strict-on, collapsed, and validation states have the same semantics on phone.
Long translations wrap without horizontal overflow or hiding Save.

Order, grouping, visible disabled values, and shared state are requirements;
ASCII spacing and the illustrative model names are not pixel specifications.
UI-01/UI-02 map to AC-AGENTS-NO-SILENT-MODEL-FALLBACK-001.6 and .7.

## Tests

The following checks were run after implementation.

| Acceptance | Evidence |
| --- | --- |
| 001.1/.4/.5, 002.1 | Lifecycle policy table and focused lifecycle run passed. It covers strict/compatible selection, fallback precedence, apply errors, and unique variations. |
| 001.3/.6 | SQLite migration, round-trip, explicit clear, schema replay, controller validation, and generated discovery checks passed. Postgres evidence was not run because no isolated `KANDEV_TEST_POSTGRES_DSN` was available. |
| 001.1/.3/.8 | Lifecycle reset, rebind, replacement, and profile-resolution regressions passed in the focused run. |
| 001.8 | Replacement-client catalog, cancellation, and strict reset/rebind tests passed in the focused lifecycle run. |
| 001.9 | Focused orchestrator model/profile/reuse/drift tests passed, including strict unknown drift and the validated empty candidate path. |
| 001.2/.10 | Existing desktop/mobile mismatch E2E tests passed with explicit strict fixtures and persisted compatible warnings. |
| 001.6/.7 | Frontend normalization, settings, editor, dirty/reconciliation, and CLI tests passed; typecheck, i18n checks, and targeted lint passed. |

## E2E tests

Extend existing specs; do not replace strict fixtures with compatible fixtures
and lose strict coverage. Set `require_exact_model=true` in strict fixtures.

| File under apps/web/e2e/tests | Project | Acceptance and scenario |
| --- | --- | --- |
| `settings/no-silent-model-fallback.spec.ts` | chromium | 001.6/.7/.10: default off, enable/save/reload, disable restores dormant settings, strict validation keeps form open, desktop help and dimensions |
| `settings/mobile-no-silent-model-fallback.spec.ts` | mobile-chrome | 001.6/.7: same edit/save/reload flow, drawer dismiss then toggle/save, 44px hit targets, no overflow, long labels |
| `session/model-mismatch-warning.spec.ts` | chromium | 001.1/.2/.3/.4/.5: old-style profile omits field and starts on default with one reloaded warning; explicit strict profile fails before prompt; explicit fallback and auto=true remain distinct |
| `session/mobile-model-mismatch-warning.spec.ts` | mobile-chrome | 001.1/.2/.6: phone selection/launch reflects saved compatible/strict policy; warning survives reload |

Mock E2E proves the user flow. The store-to-transport regression proves actual
migration compatibility; the Kubernetes transport test proves replacement
catalog ownership. None substitutes for the others.

## Work orders

- [x] [Task 01: Historical strict identity implementation](task-01-enforce-launch-identity.md)
- [x] [Task 02: Persist and apply explicit strictness](task-02-explicit-profile-policy.md)
- [x] [Task 03: Expose profile controls on desktop and phone](task-03-profile-strictness-controls.md)
- [x] [Task 04: Prove user flows and update public guides](task-04-compatibility-acceptance.md)

## Verification results

Implementation checks passed on 2026-09-15 unless noted below:

- `python3 scripts/list-docs.py validate`: 269 decisions and 902 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed.
- Backend focused lifecycle, settings, controller, handlers, executor, backendapp,
  and orchestrator checks passed. A broader combined run reached unrelated
  pre-existing orchestrator failures and an SSH close test timeout; the changed
  focused tests passed.
- Frontend typecheck and 83 focused Vitest tests passed.
- Desktop settings E2E: 3 passed. Mobile settings E2E: 3 passed.
- Desktop session E2E: 2 passed. Mobile session E2E: 2 passed.
- Public docs validators and specification validators passed.
- `go run ./cmd/settings-catalog -check`, `pnpm run i18n:check`, and
  `pnpm run i18n:ratchet` passed.
- Package-relative links and work-order REQ/AC references resolve.
- Catalog queries discover the amended specs and the new decision.

The Postgres migration check remains explicitly pending until an isolated test
DSN is available. The implementation does not change the Postgres migration
contract, but SQLite and all application projections are covered.

## Risks

- Missing a DTO/draft/copy path can silently clear or drop strictness.
- A bulk auto-fallback migration can change fallback priorities and tolerated errors.
- Treating a recovery timeout as transport success can hide a broken session.
- Old session snapshots omit the new field; they must not imply strictness.
- Restoring compatible variation selection must never affect strict mode or edit saved IDs.
- Existing custom TUI/dynamic profiles do not have an enforceable concrete ACP model.
- Supported Postgres migration evidence requires an isolated test database; a skipped test is not a pass.

---
created: 2026-10-05
status: completed
requirements:
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
system_design:
  - ../../specs/workspaces/system-design/repository-secrets.md
legacy_specs: []
---

# Implementation Plan: Workspace secret list isolation

## Overview

Preserve Workspace Secrets metadata during unrelated Global loading. One sequential work
order adds real-reactivity regression coverage, separates the existing hook effects, and
verifies compatibility. Workspaces owns the metadata scope and lifetime; the existing
requirement/design pair remains authoritative. ROOT reviewed the design package and released
implementation in this primary session; local implementation is complete. Hosted delivery
and verified merge remain separate gates.

## Exact source and test scope

- Production: `apps/web/hooks/domains/settings/use-secrets.ts` only.
- New regression tests: `apps/web/hooks/domains/settings/use-secrets.lifetime.test.tsx` and
  `apps/web/components/settings/secrets-settings.lifetime.test.tsx` only.
- Existing compatibility suites run unchanged: `use-secrets.test.ts` and
  `components/settings/secrets-settings.test.ts`.
- Delivery documentation: the owning repository-secrets requirement/design and these two
  plan/work-order files. The amended owner remains active/current; the plan/work order
  records completed local implementation. Hosted delivery evidence stays in the task plan.

## Scope

### In scope

- AC-WORKSPACES-REPOSITORY-SECRETS-001.14: independent Workspace metadata and read lifetime.
- Compatibility controls for .1/.2/.4: existing scope identity, Global sharing/filtering,
  local Workspace mutations, initial list replacement, and request cleanup.
- Faithful React provider/store/hook and rendered settings acknowledgment tests.

### Out of scope

- Secret values, reveal, access, authorization, backend/API/schema, profile binding policy.
- Generic caches, cross-instance Workspace sharing, stale-navigation mutation callbacks,
  same-list read/mutation ordering, error/retry redesign, and new frameworks.
- UI layout, copy, touch, navigation, browser/build/E2E changes; test-only production
  callbacks/modules; dependency/lockfile/harness changes and unrelated sibling work.

## Root cause and accepted evidence

The single `useSecrets` effect includes `globalLoaded` and `globalLoading` alongside
Workspace initialization and cleanup. A second Global consumer changes those store flags;
the Workspace hook reinitializes from `initialItems` or an empty array and cancels/restarts
its read. Accepted local creation/rename/deletion is lost even though the workspace identity
and initial list have not changed. Existing hook tests replace store subscriptions with a
mock selector, and existing settings tests mock the hook/store, so they miss this boundary.

ROOT's immutable archive `/tmp/kandev-secrets-global-hydration-repro.test.tsx` has mode 0444
and SHA256 `912216472bf6ba447cd629c40a99681d0a2ec8b4f95e6d917af90bcca8730d71`.
The companion `/tmp/kandev-root-secrets-hydration-proof-receipt.json` and
`/tmp/kandev-root-secrets-hydration-repro.log` record real `StateProvider`/`createAppStore`/
`useSecrets` with only `listSecrets` mocked: ordinary Workspace add passed, while the
second Global consumer published its rows/flags and removed the accepted Workspace row
during and after its load. Actual Vitest exit 1, one pass/one expected failure; handle
17872 ACTUALLYJOINED; 2.72s total/36ms tests. The earlier unpinned pnpm attempt failed
before tests and supplies no test result. The temporary repository source was removed.

Proof base: `e095ca17790d3dd0e4700b473780add77ac7a230`. Design HEAD:
`3328fe887f0e9ea2ffb11a00c4c5d0a94a7b88ed`. Relevant hook, settings, initial-items route,
provider, store, and settings-slice blobs match the proof base. The archive was read and
hashed without replay or removal. Permanent regression RED and GREEN results appear below.

## Technical approach

Split the combined effect into guarded Global and Workspace effects in the same hook.
Keep Global store selectors/setters and the existing loaded/loading sharing gate. Keep
Workspace local items/flags, `scopedKey`/`loadedScopedKey`, initial-list reference semantics,
and abort/cancel cleanup. Only scope/Workspace identity/initial-list changes may initialize
the Workspace list. Neither Global pending nor successful/failed settlement may restart it.

No new state container or callback abstraction is needed. Global responses retain
`filterGlobalSecrets` behavior, including legacy metadata without a scope. Workspace scope
switches still hide previous rows; explicit empty initial lists still count as loaded;
missing workspace IDs still suppress fetching. Workspace reads retain existing failure
settlement and reject publication after cleanup.

### Direct consumer inventory

All direct calls except Workspace `SecretsSettings` use the default Global scope:

- `components/settings/secrets-settings.tsx` (Global and Workspace).
- `components/app-sidebar/sections/settings/settings-tree.tsx`.
- `components/settings/agent-profile-page.tsx`.
- `components/settings/plugin-executor-profile-page.tsx`.
- `components/settings/profile-edit/profile-env-vars-section.tsx`.
- `app/settings/agents/[agentId]/page.tsx`.
- `app/settings/executors/[profileId]/page.tsx`.
- `app/settings/executors/new/[type]/page.tsx`.

These paths are under `apps/web`. `app/settings/workspace/[id]/secrets/page.tsx` supplies
`initialItems`; active SPA `src/settings-routes.tsx` renders Workspace `SecretsSettings`
without them and renders default Global settings separately. No consumer changes are needed.

## Tests

The acceptance matrix below is covered by the two new suites. They use the real
`StateProvider` (which constructs `createAppStore`), real `useAppStoreApi` probes and
`useSecrets`, deferred transport, and distinct Global/Workspace fixtures. Only transport
functions are mocked; never mock hooks, store, settings UI, save coordinator, or row widgets.
Use real `ToastProvider`, `SettingsSaveProvider`, and required existing UI providers for
settings tests. Helpers stay within the new test files. Settle owned deferred promises and
unmount/cleanup all renders in every path.

| Acceptance | Test file and acceptance case | Decisive assertion |
| --- | --- | --- |
| .14 | `use-secrets.lifetime.test.tsx`: `preserves acknowledged workspace mutations across global loading and settlement` | Add/update/remove metadata survives both Global pending and success/failure; stable supplied list and a settled fetched list; ordinary no-Global controls. |
| .14 | Same: `does not abort or refetch a pending workspace list during global loading` | One admitted scoped transport call, same unaborted signal and Workspace flags during Global pending/success/failure; original Workspace result can still publish. |
| .14 | Same: `keeps a settled workspace list and flags when global loading settles` | No extra Workspace transport call or flag restart; seeded and fetched results persist. |
| .1/.2/.4 | Same: `shares global loading and filters workspace rows` | A second consumer admitted while Global loading sees the same pending state and result without another fetch; loaded consumers reuse it; mixed Global/Workspace/legacy rows filter to Global/legacy. |
| .1/.2 | Same: `replaces scoped data only on intentional workspace or initial-items changes` | A-to-B hides A immediately, aborts A, accepts B, and rejects late A; a new initial-array reference replaces accepted local state; explicit empty supplied list is loaded with no fetch. |
| .2 | Same: `cleans up scoped reads on scope changes and unmount` | Workspace-to-Global aborts scoped read and suppresses its late settlement; a new scope/ID admits its own lifecycle; missing ID never fetches; unmount aborts. |
| .14/.1/.2 | `secrets-settings.lifetime.test.tsx`: `retains acknowledged create rename and delete while a sibling global consumer loads` | Fill real create/edit forms, use real shared Save, preflight and confirm real delete, and resolve only transport acknowledgments; preserved new/renamed rows and absent deleted row before, during, and after successful/failed Global load. |

Parameterize relevant cases without redundant markup assertions. Settings mutation cases cover
both stable supplied `initialItems` and a settled no-initial-items read, with an ordinary
mutation control before mounting the Global consumer. Start Global hydration through a
second real hook consumer in the same provider; do not simulate the defect by setting flags
on a mocked store. Assert Global rows and actual loading/loaded transitions as positive
evidence that the unrelated load ran. Hook read-lifetime cases cover pending Workspace
success and failure independently; no-initial mutation cases acknowledge only after the
initial Workspace read settles, keeping same-resource read/mutation races out of scope.

## Rendered flow and mobile parity

The rendered settings test proves the full client path from form/save/delete interaction to
transport acknowledgment, hook publication, and surviving rendered metadata. This is a
state/data-only correction inside the existing surface, with no layout, touch, scrolling,
navigation, or viewport-dependent interaction change. The mobile-parity narrow exception
allows targeted provider/store and component tests. No ASCII redesign or new Playwright,
browser, Vite build, or E2E command is part of this package.

## Documentation impact

Public authentication and agents/profiles docs already describe the existing Global/Workspace
scope, settings lists, and profile selection. The fix restores that behavior without a new
user operation, terminology, API, or policy. Internal docs only. Reassess public docs and
scoped `apps/web/AGENTS.md` before publication; no architectural convention changes require
an AGENTS edit. Existing repository-secrets delivery records remain historical; no linked
work order is reopened and no migration is expanded. The existing scope-and-merge ADR
continues to own the credential boundary.

## Work orders

- [x] [Task 01: Isolate secret list lifetimes](task-01-isolate-secret-list-lifetimes.md).

One sequential work order. No dependency on the sibling backend content-search task.
Implementation, checks, and full delivery must use this existing primary session. The work
order contains exact commands and resource gates; it does not authorize delegation.

## Verification results

Design checks passed on 2026-10-05: catalog validation (351 decisions/1353
specifications), all 36 specification-linter tests, full specification lint,
owning-pair catalog discovery, and whitespace checks. The projected seven-file
production/test/documentation scope passed the repository's `validateCoverage`
preflight with one changed work order, one accepted owning design/requirement
mapping, and no reference errors. An initial wrapper incorrectly expected four
mapping entries and exited 1 despite validator success; the corrected wrapper
checked the actual single owning mapping and exited 0. No application test was run during design.

Implementation checks passed on 2026-10-05 under ROOT's current local-heavy lease:

- One conditional frozen install used Corepack pnpm 9.15.9; lockfile unchanged.
- Corrected permanent RED: 36 cases, 28 expected behavioral failures and eight compatibility
  passes, exit 1, 12.71s. The first run also exposed a test setup callback returning a mock;
  fixing only that callback removed fixture errors before the production edit.
- Minimal guarded effect split: Global readiness no longer controls Workspace initialization
  or cleanup; all existing scoped identity, supplied-list, and cancellation logic remains.
- Full affected GREEN: four files, 47 cases passed, exit 0, 19.35s. After test formatting,
  the 36 new cases passed again (12.46s). Splitting one long test registration block to
  satisfy lint preserved all 24 hook cases, which passed again (3.02s).
- Changed-file ESLint with zero warnings, TypeScript typecheck, i18n check and base-pinned
  ratchet passed. Typecheck's generators produced no tracked drift. Existing catalog orphan
  warnings are unchanged; no UI copy was added.
- Final catalog, 36 specification-linter tests, full specification lint, actual seven-file
  documentation reference coverage and whitespace checks passed.

The amended owner remains active/current; Task 01 records completed local work. Normal
hooks and hosted CI/review/merge receipts are kept in the durable Kandev task plan. Work-order
completion does not imply publication or merge. Public docs and scoped frontend guidance
were reassessed: existing scope descriptions remain correct, with no new public operation
or engineering convention to document.

## Risks

- Removing all scope/initial-items dependencies would preserve obsolete rows; only Global
  readiness dependencies should leave the Workspace effect.
- Inline new arrays are intentional replacement inputs. Tests must retain the same array
  reference when proving independence and use a distinct reference for replacement controls.
- Mocking the store or hook would hide the regression. Rendered save/delete tests require
  real provider composition; do not alter production APIs to simplify tests.
- ROOT controls the single local-heavy lease and the separate merge lease. A timeout or
  unrelated failure is a checkpoint, not passing evidence or permission to expand scope.

## Delivery checkpoint

All local commands are serial under ROOT's single global local-heavy lease. Return that
lease explicitly after actual publication and zero outstanding local handles; later fixups
require reacquisition. One owned all-terminal hosted collector follows publication. ROOT's
separate merge lease is required after all required gates and substantive current-head
review are complete. Preserve the published candidate unless an actual correction is needed.

---
created: 2026-10-23
status: completed
requirements:
  - REQ-EXECUTORS-SSH-CONNECTIONS-001
  - REQ-EXECUTORS-SSH-CONNECTIONS-004
  - REQ-EXECUTORS-SSH-CONNECTIONS-005
system_design:
  - ../../specs/executors/system-design/ssh-connection-manager.md
legacy_specs: []
---

# Implementation Plan: SSH connections under Executors

## Overview

Place SSH connection management under Executors, where users configure remote
execution. Keep connections reusable and installation-wide. Add contextual key
creation and a reliable return to SSH executor setup so users can finish the
setup without reconstructing entered details.

The user requested planning on 2026-10-23 after questioning the independent
settings entry. The user explicitly requested implementation of this package in a later turn.
Baseline: local commit `b386a0d3d1c2365194ceacb7c6cac82082d1179b` on
`feature/recover-and-resume-s-1mt`. Both work orders are implemented and verified locally.
The user requested a commit and branch push on 2026-10-24. No PR creation,
delegation, or test-instance change is requested.

Sources: [requirements](../../specs/executors/requirements/ssh-connection-manager.md),
[design](../../specs/executors/system-design/ssh-connection-manager.md#settings-and-mobile-composition),
[accepted custody ADR](../../decisions/2026-09-19-managed-ssh-credential-custody.md),
and [original package](../ssh-connection-manager/plan.md).
Current Kandev task: `a2845dea-8086-4b3a-824c-65e1941d4cd8`;
session: `13973a35-0812-478d-a620-2e36314c61f1`.

## Scope

### In scope

- Executors page entry, nested manager route, breadcrumbs, settings search,
  active navigation, and compatibility for old bookmarks.
- Existing manager tabs and all internal links, including adoption and bastion
  setup; contextual identity creation and return to the managed executor editor.
- Desktop, phone, coarse-tablet behavior, localization, and public instructions.

### Out of scope

Backend/API/schema changes, connection scope or permission changes, feature
promotion, remote Docker managed binding, key custody changes, enrollment or
host verification redesign, a general credentials/settings hierarchy, and
resolution of the original package's remaining acceptance gaps. Do not alter
live SSH access or either running Kandev instance to implement this package.

## Technical approach

### 01: Nested navigation and compatibility

Register `/settings/executors/ssh-connections` in `src/settings-routes.tsx`;
dynamic routes currently resolve first. Exclude the reserved `ssh-connections`
segment from the profile matcher, following the existing `agents/browse` guard,
so the static route can render without changing global routing precedence. Keep
`/settings/ssh-connections` as `SettingsRedirect` with the new pathname only.
The existing `resolveSettingsRedirect` preserves query values and hash and uses
history replacement. Test this contract instead of introducing a second redirect
implementation. The backend API prefix remains `/api/v1/ssh/managed`.

Create `lib/settings/ssh-connection-settings-routes.ts` for the canonical href
and typed manager query construction. Move callers in `settings.tsx`,
`managed-binding.tsx`, and `adoption.tsx` to it. Preserve `tab`, `id`, `new`,
`adopt_executor`, `adopt_digest`, `adopt_connection`, and `return_connection`
through applicable navigation. An identity detour in order 02 must also preserve
outer adoption and bastion context; distinguish its return destination explicitly.
The legacy redirect preserves arbitrary existing query/hash values without
interpreting them as effects or external return URLs.

Remove the top-level SSH row in `settings-menu-sections.ts`. Do not add a new
sidebar branch: Executors is the visible parent and its existing prefix matching
marks it active. Add a secondary SSH connections navigation row above profiles
in `app/settings/executors/page.tsx`, including the empty state. Keep it gated by
`useFeature("sshConnectionManager")` and `useIsAdmin`, as on the manager. In
`lib/settings-discovery/catalog/executors.ts`, retain the stable item ID and
page kind, set `parentId: "executors"`, and change href. Check breadcrumb label
resolution and remembered-settings restore, including old stored paths. Add an
exact manager breadcrumb rule before the generic executor-profile pattern so
the title does not become Profile or depend on a profile lookup.

Update public navigation instructions and `docs/public/coverage.json` in the
same implementation slice. Preserve the old route as an explicitly supported
alias in coverage if the schema expects all supported paths.

### 02: Contextual setup without draft loss

`ConnectionDetails` currently offers identity selection but no adjacent creation
action. Add Save draft and create identity beside the field. Reuse the existing
save coordinator/exclusive mutation guard and `IdentityForm`; do not build a
second identity editor. After metadata save succeeds, route to identity creation
with `return_identity_connection=<saved ID>`. Preserve all outer context. After
creation, return with only the public identity ID as a candidate selection;
apply it to the local connection draft, not an automatic server write. Cancel
returns without changing selection. Invalid/deleted records show a recoverable
message and do not bind anything. Clear the one-use candidate after consumption
so Back/reload cannot repeatedly override the user's choice.

`ManagedBinding` currently holds name, selected connection and check state in
local React state; its Manage connections link does not retain them. Introduce
a small flow-specific React context under the persistent settings shell, with
one active continuation. Mount it outside `SettingsSaveProvider key={pathname}`,
which remounts when the route changes; keep the existing save-provider reset.
It contains only the known managed editor route,
name, selected connection ID, new-profile owner ID, and baseline binding evidence. No generic
draft framework, storage persistence, secrets, trust receipt or check success.
Keep new-editor and existing-editor continuations distinct; construct return
routes from validated editor type/ID, never a caller-supplied URL.

Capture the continuation before leaving. Transfer its draft ownership out of
the editor's save contributor through the existing coordinator, without a
Save/Discard prompt that would destroy the intended detour. Do not globally
bypass the navigation blocker. Returning restores the draft contributor and
reloads current records. If server baseline changed, show reconciliation rather
than saving over it; if unavailable, retain a retryable draft. Return to executor
keeps the original selection. Use connection sets a candidate only when that
route is ready. In both cases reset executor-check success and require Test
executor and Save. Neither return operation saves a profile automatically.

Clear the continuation on successful profile save, explicit discard, navigation
outside this setup flow, logout, or settings-shell unmount. Reload may lose this
unsaved executor draft; saved connection metadata remains durable. Browser Back
within the flow restores the same continuation. A direct manager visit has no
executor return action. Leaving identity creation clears secret input even if
the request is still running; a late result must not navigate or select a key.
For nested bastion creation retain the outer executor/adoption context and
return to the destination before allowing use by the executor.

### Preserved boundaries and rationale

Nesting under Executors matches the only current managed consumer. Keeping the
backend shared avoids duplicating identities or pins across profiles. A separate
Security or Connections top-level category would add a new grouping without a
current consumer need. No new ADR is needed: the existing custody decision
already assigns ownership to Executors. This package changes UI placement and
setup continuity within that boundary.

Keep all API/service authorization, disabled-path rejection, strict host trust,
remote-effect confirmations, and legacy agent/file behavior. Only existing public
metadata may enter route context. No test needs a developer host or private key.

## UI preview

These structured views replace ASCII sketches to follow the task's explicit
rich-output instruction. They describe proposed structure, not rendered evidence.

### UI-01: Executors owns SSH connections

Entry: Settings navigation or settings search. The design baseline showed SSH connections
and Executors as peers; the implemented settings menu contains only Executors.

| Region | Desktop | Phone |
| --- | --- | --- |
| Settings navigation | Executors active | Settings index > Executors |
| Page hierarchy | Settings > Executors | Back to Settings; Executors heading |
| First page row | SSH connections, secondary navigation action | SSH connections, full-width navigation row |
| Existing content | Current profiles, then Create New Profile cards | Current profiles and creation cards in normal page flow |
| Manager | Settings > Executors > SSH connections; Connections / Identities; list and detail | Back to Executors; Connections / Identities; list, then focused detail with Back to list |
| Empty | Row available with zero profiles; manager has New connection | Same capabilities, without a compressed table |
| Disabled / non-admin | SSH entry and search result absent; direct route shows existing unavailable state | Same policy |
| Loading / failure | Loading status or retry in manager; no duplicate controls | Same status and retry in one scroll region |

Maps to AC-EXECUTORS-SSH-CONNECTIONS-001.5, 004.4, 004.6, 005.3 and 005.6.

### UI-02: Finish setup in context

Entry: managed SSH executor editor > Manage connections > connection detail.

| Region / state | Desktop | Phone |
| --- | --- | --- |
| Executor | Name; Connection picker; Manage connections; Test executor | Same order, stacked with touch-sized controls |
| Connection identity | Identity picker; Save draft and create identity | Identity picker opens temporary Drawer; creation opens a full-page form |
| Identity creation | Existing Generate / Import / File / Agent form; Create and Cancel | Focused form; Back/Cancel clears secrets and returns to connection |
| Successful creation | Return to same connection; new identity selected but unsaved | Same result; focus returns to identity field |
| Return to executor | Return to executor; Use connection for ready destination only | Separate touch actions in normal page flow |
| Returned editor | Name retained; selected connection retained or explicitly changed; test required | Same state; focus returns to connection picker |
| Failed draft save / stale record | Stay in setup, explain failure, offer retry/reload; no identity request or profile write | Same recovery, no stacked dialogs |
| Cancel / reload | Cancel retains saved connection; reload can clear unsaved executor draft | Same behavior; no secret recovery |

Maps to AC-EXECUTORS-SSH-CONNECTIONS-001.1, 001.2, 004.1, 004.3, 004.4,
004.5 and 004.7. The outer adoption/bastion flow stays intact (005.2, 005.6).

Hierarchy, action order, direct phone navigation, explicit effects, draft
behavior and gates are structural. Labels/spacing are illustrative and must
be localized. Reuse shared settings classes, focus styles and existing tokens;
fine-pointer controls remain 28px and coarse targets at least 44px. One settings
scroll owner, safe-area/keyboard clearance, bounded long values, no horizontal
document overflow. Verify 390px, 767/768px, 1280px and a coarse 820px tablet.

## Tests

The cases/files below are implemented. Existing test files are named explicitly.
Every implementation order starts with failing behavior tests under `/tdd`.
ACs below use prefix `AC-EXECUTORS-SSH-CONNECTIONS-`.

| ACs | File and case |
| --- | --- |
| 004.6, 005.6 | Existing `src/settings-routes.test.ts`, `src/settings-route-helpers.test.ts`: nested static route wins; old alias retains query/hash; Back skips alias |
| 004.6 | Existing `lib/settings-discovery/catalog.test.ts`, `components/settings/settings-breadcrumbs.test.ts`, `components/app-sidebar/sections/settings/settings-tree.test.ts`: nested search breadcrumb, Executors active, no SSH peer |
| 001.5, 005.3 | Existing `settings-tree-render.test.tsx`; new `app/settings/executors/page.test.tsx`: entry gated and present with zero profiles |
| 005.2, 005.6 | New `lib/settings/ssh-connection-settings-routes.test.ts`: adoption, bastion and identity detours retain context; only allowed return routes |
| 004.1, 004.3, 004.7 | New `components/settings/ssh-connections/connection-details.test.tsx`: save fails before detour, create/cancel return, stale/deleted destination, no implicit save, late response ignored |
| 004.3, 004.5, 004.7 | New `setup-continuation.test.tsx`, `managed-binding.test.tsx` beside component: name/selection round trip, guarded handoff, explicit use, reset checks, changed baseline, discard/logout/exit cleanup |
| 001.1, 001.2, 004.3 | Existing `identity-form.test.tsx`: secret lifetime and duplicate submission remain covered |

## E2E tests

Use the existing real-backend fixtures with temporary feature enablement restored
in `finally`. Mobile specs stay under `tests/settings`, not `tests/ssh`.
Do not move SSH effects into the normal mobile suite or override worker budgets.

| Flow and ACs | File / project |
| --- | --- |
| UI-01: navigation, empty hub, search, old links, Back/Forward, query preservation, canonical URL on reload; 004.4, 004.6, 005.6 | New `tests/settings/ssh-navigation.spec.ts` / chromium and `mobile-ssh-navigation.spec.ts` / mobile-chrome |
| Disabled and permissions; 001.5, 005.3, 005.6 | Existing `tests/settings/ssh-connections-disabled.spec.ts` / chromium; `tests/auth/ssh-connections-permissions.spec.ts` / auth, exercise both paths |
| UI-02: create/cancel identity from connection, draft round trip, explicit selection, Back, no secret persistence, changed baseline; 004.1, 004.3-004.5, 004.7 | Extend `tests/settings/ssh-connections.spec.ts` / chromium and `mobile-ssh-connections.spec.ts` / mobile-chrome; use shared helper with separate project files |
| Adoption and nested bastion context; 005.2, 005.6 | Route/component tests plus manager browser query assertions; full real-host adoption acceptance remains in original package |

Normal tests can prove return without a ready route. For Use connection, seed
public ready-route metadata in a disposable fixture and assert selection only;
mock only the isolated executor-test response for UI state-machine coverage.
Do not claim this proves SSH authentication. Existing real OpenSSH suites remain
the original package's evidence, not a new prerequisite for a navigation change.
Check key actions with keyboard, touch geometry, pseudo-locale, long labels,
focus restoration, and overflow at the documented viewports. Track mutations
causally to prove redirects and return actions do not create or bind records.

## Work orders

- [x] [Task 01: Nest SSH management and preserve old links](task-01-nested-navigation.md)
- [x] [Task 02: Preserve setup context and create keys in place](task-02-contextual-setup.md)

Dependency: 01 then 02, sequentially in the primary session. Task 01 can ship
independently with current identity setup behavior; Task 02 completes the
contextual setup outcome. Neither marks the original SSH package complete.

## Verification prerequisites

Commands in each order run from repository root using isolated subshells. In a
fresh worktree run `(cd apps && pnpm install --frozen-lockfile)` once. Use the
managed Playwright runner, which rebuilds fixtures, sequentially. No Docker or
live remote host is required for these UI slices. Public documentation changes ship with their corresponding behavior.

## Verification results

Design checks on 2026-10-23:

- `python3 scripts/list-docs.py validate`: passed, 393 decisions and 1673 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; untracked plan/work-order whitespace checked separately.
- Local reference audit: all three new artifacts have valid requirement/AC IDs
  and local file links. PR-documentation `validateCoverage` reports the actual
  docs-only diff exempt; an in-memory preflight adding the proposed route path
  reports covered, with all five changed work orders and their contracts valid.
  The preflight did not modify a source file or contact GitHub.
- `git status --short`: only six existing documentation files and the three new
  package files changed; nothing staged. No production or permanent test changes.

Implementation verification on 2026-10-23:

- Both work orders completed under TDD. Initial regressions failed for route
  classification, breadcrumbs, search hierarchy, menu placement, empty-list
  discovery, route context, identity detours, and executor draft restoration.
  A real browser regression then showed Back returning to an empty new-connection
  form. Replacing that history entry with the saved connection fixed the failure.
- Focused Vitest suite: 17 files, 197 tests passed. Two added guard/save-cleanup
  cases passed in the final 8-test managed-binding run (199 distinct cases total).
  The final route/test-constant adjustment also passed its 57-test subset.
- Playwright, sequential one-worker runs: 12 chromium, 8 mobile-chrome and
  1 auth checks passed (21 total). These comprise the combined 11/7 navigation
  and setup suites, plus one concurrent-editor check per browser project and
  the permission check. Widths include 390, 767, 768, 820 coarse, and 1280px.
  Coverage includes real disposable identity generation/import, create/cancel,
  Back/Forward, reload, focus, touch target sizes, overflow, pseudo-locale,
  preserved query/hash, disabled and member rejection on both URLs.
- Explicit ready-route selection, executor-test success, and concurrent-editor
  browser scenarios use labelled public-metadata/UI fixtures. These prove UI
  behavior, not SSH authentication. No host enrollment or live access changed.
- Fresh managed backend/web builds passed. Later projects reused those artifacts
  with `--no-build`; the final auth build also included the equivalent literal
  static route key required by public-doc inventory detection.
- Typecheck, full frontend lint, changed-test lint, `i18n:check`, and
  `i18n:ratchet` passed. All supported locale catalogs include the six new labels.
- Public docs: 63 validator tests and all 54 published pages passed. Spec index
  validation (393 decisions, 1673 specifications), 36 linter tests and full spec
  lint passed. PR-document coverage preflight covered all five changed work
  orders without errors. Whitespace checks passed.
- Desktop and phone conflict screenshots were inspected: the entered name is
  retained, saving is disabled, the recovery action is visible, and the page
  retains one settings scroll container. Captures remain local test artifacts.

Local environment: `/tmp/ssh-navigation-run.py` launched the standard managed
runner with inherited `KANDEV_*` settings removed, localhost binding and Nix
Chromium shared-library paths set only in the child process. Initial browser
startup failures were environment issues. An auth run collided with another
process on port 18081; rerunning with `E2E_PORT_OFFSET=23` passed. No other
process was stopped and no service configuration changed. Diagnostic logs are
under `/tmp/ssh-navigation-*.log`; disposable backend fixtures were torn down.

At the implementation handoff, files were unstaged and uncommitted. The running
main and seeded test instances remain unchanged. Original SSH acceptance gaps
remain tracked in the original package; this completion applies only to these
two navigation/setup work orders.

## Risks

- Losing adoption or bastion context can break an in-progress migration; central
  route tests must cover composed detours, not just bare links.
- Transferring draft ownership can bypass save guards if implemented globally;
  constrain it to the managed setup continuation and test cancelled navigation.
- A key can be created while its form is dismissed. Ignore late navigation and
  keep the saved identity visible for later use; do not delete it automatically.
- Concurrent administrator edits require fresh baseline comparison on return.
- A relocated manager must remain available with no executor profiles and must
  remain unavailable before feature/admin gates pass.
- Original acceptance gaps remain in the original package: deletion dependency
  detail, full browser adoption coverage, orphan-sweep integration, desktop
  acceptance, PostgreSQL coverage and final acceptance. This plan closes none
  of those by declaration.


## Proposed UX follow-up (2026-10-26)

The user requested a less crowded SSH manager after testing this implementation.
The [focused management package](../ssh-connections-ux/plan.md) proposes a new
collection/overview/access composition while preserving this package's verified
security and continuation work. The follow-up remains pending. The completed
results above describe this package, not the proposed UI.

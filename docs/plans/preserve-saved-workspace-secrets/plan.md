---
created: 2026-10-10
status: completed
requirements:
  - REQ-WORKSPACES-SECRET-CATALOGUE-001
  - REQ-WORKSPACES-REPOSITORY-SECRETS-001
system_design:
  - ../../specs/workspaces/system-design/workspace-secret-catalogue.md
legacy_specs: []
---

# Implementation Plan: Preserve saved Workspace secrets

## Overview

One sequential work order preserves accepted workspace-local metadata when an
earlier initial listing succeeds. ROOT reviewed the full four-artifact design
package and explicitly released implementation to this same primary session
under exclusive local-heavy grant112 on 2026-10-10. Local implementation and
scoped checks are complete. Hosted delivery and merge remain separate gates.
No delegates, recursive tasks, new sessions, or model switches were used.

Workspaces owns the current metadata lifetime. The focused
[requirement](../../specs/workspaces/requirements/workspace-secret-catalogue.md)
and [design](../../specs/workspaces/system-design/workspace-secret-catalogue.md)
extend same-list workspace ordering without rewriting the sibling's Global
contract or the completed
[list-isolation package](../workspace-secret-list-isolation/plan.md).

## Evidence and assumption check

At base `19237eb2323b835c72632ba5d32260f08f8eed60`, ROOT independently qualified
the real Vite route with no supplied items: an initial workspace GET remains
pending, a real form/shared Save sends the exact workspace POST payload and
accepts HTTP 200, the saved row renders, and the earlier HTTP 200 empty GET
then removes it. Cause: unconditional `setScopedItems(response ?? [])` in the
workspace success callback overwrites acknowledged local changes.

Permitted evidence is
`/tmp/kandev-root-workspace-secret-bootstrap-20261010/qualified-proof.json`
and `qualifier.json`/log. Original native 54930 actually joined via
`fe6c22`, exit 1: one causal RED, two ordinary controls PASS, and zero setup,
transport, or unhandled failures. The controls are ordinary GET metadata and
accepted creation with supplied `initialItems=[]` and no GET. Wrapper 3981298,
group 3981320 absence, checksum-only scratch removal, and RETURN are recorded
there. No child implementation command or test has run.

The protected `candidate.test.tsx` in that directory must never be read, copied,
changed, or deleted. Tests in this package are independently authored later.
Creation is the qualified principal loss. Callback update/remove cases are
bounded companion coverage for the same success-publication fence, not claims
that rendered rename/deletion while initial loading have been qualified.
Catch/rejection loss is unproved and excluded. Intent, scope, and cancellation
controls are settled by the parent instruction; no material design question
remains.

## Scope

### In scope

- Workspace effect's successful initial publication after accepted local
  metadata add/update/remove, including mixed rows and current-empty restoration.
- Existing scope/workspace/initialItems cancellation, loaded/loading settlement,
  and ordinary uncontested publication.
- Independent real workflow regression plus meaningful hook controls.

### Out of scope

- Global success effect, sibling PR 4398's tests/spec edits, and
  REQ-WORKSPACES-REPOSITORY-SECRETS-002 allocation.
- Catch/failure-loss repair, late mutation callback lifecycle hardening, shared
  owners, API/backend/auth/encryption/value/reveal/binding/transfer/cache changes.
- Layout, navigation, touch, copy, E2E/build/broad audits, other old-plan edits,
  or incidental dependency/cache cleanup.

## Technical approach and compatibility

Production ownership is only the workspace branch in
`apps/web/hooks/domains/settings/use-secrets.ts`, its workspace mutation callbacks,
and the necessary ref import. Use a synchronous hook-local monotonic mutation
generation captured at initial-read admission; advance it before scheduling
each current workspace callback update. Keep contents on superseded success,
settle readiness, and preserve effect dependencies and cancellation. See the
[design fence](../../specs/workspaces/system-design/workspace-secret-catalogue.md#success-publication-fence).

| Consumer/transport | Identity and behavior | Evidence |
| --- | --- | --- |
| Vite Workspace Secrets, real HTTP adapters | Workspace ID; no initialItems; accepted create survives earlier empty 200 GET | Principal rendered regression |
| Same hook's workspace callbacks | Current lifetime; complete mixed list/empty/restored values remain authoritative | Hook companion cases |
| Supplied-list page | Explicit array, including empty; no initial GET; new array replaces local changes | Rendered ordinary control and hook replacement cases |
| Changed workspace/scope or unmount | Abort and cancellation retain ownership; late result cannot publish | Hook lifecycle controls |
| Global consumer | Existing shared store lifetime and filtering; sibling effect unchanged | Existing suites and .14 compatibility |
| Missing workspace ID | No scoped request; existing flags retained | Hook ordinary control |

Sibling111's reviewed exact pair was read-only inspected at
`/home/jcfs/.kandev/tasks/preserve-saved-globa_dxstz8sa/kandev`, head
`a589325537d2b70ff66f02f289c1d1ad62f88127`, PR 4398. Coordination message sent
to its existing task. Both packages edit distinct portions of the same hook
but distinct specs/tests. ROOT serializes actual implementation/integration;
re-read current base and sibling contracts after movement. Preserve sibling
definitions rather than overwriting or reallocating them.

## Tests and all-AC mapping

The one work order owns every row below. Proposed test names describe independently
authored later tests; none is reported as already implemented or passing.

| Acceptance criterion | Planned test and boundary |
| --- | --- |
| AC-WORKSPACES-SECRET-CATALOGUE-001.1 | Component: `keeps the accepted workspace row after the earlier empty initial GET` |
| AC-WORKSPACES-SECRET-CATALOGUE-001.2 | Hook: `preserves all mixed local changes against older metadata`; `keeps an empty current list after add and removal`; `keeps local authority after rename and restoration` |
| AC-WORKSPACES-SECRET-CATALOGUE-001.3 | Hook: `publishes uncontested complete and empty lists`; `settles superseded success without readmission`; component ordinary GET/no-GET controls |
| AC-WORKSPACES-SECRET-CATALOGUE-001.4 | Hook: `accepts a new workspace after canceling the old read`; `cancels on scope change and unmount`; `replaces mutations with supplied initial items including empty` |
| AC-WORKSPACES-REPOSITORY-SECRETS-001.1 | Real workspace create request/payload and supplied-empty creation control |
| AC-WORKSPACES-REPOSITORY-SECRETS-001.2 | Hook workspace/scope cancellation and missing-ID controls; existing lifetime suites |
| AC-WORKSPACES-REPOSITORY-SECRETS-001.14 | Existing real hook and settings lifetime suites, unchanged |

## Rendered workflow and mobile parity

The component test covers the entire client workflow from real form and shared
Save through real adapters, HTTP acknowledgment, local publication, and rendered
row retention. Only external fetch transport is controlled. This state/data-only
correction uses the existing desktop/phone settings surface without layout,
touch, scroll, navigation, or new copy changes. The mobile-parity exception
permits hook/component tests; no new E2E, browser/build, or ASCII redesign.

## Documentation impact

Internal requirements/design/delivery records only. This restores the existing
save outcome without a new public operation, terminology, API, or operator step.
No system boundary or engineering convention changes; no README/AGENTS/ADR edit.
Reassess public documentation if implementation expands the reviewed scope.

## Work orders

- [x] [Task 01: Fence workspace initial success publication](task-01-fence-workspace-publication.md).

One sequential work order. Execution authorization came from ROOT's later
explicit release; this manifest did not grant it.

## Verification results

Design checks passed on 2026-10-10: catalogue validation (369 decisions and
1519 specifications), all 36 specification-linter tests, full specification
lint, and owning-pair catalogue discovery. The four actual paths are untracked,
unstaged, and uncommitted; tracked production/test files remain unchanged.
All 25 local Markdown links/anchors, draft/draft/draft/pending statuses,
one work order, all seven AC mappings, and whitespace including untracked files
passed. Receipt: `/tmp/kandev-child112-design-20261010/validation.json`.

The unfiltered actual four-document inventory passes documentation coverage as
`exempt`, errors=[]. Separately, the seven-path projected production/test/docs
inventory passes `covered`, errors=[], with one changed work order and one
owning design declaring both requirements. This projected result is a
production coverage preflight, not actual production evidence. Implementation
must replace it with the full actual changed-path inventory.

The first design documentation preflight attempt exited 127 because this shell did
not expose `node` on PATH. Re-running only that light script with the existing
Node 24 binary succeeded, exit 0; no installation was performed during design.

Implementation under ROOT's later grant112 passed on 2026-10-10:

- One conditional frozen install: original29228, actual JOIN0/57f002. Dependencies
  were missing; pnpm 9.15.9 reused the package cache, and the lockfile is unchanged.
- Initial independent RED91032, actual JOIN1/473847, exposed the principal row
  loss and companion failures plus one ordinary `it.each` fixture mistake.
  Correcting only that test's table shape removed the fixture error before any
  production edit.
- Qualified RED31268, actual JOIN1/3bd5f1: two files, four expected publication
  failures and ten passing controls. The principal real form/shared Save/adapters
  case loses the saved row only after the older successful empty GET; both
  ordinary rendered controls pass. No setup/transport/unhandled error remains.
- Minimal synchronous local mutation-generation fence: nine added lines net
  production change, preserving Global effect, workspace catch, dependencies,
  cancellation, ordinary reads, and successful loaded/loading settlement.
- Owned TS files formatted before checks. Affected GREEN5197, actual JOIN0/155ead:
  six suites and 61 tests pass, including all 14 independently authored cases
  and the 47 existing compatibility cases. This single affected run supplies
  GREEN for both the new suites and final affected scope without a redundant replay.
- Scoped ESLint72599 JOIN0/1b0659 (zero warnings), typecheck86624 JOIN0/73c769,
  and staged-file i18n ratchet54052 JOIN0/156406 pass. Typecheck generators cause
  no tracked drift. No UI copy, layout, E2E, browser, or build change was needed.

Original command receipts and logs are in
`/tmp/kandev-child112-implementation-20261010/`. Each retained original command
actually joined, with its own process group absent at completion. The protected
candidate and sibling fixtures were never accessed. Public authentication and
agents/profiles documentation already describe the unchanged workspace save and
scope contract; no new public operation or engineering convention needs editing.

Final actual seven-path coverage passed (one work order, both owning
requirements, all seven ACs, 25 links/anchors, errors=[]), along with catalogue
validation, all 36 spec-linter tests, full specification lint, and whitespace.
Actual documentation receipt is `docs-validation.json` in the implementation
receipt directory. Normal commit hooks and hosted evidence are separate
delivery receipts. Local checks do not imply hosted readiness or ROOT merge
authorization.

## Risks and delivery boundaries

- Equality/length-based guards miss add/remove or restored-value histories;
  refs advanced inside React updater functions may be too late for publication.
- A skipped success must still settle loaded/loading; adding mutation/item
  dependencies would readmit stale reads and reset accepted work.
- Generation equality alone cannot replace scope cancellation. Catch behavior
  remains unchanged unless independently proved and reviewed.
- Protected and sibling fixtures/resources are not available test inputs.
- Later authorized delivery uses one conditional normal frozen install, actual
  causal RED/GREEN joins, scoped checks and active hooks, commit/push/ready PR.
  Return ROOT's heavy lease only with every original local handle actually joined
  and current known owned process groups absent; preserve managed worktrees,
  dependencies/cache, foreign resources, paused `a6032d95`, and anonymous volume
  `2c48e791`.
- Then use the sole same-turn hosted observer for 90 minutes, cadence 60 seconds,
  GNU timeout 91 minutes with TERM and kill-after 10 seconds. Preserve that
  observer and original clock across interrupts/base moves; no duplicate observer
  or fabricated join. No automatic manual full bot review after each push.
- Hosted readiness requires six required checks plus three actual current product
  parents SUCCESS, fresh complete errors=[], zero actionable visible/hidden human
  threads, clean head, and adequate substantive review. ROOT retains one static
  review and separate serial normal expected-head squash authorization. This
  design checkpoint does not complete the task or authorize merge.

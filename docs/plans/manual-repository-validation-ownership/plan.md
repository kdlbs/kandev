---
created: 2026-10-06
status: implemented
requirements:
  - REQ-WORKSPACES-LOCAL-REPOSITORIES-001
system_design:
  - ../../specs/workspaces/system-design/local-repositories.md
legacy_specs: []
---

# Implementation Plan: Manual Repository Validation Ownership

## Overview

Keep workspace settings validation and draft confirmation bound to the current
dialog visit, workspace, trimmed input and newest validation attempt. Deliver
one local correction with real hook/form regressions in one sequential work
order, after a later explicit ROOT implementation release in this same primary.

Workspaces owns explicit repository attachment and canonical selection. The
existing [local repository requirement](../../specs/workspaces/requirements/local-repositories.md)
already requires canonical validation before save (001.3). Its new 001.9 through
001.11 specify the missing settings draft authority. No new incident requirement,
shared presentation contract or ADR is needed for this local correction.

## Confirmed evidence and assumptions

ROOT's accepted proof is read-only and must not be replayed or altered:
`/tmp/kandev-manual-repository-validation-repro.test.tsx`, SHA256
`9926c8a12844cef20fbeb27c0696f4d775342b86ef85e68e3162c9e0e0f62cc7`;
receipt `/tmp/kandev-root-manual-path-validation-proof-receipt.json` plus its
log/exit. ROOT joined handle 40643, exit 1 in 9.55 seconds: three causal failures
and two successful/invalid controls passed, with no setup/teardown failure.

The actual exported hook with StateProvider/createAppStore, discovery, router
and actions, mocking only fetch, admitted Alpha after an editable input changed
to Beta, enabled confirmation in a reopened blank visit, and published an Alpha
rejection onto Beta. Confirmation's canonical-path precedence demonstrated an
actual wrong local draft. It did not prove a persisted wrong backend write.

Proof base: `8776877048ea029b8f861b4d658be719dfbadff2`. Current audited checkout:
`5ab4e285b94dfa77051a647d987bb304af39252f`. Source blobs are unchanged:
client `06fdf26043368df007fe3ff144730e65372afff8`, dialog
`47fd2fd0e2bc70865594f950f1fa19e2d8feafe6`. ROOT separately audited current main
in `/tmp/kandev-root-manual-path-validation-current-main-audit.json`.

Confirmed choices: preserve editable inputs, canonical response paths, ordinary
same-input success, discovered selection and caller settlement. Trim-equivalent
edits preserve validity; different identity retires it even after A-to-B-to-A.
No material product question remains. Wider dependencies require a bounded ROOT
evidence checkpoint before scope expansion.

## Scope

### In scope

- Nested `useDiscoverDialog` ownership, current render projection and callback
  admission in `workspace-repositories-client.tsx`.
- Immediate confirmation/build/close glue in that same file where necessary.
- Faithful permanent integration tests through the actual form and page hook.
- The existing requirement/design pair and this single-order delivery record.

### Out of scope

- Generic `useRequest`, backend, action/transport/API changes, config/schema,
  cache/store/discovery owners, global coordinators or broad consumer audits.
- Layout, copy, routes, touch, scrolling, navigation, disabling editable input,
  browser/build/E2E work or runtime/harness edits.
- Installs, production/permanent test edits, product checks, DB/app/browser,
  stage/commit/push/PR during DESIGN; delegates, extra tasks/tabs/sessions.

## Current entry points and closure inventory

| Current source boundary | Authority gap or preserved behavior |
| --- | --- |
| `src/settings-routes.tsx` repositories tab -> `WorkspaceRepositoriesRoute` in `src/settings-routes.workspace-data.tsx` | Loads workspace/repositories and removes the client while route data changes; hook also needs safe prop replacement and unmount behavior |
| `WorkspaceRepositoriesClient` -> exported `useWorkspaceRepositoriesPage` | Add Local Repository calls `openDialog`; hook owns unsaved items and save handlers |
| Nested `useDiscoverDialog.openDialog` | Clears fields but does not revoke earlier request closures |
| `handleManualRepoPathChange` / `handleSelectRepoPath` | Reset displayed validation; pending request remains admitted; input remains editable |
| `handleValidateManualPath` | Captures workspace/input, awaits real validation action, then unconditionally publishes success/invalid/rejection |
| `useRequest(validateRepositoryPathAction)` | Unqualified loading follows completion order; no visit/input/attempt ownership |
| `AddLocalRepositoryDialog` -> `DiscoverRepoDialog` / `ManualRepositoryPath` | Forwards actual input, Validate, Use Repository and raw open setter; Validate is disabled while exposed busy, input is editable |
| Cancel, Radix dismiss and confirmation close | All must use the local retirement path; old callbacks cannot affect a reopened visit |
| `handleConfirmLocalRepository` / `buildDraftRepo` | Currently no validity check; selected row then validation.path then raw input determines draft path |
| `handleSaveRepository` / `saveNewRepository` / `createRepositoryAction` | Separate persistence boundary after draft editing; unchanged |
| Real `RepositoryDiscoveryControls` and `useRepositoryDiscovery` | Preserve selection, desktop root controls, shared discovery admission and refresh |

## Technical approach

Implement the [settings manual validation ownership design](../../specs/workspaces/system-design/local-repositories.md#settings-manual-validation-ownership).
Use one local context/lifetime identity and newest-attempt token. Reserve ownership
before transport; revoke before publishing changed input/selection/open state.
Fence workspace commits/unmount and project only current state during render.
Avoid a passive-effect-only reset. Captured callbacks compare their original
context with current authority; confirmation additionally checks its accepted
selection/attempt. Same-context repeated validation starts are permitted and the
newest invocation owns publication. Keep canonical response path separate from
requested trimmed input.

Replace this hook's unqualified request state with locally owned pending/result
state while calling the existing action. Guard all settlement branches and busy
cleanup; retired calls still settle without side effects. Confirmation uses the
same authority and only current selected discovered rows or accepted manual
canonical paths. Keep draft defaults, discovered name/branch, existing success
and backend diagnostic messages. No cancellation or framework change.

## Tests

Add `apps/web/app/settings/workspace/workspace-repositories-client.test.tsx`.
Use the suite prefix `manual repository validation` for exact anchored selection.
Short AC references below mean `AC-WORKSPACES-LOCAL-REPOSITORIES`.

| Criteria | Required real boundary evidence |
| --- | --- |
| 001.9, 001.11 | Deferred actual validation, edit Alpha to Beta through real input; older success/invalid/rejection cannot display feedback or enable Use Repository/draft; input remains editable |
| 001.9 | A-to-B-to-A input, including edit after accepted success; returning to A does not revive old authority; trim-equivalent spelling preserves current valid result |
| 001.9, 001.11 | Cancel and dismiss through actual dialog, settle after close, reopen blank; then close/reopen with old work still pending and settle in new visit |
| 001.9 | Replace workspace while mounted, including A-to-B-to-A and null workspace; old result/callback never admits a draft into replacement workspace |
| 001.9, 001.10 | Unmount with pending work, settle/join; retained old callbacks cannot start validation or confirm afterward; remount has independent authority |
| 001.10 | Same-input two attempts in both settlement orders, older success/invalid/rejection versus latest success/failure; older cleanup preserves latest pending indicator and disabled confirmation |
| 001.10 | Synchronous fetch-boundary reentry starts a second same-input attempt before the first returns; newest owns publication without stranded loading |
| 001.11 | Retain validation and confirmation callbacks, retire by edit/select/close/workspace/new attempt, invoke them directly; no old transport, draft, close or borrowed new result |
| 001.3, 001.10, 001.11 | Current successful, invalid and rejected requests retain ordinary feedback; successful canonical path differs from input and is exactly the draft path; unchanged success remains confirmable |
| 001.1, 001.11 | Real discovered row selection remains confirmable, with correct path/name/branch; pending manual work cannot interfere; blank/no-workspace starts no validation or draft |
| 001.11 | Use Repository creates only local draft, with zero create POSTs; current async handler and retired handlers settle according to existing void/caught-error behavior |

Mount a small test probe using the actual exported hook plus
`AddLocalRepositoryDialog`, which renders the actual dialog/controls. Capture
public handlers for retained-callback and reentry tests; observe repositoryItems
and rendered controls rather than private tokens. Use real StateProvider (which
constructs createAppStore), real router/i18n/actions/discovery and actual desktop
root controls in at least one transport-backed case. Only `globalThis.fetch`
may be mocked; no hook, framework, state, predicate, dialog or action mocks.
Fresh, structurally valid discovery responses avoid unrelated automatic scans.
Use distinct requested/canonical paths and errors so stale equality cannot pass.
Settle and join all deferred calls in failure-safe fixture cleanup; unmount and
dispose only the fixture-owned discovery singleton state after consumers release.
No fixed sleeps, weakened timeouts or mirrored ownership predicates.

## Mobile parity and E2E assessment

Desktop and phone enter Settings > Workspaces > chosen workspace > Repositories
> Add Local Repository and use the same `AddLocalRepositoryDialog`/page hook.
This changes only state/data admission inside that existing form, with no visual,
touch, scroll, navigation or viewport-dependent interaction. The narrow
`mobile-parity` exception permits targeted unit/component coverage and this
explicit note. The real action -> hook -> existing dialog -> local draft test is
the end-to-end frontend boundary evidence. No new browser/E2E/build or ASCII UI
preview is planned; no redesigned phone composition or geometry is claimed.

## Documentation and companion packages

`docs/public/use-kandev.md` correctly separates Validate, Use Repository (unsaved
card) and Save changes. `configuration.md` and `desktop-app.md` correctly describe
explicit validation independent of discovery roots. Root README and screenshot
catalog contain no affected instruction or terminology. Public guides remain
accurate; internal specs/plans only. No public wording change is needed.

The requirement file is near 20 KiB; condense duplicated scenarios/introductory
prose while preserving all unique contracts and examples. Keep the current pair
below its limits without a supplement or size exception. Linked explicit-trust,
local-only merge/rebase, consent, failure-recovery, discovery-ordering and
root-mutation packages are historical records of other completed scopes. Their
recorded tests/results remain unchanged. This order owns the additional race
matrix; no companion implementation needs reopening.

## Work orders

- [x] [Task 01: Keep validation and confirmation in the current dialog](task-01-current-dialog-validation.md)

One sequential order, no dependencies, no delegation. Implementation requires a
later explicit ROOT release in the same primary and exclusive global heavy lease.

## Verification results

DESIGN checks passed on 2026-10-06:

- `python3 scripts/list-docs.py validate`: 351 decisions and 1,369 specifications.
- Owning-system catalog lists the existing local-repositories pair.
- `python3 scripts/lint-spec-files.py --all`: all specifications passed.
- Actual repository `validateCoverage` evaluator with these four documents and
  prospective client trigger: `covered`, no errors, exactly this sole work order.
- `git diff --check` and untracked-file whitespace checks passed; status confirms
  two modified owning specs and only the new manifest/order directory. Nothing
  staged or committed; source blobs remain exactly the accepted baseline.

Requirement is below 20 KiB, design below 32 KiB. No size exception/supplement.
The preceding receipts describe the completed DESIGN checkpoint, before ROOT's
later explicit implementation release. After that release, Task 01 was
implemented with 36 passing real form/hook regressions. Changed ESLint,
typecheck, localization checks and public documentation checks passed; details,
meaningful RED failures, fixture corrections and actual joined receipts are in
[Task 01 results](task-01-current-dialog-validation.md#results). No additional
production boundary, browser/build/E2E or backend work was needed.

The private `useLocalRepositorySelection` remains inside the owning client. It
uses synchronous context/attempt admission and a layout-effect lifetime/workspace
fence; the existing nested dialog consumes its projected state. The actual
confirmation handler uses that authority before building an unsaved canonical or
discovered draft. No persisted wrong backend write is claimed or tested.

Implementation status records local completion. Publication, hosted checks/full
review and the separately authorized serial merge remain delivery gates in the
durable task plan. The sole GLOBAL LOCAL-HEAVY lease is held through normal hooks
and publication, then returned before the single hosted collector.

## Delivery and recovery

Task `d9a75a74-0259-4963-957a-6d290be15346`, primary session
`44e38d4e-f4e6-427f-9aef-6426c37cb219`; ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. ROOT watches the durable task plan and
conversation; no callback or operator approval retries. Leave exactly four
package artifacts unstaged/uncommitted and end DESIGN. The task plan retains the
system marker, standing later delivery authority, user edits and resource state.

After later ROOT implementation release, use one global local-heavy lease,
pinned frozen install only if needed, targeted TDD/checks and active normal hooks.
Checkpoint unknown/out-of-scope/resource/transport failures to ROOT before
recovery. Later publish a ready normal PR with frozen head except actual findings;
require full authenticated CodeRabbit App 347564 exact-head/all-file semantic
evidence, all six required successes and successful product-parent workflows,
terminal checks and complete zero-actionable-thread/no-human-gate snapshot.
One retained all-terminal collector must join with PID gone before replacement.
CI retries require bounded ROOT workflow/job-name grant. No broad/passing replay,
moving-main-only rebase, gate bypass or duplicate optional full review request.

Merge needs a separate serial ROOT MERGE lease and normal expected-head squash.
Prove merged SHA/tree/all owned blobs/remote inclusion independently; join and
clean only owned scratch/processes. Preserve managed worktree/deps/shared caches,
foreign resources and ROOT proof. Completion is merge plus joins, not publication;
ROOT independently verifies/archives/refills. Persist actual handles, failures and
next step for crash recovery; do not promise unattended continuation after stop.

## Risks

- Equal path strings cannot distinguish returned visits or A-to-B-to-A edits.
- Passive reset and disabled controls alone leave stale render/callback admission.
- Old request cleanup can clear newest pending state unless ownership is checked.
- Canonical paths may differ from input spelling and must not be discarded by a
  response-path equality check.
- Real shared modules require fixture isolation and terminal deferred cleanup;
  tests must not gain false passing evidence from setup failures or mocks.

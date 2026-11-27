---
created: 2026-10-09
status: in_progress
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-002
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
legacy_specs: []
---

# Implementation plan: Preserve workspace choices during settings save

## Overview

Preserve the current workspace catalogue when an older settings PATCH is
acknowledged. One sequential work order independently authors real integration
regressions, changes only acknowledgement publication and its immediate caller,
and verifies existing success, rejection, draft, payload, and navigation contracts.

The first DESIGN turn completed in task `68ca5cb7-2625-4380-aa40-6e2894cbdb88`,
original primary session `7849e128-4fbf-4733-8896-de3afebb2bf1`. Parent ROOT is
`14825981-b175-411d-999a-31ddc2aa5fc3`. Requirements/design are the existing
Workspaces settings pair; the narrow 002 extension is implemented locally.
Requirement 001 and its earlier backend plan remain untouched delivery contracts.
No parallel work order, implementation admission, or model switch is implied.

## Scope

### In scope

- [Requirement 002 and criteria .1-.6](../../specs/workspaces/requirements/workspace-settings-updates.md#req-workspaces-settings-updates-002-preserve-current-catalogue-during-settings-save).
- Current owning store read at successful acknowledgement; existing accepted
  target projection over its current rows.
- Real Page/provider/coordinator/API/registered-WS/sidebar integration coverage.
- Read-only adjacent caller inventory, mobile exception, and public-doc assessment.

### Out of scope

Generic cache/global revision, store schema or action changes, API/backend/schema
migration, target-settings revision arbitration, permission redesign, corrected
callers, delete/create/placement route repairs, or backend deletion claims.
The original DESIGN turn excluded production/permanent tests/install/product
checks/staging/commit/PR. ROOT later explicitly released this reviewed work order
in the same original primary with the exclusive GLOBAL LOCAL-HEAVY CHILD94 lease.

## Technical approach

Use `useAppStoreApi` in `useWorkspaceEditForm` to pass the owning store reader to
`buildWorkspaceSaveHandler`. Replace its captured catalogue input with the current
reader. Read current items and setter after PATCH resolves, immediately before
synchronous mapping/publication. Preserve the exact existing projected settings
and all other row fields; preserve nonmatching rows and order. Keep the existing
array/setter inputs used by `useWorkspaceDeleteDraft` intact. No production file
beyond `workspace-edit-save.ts` and this immediate wiring is admitted.

See [current catalogue publication and inventory](../../specs/workspaces/system-design/workspace-settings-updates.md#current-catalogue-publication).
Existing backend partial presence remains authoritative; the client fix neither
supplies omitted settings nor arbitrates same-target response revisions.

| Boundary | Behavior | Evidence / fallback |
| --- | --- | --- |
| Page + owning StateProvider | Current catalogue from the same store as the picker | Real provider/store; no global substitute |
| Real PATCH action/useRequest | Existing partial JSON and response/error behavior | Strict external fetch tickets; unexpected transport fails |
| Registered workspace notifications | Independent current additions/updates/removals | Actual registered handlers with external delivery stimulus |
| Save coordinator / navigation guard | Success, error, newer drafts and canLeave | Real registration/saveAll and real routing controls |
| Real sidebar/shared phone picker | Current choices, names, accepted target, ordinary selection | Real primitives and rows; no product mocks |
| Other callers / target arbitration | Existing independent contracts | Read-only audit; checkpoint new scope rather than fallback |

## Tests and traceability

All case names below belong to
`apps/web/app/settings/workspace/workspace-edit-save.integration.test.tsx`.
The matrix was independently implemented after release. Actual results and
retained failures are below; the fixture contract is in the single work order.

| Criteria | Independently authored case names | Required observation |
| --- | --- | --- |
| .1, .2 | `preservesCreatedWorkspaceChoiceWhenSaveIsAcknowledged` | Registered creation during held PATCH; choice/store present before and after; accepted target; select retained choice using real navigation |
| .1, .2 | `preservesUpdatedOtherWorkspaceChoiceWhenSaveIsAcknowledged` | Registered update during held PATCH; current other name/values survive in store and real menu |
| .1, .2 | `preservesRemovedOtherWorkspaceChoiceWhenSaveIsAcknowledged` | Registered removal during held PATCH; removed row stays absent in store and menu; remaining order survives |
| .3 | `preservesCurrentTargetMetadataAndActiveSelection` | Intervening target metadata event and real selection action; projection accepts target settings without overwriting current nonprojected fields, activeId or activeIdRevision |
| .3, .4 | `acceptsOrdinarySaveAndClearsContributor` | No intervening catalogue update; accepted projection/fallbacks/baseline, clean contributor and existing route |
| .4, .6 | `doesNotPatchPristineWorkspace` | Real coordinator with clean form issues no PATCH |
| .4 | `acceptedSaveRetainsNewerDraftAndRouteGuard` | New Name typed after transport admission; submitted value accepted, newer value still dirty, canLeave false and navigation remains guarded |
| .5 | `rejectedSavePreservesCurrentCatalogueNewerDraftAndRoute` | Registered addition plus newer Name; rejection leaves current catalogue/draft, failed contributor, toast, unchanged accepted target/baseline and real route guard |
| .6 | `sendsOnlyChangedWorkspaceSettings` | Parameterized real-control edits/captured PATCH: trimmed name, each default selection/clear, idle false, timeout, mixed edit; unchanged keys absent |
| .6 | `invalidOrUnmanagedWorkspaceDoesNotPatch` | Existing name/timeout validity and manage-scope controls remain; no unauthorized submission introduced |

The first three overlap cases must each fail causally on the pre-fix source,
with real before/after store and menu assertions reached, accepted target
successful, and no unhandled error/unexpected transport. Establish the metadata
case's pre-fix causal failure where it claims current target preservation.
Success/rejection controls must pass independently. No passing-control failure
or fixture exception counts as the regression. Additional contract claims need
corresponding independent causal coverage.

## End-to-end and mobile evidence

The requested end-to-end boundary is the actual frontend Page through owning
providers, coordinator, real action/useRequest, registered notifications, and
real sidebar consumer. Only external transport is mocked. Direct save-helper
unit tests, synthetic store replacements, mock picker rows, or the older picker
test's mocked primitives are insufficient. Backend event production and physical
browser layout are outside this boundary and must not be claimed as tested.

Mobile parity applies the pure state/data exception: no JSX, control, copy,
touch, scrolling, navigation, or breakpoint changes. `AppNavSheet` shares
`AppSidebarWorkspacePicker` and the owning store. The integration matrix proves
the shared catalogue outcome. No new Playwright case/preview/product build is
planned; a rendered change checkpoints ROOT and invalidates this exception.

## Work orders

- [ ] [Task 01: Preserve current workspaces at save acknowledgement](task-01-preserve-current-workspaces.md) — in progress; local implementation/checks complete, publication and later hosted/merge gates pending; sequential; no task dependency.

## Design evidence and assumptions

Confirmed: ROOT qualified the normal mounted real Save and independent choice
loss, bounded production ownership, independent test authorship, and later
release gates. No material product question remains. Verified once: checkout and
remote authoritative main both `d189d5fd11b843e0e2f22d7dbbbb16aafa8d12d9`;
all four qualified blobs match (`7e0dfb46`, `f532cf1e`, `8889a652`, `2470362a`).
Initial worktree clean. No proof replay was performed.

Only `qualified-proof.json` and `future-design-brief.md` were read from the ROOT
namespace. Accepted ROOT52326/df8880/5e5092 actually joined1: one causal failure,
two strict controls, current catalogue and real choices reached, target accepted,
zero unhandled/unexpected transport. Earlier16376/f09893 was a noncausal empty
active-executor fixture error. Protected mode0400 candidate source SHA256
`8d23320edbc6463a029ae4f7ddd9dee65299323abdbd2207d46169e72c3d2580`
remains ROOT-owned: never read/copy/import/replay/edit/chmod/remove it or another
ROOT .tsx proof/fixture source. ROOT alone releases proof after merge/archive.

## Phase and resource gates

The original DESIGN allowed source/document inspection and light catalog/spec/
coverage/whitespace checks only and ended with the actual four-file handoff.
ROOT reviewed all four artifacts and explicitly released implementation in the
same original primary with the exclusive GLOBAL LOCAL-HEAVY CHILD94 lease.
Review receipt: `/tmp/kandev-root-child94-design-review-release-20261009.json`.
Producing artifacts alone supplied no release.
Exactly two persistent tasks remain authorized; no new tasks/delegates/tabs/sessions.

Existing mise Node24.21.0 is available via Bash `login:false` and its existing bin
PATH. `apps/node_modules` and `apps/web/node_modules` were absent at design. After
release, the one conditional frozen install in `apps/` completed with pnpm9.15.9.
The earlier root-cwd version probe returned 12.4.2 and the original install had
already been launched before that probe was reconciled. The original apps install
and subsequent apps version readback establish 9.15.9; zero packages downloaded,
no alternative runtime/launcher, repeated install or cache wipe was used.
No runtime download/substitution, shared-cache cleanup, foreign kill, or hook bypass.

ONE global local-heavy command at a time, Node4GiB/maxWorkers1. Commands below
are truly serial, each original native handle retained and joined. Record exact
cwd/argv, UTC, PID/PGID, CLI/GNU/kill bounds, initial and terminal native receipts,
exit and fresh observed owned groups gone before explicit heavy RETURN. Routine
fixture/lint/format corrections rerun affected checks only. Resource/timeout/
transport/unknown/out-of-scope failures checkpoint ROOT before retry/alternatives.
Preserve managed worktree, dependencies, caches and foreign resources, including
paused task `a6032d95-cc1e-4db8-adca-5b643ae4138d` / `serialize-workspace_e3isbq2j`
and unproved volume `2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`.

Later publication uses normal active hooks and task-defined checks. Freeze ready
SHA except actual findings: no moving-main rebase, synthetic tests, broad passing
replay, assertion/race/timeout weakening or optional polish. After ready PR, use
caller-bound canonical associated repository `16026b06-bd79-47c0-aed1-dc7ca95f63d9`,
complete/errors[] readback and all FIVE automation flags FALSE. Already-correct
readback means zero relink/patch. No slug/task-repository row ID or unattached
GitLab mutation; reconcile unknown responses before duplication. Verify live body
retains exact unchecked template and bot additions.

ROOT independently qualifies all original local joins before the first hosted/
semantic release. Later retain ONE original attached all-terminal90m observer,
GNU91m/kill10/cadence60 and exact native handle across crashes. No duplicate timer,
observer/replacement until join/gone and ROOT authorization. Configured CodeRabbit
App347564 requires substantive FULL CURRENT ALL actual files, source=covered and
kind=reviewed. Sufficient automatic report means zero requests; at most one necessary
actual skip/gap request after inspection. Processing/ACK is not completion. Ground
findings; all SIX required contexts plus actual Backend/Frontend/E2E parents must
be SUCCESS on exact head with fresh complete/errors[] and zero actionable,
unresolved, hidden, changes-requested or human gate.

MERGE NONE until separate ROOT serial static compatibility grant. Then normal
expected-head squash, no admin/rebase; independently prove actual SHA/parent/tree/
all blobs/main inclusion, join own cleanup END. ROOT independently verifies/FFs/
archives/readbacks ABSENT/releases/refills. Task/session/system marker and user
edits stay in own task plan. Critical parent-question call ends turn immediately.
Queued callbacks are optional; a full queue never gates/retries; child-to-parent
interrupt is forbidden.

## Documentation assessment

Workspaces already owns catalogue identity; no new system/ADR/README boundary is
needed. Existing settings pair's client-store exclusion is narrowed only for 002;
all backend .1-.8 and independent consumer contracts are retained. No edits to the
earlier backend plan/work order or corrected callers. Public how-to guidance in
`docs/public/tasks-and-workflows.md` and access/placement reference in
`docs/public/team-access.md`, root README and screenshot catalogue remain accurate:
no new user operation, control, label, API, or screenshot. Public changes are
unnecessary for this design and the actual bounded implementation.

## Verification results

Light design validation passed on 2026-10-09: catalog validation (365 decisions,
1480 specifications), all 36 specification-linter tests, full specification lint,
four-file relative-link/whitespace checks, and `git diff --check`. Requirement 001
and all backend AC .1-.8 are byte-identical to HEAD. The existing pair appears in
the Workspaces catalog. Prospective PR-documentation `validateCoverage` with the
two planned production paths reports `covered`, `ok:true`, `errors:[]` and the
single work order's complete 002 reference chain. This evaluates references only;
those production files were not edited.

At DESIGN END, exactly four artifact files were changed/untracked with an empty
index. The
read-only caller audit also confirms Configuration Chat's best-effort default
save already calls its current-store updater; it remains excluded. Existing
Node24.21.0 was observed without install, and both dependency directories were
absent. No product, permanent integration, install, Playwright, hosted, commit,
or PR checks ran in that turn. The design handoff did not release implementation
or LOCAL-HEAVY.

## Risks

- Taking the store snapshot before await recreates choice loss; causal overlaps
  must reach real store and menu assertions, not stop on setup errors.
- Spreading the whole response would overwrite current target metadata; retain
  the exact projection and require the independent metadata control.
- Fixture setup must supply normal active executor/profile/scopes and legitimate
  transport responses so a Radix empty-value exception is not mistaken for proof.
- Target response revisions remain intentionally unarbitrated. Other captured
  callers require separately qualified evidence; no route expansion is admitted.

## Released implementation results

The implementation owns eight files: four delivery artifacts, the exact two
production paths, and two independently authored integration/fixture files.
No ROOT proof source or product mocks were used. The owning provider is read
after successful PATCH; the existing target projection is unchanged.

Receipts and raw logs are retained under `/tmp/kandev-child94-runs/`. Each run
records exact cwd/argv, UTC, PID/PGID, Node4GiB and timeout/kill bounds, original
native initial/terminal handles, subprocess wait, and fresh empty owned group.
All runs through 18 are actually joined. These are local checks, not hosted
CI, backend event production or physical-browser layout claims.

| Original run | Actual result | Disposition |
| --- | --- | --- |
| 01 install | Frozen apps install passed; pnpm9.15.9; downloaded0 | Single conditional install; probe anomaly recorded above |
| 02 RED | 19 cases:7 failed/12 passed; unsupported matcher fixture errors | Does not count as qualified full causal proof; native DOM assertions corrected |
| 03 corrected RED | 19 cases:4 causal failures/15 strict controls passed | Addition/update/removal store and real menu both reached; metadata independently causal; accepted target succeeded |
| 04 addition/control RED | 1 causal failure/2 ordinary controls passed;17 unselected | Final missing-choice assertion verified before production change |
| 05 GREEN | 20 cases:19 passed/1 failed | Store/menu/metadata repaired; remaining real navigation assertion expected wrong existing path |
| 07 affected navigation | 1 passed/19 unselected | Correct existing `/?home=overview&workspaceId=...` expectation; no production change |
| 08 compatibility | 4 files/79 tests passed | Real action, coordinator, registered handlers and picker compatibility |
| 09 scoped lint | Exit0 with2 warnings | Test grouping/literal corrected for normal max-warnings0 hook |
| 11 scoped lint;12 format | Both passed, zero warnings | Four owned source/test files |
| 13 typecheck | Exit2;4 helper-only typing diagnostics | Profile version and handler-union narrowing corrected |
| 15 affected notifications | 5 passed/15 unselected | Addition/update/removal, metadata and rejection after helper typing correction |
| 16 helper lint;17 helper format;18 typecheck | Passed | Final helper and complete web typecheck clean |

Runs06/10/14 were bounded formatter invocations, all passed and joined. No full
20/20 GREEN command is claimed: run05's19 passing cases and run07's affected
passing navigation case establish the matrix, followed by run15's5 affected
notification cases after the final helper change. No assertion/race/timeout was
weakened or passing suite broadly replayed. Unexpected transport/unhandled errors
were absent from qualified evidence. Full normal hook/publication receipts and
explicit LOCAL-HEAVY RETURN are recorded in the task plan after publication.
Hosted observer/semantic HOLD and MERGE NONE remain until separate ROOT grants.

The first normal commit attempt (run22) was rejected solely by `i18n-new-code`
on9 synthetic helper literals. Other preceding active hooks passed; no commit
was created and no bypass used. The helper now records explanatory synthetic
test-data exemptions/constants and uses the existing `common:back` translation
for its actual navigation Link. These are fixture-only lint corrections; no
production, transport or assertion semantics changed. Run23 formatter, run24 affected4
navigation/control cases and run25 final typecheck passed. The next normal hook
result is retained in the task plan before explicit LOCAL-HEAVY RETURN.

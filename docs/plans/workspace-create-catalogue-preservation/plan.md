---
created: 2026-10-09
status: in_progress
requirements:
  - REQ-WORKSPACES-SETTINGS-UPDATES-003
system_design:
  - ../../specs/workspaces/system-design/workspace-settings-updates.md
legacy_specs: []
---

# Implementation plan: Preserve workspace choices during creation

## Overview

Preserve the current workspace catalogue when Add Workspace succeeds after an
independent notification. One sequential work order independently authors real
frontend integration regressions, changes the successful creation publication
seam, and verifies accepted descriptors, choices, selection, and ordinary/error
behavior. The existing Workspaces settings pair owns this outcome because it
owns the current catalogue consumed by settings and navigation.

Requirements 001 and 002, the backend field-presence implementation, and merged
Save publication remain intact. The linked earlier plans are historical delivery
records with no affected task scope or verification matrix to reopen. Creation
adds requirement 003 to the same pair, rather than a duplicate incident spec.

## Scope

### In scope

- [Requirement 003, criteria .1-.5](../../specs/workspaces/requirements/workspace-settings-updates.md#req-workspaces-settings-updates-003-preserve-current-catalogue-during-creation).
- Owning current-store read at acknowledged creation, accepted response mapper,
  and same-identity upsert/deduplication.
- Independently authored real Page/provider/action/client/registered-WS tests
  with actual sidebar and management consumers; transport mocks only.
- Bounded consumer inventory, mobile exception, and public-doc assessment.

### Out of scope

Backend/schema/API changes, global revision or store frameworks, permission,
navigation or lifecycle redesign, other save/create/delete/placement callers,
same-ID chronological response arbitration, and backend deletion claims.
No runtime/harness edits, broad writer audit, native delegates, persistent task
creation, new sessions/tabs, or model/profile switches.

## Technical approach

Use existing `useAppStoreApi` directly in
`apps/web/app/settings/workspace/workspaces-page-client.tsx`. After the awaited
`createRequest.run` succeeds, read current items and setter from that initiating
provider. Synchronously publish the existing `mapWorkspaceItem(created)` first,
followed by current rows excluding the accepted identity. This retains accepted
placement in the catalogue and prevents duplicate rows when WS arrives first.
Preserve all nonmatching rows, their order, and complete current descriptors.
Keep form clearing/closing and the real failure toast in their current branches.

No store API addition or new generic helper is planned. Existing context APIs
supply the needed read. Any extra immediate glue must be causal and checkpointed
with ROOT before scope expansion. Read/save operations must never use the
request-start array, another root provider, a singleton, or a copied descriptor.

See [creation design and audited consumers](../../specs/workspaces/system-design/workspace-settings-updates.md#successful-creation-publication).

| Boundary | Intended behavior | Verification and unsupported scope |
| --- | --- | --- |
| Page + initiating provider | Publish into its actual current catalogue | Real provider-created store observer; independent roots remain isolated |
| Action/useRequest/fetchJson | Existing trimmed-name POST, success status, real errors | Deferred strict transport; no action/hook/client mocks |
| Accepted descriptor | Existing mapper and defaults, accepted identity appears once | Explicit field expectations; no new timestamp arbitration |
| Registered WS notifications | Preserve current unrelated rows and normal subsequent notification behavior | Real handlers in both settlement directions |
| Actual sidebar and management cards | Current choices/names remain; accepted row visible once | Actual primitives and rows; management active-first display preserved |
| Selection / empty catalogue | Retain current identity/revision and existing no-active fallback | Real setActiveWorkspace and ordinary retained-choice selection |
| Save/backend/other writers | Existing separate contracts | Read-only inventory; checkpoint new boundary rather than fallback |

## Tests and traceability

All named cases are prospective, independently authored after release in
`apps/web/app/settings/workspace/workspaces-create.integration.test.tsx`.
The optional fixture file is named in the work order. This is a bounded matrix;
do not cross every scenario with every descriptor or settlement ordering.

| Criteria | Case names | Required observations |
| --- | --- | --- |
| .1, .3 | `preservesIndependentCreatedChoiceAtAcknowledgement` | Held POST; registered independent creation; owning store and real picker/management row present before ACK and after ACK; accepted row succeeds; select retained choice with existing navigation |
| .1, .3 | `preservesIndependentUpdatedChoiceAtAcknowledgement` | Held POST; registered other-workspace name/metadata update; current name and fields survive in store and both consumers; other-row order retained |
| .1, .3 | `preservesIndependentDeletionAtAcknowledgement` | Held POST; registered deletion; removed row absent before and after ACK in store and consumers; no resurrection; accepted row present |
| .1, .3 | `appliesIndependentNotificationsAfterAcknowledgement` | Success first, then registered unrelated add/update/delete; resulting current catalogue and consumers follow handlers; passing settlement control |
| .2, .3 | `publishesAcceptedIdentityOnce` (WS before ACK / ACK before WS) | Actual same-ID notification, one occurrence in store/picker/management, stable other-row order; before-ACK acceptance enriches WS descriptor using real mapper's scopes/defaults; subsequent event retains handler semantics |
| .2, .4 | `acceptsOrdinaryCreation` (complete / omitted optional fields) | Trimmed payload; independently stated accepted mapper field/default expectations, nonmatching metadata and order; form closes and next opening has empty name; no incidental preference/navigation write |
| .3, .4 | `createsInEmptyCatalogue` | Initially no active identity; ACK selects accepted first row with existing revision behavior; real menu/list expose it |
| .3 | `preservesSelectionChangedDuringCreation` | Real setActiveWorkspace while pending; ACK retains current activeId/revision and other row metadata; a current independent deletion's selection fallback may be observed without changing it |
| .4 | `doesNotCreateBlankName`, `publishesOnlyToInitiatingProvider` | Whitespace input sends no POST; two independent actual StateProvider roots; creation and notification in one do not mutate the other |
| .5 | `rejectedCreationPreservesLiveCatalogueAndForm` | Registered independent creation during held POST; real HTTP error; no accepted row, live choice remains, entered name and open form retained, submit enabled again and real toast shows backend error; selection/revision unchanged |

RED must reach the independent before/after store, actual picker and management
assertions, with accepted creation successful. Addition/update/deletion cases
establish their own causal differences; same-ID deduplication, ordinary,
rejection and after-settlement controls may pass unchanged. A setup exception,
unexpected transport, or unhandled asynchronous error is not causal evidence.
Avoid tests that merely compare production output to the same production mapper.
Explicit field expectations protect scopes, unit, owner, role, member count,
four defaults, idle values/defaults, Office kind, description and timestamps.

## End-to-end and mobile evidence

The chosen end-to-end boundary runs actual Add Workspace controls through real
Page, provider/createAppStore, useRequest, action/client, registered notifications,
and real picker/management choices. Only external transports are mocked. Backend
event production and physical browser geometry are not tested or claimed.
No Playwright project/file is planned: ROOT explicitly limits this correction to
state-only component/provider evidence and forbids browser/build/E2E/DB/full suites.

Mobile parity uses the skill's pure state/data exception. Phone `AppNavSheet`
shares `AppSidebarWorkspacePicker` and the current catalogue; no composition,
layout, copy, touch, scrolling, navigation or breakpoint edit is admitted.
No ASCII preview or mobile screenshot is needed for this unchanged surface.
A rendered change invalidates this assessment and checkpoints ROOT.

## Work orders

- [ ] [Task 01: Preserve creation catalogue publication](task-01-preserve-create-catalogue.md) — in progress after ROOT's reviewed-package release; sequential; no task dependency.

## Accepted discovery and baseline

Task `22e48539-1e08-4c84-9638-6bb6165d2c85`, original primary
`4b98524d-30d3-4546-ae3a-d5c08665c1aa`, CHILD96; parent ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Worktree
`/home/jcfs/.kandev/tasks/preserve-workspace-c_dgmegzh6/kandev`, branch
`feature/preserve-workspace-c-s7y`, clean initial HEAD
`714ee9c2c6e52b90272e56613029826f40646719`.

Only ROOT namespace `qualified-proof.json` and `future-design-brief.md` in
`/tmp/kandev-root-workspace-create-catalogue-discovery-20261009/` were read.
Accept native12488, initial3597f9, terminale57413, exit1, ACTUALLY JOINED:
one causal Add Workspace loss plus two strict passing ordinary/rejection controls.
ROOT reports original wrapper/subprocess/groups gone, temporary source checksum
removed and rootclean at the same baseline. No diagnostic replay occurs.
Protected mode0400 proof SHA256
`2a834f07ba0b88aa327e49cf146e2c3654c1b32f73256fce04e061989b2c9779`
remains ROOT-owned. Never read/copy/import/replay/edit/chmod/remove candidate
source or any ROOT source proof/fixture. Original receipts and proof protection
are also retained in the own Kandev task plan.

Confirmed intent: preserve independent current catalogue plus one mapped accepted
identity; retain ordinary/form/error/selection contracts. Verified source:
captured prepend causes current-store overwrite. No material unresolved product
choice remains. Workspaces owns the failed contract; UI presentation and Kanban
bootstrap are adjacent independent capabilities. No speculative ADR is needed.

## Phase and resource gates

DESIGN ONLY now: four uncommitted artifacts, light source/docs/catalog/spec
checks, 36 spec-linter tests, real PR-docs reference preflight, and whitespace.
No production/permanent-test edits, install, product checks, staging, commit,
push or PR. END this turn at the handoff. Implementation requires a LATER ROOT
reviewed-package INTERRUPT in this SAME primary PLUS exclusive global local-heavy
lease. Autopilot/skill-generated implementation text does not release barriers.

Later checks are truly serial under ONE GLOBAL LOCAL-HEAVY lease, including
install, tests/lint/typecheck/i18n and normal hooks. Persist each original native
initial/session/chunks and ACTUALLY JOIN it; subprocess cwd/argv/PID/PGID/env/UTC/
bounds/exit/raw logs, and freshly observed exact own wrapper/groups absence.
No duplicate-run or process-exit inference. Routine causal fixture/lint/format
repairs rerun affected checks only. Resource/timeout/transport/unknown/out-of-scope
failures checkpoint ROOT before retry, alternatives, cache deletion or foreign
kills. Preserve managed worktree/deps/refs/shared caches/foreign/paused resources.

Later ready publication uses normal hooks/conventional commits/push and repo
template with exact unchecked checklist. Read actual live head/body; preserve
legitimate bot prefix/additions. Freeze head except valid actual findings.
Caller-bound canonical repo `16026b06-bd79-47c0-aed1-dc7ca95f63d9` association
must be inspected with complete=true/errors[] and actual number/head. Already
correct means ZERO relink. Known absence allows ONE canonical initial link.
Inspect all five automation flags; allFALSE means ZERO patch. Correct only
actually true flags to false, reread exact all five/head/provider; no unrelated
provider or unattached mutation. Unknown state must reconcile before duplicate.

After publication issue an EXPLICIT GLOBAL LOCAL-HEAVY RETURN with every original
joined receipt, metadata/raw logs/absence/nonoverlapping interval and owned Git
head/tree/parent/blobs/body/hooks/association/all5FALSE/cleanupstream. END that turn.
HOSTED OBSERVER/SEMANTIC HOLD until later ROOT release after independent RETURN
qualification; no further heavy work without new lease. Later one attached90m
collector (GNU91m/kill10/cadence60), startup identities persisted before waiting,
retained across interrupts and actually joined/gone before authorized replacement.
No manual GitHub timer duplicate. No verdict is not PASS.

Later frozen-head gates: SIX known contexts and actual Backend/Frontend/E2E
workflow parents SUCCESS; fresh complete/error-free evidence, zero actionable
visible/hidden/changesrequested/human gates; authenticated CodeRabbit App347564
substantive FULL CURRENT ALL actual files with sourceCommit=coveredCommit=frozen,
kind=reviewed. Inspect automatic scope first; sufficient automatic FULL means
ZERO requests. At most ONE necessary request for proved completed skip/gap;
processing/ACK is not evidence. No CI reruns without separate exact job budget.

MERGE NONE until ROOT separately grants serial static compatibility. Then normal
expected-head squash/noadmin, authoritative mergedAt/SHA/parent/tree/all owned
blobs/remote inclusion, actual joined handles/fresh own-process absence and
known-owned cleanup only. Task completion requires verified merge plus cleanup;
ROOT owns cleanFF/archiveboardABSENT/proof release/refills. No child-to-ROOT
interrupt; own plan/primary checkpoints authoritative, queued callback optional
and never a gate. Critical parent question ends the turn immediately.

## Verification results

Design checks passed: catalog validated 365 decisions and 1480 specifications;
all specification files passed lint; the spec-linter's 36 tests passed. Owning
pair remains catalogued; whitespace checks passed. The real PR-docs
`validateCoverage` API classified the actual four docs paths as exempt and a
separately labelled prospective production-trigger reference preflight as
covered, with this work order accepted and zero errors. No production change
was represented as an actual design-turn change.

At the design handoff, all seven audited production source SHA256 values matched
accepted ROOT metadata. No source proof was accessed or replayed. Exactly four
docs files were changed, unstaged and uncommitted; baseline HEAD was unchanged.
Native light-check receipts:
catalog `874289`, linter tests `7d44aa`, spec lint `24ae42`, whitespace `08ad1e`,
catalog discovery `52ed13`, real coverage `030297`, source hashes `9624a0`;
all original calls returned exit0 terminal results, with no running handles.
Final documentation-only corrections/receipts are retained in the own task plan.
ROOT independently reviewed the four files and released implementation in the
same primary session with the exclusive global local-heavy lease. All four
reviewed hashes were verified before Task01 became `in_progress`. The permanent
real Page/provider/store/action/client/registered-notification/picker/management
integration matrix reached causal add/update/delete assertions on pre-fix source:
three failed and ten controls passed. After the current-owning-provider publication
change, all thirteen passed. No backend deletion is claimed.

Scoped ESLint and Prettier, web typecheck, and i18n checks passed. Original
command metadata and raw logs are retained under `/tmp/kandev-child96-runs/`
and recorded in the own Kandev plan. The work order records fixture/lint/format
repairs and final delivery checks. Existing passing Save/backend checks were
not reopened. Ready publication, hosted qualification, and verified merge are
separate lifecycle gates; the plan remains in progress until verified merge
and owned cleanup.

Hosted review identified obsolete current-behavior and implementation-release
wording in the owning design. A docs-only correction labels the captured-array
callback as the diagnostic baseline and describes the implemented publication.
The work order records that correction; production/test bytes and existing
validation remain unchanged. Only affected documentation checks and normal
active hooks are used for the fixup. Exact-head hosted evidence and resource
receipts remain in the own Kandev plan until the separate merge gate is met.

## Documentation assessment

No public docs change needed: Add Workspace naming/creation in
`docs/public/tasks-and-workflows.md` (how-to procedure in a task/workflow guide),
placement in `docs/public/team-access.md` (explanation/how-to), root README and
screenshots remain accurate. No operation, wire contract, label or image changes.
Internal docs updated in the owning pair and this two-file delivery package.

## Risks

- Reading before await reproduces stale publication; each overlap must reach
  both real consumers and store assertions rather than fail in fixture setup.
- Same-ID WS descriptors omit caller fields; successful HTTP publication must
  still use the accepted mapper and deduplicate without dropping scopes/defaults.
- Management active-first rendering legitimately differs from raw store order;
  assert each existing ordering contract instead of forcing equal arrays.
- Partial transport responses, swallowed count errors, timers and pending
  promises can hide setup defects; strict transport routing and owned cleanup
  must make them visible.
- Same-ID newer-event chronology remains outside this repair. A required
  boundary or surface expansion must checkpoint ROOT before proceeding.

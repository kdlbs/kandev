---
created: 2026-10-09
status: implemented
requirements:
  - REQ-EXECUTORS-PROFILE-EDITOR-001
system_design:
  - ../../specs/executors/system-design/profile-editor.md
legacy_specs: []
---

# Implementation Plan: Preserve executor choices during policy saves

Previous implemented continuation: ROOT admitted the scoped shared-fixture cleanup
finding from review5469963689 on frozen98f2e6ea. The same order is implemented
for its owning spec only: restore baseline and remove exactly its three untracked
paths in nested cleanup, with real-repository post-cleanup assertions. Original
observer39101 was explicitly stopped and actually joined exit143, with its last
25passed/0failed/24pending snapshot retained without a CI verdict. ONE fresh
managed Pierre case, changed-spec lint/format and docs/coverage precede normal
corrective publication. Evidence lives in
`/tmp/kandev-child98-review-cleanup-20261009/`; earlier product results/history
remain unchanged. No new public contract or docs impact. Return heavy and END
before separately released hosted collection; merge remains unauthorized.

The own fresh cleanup GREEN passed exactly one test in 16.3s with strict guards,
fresh backend/Vite/plugin artifacts and all 63 observed own descendants absent
at actual native/subprocess join0. Changed-spec lint and formatting passed.
The [same work-order continuation](task-01-preserve-policy-catalogue.md#later-scoped-review-correction-2026-10-09)
records scoped ownership, real post-cleanup assertions and raw receipt references.
Later doc/coverage gates and normal publication are retained separately; no
earlier product result or interrupted hosted verdict is promoted.

Current continuation: ROOT released the reviewed preview-history setup-failure
rollback correction to this same primary. The same order is implemented for
exactly the existing seed helper and one real-Git regression file. The source
causal RED is already ROOT-qualified and is accepted without replay. This
candidate passed five real-Git helper cases and one fresh managed desktop
Pierre control in 17.0s; affected helper/test lint and formatting passed.
Docs/eleven-path coverage and normal publication retain separate receipts. No executor-policy behavior or public docs change.
Original replacement observer31443 was explicitly stopped and joined exit143;
its last39passed/3failed/7pending snapshot is retained with no CI terminal
verdict. The three unrelated CI causes remain unknown. Evidence lives in
`/tmp/kandev-child98-seed-failure-fixup-20261009/`. Later hosted and merge gates
remain separate after explicit heavy return and actual END/WFI.

## Overview

Correct the executor MCP-policy form's successful acknowledgement publication
so task-start choices retain independently received catalogue changes. One
sequential work order owns independent regressions, the bounded page correction,
and evidence. The existing executors [requirement](../../specs/executors/requirements/profile-editor.md)
and [design](../../specs/executors/system-design/profile-editor.md#executor-policy-acknowledgement-publication)
own the contract; Tasks and UI are consumers, not additional specification owners.

## Admission and evidence

CHILD98 task `fa9bcee2-04fc-49ba-b49d-a1bfb0d19282`, primary session
`6707a8f8-c778-46d4-b068-ccbfbc00591d`; parent ROOT
`14825981-b175-411d-999a-31ddc2aa5fc3`. Same primary/model/profile throughout;
no agents, delegates, recursive tasks, new tabs/sessions, or model changes.

The design turn ended at its concrete four-artifact handoff, with all documents
unstaged/uncommitted and no product work. ROOT later reviewed those exact hashes
and explicitly released this same primary, assigning CHILD98 the exclusive
serial local-heavy lease. This implemented package does not release hosted
collection or merging; both retain their later separate ROOT gates.

ROOT qualified the defect once on baseline
`b8b740f128993b804745704591374f5a9fc7b0ab`: original native `15953`, chunks
`89ca90` -> `7eb109`, actually joined exit 1, exactly two causal assertion
failures, one ordinary positive-control PASS, no unhandled/setup errors. The
fixture rendered the real page/store/provider/coordinator/registered events and
real task-start options; only network transport and external editor rendering
were controlled. PATCH payload, accepted own policy, and dirty clearing were
checked. Initial native `58444` had a missing loader mock export and is not
qualified evidence.

Read-only receipts live in
`/tmp/kandev-root-executor-edit-catalogue-discovery-20261009/`:
`qualified-proof.json`, `proof-process-corrected-fixture.json`, and
`proof-corrected-fixture.log`. The metadata retains original argv/cwd/env,
180-second bound/kill10, wrapper PID/PGID `1138704`, subprocess PID/PGID
`1138726`, actual joins, raw exit and log SHA256
`35c07cc8e08f069dbbbf374f04808e1f8f979c20298c3156d9973ff27feb5270`.
ROOT reports own groups gone, temporary source removed with checksum proof,
clean root and returned heavy lease. Accept this evidence without replay.

NEVER read, copy, import, replay, edit, chmod, or remove ROOT's protected
`candidate.test.tsx` in that directory (0400, SHA256
`d2d896e659b5c40b5e7dda1c13c670ac8452084af5c94c480714cc99579db55a`).
Permanent regressions must be authored independently after release.

## Scope

### In scope

- `ExecutorEditForm.handleSave` in `apps/web/app/settings/executor/[id]/page.tsx`.
- Current owning-AppStore publication and faithful component/consumer evidence
  for AC-EXECUTORS-PROFILE-EDITOR-001.19 and .20.
- Minimal additions to the existing owning requirement/design, plus this package.

### Out of scope

- Backend/API/schema, global store contracts, WS mapping or transport redesign.
- Other settings saves, executor deletion, profile-card create/delete refresh,
  route lifetime, navigation, layout, copy, touch, scrolling and breakpoints.
- Same-executor server arbitration, stale full config drafts, generic catalogue
  revision or migration, optional polish, broad replay or synthetic merged tests.

## Technical approach

The form closes over `executors.items`, awaits `executor.update` or
`updateExecutorAction`, then replaces the entire array with the captured map.
Registered live profile handlers correctly update a different executor during
the wait. Successful policy save subsequently erases a new usable choice or
restores a removed choice.

Use `useAppStoreApi` in this form, read `appStore.getState().executors.items`
after transport succeeds, and synchronously map only the accepted executor ID
with the existing current-item/response spread. No intervening await or missing
owner insertion. Keep current `setSavedMcpPolicy`, raw draft comparison,
contributor, validation, payload and rejection behavior. Both backend update
handlers use `dto.FromExecutor`, which omits `profiles`; the merge preserves
current saved-owner membership as well as all unrelated current executors.

| Path | Existing boundary | Planned behavior and evidence |
| --- | --- | --- |
| System executor policy | REST PATCH / WS `executor.update`; no submitted name | Same accepted config and current catalogue; real form and payload controls |
| Non-system executor policy | Same transports plus existing submitted name | Same publication; payload, success and failure controls |
| Task creation/subtask | Current store, fallback owner metadata, actual `useExecutorProfileOptions` | Eligible live choices retained; deleted choices absent; gates unchanged |
| Missing saved executor | Page resolves unavailable owner | Current map cannot resurrect it; deletion-versus-save server ordering unclaimed |
| Profile card | `refreshProfiles` after card create/delete only | Audited separate captured writer; no policy-save dependency, no production edit |
| Executor deletion | Separate `DeleteExecutorSection` | Audited separate writer; excluded |

## Tests

New independent suite:
`apps/web/app/settings/executor/[id]/executor-policy-catalogue-publication.test.tsx`.
A same-directory `.test-helpers.tsx` may hold fixture mechanics only if needed
for source limits; no production helper or copied protected fixture.

| Criterion | Planned cases |
| --- | --- |
| .19 | `retains another executor live created profile and usable choice`; corresponding update and delete cases |
| .19 | `retains inserted owners and keeps removed owners absent`; mixed add/update/delete/owner changes with retained and removed rows |
| .19 | `retains current saved-owner profile membership`; `does not resurrect a removed saved executor` |
| .19, .20 | `isolates acknowledgements between live providers`; failure after live events preserves current catalogue/options |
| .20 | `unchanged catalogue success submits and accepts policy`; system/non-system REST/WS controls; absent response profiles retain existing rows |
| .20 | `uses normalized accepted baseline without rewriting raw draft`; discard reads accepted baseline |
| .20 | `keeps newer draft while acknowledging captured submission`; unchanged failure retains draft, dirty and coordinator failure |

Use the real rendered page, providers, coordinator, action adapter, registered
profile handlers and options hook. Only external transport and external Monaco
rendering may be controlled. Preserve loader exports. Hold transport explicitly,
prove actual payload/admission and live options before settlement, then assert
current catalogue and actual eligible options after acknowledgement. Clean up
all held work/providers/guards/connection state/timers even when assertions fail.

## End-to-end and mobile evidence

This package changes state/data publication only. The mobile-parity exception
is explicit: real rendered component through transport acknowledgement, live
registered events and actual task-start view-model evidence covers the shared
desktop/phone semantics. No layout, touch, scrolling, navigation or breakpoint
change is planned, so no UI sketch, browser/build, or new Playwright test is
required. Reassess scope before changing any of those surfaces.

## Documentation and decisions

Internal docs updated: the owning pair gains .19/.20 without promoting or
resetting the broad active/current statuses. Existing companion package
statuses/history remain intact: unification `complete`/`done`, prior catalogue
and creation repairs `implemented`/`done`, script repair `draft`/`in_progress`.
This new pending work does not imply any of those packages were rerun.

Public docs audit: `docs/public/executors.md` (reference/explanation) already
describes profile selection, settings and MCP policy; `README.md` and
`docs/screenshots.md` introduce no conflicting policy-save steps. The correction
restores that existing behavior without new configuration, API, terminology or
screenshots. No public edit or new ADR is needed.

## Work orders

- [x] [Task 01: Publish policy acknowledgements over the current catalogue](task-01-preserve-policy-catalogue.md)

Sequential only. No prerequisite implementation work order; later ROOT release
and local-heavy lease are mandatory execution barriers. Exact bounded runtime,
test, lint, typecheck, i18n and actual changed-path coverage commands live in
the work order.

## Verification results

Design-document checks passed on 2026-10-09: catalog validation (368 decisions,
1493 specifications), all 36 spec-linter tests, full specification lint, and
diff whitespace check. Existing Node `v24.21.0` was qualified in the apps cwd.
Actual four-path docs-only coverage returned `exempt`, errors[]; separate
planned-page reference validation returned `covered`, errors[], one work order.
The latter is reference validation, not actual implementation evidence.

Original native handle `18620`, initial chunk `b7a52b` -> terminal `17d9ff`,
actually joined exit0. All six subprocesses joined exit0; exact OWN groups gone,
wrapper PID `1176048` and group `1176046` independently observed gone. Raw
argv/cwd/env/bounds/exits/log hashes are retained in
`/tmp/kandev-child98-policy-design-20261009/process-receipts.json`, with individual
logs and `completion.json`. All four artifacts remain unstaged/uncommitted.

At that historical checkpoint, product RED/GREEN, installation and delivery
were deferred and no local-heavy lease was held. ROOT's receipt was defect
evidence, not a permanent regression or GREEN result.

### Implementation verification

ROOT subsequently released this same primary under CHILD98's exclusive lease.
The bounded publication correction and all task-defined checks passed. The
[work-order results](task-01-preserve-policy-catalogue.md#implemented-result-2026-10-09)
record exact original commands, RED/GREEN counts, known fixture-only lint/type
remediation, joins and raw receipt references. Final GREEN is 25 PASS; actual
seven-path coverage is covered, errors[], one work order. The production diff
is confined to the policy form; all exclusions and earlier histories remain
intact. Heavy return follows normal ready publication, before separately
authorized hosted collection and merge.

## Execution and delivery boundaries

The durable Kandev task plan preserves the system marker, identities, question
barriers and full ROOT delivery instructions. One global serial local-heavy
lease is required, with explicit return after ready publication. Retain original
native and subprocess handles, argv/cwd/env/bounds/raw exits/hashes and proof
that exact OWN PID/groups are gone. Resource, timeout, unknown or out-of-scope
failures checkpoint ROOT before alternatives; no retries, cache wipes, foreign
kills, hook bypass, optional polish or weakening.

After release and checks: clean frozen head/branch, normal unchecked PR template
preserving bot additions; inspect canonical automatic association before any
necessary initial link. All five automation flags FALSE, zero unnecessary
relink/settings patch/body churn. End implementation after explicitly returning
heavy, before later ROOT hosted release.

Hosted work requires ONE attached original90m all-terminal `scripts/pr-await`
(GNU91m/kill10/cadence60) with retained original handles/startup across interrupts.
No duplicate/replacement until joined-old and ROOT direction. Require six known
required contexts plus actual Backend/Frontend/E2E parents SUCCESS at CURRENT
head, fresh complete/errors[]/zero visible-hidden-actionable/changesrequested/
human gates. Require authenticated configured CodeRabbit App347564 substantive
FULL CURRENT ALL/source=covered=frozen/kindreviewed evidence. Sufficient auto
coverage means zero requests; inspect skip/gap before at most one necessary full
request/candidate. ACK is not evidence; no optional second-review wait. Ground
every finding disposition; frozen head changes only for bounded real findings
under a lease.

At hosted readiness join all own processes, END-WFI. A LATER separate ROOT
serial MERGE grant alone authorizes expected-head normal squash. Independently
verify actual mergeSHA/tree/all owned blobs/remote inclusion and joined OWN
cleanup. Preserve managed worktree/deps/localrefs/raw evidence/shared caches,
foreign/paused resources and protected ROOT source for ROOT archival. ROOT reads
this plan/primary directly; queued callback is optional, never a gate, with no
queue-full retry or forbidden child-to-parent interrupt.

## Risks

- The response owns saved-executor scalar fields/config, not a coherent newer
  same-owner server view; this package must not claim broader arbitration.
- Normalized policy text can remain dirty because raw draft and accepted baseline
  differ. Preserve this existing behavior rather than adding normalization policy.
- Card-refresh and executor-delete writers remain independent excluded concerns.
- An invalid fixture/mock or unjoined resource is not causal evidence; stop at
  ROOT's checkpoint instead of replaying protected source or broadening work.

## Later targeted CI remediation (2026-10-09, implemented)

ROOT released the same primary with an exclusive serial heavy lease after the
original implementation and diagnostic handoffs. Actual current-head Shard11
failed at the Pierre visible-anchor readiness assertion with NULL before refresh.
ROOT independently qualified the matching shared-helper cause, permanent RED and
Pierre/Monaco/mobile GREEN at source candidate
`cd8ffc66a937e33e6fec1eab9e9e8c53129a33cf`. Those original causal receipts are
accepted without replay: `/tmp/kandev-root-child97-causal-helper-checks-qualified-20261009.json`.

This later exception to the original browser/E2E exclusion admits exactly
`apps/web/e2e/tests/git/git-refresh-continuity-helpers.ts`,
`apps/web/e2e/tests/git/diff-refresh-continuity.spec.ts`, and
`apps/web/e2e/tests/git/git-continuity-preview-history.ts`. The readiness poll
aligns the selected section and scroll root using their common viewport
rectangles on each existing attempt. A private three-commit preview history
establishes the 14-file regression and restores its original private repository
head in the existing cleanup. Existing timeout, refresh assertions, strict WS
accounting and fail-on-flaky policy remain intact. This changes only E2E setup;
the executor-policy production fix, owning requirement/design and page tests
retain their exact previously verified bytes. No new plan or specification owner.

Transfer is only the ROOT-reviewed patch with SHA256
`a8e7e332c3214fdb8b1835df4db5489769512b21a5449a2f7a4a9fe41d591f22`,
manifest `/tmp/kandev-root-shared-pierre-correction-20261009/manifest.json`;
both original changed code blobs match this candidate exactly. One new helper
was absent. No sibling worktree/protected proof or source build artifact reused.

Only ONE fresh candidate desktop Pierre continuity GREEN is admitted, with
managed fresh backend/Vite/plugin builds, normal guards, one shard/worker,
retries0/repeat1, strict fail-on-flaky, GNU15m/kill10, GOMAXPROCS2,
GOFLAGS=-p=2, GOMEMLIMIT512MiB and Node4GiB. Exact selection is
`chromium`, `tests/git/diff-refresh-continuity.spec.ts`, anchored grep
`desktop Git diff refresh continuity retains counts and reading state with pierre-diffs$`.
Changed-three-file ESLint/format, docs validation and actual ten-path coverage
plus discovery and normal commit hooks precede the authorized fixup commit/push.
No broad page/type/i18n/backend-lint replay or expanded E2E typecheck is admitted.

The prepared mobile observation remains unapplied/unrun. The distinct Shard5
mobile setup/follow cause is unproved; its assertion/fixture/production behavior
is unchanged. A fresh automatic first CI attempt on the substantive helper
correction is distinct from a manual hosted retry, whose budget remains unspent
and unauthorized. No CI collector, manual review request or merge during this
local turn; later hosted and serial merge releases remain mandatory.

Original raw failed checks remain truthful. Retain every original process/native
join/argv/cwd/env/bound/raw hash and fresh exact own process/group absence under
`/tmp/kandev-child98-ci-remediation-20261009/`. Resource, timeout, unknown or
out-of-scope failures checkpoint ROOT before alternatives. Publish one frozen
candidate, preserve canonical association/FIVE flags FALSE/unchecked author body
and bot additions, return heavy explicitly, then END/WFI. No foreign cleanup.

### Own candidate remediation result

The reviewed three-file transfer matched its admitted patch SHA and all new
code blobs. On this candidate, guarded discovery found exactly one intended
desktop Pierre case. The ONE original managed GREEN rebuilt this worktree's
backend, Vite assets and fixture plugin from its current source, then passed
one test in 18.8s, with zero retries/failed/flaky/skipped results. Normal guards
reported host/shard1/chromium/strict1/workers1; no source build reused or freshness
bypass. The original native90356/b99060 -> 1694af actually joined exit0; all
148 observed exact own descendant identities were absent at its join. Five
fresh artifact hashes/mtimes and exact transferred blobs are retained in
`green-qualified.json`, raw managed log SHA256
`738a175f460cafdda886ccea6bf4c34eab1546ea666e0dfcf39003923ecdb8e2`.

Changed-three-file Prettier and ESLint passed (original29582/96cb46 -> 86216b;
4947/372b1e -> f774f6). Catalog validation passed (80772/449f57 -> b65bb2),
as did full spec lint (inline117b7c). The original docs-spec-lint returned
inline exit0 before a checkpoint writer rejected an absent optional session
field; its exact result was retained before that writer error and reconciled
without replay. There was no product/check failure or unknown live process.
Final diff/actual ten-path coverage and normal hooks/publication retain their
separate original receipts in the same evidence directory.

The original page fix, its two permanent test files and owning requirement/design
remain byte-identical to the earlier frozen head. No public documentation
change: the correction affects private E2E setup/readiness only, with no new
user workflow, copy, configuration, API or executor behavior. The shared helper's
Monaco/mobile controls were already causally qualified by ROOT and are not
duplicated. The distinct mobile-last-prompt observation remains unapplied/unrun;
its cause and eventual new-head CI outcome are not represented as resolved.
No passing product/type/i18n/backend suites replayed. Historical failed checks
and earlier barriers remain retained; same work order, no new plan. The task is
not complete merely because this local remediation is implemented: later ROOT
hosted and serial merge grants remain mandatory after explicit heavy return.

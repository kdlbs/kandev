---
created: 2026-10-22
status: completed
requirements:
  - REQ-AGENTS-MCP-ASSIGNMENT-001
  - REQ-AGENTS-MCP-ASSIGNMENT-002
  - REQ-AGENTS-MCP-ASSIGNMENT-003
  - REQ-AGENTS-MCP-ASSIGNMENT-004
system_design:
  - ../../specs/agents/system-design/workspace-mcp-assignment.md
legacy_specs: []
---

# Implementation plan: Assignment after MCP registration

## Overview

After registering a definition, show Saved with Done and optional Use in.
Use in opens an existing owner editor without changing a selection. The owner
retains save behavior and runtime timing. Build conditional selection safety,
then saved-only completion, then the three owner handoffs and recovery.

Implementation was explicitly authorized on 2026-10-23. Work orders 01 and 02 are
complete. Work order 03 is implemented and verified after integrating the
committed management navigation and profile admission prerequisite. Parent PR #3293 was squash-merged as
`0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab`. The assignment branch was rebased
onto that verified current-main tip on 2026-10-24, then stacked onto the published
management design branch for the authorized draft checkpoint below.

## Baseline and coordination

- Task: `5a8cb5cd-22ee-42e5-bd6d-e2179e899c8c`.
- Workspace: task-owned isolated Git checkout.
- Own branch: `feature/clarify-mcp-assignme-mhh`.
- Historical fetched parent: `feature/plan-workspace-mcp-c-dcc` at
  `193de56ba2597b335a97b1e5ab01d09340f22647`, verified 2026-10-22.
- Historical post-merge PR base: `main` at
  `e5c38184cf9c191075dc88c1ffad28c636858bf5`, verified against the remote on
  2026-10-26. This is the actual squash merge of management PR #4759.
  That checkpoint authorized draft publication only. The user's later fixup
  request authorized ready status, bot review requests, CI monitoring, and
  comment remediation. No merge is authorized.
  Do not mutate the parent or sibling workspace.

### Draft publication stack, 2026-10-24

The user authorized publishing all follow-ups as drafts through the coordinator.
The stack is main -> management -> assignment -> exclusions. Management's
six-file design commit does not implement task 02 navigation. WO03 remains
pending; this checkpoint publishes WO01/WO02 and records the incomplete scope.

Backup `backup/clarify-mcp-assignment-pre-stack-20261024` preserves old assignment
tip `9c9abfb13b351ced2869185eb1e730f682e30637`. Only the four assignment commits
after `0b81ae48d4e2c358f80ac489e39388dfd8d7e6ab` were replayed onto the verified
management head. No conflicts occurred; all four patches are identical under
`git range-diff`. Incoming changes do not overlap assignment-owned files.

| Work | Previous commit | Stacked commit |
| --- | --- | --- |
| WO01 conditional writes | `23a403dc2224a668e7f35c60ff5892952608fd3b` | `055da90bc66b7cdbfce358c9b94f798b7fc8b661` |
| WO02 saved registration | `6bad3f57f3445fd5cd46dfa0bd5d1313543a49f4` | `c6423514b5f52fb5851a5ca7744004ec75e48443` |
| Native cancellation fixture | `c49beff601fa0ce0d324efdccb1b1bb205e7c49c` | `d44e77e7157a019f2dbb553a12a6dcaad56a97ce` |
| Main rebase record | `9c9abfb13b351ced2869185eb1e730f682e30637` | `27f8d626548b28c0312f9d68840c6ac714baa0e6` |

Exclusions has already integrated the earlier assignment tip. Its subsequent
publication rebase must use OLD `9c9abfb13b351ced2869185eb1e730f682e30637` as
the cut point. This supersedes the earlier WO01-only boundary for that next
operation. No exclusions branch or workspace was changed here.

Fresh synthetic desktop and phone captures show saved registration, uncertain
save recovery, and conditional-write conflict recovery. Temporary capture specs
were removed. Assets are ignored, inspected, compressed, and recorded in the
local PR manifest. The incoming global CSS change targets walkthrough drawers
only and does not alter these captured settings surfaces.

Post-stack validation passed: the four-package `MCP|Selection|Effective` Go race
selector, 45 frontend tests across eight files, web typecheck, documentation
catalog/spec/public-doc checks, whitespace, and three desktop plus three phone
assignment E2E cases with retries disabled. Earlier real PostgreSQL concurrency,
Go lint, and localization results remain applicable to unchanged source; they
were not rerun for this second, conflict-free rebase. Mock fixtures do not certify
real provider connectivity. No CI or automated review waiting/handling is part
of this publication request. The final publication SHA and URL are recorded in
the saved task plan and coordinating handoff after the push and draft creation.

### Main rebase, 2026-10-24

The user authorized the local rebase through the coordinating session. The
worktree was clean. Backup branch
`backup/clarify-mcp-assignment-pre-main-rebase-20261024` preserves old tip
`f442aa28d8099b4a41152b9a895e8d8572ae67fb`.
Only the three task commits after `193de56ba2597b335a97b1e5ab01d09340f22647`
were replayed. There were no conflicts. `git range-diff` reports identical
patches for all three commits; no parent PR commits were replayed.

| Work | Old commit | Rebased commit |
| --- | --- | --- |
| WO01 conditional writes | `f9137074309bebe99e2c7576baefe936bc20358f` | `23a403dc2224a668e7f35c60ff5892952608fd3b` |
| WO02 saved registration | `ea9246963405d2559dd33670d68212a89445ae1e` | `6bad3f57f3445fd5cd46dfa0bd5d1313543a49f4` |
| Native cancellation fixture | `f442aa28d8099b4a41152b9a895e8d8572ae67fb` | `c49beff601fa0ce0d324efdccb1b1bb205e7c49c` |

Exclusions must retain OLD WO01 `f9137074309bebe99e2c7576baefe936bc20358f`
as its historical branch boundary for its separately authorized rebase. The new
WO01 is the replacement prerequisite, not the old range boundary. No exclusions
workspace or branch was changed here. Its committed saved-selection comparison
and neighboring E2E corrections remain a separate integration follow-up.
WO03 still requires committed management task 02 navigation.

Rebase validation on 2026-10-24:

- Refreshed workspace dependencies with `pnpm install --frozen-lockfile`.
- Backend `go test -trimpath -race` across MCP config, settings store, settings
  handlers, and backendapp with `-run 'MCP|Selection|Effective' -count=1`: passed.
- Disposable PostgreSQL 17 conditional-writer, resolution, import, and confirmed
  deletion tests under the race detector: passed.
- Eight focused frontend test files: 45 tests passed.
- Web typecheck (8 GiB Node heap) and `i18n:check`: passed.
- Changed Go package lint against the new main base: passed, zero issues.
- Focused assignment E2E: three desktop and three phone cases passed with
  retries disabled. Managed runners ran sequentially against isolated fixtures.
- Documentation catalog, specification lint, public documentation validation,
  and `git diff --check`: passed.

All evidence above was rerun after the rebase. Historical commit hook receipts
remain historical; rebasing does not mean those hooks ran again. The subsequent
plan-only commit uses the normal hooks. No push or merge was performed.

Read the current parent head and specifications again before implementation.
The parent's testing guide is historical context; its old ports and counts
are not current evidence. Never use ports 9998, 48593, or 48595 for this work.

Management sibling `000d477e-f792-4c9d-8503-50ed73bb9646` owns the missing
global-profile workspace picker and authorized owner navigation. Its full saved
plan was read; no implemented destination was available at planning time.
Work order 03 depends on its committed `task-02-management-navigation.md` result.
Reconcile the exact owner-link interface before that work order. Do not add
another management hub or copy uncommitted files. Work orders 01 and 02 do not
need that sibling. Inherited exclusions and built-in management remain excluded.

Exclusions sibling `15641d1a-d7ba-4216-aefd-5a4c36a88d73` proposes the session
`selection_revision`/`expected_revision` pair and optional
`excluded_definition_ids`. Its draft was read for coordination. Assignment uses
that one session precondition and leaves exclusions omitted/preserved. Existing
same-set application retries remain supported. Exclusions storage and controls
remain sibling-owned. Assignment work order 01 owns the shared conditional-write
primitive. Exclusions work order 01 depends on that committed primitive and
extends the same session transaction. Assignment has no exclusions prerequisite.
Record the actual operation/lock/type contract in work order 01's results.
This replaces the earlier whichever-lands-first option. The user authorized this implementation on 2026-10-23; sibling implementation
remains separately owned.

## Scope

### In scope

- Custom and marketplace registration completion with saved-only exit.
- Optional repository, workspace-contextual profile, and existing-session handoff.
- Same-definition recovery after assignment failure or lost acknowledgment.
- Conditional selection replacement, eligibility, and context race protection.
- Desktop/phone composition, localization, focused tests, and public guide updates.

### Out of scope

- Bulk or task-level assignment, owner creation, automatic selection or enabling.
- Used in projection, global-profile management implementation, or duplicate hubs.
- Session inherited exclusions, built-in management, probes, new runtime timing.

## Technical approach

The [design](../../specs/agents/system-design/workspace-mcp-assignment.md) defines
the full boundary. `mcp_selection_handlers.go`, `SelectionService`, and
`settings/store/mcp_selections.go` add a direct-membership precondition for
non-session owners and use the shared session revision precondition for sessions.
The web editor sends its accepted baseline/revision, not its draft or inherited
union, and addition-only writes omit exclusions. Conflict recovery uses explicit
reload/review and preserves creation.
Atomic writes return their own committed result. No migration is proposed.

`MCPSettings` and `MCPMarketplaceView` preserve the create/install result.
`useMCPWorkspaceSettings` guards workspace generations and reconciles rows by ID.
New focused completion and owner-choice components stay under
`components/settings/workspaces/`; data/controller hooks stay under
`hooks/domains/workspace/`. Their names below are proposed, not existing APIs.

Owner destination builders come from the management sibling. Reuse
`RepositoryMCPSelectionSection`, `ProfileMCPSelectionCard`, session Customize,
and `linkToTask`. Consume the management sibling's typed owner routes. Assignment
owns the reserved focus-only `mcpServerId` hint described in the system design.
Settings can reveal its row; sessions wait until the user opens Customize. Do
not stage a checkbox, change search, PUT selections, start/ensure an agent, or
replay a consumed hint after reload. Existing Save changes or explicit session selection commits.

### Provider compatibility

| Path | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- |
| Registration and owner discovery, every transport | Storage and authorized reads only | Request spies and catalog/service tests | Readable discovery failure; Done remains available |
| Repository/profile selections | Future normal attachment uses saved additive set | Existing selection tests plus handoff E2E | Definition remains saved; failed selection is local |
| ACP with verified safe same-session reconnect | Existing idle apply pipeline only | Existing provider timing tests; mock E2E checks UI state | Existing deferred or failed state, not invented support |
| Active turn | Existing pending-idle state | Focused session handoff E2E | No mutation of delivered turn |
| Passthrough, project-file consumers, unsupported ACP reconnect | Saved selection waits for normal next start | Deferred-state fixture | No forced restart or package preparation by chooser |

Mock E2E proves UI and persistence only. No new claim about real provider
connectivity or npm materialization is part of this package.

## UI previews

These structured previews replace ASCII drawings in accordance with the task's
Kandev rich-output instruction. Control order and save ownership are required;
example names and spacing are illustrative. Use existing tokens and primitives.

### UI-01: Saved completion, both viewports

Entry: acknowledged custom or marketplace registration, Configured view.

| Order | Content/control | Behavior |
| --- | --- | --- |
| 1 | Saved to Acme / Atlas docs | Announces registration only |
| 2 | Choose where to use this server, or finish now | No assignment required |
| 3 | Done; Use in | Done completes; Use in opens the chooser |
| 4 | Existing catalog cards | Saved definition stays visible; each card offers Use in |

Phone uses a single column and wrapping actions. No setup dialog remains open.
Maps to AC-001.1 through .4 and AC-004.1 through .4 (full prefix
`AC-AGENTS-MCP-ASSIGNMENT`).

### UI-02: Owner choice and consequence review

| Region | Desktop bounded dialog | Phone inset Drawer |
| --- | --- | --- |
| Fixed header | Use Atlas docs in / Acme / Close | Same identity and contained Close |
| Scroll body, first view | Repository, Agent profile, Existing task agent session | Three full-width rows |
| Scroll body, next view | Search; authorized named results; task then session if needed | Replace content in the same Drawer; Back stays visible |
| Review | Named owner and consequence text | Wrap names and consequence above actions |
| Footer | Back; Open MCP settings | Safe-area footer, at least 44px targets |

Open navigates to the exact existing owner. Repository/profile reveal MCP and
can highlight Atlas docs once without changing selection or search. Session
navigation opens only the conversation; the hint waits until the user opens
Customize. Repository/profile show shared Save changes only after a user edits.
Session Customize explains autosave before its checkbox.
Maps to AC-002.1 through .6, AC-003.2/.4, and AC-004.1 through .4.

### UI-03: Partial success and unavailable targets

| State | Required presentation and recovery |
| --- | --- |
| Loading owners | Keep Saved and Done visible; status text; no stale choices |
| Empty/search empty | Explain no eligible targets or no matches; Back/Done |
| Owner unavailable | Readable reason; choose another owner; no hidden fallback |
| Selection conflict in owner editor | Choices changed; Reload choices; retain local draft until explicit reconciliation |
| Saved selection, runtime failure | Existing Customize failed notice and retry; no registration retry |

The catalog's Use in action remains the recovery entry after closing or reload.
Phone has one scroll owner and never places a recovery action behind an overlay.
Maps to AC-001.2/.4, AC-002.5, AC-003.1/.3/.4, and AC-004.2/.4.

## Tests

Names here are planned tests to create with TDD, not claimed existing evidence.

| Criteria | Test file and scenario |
| --- | --- |
| AC-003.1/.2/.3 | `mcp_selection_concurrency_test.go`: `TestMCPSelectionConditionalReplace`, `TestMCPSelectionCommittedResponse`, and `TestMCPSelectionRetryPreservesAcceptedState` |
| AC-002.3/.5, AC-003.4 | `mcp_assignment_eligibility_test.go`: unauthorized, wrong workspace, disabled profile, archived/deleted task/session; late request ownership in hook tests |
| AC-003.1/.3 | `postgres_mcp_selection_concurrency_test.go`: independent writers, known-empty set, replacement vs deletion/legacy import, session revision conflicts; after integration, mixed additions/exclusions clients preserve omitted fields |
| AC-001.1/.2/.4, AC-003.4 | `use-mcp-registration-completion.test.ts`: exact saved identity, rejected target lookup, uncertain ACK, context change, no second create |
| AC-002.1 through .4/.6, AC-003.2/.3/.4 | `mcp-assignment-navigation.test.ts`: typed links preserve query/hash; hint parser rejects malformed/duplicate IDs; wrong workspace, removed, disabled and filtered-out rows; delayed inventory and context changes; one-time consumption; no checkbox/search/draft writes or session overlay opening |

Existing selection editor tests must remain covered when changing its API.
Work order 01 includes service, repository, handlers, backend owner validation,
typed client, and hook checks. Do not claim PostgreSQL coverage when its DSN is
absent. Use a disposable database and record that limitation explicitly.

## E2E tests

Create `e2e/tests/settings/mcp-assignment.spec.ts` (`chromium`) and
`mobile-mcp-assignment.spec.ts` (`mobile-chrome`), with helpers under
`e2e/helpers/mcp-assignment.ts`.

| Flow | Criteria | Required proof |
| --- | --- | --- |
| Custom and marketplace saved-only | AC-001.1/.2/.3/.4 | Exactly one definition, reload persistence, zero selection/start/probe requests, edit does not repeat completion |
| Repository and profile handoff | AC-002.1/.2/.3, AC-003.2 | Correct owner, explicit workspace for global profile, no PUT on navigation, Save/discard, other choices preserved |
| Existing session handoff | AC-002.4/.6, AC-003.2/.3 | Exact sibling session, no ensure/start or automatic Customize opening; highlight only after user opens it; explicit checkbox autosave; pending/deferred/failed distinct; transcript retained |
| Focus-only hint | AC-002.6, AC-003.4, AC-004.3 | Existing selected state and search unchanged; invalid/unavailable/filtered rows never broaden inventory; async load and workspace race safe; consumed key removed with other query/hash intact; rerender, reopen and reload do not replay |
| Partial success and races | AC-001.2/.4, AC-002.5, AC-003.1/.3/.4 | Failed/uncertain selection and 409 recovery retain one definition; disable/delete/access change fails safely; stale workspace responses ignored |
| Phone and keyboard | AC-004.1 through .4 | All owner types and saved-only reachable; final result reachable; 44px targets, one scroller, focus return, no horizontal overflow, locale wrapping |

Phone checks include 320px, configured Pixel 5, 767px fine pointer, 768px
boundary, and 900px touch tablet. Measure controls and container bounds. Test
desktop-to-phone-to-desktop while search/choice is set; state must survive.
Use one failing request fixture rather than disabling real services. Never run
overlapping full suites. Commands below are executed sequentially.

## Work orders

- [x] [01: Guard selection replacement](task-01-guard-selection-replacement.md)
- [x] [02: Preserve saved registration completion](task-02-registration-completion.md)
- [x] [03: Connect optional owner handoffs](task-03-owner-handoff.md)

Order: 01, 02, then management sibling navigation prerequisite, then 03.
No delegation is authorized. Work order 02 is an intermediate saved-only result;
the overall feature is incomplete until all three owners pass work order 03.

## Verification results

Design checks completed on 2026-10-22:

- `python3 scripts/list-docs.py validate`: passed (397 decisions, 1693 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check -- docs/specs docs/plans/mcp-assignment-after-registration`:
  passed. A separate whitespace scan covered all six untracked draft files.
- Repository PR-documentation `validateCoverage` preflight: passed as docs-only
  exempt. A separate package traceability check covers REQ/AC/design references.

Historical design checkpoint: runtime, product unit, and E2E checks were not run. No
production or permanent test files changed. Planned implementation commands
and evidence belong in the corresponding work orders. Work orders 01 and 02 now record implementation evidence; none of the parent guide's historical counts were reused.

Implementation checkpoint 2026-10-23: conditional writes and saved-only
registration are implemented and tested. Six desktop/phone E2E cases passed.
Management plan version `40b577c3-0d95-43b5-9a57-ae8af8ee149b` is unchanged
and still records uncommitted design only. Its committed task 02 navigation
prerequisite is unavailable. Work order 03 remains pending; no optional owner
chooser, owner-route hint, or full-feature completion is claimed. Do not copy
uncommitted sibling code or implement its ownership here.

## Risks

- Management navigation is not implemented in the baseline. Work order 03 must
  consume its reviewed committed result or return an integration blocker.
- Selection writes currently lack conditional browser protection. A client-only
  reread or mutex cannot prove concurrent database correctness.
- Task owners and settings storage have separate service ownership. Retain
  authorization and deletion cleanup; do not claim a new cross-service transaction.
- Existing settings contributor ACK reconciliation must preserve newer local
  drafts. A null/stale hook result cannot count as a successful save.
- Sibling work may touch shared MCP files/locales. Reconcile commits by intent;
  do not overwrite sibling changes or repurpose their requirement IDs.


## Management implementation integration, 2026-10-25

Consumed published task01+02 `fea04b3413acf020d63e218af724889d5aa82aaa`,
then final management `45e90466378f982e13cc762c52432f14ab43c32c` (including
neutral evidence and the workspace-profile admission correction). Only assignment
commits after each old management boundary were replayed. No parent or sibling
workspace was changed. Backups preserve the published assignment cut point
`6faaa61b817dc9dddc60d2c12026cc1273748a13` and local preparation:
`backup/clarify-mcp-assignment-published-20261025`,
`backup/clarify-mcp-assignment-pre-management-task02-20261025`, and
`backup/clarify-mcp-assignment-before-profile-admission-20261025`.

Current WO01 is `b1df2c4850cbf5b8179dd96823d30fce328c448b`; WO02 is
`5ab7eade6`. Saved-ID comparison is `fbd32e5ca`, and independent discovery/hint
preparation is `dc06ac4f58100da8f825f8c9b4276c2b2fba9927`. Exclusions must use
its old assignment boundary and retain the identical saved-ID comparison and
neighbor helper fixes once; no whole exclusions dependency was introduced.

Integration preserved management's manual settings draft, legacy status,
profile context and exact-owner route validation, alongside assignment's
conditional snapshots and Reload choices interlock. Transport tests with
fetchJson mocks live separately from real-fetch conditional-write tests. The
usage test's deliberate stale foreign association is inserted directly so the
new guarded writer does not invalidate the fixture setup. Management's settings
draft test now reloads after a rejected save before retrying.

WO03 now uses the committed metadata-only profile owner endpoint, including its
cursor pages, instead of global agent inventory or Office instance identity.
The final browser, backend, localization and documentation results are recorded
in task-03-owner-handoff.md. PR #4760 remains draft. No CI waiter, review handling,
auto-merge or merge is authorized.


## Completed implementation, 2026-10-25

All three work orders are complete. The current implementation supersedes the
historical design-only and dependency-blocked checkpoints above. Registration
can end at Done or continue through Use in to an authorized repository, profile,
or existing task agent session. The management route and profile metadata
prerequisite is integrated; no remaining implementation dependency is open.

Final validation passed: 71 focused frontend tests; the session domain and real
chat consumer (108 files, 1059 tests); full web typecheck; targeted frontend and
Go lint; race-enabled MCP/Selection/Effective/Usage/ImportStatus checks across
config, handlers, backend composition, and the SQLite/PostgreSQL settings store.
The PostgreSQL cases used a disposable PostgreSQL 17 instance. Desktop and phone
each pass all eight assignment and four management integration journeys, with
zero retries. Detailed commands, correction history, and evidence are in WO03.
Localization, public docs, spec catalog/lint and actual-diff documentation
coverage also pass. Browser captures show saved completion, scope review,
repository draft ownership and explicit session Customize on both viewports.

Provider behavior uses the existing mock-provider E2E contract; this does not
certify live third-party endpoints. PR #4760 stays draft. Publication does not
authorize CI waiting, review handling, auto-merge, or merge. Exclusions must
preserve its old assignment boundary `6faaa61b817dc9dddc60d2c12026cc1273748a13`
and consume the verified new remote tip after publication.


## Inline profile integration, 2026-10-25

Management published the user-requested inline profile selector cleanup at
`7aca966a6ec262eb1ce839fa883f66e23441d477`. Backup
`backup/clarify-mcp-assignment-before-inline-profile-20261025` preserves assignment
`fe08307e9f4701715e9fc906b1d65916c18971e1`, the next exclusions replay boundary.
Only the eight assignment commits after old management `45e904663` were replayed.
The sole conflict was profile-card composition: keep management's inline
MCPSelectionContent and workspace-keyed search, attach assignment's existing
focus-only props directly, and remove the obsolete picker opener/reveal prop.
No backend, selection-write, authorization or provider timing contract changes.
Affected validation and fresh profile evidence are recorded in WO03.


## Management squash reconciliation, 2026-10-26

Management PR #4759 merged as `e5c38184cf9c191075dc88c1ffad28c636858bf5`.
GitHub retargeted draft assignment PR #4760 to main. Verified the live merge
receipt and fetched main, preserved old assignment
`f3be6a5186f43dcce4487371c7a493be6c9ec280` in
`backup/clarify-mcp-assignment-before-management-squash-20261026`, and replayed
only the nine task commits after actual integrated management base
`7aca966a6ec262eb1ce839fa883f66e23441d477`. No management or parent commits
were replayed. Exclusions must keep that old assignment tip as its next cut point.

Range-diff shows eight identical assignment patches. WO01 differs only by
moving the newly merged usage-pagination transport regression into the existing
mocked `mcp-usage-api.test.ts` suite, preserving the real-fetch conditional-write
suite independently. Current WO01 is
`84f880ae45c143b3bda7349d47a9b46907937443`. The merged shared draft hook retains
its layout-effect context/draft refs. Management's error/retry guards, deferred
legacy editor and exact-session admission corrections are preserved alongside
assignment's accepted-selection focus readiness and conditional writes.

The new base includes unrelated main changes; these are inherited and absent
from the assignment PR diff. Affected validation is recorded in WO03. Existing
backups and user work are preserved. Keep PR #4760 draft, with no review handling,
CI waiting, auto-merge or merge.

## PR fixup after ready-for-review request (2026-10-26)

The user authorized marking PR #4760 ready, requesting bot reviews, fixing CI,
and dispositioning review comments. No merge is authorized. Historical draft-only
and no-review restrictions are superseded for this fixup. Backed up the published
head at `backup/clarify-mcp-assignment-before-ci-fixup-20261026` before rebasing
assignment-only commits onto current main. Current-head CI/review evidence will
be reported separately after the final push; earlier exact-head claims are historical.

Remediation scope:

- Task edit exposes the existing Reload choices feedback after a rejected MCP
  save. Reload preserves task fields and the MCP draft and performs no PUT;
  another explicit submission retries. Desktop and phone use the same recovery
  action in the existing internally scrolling dialog (no additional overlay).
- The profile draft assertion uses its inline choices; only repositories open
  the collapsed picker. It still proves an in-flight acknowledgment retains a
  newer draft. The task edit regression now exercises reload before retry.
- Session chooser/review detail includes the exact ID, also exposed as the
  choice's accessible description. Destination validation keeps the button name
  stable and announces translated loading status. A deferred catalog response
  proves the checking state on desktop and phone.
- Shared selection editor/draft guards update after committed layout effects;
  a discarded suspended replacement render cannot invalidate an accepted save
  or keep saving/dirty state stuck. Existing context-switch rejection remains.
- Internal session launch writes retain atomic persistence and basic owner,
  workspace and catalog validation without the HTTP settings runtime eligibility
  gate. Settings writes still reject ineligible additions. Non-session admission
  checks remain unchanged. The regression uses separate settings/task stores,
  matching production; an initial same-connection test fixture was stopped
  because it deadlocked its own SQLite writer/read setup.
- CI E2E corrections preserve their assertions: wait for drawer closure before
  reopening port management, wait for finite workflow drawer animations before
  measuring the unchanged 44px target, and keep the manual Stats recovery
  fixture in its failed state with a server Retry-After hint for the test window.
  No production timeout, size threshold or retry masking changed.

Bot findings: Greptile's session identity and live-status threads are actionable;
CodeRabbit's internal launch and stale profile-test threads are actionable.
Its aggregate discarded-render suggestion and default-timeout nit are also
addressed. Claude's backend-only review found no blockers: its suggested
same-set deduplication conflicts with the intentional explicit application Retry
contract, and the existing owner-admission comment already distinguishes the
settings transaction from task-service validation. Codex review was requested,
but its connector replied that the requesting account needs a GitHub connection;
no Codex review result is claimed.

### Local verification for the remediation

- Frontend: 76 tests across eight dialog/assignment suites, plus 30 tests across
  four shared editor/draft/session suites passed. The suspended-render regression
  failed with a null accepted result before the layout-effect fix and passed
  afterward. Full typecheck and affected ESLint passed.
- Backend: the real settings-store/session-service regression rejected the
  internal launch write before the fix. Afterward the MCP/Selection/Effective/
  Usage/ImportStatus race command passed in mcpconfig, settings handlers and
  backendapp. The same regression preserves HTTP rejection, workspace isolation,
  disabled-definition rejection and desired-state advancement. Changed-package
  Go lint passed with zero issues.
- Browser: 14 desktop journeys and 21 phone journeys passed, retries disabled.
  These include the new task-edit recovery, delayed destination check, exact
  session review and all three reported failing E2E scopes. The final binary's
  phone assignment/recovery subset is rerun before publication. Fresh synthetic
  images replace affected evidence. No real endpoint connection was tested.
- Catalog/spec/public-doc checks passed. Incoming main's harness change also
  passed harness and spec validator tests/lint. Normal hook receipts and exact-head
  remote CI/review results are recorded separately with publication.

Commands and logs: focused Vitest results are recorded in
`/tmp/pr4760-fixup-unit-final.log` and `/tmp/pr4760-fixup-guards-final.log`;
backend race in `/tmp/pr4760-review-backend-green.log`; browser runs used managed
host mode, one shard and sequential projects (`/tmp/kandev-run.e2e.YETxqUNT.log`,
`/tmp/kandev-run.e2e.RhOn7ji6.log`). Authoritative base for this remediation is
main `b4e9cf5ede3545ca6a8e58165e580ad7c4988afa`.

Final-binary phone assignment/recovery subset passed all nine journeys, with no
retries (`/tmp/kandev-run.e2e.peMTAxLe.log`). Hook/remote review evidence is recorded
separately for each published remediation head.

## CI synchronization follow-up (2026-10-26)

Historical head `dcc10726bec22c8d1b845ed742031cae833838f8` finished with
85 passed, 13 skipped, 1 neutral, 3 failed checks and no pending checks or
unresolved review threads. Both frontend test shards and the backend gates
passed. The remaining failures were normal E2E shards 3 and 21 and their
aggregate, from three first-attempt failures that passed on CI retry.

The three exact cases passed three local runs before changes. A controlled
250 ms Office PATCH delay then reproduced the persisted-status assertion
reading `created` while the optimistic UI already displayed In Progress.
The assertion now awaits that successful PATCH before reading persistence.
Task mention restoration now explicitly foregrounds the public conversation
surface after reload, instead of assuming dockview hydration has completed.
The CI screenshot showed the correct chips after the assertion timed out.
Capacity cancellation now waits for persisted attempt 6 after advancing five
retry ticks; the previous driver returned immediately after advancing tick 5.
The same barrier covers success/restart journeys. The helper's eleven-attempt
exhaustion behavior is preserved. No product behavior, UI, timeout budget or
assertion threshold changed.

After fixes, all nine exact-case repetitions passed with retries disabled.
Typecheck, affected lint and 53 message/mention/rich-output tests passed.
The original CI process groups 3-3, 21-1 and 21-4 passed locally from the
downloaded manifest, sequentially with one worker and zero retries: 137 passed
and two skipped. The four phone capacity scenarios also passed. These results
and the new head's remote checks remain delivery evidence in the PR, not a
claim that the failed historical head was green.

Main advanced to `7fb28fc022d5320922608bf57cc9fd9351a8b8da`. The old published
assignment tip is preserved at
`backup/clarify-mcp-assignment-before-e2e-followup-20261026`; only assignment
commits were rebased. Incoming rich-output parsing changes have focused test
coverage and are not fixes for these E2E races. This follow-up changes tests
and this plan only. Durable requirements/design and public behavior need no
additional update. Existing MCP screenshots remain valid because the rendered
feature source did not change. Normal hook receipts and exact-head CI/review
results are recorded with publication. No merge is authorized.

## Fresh review follow-up (2026-10-26)

CodeRabbit reviewed the published synchronization follow-up and reported two
minor findings, a public-doc paragraph nit, and an unverified local-path nit.
The task-edit browser helper now deletes its workspace MCP definition in a
`finally` block. A temporary fault-injection probe made task creation fail
after definition creation. It proved the leak before the fix and cleanup after
the fix; the probe was removed after verification. The normal desktop recovery
journey also passed. The helper's assertions and product behavior are unchanged.
The phone recovery journey, full web typecheck and affected ESLint passed too.
Catalog/spec validation and both public-doc validators passed. This is fixture
cleanup and editorial clarification; no new product behavior or UI capture is
required. Normal hooks and exact-head CI are recorded with publication.

The system design now dates its former management proposal and describes the
completed work orders retrospectively. The public reference splits stale-save,
editor-save and application-retry guidance into separate paragraphs. The plan
removes its machine-specific workspace path and distinguishes the initial
draft-only checkpoint from the later ready/fixup authorization. Historical
temporary log paths in this ledger identify local verification artifacts;
reproduction uses the recorded commands rather than those ephemeral files.

At historical head `70f87d3ccd4b298a9ce2b830fb88373f2c2717db`, CI finished
with 86 passed, 13 skipped, one neutral, two failed and zero pending checks.
The walkthrough model deadline and Windows cold-cache compilation timeout both
passed on one unchanged-head retry. The remaining leaf failure was a branch
picker click in normal shard 11; the case passed its CI retry and three local
repetitions with the suite retry override disabled. No branch-picker code or
assertion was changed. The failed E2E jobs were requested once more at that
same head. Their outcome and the next published head's checks are delivery
evidence, not an assertion that the initial run passed.

## Main integration and selector diagnostics (2026-10-26)

Main advanced again to `6a1a2ae10003c6bccc754ab144c6035ac38158d1`, containing
attached-repository worktree placement. The task's 13 commits were replayed
without conflicts, and every range-diff entry was unchanged. Backup
`backup/clarify-mcp-assignment-before-review-rebase-20261026` preserves the old
cleanup tip. Conditional-write WO01 maps from `523de156803f9db4ff2711579e2d9e59aea2e1ce`
to `d1cd10ec24cf753cc513860b7153aee122850ac4`. The published cut point for
dependent branches remains `70f87d3ccd4b298a9ce2b830fb88373f2c2717db` until
this task's replacement head is published. The selection protocol is unchanged.

A fresh managed build passed all eight desktop branch-split and task-recovery
cases with suite retries disabled. After an interrupted test process, the
remaining backendapp and worktree race scopes were rerun to completion. Both
passed, including all three persisted-attachment placement/rollback/retry
tests. MCP configuration and settings-handler race scopes had already passed
on the same integrated source. PostgreSQL-only placement cases were skipped
without a configured PostgreSQL test database.

The old-head shard-11 retry failed in a different selector case: choosing an
executor profile before task submission. Both CI logs show an option detached
while Playwright waited for it to become stable. Six further exact-case runs
passed locally. Controlled opening/closing animation variations also passed;
those temporary probes were removed. No selector behavior, timeout or
assertion was changed. This file now retains first-attempt failure traces:
the previous artifact contained only a successful retry trace. The original
14-file CI process group completed with 34 passes, including every selector
case, and two setup failures before assertions (readiness timeout and seed
workspace 404). The desktop setup case passed in isolation on an explicit
port block. The phone focus replay exposed missing native Chromium libraries;
after adding already-installed Nix libraries to that process's library path,
it passed. Phone MCP recovery also passed on the integrated build. These local
environment adjustments change no repository dependencies or product code.
Fresh CI on the replacement head remains a delivery gate.

## File-search fixture follow-up (2026-10-26)

The next CI leaf failure was the collapsed-folder file-search scenario in
normal shard 3. Its screenshot showed only the baseline repository file.
The search fixture committed its files on the shared checkout's current
branch, but task creation selected `main`. A preceding PR-watcher scenario
can leave that checkout on another branch. An isolated worktree-executor
probe passed and did not establish the initial remote-publication hypothesis.
A temporary non-main checkout reproduced the exact missing-folder assertion;
restoring the existing immutable fixture baseline before seeding made that
same probe pass. Both probes were removed after verification.

The two file-search scenarios that seed files now call the existing
`resetSeedRepositoryCheckout` helper before their writes. Search assertions,
application code, executor defaults and timeout budgets are unchanged. The
original 15-file CI process group passed all 48 desktop/phone checks with
one worker and zero retries on a fresh managed build. Affected ESLint passed.
This is test setup only; durable feature requirements, system design, public
behavior and the existing MCP screenshots need no change.

Main advanced to `fb4c6f19c00facd063b998008193dba2600c5a89`, fixing removed
worktree cache aliases. The 14 assignment commits were replayed without
conflicts; every range-diff entry was unchanged. Backup
`backup/clarify-mcp-assignment-before-file-search-ci-20261026` preserves the old
published `d1f433a1154f0c006c7a8c423b4b516cad1fc6ff` dependent-branch boundary.
Conditional-write WO01 maps to `608362f6d` with the same selection protocol.
The next published head requires its own CI and automated-review evidence;
local reproduction does not waive the historical failed remote check.

The second remote leaf failure was a phone workflow-override preview expecting
the profile default after an actual session binding existed. The existing
workflow-agent-overrides design requires the bound session's effective model,
which can differ from that profile default. A controlled explicit model change
reproduced the old assertion failure with a correctly reused session. The
fixture now deliberately selects `mock-smart` on the replacement session,
checks the exact bound session/profile IDs, and verifies that effective model
on the tablet and phone disclosures. Planned new-session previews still check
the profile's `mock-slow` model. Three zero-retry repetitions passed.

The second original 14-file process group completed with 38 passes, one skip
and one local layout failure; the reported workflow case passed. The extra
failure measured four lines instead of three for the long coordinator
containment notice at 390px. It reproduced alone with Figtree fully loaded,
so a font-readiness hypothesis was rejected. No coordinator source or test
change remains. This is a recorded local validation limit, not a claim that
this entire replay passed; its exact case passed the remote run being
investigated. Temporary layout probes were removed.

Race-enabled backendapp/worktree integration scopes passed on the new main,
including live attachment retries and removed-worktree cache alias cases.
Orchestrator preview model/session/override race tests also passed. Full web
typecheck, affected ESLint and catalog/spec validation passed. The feature
contract and rendered MCP UI are unchanged; normal hooks and the replacement
head's exact CI/review result remain publication evidence.


## Main conflict reconciliation (2026-10-26)

Main advanced to `5a0e976d4842a5344cd5d099618898c7769e2634`, including
Quick Chat draft restoration and Codex throttle recovery. Backup
`backup/clarify-mcp-assignment-before-main-5a0-20261026` preserves the old
published assignment head `2a0c8500e9a780a3b53b7390053453c9d47fdf8c` for the
dependent exclusions rebase. Only the 15 assignment commits after the previous
base `fb4c6f19c00facd063b998008193dba2600c5a89` were replayed onto this main.
Conditional-write WO01 now maps from `608362f6d` to `ea7885cd9`, with an
unchanged API, locking and retry contract.

The sole conflict was the mobile Stats recovery fixture. It keeps upstream's
300-second server backoff and our assertion that no automatic second request
occurred before the explicit Retry action. This changes no locator timeout or
production behavior. The range-diff is otherwise unchanged. Existing MCP UI
captures remain valid; no rendered MCP code changed in this reconciliation.

The focused frontend suite passed 100 tests in nine files, including Tiptap
draft restoration and MCP selection, registration, target and focus hooks.
Full web typecheck, affected ESLint, catalog/spec and public-doc validation
passed. Race-enabled Codex classification, mock-provider and orchestrator
retry scopes passed. A fresh managed build passed all 15 focused phone checks
with one worker and zero retries, including the resolved Stats scenario,
Quick Chat/Codex recovery, MCP assignment/save recovery and bound-session
workflow preview. The desktop
follow-up passed all 20 checks with one worker and zero retries, including
file search, MCP assignment/save recovery, Quick Chat draft recovery and Codex
retry/cancellation. Normal commit hooks are required for this ledger commit.
The replacement head requires fresh CI and bot review evidence; the old head's
queued aggregate is not a passing result.


## Session-restart fixture race (2026-10-26)

At published head `d4f28bab5`, E2E shard 18 rejected a retried background-activity
restart scenario. The initial session-list barrier did not pause the separate
session-detail reconciler. Under a two-CPU limit, the scenario reproduced its
missing background-status assertion once in six zero-retry repetitions. A
temporary store-transition trace identified `session-state-reconciler.ts`
applying an authoritative detail snapshot through `setTaskSession`, correctly
clearing the injected stale activity while the list request remained held.
The production reconciliation behavior is correct; all diagnostic probes were
removed after this evidence was captured.

The fixture now pauses detail reads before reload. It holds those responses
while releasing only the explicit foreground session-list refresh, so that
independent snapshot must clear the stale status before detail reconciliation
can resume. Cleanup releases both gates and drains their handlers before
removing the routes. The corrected scenario passed six zero-retry repetitions
under the same two-CPU limit. No assertion, timeout or production behavior was
weakened. The existing background-work-liveness requirement and design remain
authoritative, and no public documentation change is needed for this fixture.

Main subsequently added the new-session MCP picker and merged-task file links
at `baa92e5dcf53eca4dac223b0deba5e26c1ee08fb`. Backup
`backup/clarify-mcp-assignment-before-main-baa-20261026` preserves the published
`d4f28bab55fe56641386cfa2c0cd3bba9b07ffdd` dependent-branch boundary. The 16
assignment commits after `5a0e976d` were replayed onto this main with every
range-diff entry unchanged. The owned fixture patch was preserved separately
and reapplied. Conditional-write WO01 now maps from `ea7885cd9` to `63a7e82fb`.
Incoming new-session, file-link and MCP consumer unit suites passed 78 tests;
full typecheck and affected ESLint passed. The exact 12-file CI process replay
uses a fresh managed build, two CPUs, one worker, both original projects and
zero retries; existing describe-level retry overrides are disabled only during
that verification and restored afterward. All 28 checks in that process
passed. Additional new-session and MCP assignment integration passed all 24
desktop/phone checks with one worker and zero retries on the same build.

The same published head's walkthrough generator hit its 600-second deadline
with zero model stdout and verification not run on both the original attempt
and one failed-job retry. Setup, trusted context and artifact upload passed.
No further retry, provider, timeout or required-check policy change was made.
The exact job and diagnostic evidence is recorded in the PR comment at
<https://github.com/kdlbs/kandev/pull/4760#issuecomment-6429317057>.

A temporary assertion failure with both HTTP reads held verified the failure
cleanup: all held handlers drained, no secondary route-handling error occurred,
and the exact source was restored. The previous head's final CI result was
82 passed, three failed and zero pending. Those failures were the diagnosed
restart-test flake, its dependent E2E aggregate, and the separately evidenced
walkthrough model deadline. Fresh CI must validate the corrective commit;
local green results do not replace that gate.


## Pagination archive readiness (2026-10-27)

At published head `e14cef820`, E2E shard 27's phone sidebar pagination case
received a 500 from a direct fixture unarchive call. The backend log identified
an archive cleanup job already in `running` state. The exact CI merge tree
`edee6366` matches this head's source tree. The unmodified phone spec passed
nine local checks across three repetitions; that does not invalidate the CI
failure or its lifecycle evidence.

Current service coverage intentionally rejects an unarchive that races claimed
cleanup. Both relevant refusal tests passed with the race detector. Newer
archive join/fence design documents are draft proposals, so this correction
does not implement or assume that different production policy. The pagination
fixture does not test immediate Undo. It now uses the existing durable
`waitForCascadeArchiveCleanup` observer before unarchiving either task, through
one shared fixture helper used by desktop and phone. It makes one unarchive
write after cleanup succeeds, with no mutation retry or timeout increase.
All 101-task, pagination, navigation and active-conversation assertions remain.
No public product behavior or documentation changes are needed.

The corrected desktop and phone cases passed three repetitions each under two
CPUs, one worker and zero retries. Full web typecheck and affected ESLint
passed. The original 13-file CI process is replayed with both original
projects, one worker, two CPUs and zero retries. Any describe retry overrides
are temporary verification changes and must be restored before committing.

The original 13-file process passed all 39 checks under those limits, including
the reported phone case and neighbouring MCP selection/recovery scenarios.
The temporary describe override was restored. This fixture-only correction
preserves all existing pagination and conversation assertions and requires a
new exact-head CI result after publication.

### Office session fixture isolation (2026-10-27)

The current CI run left the seeded original session RUNNING after reassignment.
The unmodified isolated case, full lifecycle file, and original 15-file process
passed locally. The process replay completed 54 checks under two CPUs, one
worker and zero retries. Its diagnostic snapshots nevertheless showed a real
assignment wake creating another session for the old agent after the seeded
row completed. The test was mixing a synthetic RUNNING row with normal
executor admission.

The row-lifecycle case now pauses the original agent before task creation and
the initial assignment, verifies that assignment has created no session, seeds
the artificial row, and reassigns through the same real PATCH. It preserves
the exact original-session terminal assertion and restores the agent to idle
in finally. It does not change Office production termination, capacity guards,
or runtime behavior. The corrected full lifecycle file passed three
repetitions (12 checks), including all three reassignment cases, after a fresh
managed build. Affected ESLint and web typecheck passed. Exact-head CI remains
pending publication of the collected fixes.

### Review touch-scroll admission (2026-10-27)

The original review process passed all 33 checks locally. A separate one-CPU
baseline reproduced CI's first-pass checkbox failure. Native event diagnostics
showed the scroller moving from 0 to 185 without any touchmove delivery.
The interaction hook only released suppression for touchmove or a pointer-down
whose target was the root itself. A gesture starting on diff content could
therefore consume its crossing while suppression remained set. The baseline
was stopped after retaining the matching failure and its event evidence.

The regression first failed with no review callback; all 13 auto-mark unit
cases now pass. Touch pointer-down on a descendant authorizes the next real
scroll crossing. Contact alone does not mark a file, and descendant mouse/pen
clicks remain suppressed. The existing geometry, layout-shift, stale, manual
uncheck, observer cleanup and navigation guards remain in place. The existing
review-viewed-toggling design records this event contract; no new public
behavior or API is introduced. Public sessions-and-review already promises
automatic marking after manual scrolling.

Desktop keeps the review sidebar checkbox, and phone keeps the file-header
checkbox in the same full-height review surface with one internal scroll
owner. No layout, target size, copy or saved preference changed. After a fresh
managed build, the full Office and desktop/phone review cases passed three
repetitions each (18 checks, two CPUs, one worker, zero retries). The corrected
phone review case then passed six more repetitions on one CPU without retries.
Web typecheck, affected ESLint, documentation catalog and specification lint
passed. A new exact-head CI result remains required after publication.

### Phone task-switch fixture readiness (2026-10-27)

CI's oversized-message case closed the Tasks picker but remained on the
original task URL. The original full case passed locally, and its original
13-file process passed all 35 checks on one CPU with no retries. Ten further
diagnostic repetitions through the first switch also passed. Their pointer
and history records selected the requested row and committed the matching
URL. Those ten prefix runs do not verify the later message scenarios, and the
exact failing gesture was not reproduced locally.

The fixture previously selected a newly starting target while the picker
could still be animating. It now waits for that exact target session to finish
starting before opening Tasks, waits for finite picker animations, and checks
the row's task ID before the same touch action. Both original URL assertions
and all content, download, persistence, queue, steering and failure checks
remain. No production navigation behavior, locator timeout or retry count
changed. The spec retains a trace from its first failure to preserve input
evidence if the intermittent gesture recurs.

The corrected complete case passed three repetitions on one CPU, one worker
and zero retries. Affected ESLint and full web typecheck passed. These controls
address the concrete fixture transition races; they are not presented as a
captured reproduction of the original failed tap. Fresh CI remains the
delivery gate for this case and the other three leaf failures.

### Main reconciliation and collected CI corrections (2026-10-27)

The authoritative main advanced to
53bffc00e34b0eeb1bd3384c565a1e4d132caeb4 with the exact-label-removal fix.
Its 13 changed paths do not overlap the assignment branch. Backup
backup/clarify-mcp-assignment-before-main-53b-20261027 preserves the complete
pre-rebase tip e99386cb289fedc04f1da29623fa8241866e4900.
Only the 21 task-owned commits after
baa92e5dcf53eca4dac223b0deba5e26c1ee08fb were replayed. No conflicts occurred,
and range-diff reports every patch unchanged. The WO01 conditional-write
commit maps from 63a7e82fbf1252bc3d267b0801cc50bcac0baed9 to
f88fae0147f285b9526bb3f91eb402cc1c8795b7. The old published assignment head
e14cef8206cf526654ff16f66569686694ded3d6 remains the prior remote boundary;
a dependent task must preserve its actual integrated boundary when rebasing.

Fresh integration verification passed 77 focused frontend cases across nine
files, including review scrolling, MCP selection/API/new-session consumers and
incoming label-removal consumers. Race-enabled label handler and backend
composition tests passed. Web typecheck, documentation catalog and full
specification lint passed. A fresh managed build then passed all 13 cases in
the complete desktop/phone pagination, Office lifecycle, review-viewed and
phone oversized-message files with two CPUs, one worker and zero retries.

The preceding published head's terminal CI result was 83 passed, 11 skipped,
one neutral and five failed checks, with no pending checks or unresolved
review threads. Four E2E leaf failures caused the E2E aggregate failure.
Their corrections and reproduction limits are recorded above. Core assignment
rendering is unchanged; its 18 published synthetic captures remain valid.
Fresh desktop/phone review captures verify retained manual clearing after the
touch-input correction. The relevant rendering sources are unchanged by this
rebase. Publication must start fresh exact-head CI and bot review; these local
results do not mark that external delivery gate complete.

### Further CI fixture transitions (2026-10-27)

The next exact-head run at 33d0c75e46e550732bf25e048f48871f917c0f7d
finished with 82 passed, 14 skipped, one neutral and four failed checks.
There were no pending checks, unresolved or hidden review threads, or
actionable issue comments. Three failed E2E shards contained five failing
cases; the fourth failure was their dependent aggregate. Passing retries did
not clear those failures.

The corrections retain the original application assertions:

- Fresh-branch creation waits for the repository popup to unmount and for the
  selected repository label before it opens the executor picker. The failed
  trace showed the first popup still closing and the executor popup already
  closed before its option click. Three unmodified local repetitions and
  separate focus/animation probes passed; the exact failed focus sequence was
  not reproduced. The change establishes fixture transition readiness without
  changing production focus behavior or retrying the opener.
- The accepted-silent-prompt test uses the existing mock provider's file gates
  instead of ten-second and thirty-second delays. Output starts only after
  the real watchdog notice is visible, and the turn stays open through its
  output, reload and running-state assertions. Both markers are released in
  cleanup. CI's failed trace and snapshot already contained the output when
  the test first expected the notice, demonstrating that the timed output
  window had expired during setup.
- Returning to a published canvas uses the existing positive-width Dockview
  readiness helper before the original layout and exact-panel assertions.
  Three unmodified local repetitions passed; the original cold-loading delay
  was not reproduced. The spec now retains the first failed attempt's trace.
- Parked-work setup waits for the exact session's authoritative parked
  projection after its foreground turn settles. The production settle path
  persists the waiting state before it samples and publishes the background
  projection. The existing three-sample UI assertions, negative idle-icon
  assertions and WebSocket-only settle-to-clear action remain intact. The
  spec retains a first-failure trace; the initial CI failure had only a
  screenshot and no captured first-sample projection state.
- The phone plugin-modal test uses its existing touch-dimension helper for
  both dimensions in both parent surfaces. That helper allows only 0.01 CSS
  pixels of floating-point noise; CI reported 43.99993896484375 for a 44-pixel
  target. The original keyboard activation, modal close and parent-focus
  assertions remain. A first-failure trace is retained.

No production behavior, user-visible copy, screenshot composition, selector
write contract, locator timeout or CI fail-on-flaky setting changes in this
batch. Public documentation and the published UI captures still describe the
same behavior. This is test-fixture verification work; no new product design
or implementation scope is introduced.

Local verification passed 200 checks across seven managed runs. The full
picker/canvas specs passed 19, and all five desktop stall checks passed. The
original canvas process passed 37 with one existing skip, the phone-stall
process passed 31, the branch-creation process passed 37, the parked-work
process passed 36, and the phone plugin process passed 35. Each run used two
CPUs, one worker and zero retries. An existing describe-level retry in the
repository-script neighbor was temporarily set to zero for its replay and
restored immediately afterward; no retry-policy change is committed.
Affected ESLint, web typecheck, documentation catalog, full specification
lint and whitespace checks passed. All temporary focus probes were restored.
Fresh exact-head CI and bot review remain external delivery gates.

### Sidebar geometry precision (2026-10-27)

CodeRabbit completed its review of published head
7440148ef0345a6324cb74a6879ab7491505e2e4 with no actionable comments and
minimal reported merge risk. The current-head snapshot had no unresolved or
hidden review threads. Its docstring-analysis and optional description notes
are informational; they do not establish complete coverage. The separately
requested Claude run ended with two permission denials and no review report,
so workflow success is not counted as a completed review.

A later failed shard at that head reported the coarse-pointer sidebar action
height as 43.9999 CSS pixels after four-decimal rounding, against a 44-pixel
minimum. The existing fixture already accounts for drift between transformed
DOMRect endpoints, but its rounding precision was too fine for that observed
value. Both height checks now round to three decimal places. Width, locale,
breakpoint, containment, navigation and action assertions remain unchanged.
The spec retains its first failed attempt's trace. No application code, target
size or retry setting changes.

The first local baseline stopped at a different assertion because the Nix
host had no Japanese font and rendered missing-glyph boxes. The width check
was preserved. A task-local Fontconfig file loads Noto Sans CJK from the Nix
store for these isolated browser processes; it does not change the system
font configuration or another runtime. The unmodified full spec then passed
both cases with that prerequisite present. The exact CI height drift was not
reproduced locally. The corrected desktop/coarse-pointer spec and neighboring
phone overflow spec passed all three cases with two CPUs, one worker and no
retries. The original 49-case CI process then passed in full under the same
CPU, worker, font and retry settings. Affected ESLint, documentation catalog
and whitespace checks passed. The application's touch control remains 44 CSS
pixels, and a 43.99-pixel measured height still fails this assertion. Public
documentation and UI captures require no change. Fresh exact-head CI remains
pending; the current published run is still collecting its complete result.

### Remaining CI fixture contracts (2026-10-27)

The published 7440148ef0345a6324cb74a6879ab7491505e2e4 run finished with
83 passed, 12 skipped, five failed and no pending checks. Four E2E shards
contained six failed cases; their aggregate was the fifth failed check.
The sidebar precision correction is recorded above. The remaining fixtures
now establish their own identity, timing and transition preconditions:

- Profile deletion uses the ready seed profile's stable ID through the existing
  API helper. The list-row target uses its profile-specific action ID. New
  disposable profiles inherit that seed profile's agent and model. A registered
  agent family may legitimately have no profiles and may appear first in the
  list; array position does not establish a test owner.
- Restart response waits use the existing restart client's 30-second launch
  budget instead of the WebSocket helper's unrelated 15-second default. The
  live-sibling cases correlate Stop and Resume to the exact target, require
  Resume intent, and arm after the context menu is ready. Existing draft,
  sibling, history, workspace and readiness assertions remain.
- Palette input, scope-strip and dialog rectangles are read in one synchronous
  browser evaluation. Separate protocol calls could sample a moving surface
  at different positions. The strict below-field and width inequalities remain
  unchanged; no extra overlap tolerance is introduced.
- Both branch regression files share a repository-popup exit wait before the
  executor picker opens. The branch-refresh failure's first trace again showed
  the executor popup already closed immediately after its trigger click.
  Existing selected-repository, branch, refresh-request and focus assertions
  remain. The helper observes closure and does not repeat a click.

The four newly reported cases passed an unmodified local baseline; their exact
CI timing failures were not reproduced. The changes enforce observed fixture
contracts rather than claiming a captured production focus bug. First-failure
traces are retained for the affected files. The complete four-spec check passed
23 cases. After extracting the shared close wait, both full branch suites
passed 20 cases. All runs used two CPUs, one worker and zero retries.
Original process 2-2 passed 33 cases with one existing skip, and process 22-2
passed 30 cases with two existing skips. Process 22-4 passed 36 cases but
exposed a neighboring launch-recovery fixture defect: changing the origin to
an unresolved remote retained the old origin/HEAD cache. The worktree manager
correctly used that cached default, so the test no longer exercised its claimed
missing-base condition. The fixture now deletes only that cached symbolic ref;
both local and remote-tracking main branches remain. A real-Git regression
failed against the no-op implementation and passed with the correction. All
four Git fixture unit tests pass, including the existing origin-restore suite.
The interrupted follow-up run has no terminal result and is not counted as a
pass. A fresh complete desktop/phone launch-recovery run passed all seven
cases with one worker and no retries. Existing describe-level retry overrides
were restored. Application code,
public documentation and published UI rendering are unchanged.

The user requested current main's E2E policy. Verified main
9c7359d3aa9ea3920d01fca2fc0e2794e4a900c0 removes E2E_FAIL_ON_FLAKY from
both browser and container CI jobs. This upstream policy will be integrated
with its fixture corrections; the assignment branch does not introduce a
separate bypass or change application behavior.

### Current-main reconciliation (2026-10-27)

The user requested the upstream E2E flaky policy and continued CI/review fixup.
Saved backup `backup/clarify-mcp-assignment-before-main-bd761-20261027` preserves
local f950827a10189e71e8104a76dd6db1230e5cf7f4. The incoming main is
bd7612fa9cba3989495677bbc980e41967564fd9. It contains the upstream removal of
E2E_FAIL_ON_FLAKY from browser and container CI, plus newer review-input and
fixture corrections. A merge preserves the assignment commit identities and
published 7440148ef0345a6324cb74a6879ab7491505e2e4 as the downstream boundary.

Conflicts were limited to browser fixtures. Reconciliation retains upstream
last-retry event coordination, touch-target helper reuse, animation and picker
focus checks, and canonical seed-profile ownership. It also retains the
assignment's persisted retry-state read, explicit silent-output/finish gates,
Dockview readiness, exact task-row identity, archive-cleanup wait, and
repository-popup closure wait. Profile rows use the exact action ID. Application
selection, assignment, save, route-hint and authorization contracts had no
textual merge conflict. The integrated tree passed 120 focused frontend
cases, full web typecheck, affected E2E ESLint, documentation catalog validation
and whitespace checks. Race-enabled MCP/Selection/Effective checks passed in
mcpconfig, settings store, settings handlers and backend app composition.
The merge commit is ef86ab7fb687c06707dfac106d7020a0577c0598. Its normal
23 hook results passed or were correctly skipped. The E2E workflow contract
suite passed all 27 cases. A fresh managed runtime/web/plugin build completed.

At that merge head, the strict desktop/phone browser check passed 32 cases,
including all 16 assignment journeys. One neighboring phone launch-recovery
case failed while waiting for its launch error to clear after the accepted
repository-row repair. Its trace and snapshot were preserved. The same case
passed in an isolated diagnostic run, then again with the original four phone
predecessor tests (five passed). Those runs used no retries and do not establish
a root cause or a fix for the intermittent result. Temporary log-capture probes
were restored byte-for-byte, and no recovery assertion was changed. The failed
33-case run remains failed evidence; it is not reported as a clean suite.

A separate conflict-focused desktop/phone check passed all 10 cases: tablet
workflow move, capacity retry/cancel/restart, 101-task pagination and oversized
message navigation. These runs used two CPUs, one worker and zero retries.
The local retry overrides were restored before commit. No new UI captures are
needed for the unchanged assignment and review UI. The following documentation
receipt changes no application, workflow or test code; its code tree matches
the tested merge head. Fresh exact-head CI and requested bot review remain
external delivery gates, with all reproduction limits retained in the PR body.

### Stats retry review correction (2026-10-27)

CodeRabbit's exact-head review of ac7f350edef7f0238e903c6604b16ca032880ad8
reported one actionable finding (comment 4392652995, thread
PRRT_kwDOQ2-eWs6w1EGk). The automatic merge retained the old one-request
assertion beside upstream's new virtual-clock assertion that an automatic
retry increases the request count. These assertions are contradictory. The
older no-second-request description in the 2026-10-26 reconciliation section
is historical; upstream's newer explicit automatic-retry coverage supersedes
that fixture condition.

The unmodified phone test reproduced the defect with Expected 1 / Received 2.
The correction removes only the obsolete count assertion and explicitly checks
that Retry remains visible after the automatic retry. The 300-second backoff,
automatic retry poll, 44-pixel target, enabled manual action, successful
recovery, Copy Stats and overflow checks remain. Both complete desktop and
phone Stats recovery cases then passed with two CPUs, one worker and zero
retries. This is a test-only correction with no public documentation or UI
capture impact. It will be published with the collected CI fix batch; current
ac7f350ed CI remains in progress, and the remote thread is not yet resolved.

### Membership ordering review correction (2026-10-27)

Claude posted a partial backend review at issue comment 6450588132. Its run
40559059721 used trusted main dad660099 and completed with one permission
denial; a posted report exists despite the lack of buffered inline comments.
It reviewed only the backend selection path and explicitly did not cover the
frontend or tests. It found no blockers but raised a low-confidence database
collation concern. This supersedes the earlier outstanding-request note; it
is not counted as a full current-head review.

The concern is valid. checkMCPSelectionCondition compared Go-sorted client IDs
against the database-ordered snapshot. Both a focused unit regression and an
isolated PostgreSQL 17 numeric-ICU regression failed before the correction.
The PostgreSQL case uses ordinary UUIDs, confirms the database returns a
different order, and reproduces a false conflict for unchanged membership.
The existing design already requires normalized set comparison rather than
order comparison; no new API or product policy is introduced.

The comparator now sorts a clone of the saved IDs before checking membership.
It preserves the original snapshot and rejects missing or different members.
No SQL query, schema, lock, revision, notification or legacy-omission behavior
changes. The system design and WO01 now state the collation invariant. The
focused race regressions pass, including real PostgreSQL without an ICU skip.
The broader MCP/Selection/Effective/Usage/ImportStatus race command passed in
mcpconfig, settings store, settings handlers and backend app composition with
PostgreSQL enabled. SQL guard passed, and the race-enabled persistence
conformance suite passed with PostgreSQL enabled. Normal hooks and immutable
publication remain in progress. Public docs and UI captures have no impact
because the existing membership contract and rendered UI are unchanged.

### Application-update notification localization (2026-10-27)

The subsequent exact-head CI run completed with 84 passed, 14 skipped, two
failed checks and no pending checks. The failures are E2E shard 25/28 and its
dependent final gate in run 40568606681, attempt 1. All 21 failing browser
cases report the same application-update title/body rendered in English under
the pseudo locale. These are deterministic localization failures, not the
strict flaky-test flag, which remains removed as on main.

The notification hook preferred backend provider text over existing localized
application-update messages. Two new hook regressions failed on the actual
English payload in Portuguese and pseudo before the two-line correction.
The correction uses the existing translation keys and payload version for
in-app and browser/native delivery. Occurrence identity, provider transport,
runtime notifications, subscriptions and deduplication are unchanged. This
restores AC-PLATFORM-I18N-001.1; no new product policy or API is introduced.

The hook, bridge and WebSocket notification suites pass all 57 cases. A fresh
managed production build passed all 22 pseudo-locale coverage cases without
retries. The first new desktop probe found an entrance-animation measurement
in its viewport assertion; it now uses the existing finite-animation helper
before measuring, with no timeout or geometry relaxation. The corrected
desktop and phone cases each pass without retries. They inject the real wire
shape and assert localized text and viewport containment. Affected ESLint,
i18n checks, full typecheck with CI's 8192 MB Node heap, documentation catalog
and full specification lint pass.

Commands and logs are retained under `/tmp/pr4760-update-copy-*`; the initial
22-pass/one-probe-failure browser log remains recorded separately from the
corrected cases. Post-commit checks, UI evidence publication, normal hooks and
fresh remote CI remain pending. The i18n design and
engineering guide now document frontend ownership of this copy. Public
workflow documentation is unchanged because the existing language and
notification controls retain their behavior.

### Release-notice fixture admission review (2026-10-27)

CodeRabbit reviewed the published notification correction and noted that its
new locale-setting reload reused socket readiness from the first navigation.
The release-notice fixture now sets its context locale cookie before the first
navigation and before installing the socket fixture. Its only page connection
therefore owns the readiness used to emit the notice. No shared socket helper,
production behavior, selector, assertion, retry or timeout changes.

Desktop and phone focused checks each pass without retries; full typecheck
passes with CI's 8192 MB Node heap. Normal hooks and publication remain in
progress. Earlier committed-head 57 notification and 24 browser cases remain
historical evidence. The new Claude request completed eight turns with three
permission denials and no buffered inline comments or posted report. It is
incomplete review coverage, not a clear current-head review. The earlier partial
backend report and its fixed collation concern remain recorded separately.

### Landed notification contract reconciliation (2026-10-27)

During final fixture verification, main advanced to 9a6a14314 through merged
PR #4863. It independently lands versioned application-update localization and
explicitly retains provider copy for legacy events without a version. This
landed contract supersedes the unconditional application-notice translation in
the preceding correction. Main also adds deleted-session read cancellation.

Backed up local fixture tip dc470fc33 at
`backup/clarify-mcp-before-localized-notice-main-20261027` and merged current
main into the assignment branch. Resolved the hook with main's version-aware
behavior, retained its pseudo/native and versionless regressions, and kept the
distinct Portuguese full-payload regression. Removed the duplicated local
pseudo case. Main's deterministic pseudo-locale WebSocket case and initial
locale-cookie setup are retained. The landed notification design owns the
contract; removed the duplicate note from the general i18n design and linked
that authority from the engineering guide. Own assignment/CAS behavior and
initial-cookie desktop/phone fixture correction are preserved.

Affected verification passed: 236 frontend cases, a final 58-case notification
rerun after separating the Portuguese test group, full typecheck with CI's
8192 MB heap, the backend handler package under the race detector, affected
ESLint, i18n, documentation catalog and full specification lint. All 28 staged
incoming Go files are gofmt clean. Only the two tested Git-handler files differ
from the previously PostgreSQL-validated backend tree.

The first fresh-build desktop batch passed 33 cases and found one selector
collision in main's new pseudo-notification test: both the injected and real
release shared a title. Preserved the failure under
`/tmp/pr4760-main-pseudo-selector-red`. Scope that test to the injected version,
assert exactly one toast, and separately assert the localized title and body.
The corrected desktop batch passed all 34 cases, including all 23 pseudo
cases, eight assignment journeys, the Portuguese notice and both deleted-task
cases. The phone batch passed nine cases, including all eight assignment
journeys. All browser runs used one worker and zero retries.

The older published notification head completed CI with 86 passed, 15 skipped,
zero neutral, failed or pending checks; its all-terminal helper exit 1 records
the main-branch merge conflict, not a test failure. That result remains
historical. Normal merge hooks, immutable publication and fresh exact-head
CI/review remain pending for the reconciliation.

### Main conflict reconciliation (2026-10-28)

The user requested conflict resolution for ready PR #4760. Verified a clean
owned checkout and matching local, remote and PR head
`74069f8b52326eb09180846b6fd3f42503017413`. Preserved it at
`backup/clarify-mcp-before-conflicts-20261028`, then merged verified main
`a5fdd9b12f54ee5df03c20699f61418bfac3de9a`.

Resolved three E2E conflicts. Session resume uses main's initial-detail drain,
subsequent detail hold and awaited route cleanup. Mobile profile deletion uses
main's canonical seeded-profile helper and retains exact profile-ID row
selection with uniqueness. The pseudo notification test keeps main's second
injected release and complete localized body assertion, plus this branch's
version-scoped uniqueness and separate title assertion. The canvas and public
documentation automatic merges preserve both sides. No production path needed
manual reconciliation. Trimmed one incoming plan's extra trailing blank line
reported by the staged whitespace check. Main's CI flaky-flag removal is kept.

Reconciled-source verification passed: 236 frontend cases across 28 files,
full web typecheck (8192 MB heap), affected ESLint (90 files), scoped Go lint
(four packages), five MCP/selection/profile backend race-test scopes, i18n,
documentation catalog and full specification lint. Go module tidy made no
additional changes and module verification passed; 242 incoming Go files are
gofmt clean. A fresh managed production build passed 13 desktop browser cases;
the same fresh artifacts passed 12 phone cases. Both runs used one worker and
zero retries, covering all 16 assignment journeys, all three phone profile
removal cases, the conflicted resume/pseudo cases, both overlapping canvas
cases and Portuguese notice delivery on both viewports.

Main advanced during verification to
`fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58` with a disjoint Sentry short-ID fix.
Its exact merge-result compatibility, normal hooks, immutable publication and
new-head hosted checks are tracked in the PR fixup receipt. Earlier green CI
and bot reports are historical and do not clear the new merge head.

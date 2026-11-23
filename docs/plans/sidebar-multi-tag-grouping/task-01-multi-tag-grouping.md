---
id: 01-multi-tag-grouping
title: Complete multi-tag combinations and real package proof
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-SIDEBAR-MULTI-TAG-001
acceptance_criteria:
  - AC-UI-SIDEBAR-MULTI-TAG-001.1
  - AC-UI-SIDEBAR-MULTI-TAG-001.2
  - AC-UI-SIDEBAR-MULTI-TAG-001.3
  - AC-UI-SIDEBAR-MULTI-TAG-001.4
  - AC-UI-SIDEBAR-MULTI-TAG-001.5
system_design:
  - ../../specs/ui/system-design/sidebar-multi-tag-grouping.md
---

# Task01: Complete Multi-tag Combinations and Real Package Proof

## Summary

Deliver the additive Multi-tag entry in the dedicated Tags plugin using the
existing complete generic grouping contract. Complete plugin/manifest action
coverage and real-package native desktop/phone proof in this one vertical slice.

## In scope

- Strict TDD: first write and run the behavioral plugin cases listed in plan Tests.
- Distinct manifest/action/reference, canonical nonempty subsets, stable IDs and
  live content revisions; checked expansion and unchanged action/SDK bounds.
- Preserve original Tag output and /tasks facet, shared-state privacy/no writes.
- Plugin version0.20.0, README, packaged artifact; generic fixture overlap cases.
- Actual package selectors, trees/counts/pagination and saved-state/touch proof.

## Out of scope

- Host production/SDK changes, tags schema migrations, new browser scan paths.
- Exact-set/nested alternatives, raised budgets or partial success fallback.
- Demo52121 mutation, publication/release/deploy or unauthorized commit/push.

## Acceptance

1. Two distinct discoverable normal saved groupings work without changing Tag;
   the real Multi-tag action deterministically classifies every nonempty subset.
2. Stable identities/revisions, pre-expansion limits, corruption and cancellation
   satisfy the paired design with failing-before/passing-after regression evidence.
3. All listed checks and actual0.20.0 desktop/phone package flows pass on exact
   owned source/artifact snapshots, including complete pre-page trees and recovery;
   limits and any non-green broad gates are reported rather than relabeled.

## ASCII UI preview

### UI-01: Desktop Tasks grouping picker

```text
TASKS                       [Multi-tag v] [Filters]
  Plugin groupings
    Tag
    Multi-tag

[expanded] "Backend"                     2
  Task A (Backend, Frontend)
  Task B (Backend)
[expanded] "Frontend"                    1
  Task A (Backend, Frontend)
[expanded] "Backend" + "Frontend"        1
  Task A (Backend, Frontend)
[expanded] Untagged                       1
  Task C
[Previous]  3 unique tasks / occurrences  [Next]
```

### UI-02: Phone native grouping sheet

```text
Tasks drawer (underlying scroll owner)
+---------------------------------------+
| Plugin groupings                Close |
| Tag                                   |
| Multi-tag                             |
+---------------------------------------+
```

The sheet's choice list scrolls internally with 44px targets and safe-area
padding. Selection closes the inner sheet, restores its parent drawer geometry
and focus, and selects the normal saved Multi-tag view. Existing group list,
pager and disclosure controls remain native phone surfaces. Labels shown above
are illustrative; canonical tag-ID ordering decides actual component order.
Loading/unavailable/retry use current shared controls, not empty Untagged output.
UI-01/UI-02 map to AC-001.1 through AC-001.5 of the new requirement.

See [full previews and scenario matrix](plan.md#ascii-ui-preview).

## Verification

Run the following complete block from the host source root after adding the
new behavioral cases. The first make test is the RED command before production
implementation; rerun it for GREEN. No broad unrelated audit is requested.

```bash
# Start from the host repository root. No output goes into the source worktrees.
set -eu
HOST_ROOT="$PWD"
PLUGIN_SOURCE="$HOST_ROOT/../yattdev-kandev-plugin-tags-main"
case "$(findmnt -no SOURCE --target "$HOME/.cache")" in /dev/sdd*) ;; *) exit 1;; esac
export TMPDIR="$HOME/.cache/tmp"
export GOTMPDIR="$TMPDIR"
export GOCACHE="$HOME/.cache/go-build"
export GOMODCACHE="$HOME/.cache/go-mod"
mkdir -p "$TMPDIR" "$GOCACHE" "$GOMODCACHE"
PLUGIN_BUILD="$HOME/.cache/sidebar-multi-tag-build/plugin"
HOST_BUILD="$HOME/.cache/sidebar-multi-tag-build/host"
mkdir -p "$PLUGIN_BUILD" "$HOST_BUILD"
rsync -a --exclude=.git --exclude=bin --exclude=.build --exclude='*.tar.gz' "$PLUGIN_SOURCE/" "$PLUGIN_BUILD/"
make -C "$PLUGIN_BUILD" test vet KANDEV_ROOT="$HOST_ROOT"
GOFLAGS=-race make -C "$PLUGIN_BUILD" test KANDEV_ROOT="$HOST_ROOT"
make -C "$PLUGIN_BUILD" package-host KANDEV_ROOT="$HOST_ROOT"
# Use a separate build mirror: the running user demo reads the older mirror.
rsync -a --exclude=.git --exclude=node_modules --exclude=bin --exclude=.build --exclude=dist --exclude=.kandev-dev --exclude=test-results "$HOST_ROOT/" "$HOST_BUILD/"
(cd "$HOST_BUILD/apps" && pnpm install --frozen-lockfile)
(cd "$HOST_BUILD/apps/web" && pnpm e2e:run --host --project chromium tests/plugins/sidebar-plugin-grouping.spec.ts tests/plugins/task-list-facet.spec.ts tests/task/workspace-sidebar-views.spec.ts --retries=0)
(cd "$HOST_BUILD/apps/web" && pnpm e2e:run --host --no-build --project mobile-chrome tests/plugins/mobile-sidebar-plugin-grouping.spec.ts tests/plugins/mobile-task-list-facet.spec.ts tests/task/mobile-workspace-sidebar-views.spec.ts --retries=0)
PYTHONDONTWRITEBYTECODE=1 python3 scripts/list-docs.py validate
PYTHONDONTWRITEBYTECODE=1 python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/plans/sidebar-multi-tag-grouping
```

After these commands, install the resulting0.20.0 tarball into a NEW isolated
reviewed-host runtime using the normal native upload. Use cache-drive HOME,
state, ports, browser profile and evidence; prove mock routing/no real credentials.
Drive the exact real-package flows from plan E2E tests on desktop and phone;
record source SHAs, archive SHA-256, public query counts, native interactions and
screenshots. Use the existing Playwright/browser tooling, not a new permanent
external-artifact dependency in default CI. Tear down only the newly owned
runtime/browser/scratch and retain evidence. Do not touch the existing user demo.
This manual native package smoke is a required verification step, not optional
coverage replaced by the generic fixture. Update Results with exact commands.

## Files likely touched

Paths below the plugin root are in ../yattdev-kandev-plugin-tags-main, never copied
as production source into the host:

- manifest.yaml, Makefile, README.md.
- server/actions.go, server/sidebar_grouping.go, server/sidebar_grouping_test.go.
- Optional private server/sidebar_multi_grouping.go and matching *_test.go.
- Host apps/web/e2e/tests/plugins/sidebar-plugin-grouping.spec.ts and its mobile
  counterpart/helper only for generic overlapping membership consumer scenarios.
- New package docs and the paired draft requirements/design; preserve historical
  completed implementation/review evidence in the original package.

## Dependencies

None beyond the existing completed generic host/SDK grouping contract. Read
SDKconstants/manifest/action/current plugin strict-reader tests before changing
code. Existing dependency pins require no change unless actual compatibility
checks demonstrate a concrete need; stop for design reconciliation in that case.

## Risks

- Powerset expansion must not allocate from unchecked2^n or yield partial output.
- JSON-quoted labels may exceed transport text bounds even for two tags.
- Count canonical tasks separately from repeated root/descendant occurrences.
- Respect existing runtime configuration/status publication and scope fences;
  do not add separate invalidation/retry ownership for this mode.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-multi-tag-grouping.md).
- [Design](../../specs/ui/system-design/sidebar-multi-tag-grouping.md).
- Original sidebar package, SDK decoder, Tags shared-state and action tests.
- Canonical plugin authoring guide and scoped frontend/backend conventions.

## Results

Implemented on 2026-11-18 by the user-authorized Opus implementation task
(runtime `anthropic/claude-opus-5-5`, profile `3365c84c`). All changes are
unstaged and uncommitted. Source baselines: host `3bbc8367`, Tags `b10662aa`.

Tags repository: `server/sidebar_multi_grouping.go` adds
`sidebarMultiTagSnapshot`. Before enumerating, it checks each task's subset
count, computed by bounded doubling, against the remaining value and
membership budgets. It also enforces the global 1,000-value union and the
256-byte quoted label. IDs are `set-` plus SHA-256 of the JSON sorted member-ID
array. Only singletons keep a color. `sidebarGroupingAction` now takes a
builder, so both actions share the strict reader, the cancellation checks and
the encoded 1 MiB bound. Tag output is unchanged. The manifest adds `multi-tags`
/ `sidebar-multi-tags` (authenticated, workspace, body 16); the version is 0.20.0
in the manifest and Makefile; the README documents the new view. The host adds
one generic fixture scenario to `sidebar-plugin-grouping.spec.ts`; host
production code is unchanged.

RED: `make test` failed 12 cases with `unknown action: sidebar-multi-tags`,
before any production edit. GREEN, on the final source: `make fmt` clean,
`make test vet` passed (Go plus 204 UI tests), and `GOFLAGS=-race make test`
passed. New cases: powerset memberships, stable identity/revision, expansion
bounds (9/10/64 tags, 1,000/1,001 values, 10,000/10,001 memberships, 256/257
byte labels), exact 1 MiB encoded response, cancellation, plus the existing
corrupt-state and raw-bound cases now run through both actions.

Workorder commands from the owned mirror under `~/.cache/sidebar-multi-tag-build`:
desktop chromium specs 11/11 passed; mobile-chrome specs 6/6 passed;
`list-docs.py validate`, `lint-spec-files.py --all` and `git diff --check` passed.

Package `kandev-plugin-tags-0.20.0.tar.gz`, SHA-256
`29c1b8fc589365adbf42121523647d751d1a8b3e21b8e8feffdc2dfb119c6a39`.
It was installed by native upload (HTTP 201, active) into a new owned runtime
(loopback port 53886, cache-drive HOME/state/database, `KANDEV_E2E_MOCK=true`,
onboarding listed only Mock agents, no credentials). The scripted smoke passed
67 checks, including:

- The real action returned all seven B/F/O subsets; memberships were 7/3/1/0
  for 3/2/1/0-tag roots, 111 in total. Tag output was unchanged at 4 values
  and 105 memberships.
- 103 unique tasks and 119 occurrences across two pages. The root and its
  child appeared in all 7 subsets. Selecting the root marked every occurrence.
- Keyboard selection created a normal saved view and Escape restored focus.
  Reselection reused the view. Save as copied collapsed state, and the copy's
  disclosure stayed independent after reload. Workspace journals stayed
  separate.
- Live rename and recolor kept the collapsed combination's identity. Removing
  one application and deleting a tag updated only the affected subsets.
- Ten tags on one root: Multi-tag returned 503 with an explicit
  unavailable/Retry state and no Untagged fallback, while Tag stayed usable.
  Dropping to nine tags recovered with 511 subsets. Disable/enable kept the
  saved view.
- At 320 and 393 px with true-touch emulation: trigger, choices and pager were
  at least 44 px; the choices were inside the viewport; Escape restored focus;
  the parent drawer geometry was restored; there was no horizontal overflow;
  touch pagination worked.

PNGs were inspected. This is emulation, not a physical-device test. Evidence
is in `~/.cache/sidebar-multi-tag-build/evidence`. The owned runtimes were
stopped and their state was removed. The demo on port 52121, the build mirror
it reads, and every other instance were untouched. Historical broad web and
backend failures were neither rerun nor relabeled.

Independent Sol source reviews 1, 2 and 3 reported CLEAN with zero findings on
the same implementation and all 13 working-tree file hashes. The source review
gate reached three consecutive clean rounds after the separate three-clean
plan review gate and Opus implementation. No source corrections were required.
These are read-only reviews of uncommitted changes, not fresh runtime checks
or broad-suite certification. Parent closure changes only execution status and
this receipt after the reviewed snapshot.

The recovered desktop screenshot captures loading, not a completed recovery
render. Recovery evidence is the retained action assertions and generic
recovery coverage. The 67 smoke records include screenshots and narrative
entries, not 67 independent assertions. Physical-phone and remote-CI proof
remain absent; historical broad gates remain non-green. No new commit, push,
release, deployment or user-demo upgrade was performed.

### Post-rebase round 1 corrections

The later post-publication review found six regressions in the complete feature.
The source clean streak reset to zero; earlier clean rounds remain historical.

- Compatibility now reserves an unreleased 0.103.0 minimum. Released 0.102.0
  predates this contract; development builds require the matching implementation.
  Publication must wait for the supporting Host release, not merely this floor.
- Dedicated plugin setup and workflows use reachable
  `Fclem/kandev@a717a8db0b51c29c01a4d3655cb2649f529dacec`. Setup compile-checks
  existing SDKs without resetting them. A clean sparse setup/build passed;
  the released 0.102.0 SDK was incorrectly accepted before, and is now rejected.
- A barrier regression demonstrated cold task-detail boot waiting on a plugin
  RPC. Boot now leaves grouped pages to the authenticated asynchronous read;
  the same regression also proves that later grouped reads reach the provider.
- Lifecycle generations and immediate invalidations are provider-scoped.
  Metadata refreshes preserve unchanged identities; the gateway retains plugin
  identity without publishing workspace classification. Real config/disable
  regressions and frontend identity tests failed before the correction.
- A 1,001-definition catalog with one observed singleton now succeeds in
  Multi-tag. The emitted subset, expanded-pair and encoded-byte limits remain
  independent; the existing Tag action is unchanged.
- Plugin totals remain visible on a single page without Previous/Next buttons.
  The 0/100/101-occurrence component regression failed before the correction.

The owning original system design and public plugin compatibility references
are updated. Native unrelated-plugin page-two and collapse-below-limit coverage
is included in the desktop grouping scenario. Final corrective verification,
artifact provenance and commit/push receipts are recorded in the platform task
plan after their commands complete. No release or user-demo upgrade is authorized.

### Post-rebase round 3 corrections

The third post-rebase Sol review reported eight findings. Its clean streak
remains zero pending a new independent review of the corrected commits.

- Workspace access is renewed after the provider RPC, immediately before the
  task repository read. A real member-removal barrier regression preserves the
  typed denial and has a retained-membership control.
- Catalog and resolver checks capture the committed record and scoped epoch
  together. Projection and RPCs run outside the commit guard; pending failed
  status writes cannot publish or reject provisional identities.
- Responses carry the complete provider `grouping_keys`, not only page keys.
  Ordinary queries retain exact client disclosure. Oversized retired disclosure
  waits for its captured workspace journal and reads saved preferences only to
  recover complete metadata. That recovery page is discarded; reconciliation
  preserves surviving keys and a bounded successor performs the real read.
  A different-group draft starts expanded without modifying its base view.
- Normal saved-view writes send `views_upsert`, `view_ids_delete` and `view_order`
  under the existing workspace CAS. Queued deltas use the last acknowledged
  server snapshot, so a failed earlier write cannot silently lose later intent.
  Neither the 256 KiB query cap nor the 2 MiB settings cap was raised.
- Shift ranges are sliced in rendered occurrence order before canonical task-ID
  deduplication. Selected origins survive anchor realignment and partial bulk
  failures. Row/menu callbacks bind their occurrence without changing canonical
  actions or selected styling.
- Existing typed tag names containing controls get deterministic readable
  sidebar escaping, without ID changes or read-side writes. Both real actions
  retain their label, snapshot and malformed-input bounds.
- Fresh SDK setup reserves the destination exclusively before fetching. Failure
  removes only its owned reservation; concurrent and late explicit destinations
  cannot receive nested checkouts or succeed against the wrong pin.
- The browser authoring contract now agrees on the unreleased 0.103.0 floor.
  The development SDK/package-tool pin is
  `Fclem/kandev@1154f0ff1771aef71091b8f7457ada905e97af3b`; no release is authorized.

Targeted final frontend regressions passed 152 tests in 15 files. Scoped
production typecheck and affected-file lint passed. Plugin Go/vet/race, 204 UI
tests and three real Git setup regressions passed. Targeted host race checks and
SQLite plugin grouping regressions passed. Native boundary checks and the fresh
real-package receipt are recorded below after execution; these targeted passes
do not certify the historical broad backend/web failures.

Final native verification passed desktop 11/11, then a fresh rebuild passed
the three new disclosure/copy/range boundary cases and phone 4/4. Frontend lint
cleanup was followed by 22 affected disclosure tests, a clean scoped production
typecheck and zero-warning affected frontend lint. Changed Go lint reports zero
issues; final handler/user race checks also passed after validation extraction.
Specification validation and all 55 published docs pages passed.

The fresh 0.20.0 archive has SHA-256
`b5e85ea8803f352dcdea5c15e8b535c96d95ec440dd777b461bfdeb8c3975ada`.
Native upload returned HTTP 201 and active status in an owned cache-drive
runtime on loopback 55931. Its cleared environment selected the mock profile
and separate HOME/state/database; no primary credentials or user data were used.
The driver completed with exit zero: complete powersets and repeated trees,
103 unique tasks/119 occurrences, normal saved-view reuse/copy/reload,
workspace separation, mutation, overflow/independent Tag availability and
settled recovery, disable/enable, single-page totals, and touch paging at
320/393 pixels. A separate workspace verified a human-created interior-tab
name through both actions, desktop preview/Save as and phone selectors.

Desktop control-name and settled-recovery PNGs and the 320-pixel Page 2 and
control-name PNGs were inspected. The 77 evidence records include screenshots
and narrative, not 77 independent assertions. Driver and launcher source are
retained as data in `~/.cache/sidebar-multi-tag-build/evidence/r3-smoke-evidence.json`
alongside raw logs, results and PNGs. The owned runtime was stopped, checked
for open files, and its marked state removed. A separate absent-destination
SDK setup fetched the exact development pin and compiled the plugin successfully.
Demo 52121, its separate mirror, 59390 and primary instances were untouched.

No broad backend/web/SQLite Sidebar, PostgreSQL-without-DSN, physical-phone or
remote-CI certification is inferred. The earlier long desktop stall remains
unexplained and native MaxListeners warnings remain unsuppressed. No release,
deployment or user-demo upgrade is authorized.


### Post-rebase round 4 corrections

The fourth post-rebase review reported eight findings; the independent source
clean streak remains zero until another review of the corrected commits.

- Queued writes capture intent relative to their optimistic predecessor and
  rebase only that intent on the latest ACK. Concurrently added/deleted views
  survive unrelated queued edits; terminally failed predecessor intent remains
  available to queued successors without replay after a discarded failure.
- Different-group drafts own optional disclosure independently of their base,
  including wire cloning, backend persistence, effective desktop/phone views,
  recovery lookup, Save, Save as and Discard.
  Repeated draft PATCHes decode a private record before change detection rather
  than mutating the prior draft through a shared pointer. The HTTP/SQLite
  regression failed with lost disclosure before this correction and passes
  with the handler/workspace race cases afterward.
- Disclosure compaction publishes only after its settings ACK. Terminal failure
  keeps saved intent and an actionable manual Retry instead of changing query
  identity through optimistic prune/rollback loops. Probe task rows are never
  admitted. A later explicit Retry can persist and load the bounded successor.
- Saved-disclosure recovery authorizes before personal-settings lookup and
  renews denial precedence after lookup errors. A real SQLite member-removal
  barrier with the production user-settings projection returns denial, not
  malformed-query status; retained-member and missing-view controls remain.
- Status transitions save a private registry clone before publishing it. The
  real Resolve/InvokeAction dispatch lease and gRPC action regression rejects
  committed Error but keeps healthy classification after failed persistence.
  The obsolete provisional-status mutation method is removed.
- Managed SDK setup stays private through compatibility compilation, then
  requires HEAD to match the requested pin on reuse. Explicit existing
  checkouts stay compile-only and are never reset. A compile-phase concurrency
  barrier, completed wrong-pin reuse, fetch recovery and explicit-checkout
  controls pass.
- Generated-view coverage now acknowledges real deltas and reloads their saved
  identity/reuse behavior instead of requiring the removed full-list payload.
- Tag display escapes literal backslashes and controls distinctly; Multi-tag
  JSON-quotes original names once. Encoded labels retain their real byte cap,
  applied only to emitted values. Both actions retain strict raw validation,
  canonical identities and zero read-side writes.

Behavioral REDs were observed before fixes. Three new native regressions also
failed against the unchanged round-3 production mirror: preview collapse stayed
expanded, queued edits deleted/resurrected remote views, and failed compaction
did not expose Retry. Their source baseline was byte-compared to committed
`f2b8577707b51ea4da3b1789d132c7795a9374d5`.

Targeted frontend verification passed 118 tests in 12 files. Scoped production
typecheck passed. Affected Go lint reports zero issues. Real dispatch/status race
checks and handler/user-settings race checks passed. Plugin fmt/test/vet passed,
with 204 existing UI cases and four setup regressions; a new absent-destination
setup fetched and compiled the exact SDK pin. Final native and actual-package
receipts are recorded after those runs, without waiving historical broad gates.

Round-4 runtime proof:

- Freshly rebuilt desktop acceptance: 17/17 passed. Phone acceptance: 8/8
  passed with true-touch emulation, including all three preview finish modes.
- The separate actual 0.20.0 archive was installed through native upload into
  a fresh cache-drive runtime. SHA-256:
  `c1ae9ad50f3708df1db7bf4d70d2207debb2964c4448328671e48ae90b1cf885`.
  The final driver exited zero with 82 mixed assertion, screenshot and narrative
  records, not 82 independent tests.
- Real-package proof includes 103 unique tasks / 119 occurrences, repeated
  complete trees, saved copies, live mutations, explicit overflow and settled
  recovery, 320/393 touch pagination, persisted changed-group disclosure and
  distinct tab/backslash headings in both modes. Retained PNGs were inspected.
- The first rebuilt run exposed the repeated-draft persistence defect
  described above. A second smoke run failed the driver's two-read geometry
  assertion despite logging a 44px rectangle. It now stabilizes the control,
  verifies 44px CSS height and uses one rectangle with 0.01px numeric tolerance.
  No product change was made for that measurement failure.
- Evidence remains under `~/.cache/sidebar-multi-tag-build/evidence/r4-*`.
  Historical broad gates remain non-green or unproven; no physical-phone,
  PostgreSQL, remote-CI, publication or deployment certification is added.

### Post-rebase round 5 corrections

The fifth post-rebase review reported seven findings. The independent source
streak remains zero pending review of the corrected commits.

- Journal admission is monotonic across scoped deferred WebSocket snapshots
  and delayed HTTP ACKs. Normal edits use update-only deltas, so even an unseen
  concurrent deletion cannot be undone by a queued disclosure write.
- Draft mutations rebase only their changed fields and disclosure intent for
  the same base/group. A presentation edit cannot restore keys discarded by
  acknowledged compaction or replace independently acknowledged draft fields.
- Queued compaction rechecks the captured workspace/provider fence, owner and
  expected disclosure immediately before dispatch. A retired probe is a no-op,
  not a stale settings write or save error.
- Metadata-read deadlines and automatic retries do not surround admitted
  settings writes. A slow rejected compaction remains a terminal manual Retry;
  the saved disclosure is intact and probe rows remain excluded.
- Ordinary append/delete deltas use natural survivor-plus-append ordering.
  Only an explicit reorder supplies a complete collection order, preserving
  independently added views under the existing revision CAS and transport caps.
- Projection shares the writer's blank-name semantics. A legitimately created
  U+0085-only tag is escaped and classified in both modes without read-side
  writes, while malformed storage and encoded output limits remain enforced.
- Every public SDK consumer checks pending managed setup, including an explicit
  SDK override. Only the owning compatibility compile can read its reservation;
  successful validation admits later builds and packages at the requested pin.

Behavioral REDs preceded each correction. Four permanent native journal cases
also failed against the retained round-4 production build: deleted-view
resurrection, independent append/delete conflicts, and replay of 6,000 retired
draft keys. The source fixtures exercise the exact deferred-WS, queued-provider
invalidation and 11-second rejected-write schedules; native tests exercise real
HTTP revision/CAS and visible saved-view behavior, not mocked app state.

Focused formatting and lint checks also constrained the implementation: journal
snapshot/admission helpers own a separate module, disclosure carries one context
object, and the backend update-only loop is extracted without extra allocation.
The expanded E2E typecheck exposed two errors at unchanged fixture lines also
present in the frozen upstream base (duplicate settings property and undeclared
Window flag). Those are not silently removed from verification or represented
as a green broad gate.

Round-5 final verification and runtime proof:

- Final focused frontend tests: 86/86 in 11 files. Production/unit scoped
  typecheck and affected ESLint with zero warnings pass. Backend Sidebar race
  checks pass for models/service/handlers; the extracted service helper also
  passes its final race check and affected Go lint reports zero issues.
- Fresh absent-destination setup fetched and compiled the pinned compatible
  SDK. Plugin fmt/test/vet and race targets pass, including 204 UI cases and
  five setup cases. Public pending-consumer checks include SDK overrides.
- The first corrected desktop build passed 20/20. After the final lint-driven
  refactors, native journal/workspace acceptance passed 5/5 and phone acceptance
  passed 9/9 with touch emulation. One prior phone command selected incorrect
  filenames and ran no tests; the corrected `mobile-*` paths produced the 9/9
  receipt. No product correction or retry setting hid that command failure.
- The final real 0.20.0 package has SHA-256
  `d77d9abdf6f76bd9b815507b483931c787077c94ebcad5d217edf85aae306c8f`.
  Native upload returned HTTP 201 and Active in a fresh owned cache-drive
  runtime on loopback 55937. The driver exited zero with 88 mixed assertion,
  screenshot and narrative records, not 88 independent tests.
- Real-package proof covers 103 canonical tasks/119 occurrences, complete
  repeated trees, saved reuse/copy/reload/workspace separation, live mutation,
  explicit overflow with independent Tag availability, settled recovery,
  disable/enable, single-page counts, and 320/393-pixel touch pagination.
  A normally created/applied U+0085-only name classifies and has readable
  desktop/phone headings in both modes. Native journal and real-package PNGs
  were inspected, including the visible Page 2 and settled recovery.
- The owned runtime was stopped; OWNER and open-file checks preceded removal
  of its state/profile. Driver, launcher and typecheck inputs remain as data in
  `~/.cache/sidebar-multi-tag-build/evidence/r5-smoke-evidence.json` and
  `r5-typecheck-configs.json`; throwaway executable scripts were removed.
  Demo 52121, its mirror, 59390 and primary instances were untouched.

The expanded E2E typecheck and historical broad gates remain non-green or
unproven. No PostgreSQL, physical-phone, remote-CI, release, deployment or
user-demo upgrade certification is added. Independent source review remains
open, with clean streak 0/3.

### Post-rebase round 6 corrections

The review reproduced five additional schedules: an already-dispatched full
view overwriting a concurrent rename/group change, network replay of a retired
prune, a skipped prune discarding failed predecessor intent, duplicate generated
views selected from two stale tabs, and SDK setup validating an external module
instead of its selected checkout.

Ordinary view updates now merge sparse fields and grouping-owned disclosure
inside the settings CAS. Creation is insert-only; replayed creation and later
edits remain independent. Draft updates and discard retain the captured
base/group owner rather than acquiring a newer unrelated draft's owner.
Compaction sends no automatic transport retry, and a skipped queue item is not
a persistence acknowledgement. Failed normal intent remains available to its
queued successor; terminal skip settles against the authoritative state.

Generated selection is an atomic CAS select-or-create. It preserves existing
unchanged identity and disclosure, renamed/custom views, drafts, unique names
and the view limit. The client waits for the returned active ID instead of
inserting a provisional saved view. Desktop and phone consumers use that
asynchronous admission path. SDK setup explicitly compiles the selected
`KANDEV_ROOT/pluginsdk`, preserving separate overrides for ordinary consumers.

RED receipts include service-level sparse-update/creation/draft failures,
native same-view rename/group and two-tab duplicate failures, retired transport
retry and skipped-predecessor failures, and four SDK-root selection failures.
Focused GREEN includes 88 frontend cases in 12 files, scoped TypeScript,
affected ESLint without warnings, backend Sidebar race checks across
models/service/handlers, a final service race recheck, and zero Go lint issues.
Rollback and predicate/upsert helpers were extracted to meet existing limits;
no lint rule was suppressed. Full `make fmt` and spec/catalog validation pass.

Fresh absent-destination setup fetched compatible SDK pin
`1154f0ff1771aef71091b8f7457ada905e97af3b` and completed real compatibility
compilation. Tags fmt/test/vet/race checks pass, including 204 UI and nine setup
cases. The documentation-complete 0.20.0 archive is
`5a1a1d747785352fc279e4160fa87e5abc028b0e269a4f39439e6c437f1a6d44`.
The first guarded desktop run passed 27/27 before lint-only helper extraction.
The final rebuilt native and actual-package receipts are recorded separately.

Round-6 final native and package proof:

- After the lint-only extractions, a fresh guarded artifact rebuild passed the
  eight owning desktop journal/workspace cases. The sequential phone project
  passed 9/9 with touch emulation. Both used one worker and no retries.
- Native upload of the final archive returned HTTP 201 and Active in a fresh
  owned mock-only cache-drive runtime on loopback 41515. The driver exited zero
  in 208.04 seconds with 88 mixed assertion, screenshot and narrative records,
  not 88 independent tests.
- Real-package evidence includes 103 canonical tasks/119 occurrences, complete
  repeated trees, saved reuse/copy/reload/workspace separation, live mutations,
  explicit overflow with independent Tag availability, settled recovery,
  disable/enable, single-page counts and 320/393-pixel touch pagination.
  Desktop grouping/recovery, distinct escaped headings, phone chooser and
  visible Page 2 PNGs were inspected. The phone summary logs pre-tap text;
  its assertion and Page 2 PNG establish the resulting page.
- The owned runtime was stopped. Exact OWNER and open-file checks preceded
  state removal. Driver/launcher/compiler inputs remain as data in
  `~/.cache/sidebar-multi-tag-build/evidence/r6-smoke-evidence.json` and
  `r6-typecheck-configs.json`; throwaway executable scripts were removed.
  Demo 52121, its mirror, 59390 and primary instances remained untouched.

Broad historical gates, the expanded E2E fixture type errors and the older
1800-second stall remain unwaived. No physical-phone, PostgreSQL, remote-CI,
release or deployment claim is added. The post-rebase source review remains
open at clean streak 0/3.


### Post-rebase round 7 corrections

Five review schedules were reproduced: stale Save/Save as disclosure transfer,
dependent actions during provider selection, inverse intent after a committed
response failure, duplicate-only oversized disclosure, and queued reorder
omitting an already acknowledged remote append.

`save_draft` now transfers the latest matching base/group-owned draft inside
CAS. Copies retain independent disclosure; a changed owner conflicts without
mutating the newer draft. Selection and Save expose workspace-local busy
admission across desktop and phone controls while canonical task actions remain
available.

Failed-response replay composes sparse touched assignments and explicit
disclosure removals, rather than cancelling inverse edits against an uncertain
preimage. Creation remains insert-only and updates cannot recreate deleted
views. Explicit reorder uses the latest admitted collection without widening
sparse field intent. Duplicate-only recovery carries canonicalization into CAS,
preserving concurrently added keys.

RED includes five draft-save service outcomes, two canonicalization cases,
six focused store cases and seven native desktop failures on the unchanged
round-6 artifacts. All failed at the expected consumer assertions, not setup.
Focused GREEN includes 105 frontend cases in 16 files, production/unit scoped
TypeScript, affected ESLint without warnings, Sidebar race checks across
models/service/handlers and a final owner-helper race recheck. Initial lint
warnings were resolved by helper extraction without suppressions.

Full `make fmt` passes in the owned cache mirror. Spec/catalog validation
reports 456 decisions and 2049 specifications. An initial documentation command
used the wrong working directory and validated no documents; the corrected
repo-root commands passed. Historical broad gates and the source-review
clean-streak limit remain unwaived.

Round-7 final native and real-package proof:

- A fresh guarded rebuild passed 15/15 owning desktop cases. The sequential
  phone project passed 10/10 with touch emulation, including pending-selection
  admission and an actual touch reorder queued behind a rename while another
  client appended a view. Both used one worker and zero retries.
- Native upload of the unchanged exact Tags-head 0.20.0 archive returned HTTP
  201 and Active in a fresh owned mock-only runtime on loopback 34415. Archive
  SHA-256 remains
  `5a1a1d747785352fc279e4160fa87e5abc028b0e269a4f39439e6c437f1a6d44`.
  The driver exited zero in 213.70 seconds with 88 mixed assertion, screenshot
  and narrative records, not 88 independent tests.
- Real-package proof includes 103 canonical tasks/119 occurrences, repeated
  trees and selection, saved reuse/copy/reload/workspace separation, live
  mutations, explicit overflow with independent Tag availability, settled
  recovery, disable/enable and 320/393-pixel touch pagination. Desktop grouped
  rows, settled recovery, distinct escaped headings, the phone chooser and
  visible Page 2 PNGs were inspected.
- Every owned production/test source byte matches the tested cache mirror;
  identities are retained in `evidence/r7-build-identity.json`. Runtime OWNER,
  driver/launcher sources, results and inspected screenshot names are retained
  in `~/.cache/sidebar-multi-tag-build/evidence/r7-smoke-evidence.json`.
  The owned service was stopped and no runtime files remained open.

Tags source did not change in this correction round. Demo 52121, its mirror,
59390 and primary instances were untouched. Historical broad gates, the older
1800-second stall, expanded E2E fixture errors, PostgreSQL/no-DSN, physical-phone
and remote-CI limits remain unwaived. No release or deployment is authorized;
the post-rebase source clean streak remains 0/3.

### Post-rebase round 8 corrections

Four additional schedules were reproduced: a Save queued after a rejected
prerequisite, a draft-owned prune acquiring a remotely saved owner, a queued
drag overwriting a disjoint acknowledged drag, and case-folded JSON fields
bypassing the raw Tags application preflight.

Save and Save as now wait until preceding workspace preference writes settle.
Store admission and accessible disabled/busy controls, including Save-as
confirmation, prevent acknowledgement of older data after a prerequisite fails.
The former empty-ACK optimistic-only Save test was removed; persisted-draft
failure tests cover both Save modes and field/disclosure prerequisites.

Compaction captures saved-versus-draft ownership at queue admission. A
draft-owned update additionally requires an existing matching draft inside CAS,
so a post-dispatch owner transfer cannot recreate it. Reorder journals its
moved ID, destination and original before/after placement, replaying only that
move against the latest admitted order. Missing endpoints fail explicitly;
the stale-permutation fallback was removed.

Both Tags modes reject case-fold aliases in the document, definitions and
application classification fields before the independent permissive board
decoder. This preserves exact preflight/count coverage, zero read-side writes
and the existing response bounds.

RED receipts include five store failures, saved-versus-draft admission and
real service failures, eight v1/v2 real-action alias failures, native Save/prune
failures and the owning disjoint touch reorder failure on round-7 artifacts.
The initial native sort setup chose an unchanged value and timed out; selecting
the opposite direction produced the expected disabled-Save failure. The first
long-label touch drag landed past its intended neighbor; a short rename made
the intended A/B drag deterministic and then reproduced the C/D overwrite.
Neither script issue is represented as a product failure.

Initial GREEN includes the focused ownership/move/prerequisite regressions,
scoped production/unit types, affected ESLint without warnings, affected Go
lint and user Sidebar race checks. Full Tags test/vet/race passes, including
204 UI cases and nine setup cases. Native final and fresh-package receipts are
recorded separately below.

The combined formatting/build/native command expired at its 1,800-second
deadline before Playwright started. Formatting and backend/frontend builds
completed; fixture archive/identity timestamps remained from round 7. This
timeout is a failed verification command, not a sidebar runtime failure.
The remaining fixture package prerequisite completed separately in 27.8
seconds. Verification then resumed through the supported `--no-build`
desktop/phone runner, without repeating the completed backend/frontend builds.
Receipts: `r8-make-fmt.log`, `r8-native-quiet/kandev-run.e2e.oPPK5n2q.log`
and `r8-fixture-prerequisite.log` in the retained cache evidence directory.

The resumed desktop run completed 16 cases and exposed two failures. The
disabled-button case tried to press Enter on a control that cannot receive
focus; that unreliable extra interaction was removed while retaining both
native disabled-control assertions and the store-level admission regressions.
The dispatched-prune case established a real missing successor: a remote Save
moved disclosure from draft to saved ownership without changing its effective
group or keys. The query context now includes that concrete ownership, so a
stale draft probe cannot strand the new saved owner on the same request key.
The permanent native case requires the independently compacted saved owner,
collapsed heading and reload to succeed. Its failed screenshots/context and
raw output remain under `r8-desktop-followthrough-failures` and
`r8-native-quiet/kandev-run.e2e.SSXXPazB.log`.

Final owning desktop follow-through passed 3/3 (43.8 seconds), and the
current-source touch-emulated phone set passed 11/11 (4.1 minutes). The first
desktop run's 16 passing cases and two failures are retained separately, not
presented as an all-green final 18-case invocation. Related hook/journal tests
passed 64/64; scoped production/unit types and affected ESLint passed again.

The fresh real 0.20.0 archive has SHA-256
`4de7a671be43236b79b99771e62b93229d9dd2ee97dd1611c01038be3c502c1d`.
Native install and the full isolated desktop/320/393 touch driver completed
with exit 0 in 224.82 seconds: 88 mixed assertion, screenshot and narrative
records, not 88 independent tests. It observed 103 unique tasks and 119
occurrences, saved-view reuse/copy/reload and workspace isolation, mutation,
overflow/Tag independence, settled recovery and touch pagination. Inspected
PNG evidence shows repeated trees, settled recovery, the 320px chooser,
393px Page 2 and distinct escaped headings.

The owned mock-only runtime was stopped; its OWNER, launcher/driver sources,
results, screenshot digests and backend log were retained before cleanup.
`r8-final-tested-identity.json` matches all 18 affected host source/test files
and four plugin files to their tested build mirrors. Protected demo 52121,
59390 and primary instances were untouched. Historical broad backend/web/
SQLite gates, PostgreSQL without DSN, physical phone and remote CI remain
non-green or unproven; no release, publication or deployment is authorized.

### Post-rebase round 9 corrections

Round 9 reported four source-derived Host races: retired moves poisoning
successors, post-dispatch order overwrite, Save as naming transferring a
replacement owner, and blind retry of committed provider selection.
The existing saved-view isolation contract owns these corrections.

Failing-first store regressions reproduced lost independent rename, wrong-owner
copy and repeated selection; the real service regression accepted a stale
observed order before the correction. Retired move replay is now filtered by
its current endpoints. Reorder dispatch carries `view_order_base`, checked
against committed rows inside each settings CAS attempt. Save as captures and
rechecks its naming owner; changed owners dismiss, same-owner disclosure stays
fresh. Provider selection now uses one transport mutation, like draft transfer.

Initial store/component GREEN passed 51 cases; a real user-service race
regression passed. Native RED against the retained round-8 artifacts reproduced
both stale name editors, repeated provider selection and post-dispatch touch
overwrite. The same-owner positive native case failed in seed setup with
workspace-not-found before its scenario ran; that is not a product RED or a
passing control. Final rebuilt-source results are recorded below.

Current-source owning desktop passed 15/15 (2.3 minutes); touch-emulated phone
passed 7/7 (1.1 minutes). Both include changed-group/base naming invalidation
and the same-owner positive control. Desktop covers a genuinely committed
reused selection whose transport ACK is aborted; touch covers a dispatched
move conflicting without overwriting the remote order, plus reload.
The initial final-run preflight stopped before tests on a stale delivery
fixture; the required delivery/send-now/package artifacts were rebuilt before
these passing runs. That preflight is not a product failure or test result.

Final focused store/component tests passed 51/51, scoped types and affected
ESLint passed without warnings. Affected user Sidebar race checks, Go lint and
specification/decision validation passed. Lint-driven extraction keeps naming
state in its private hook and failed-intent ownership in the existing journal.
`r9-tested-identity.json` matches all 19 affected source/test files to the build
mirror. Tags source is unchanged at `650581569554e471fca37477453d45572c9ac54d`;
the existing 0.20.0 archive is used for fresh current-host acceptance.

Fresh current-host acceptance installed that unchanged archive with HTTP 201
and Active status in owned mock-only runtime 49327. The full driver exited 0
in 233.49 seconds, retaining 88 mixed evidence records and 23 PNGs, not 88
independent tests. Public projection reported 103 unique tasks and 119
occurrences across two pages. Repeated trees, saved views, mutation, overflow,
independent Tag behavior, settled recovery and 320/393 touch paths passed.
Inspected current grouped/recovered desktop and actual 393px Page 2 PNGs,
plus the native replaced-owner phone editor; the touch reorder screenshot
captures the menu top, not visible chip-order proof. Its order proof is the
native DOM/persistence/reload assertion.

Runtime OWNER, driver/launcher sources, results and screenshot digests are
retained in `r9-smoke-evidence.json`; the backend log was copied before
owner-checked cleanup. The owned service was stopped, open-file checks were
empty, and only its state/profile and throwaway scripts/config/format copies
were removed. Protected demo 52121, 59390 and primary instances were untouched.
Historical broad backend/web/SQLite gates remain unresolved; PostgreSQL/no DSN,
physical phone and remote CI remain unproven. No release or deployment.

### Post-rebase round 10 corrections

Round 10 reported two source-derived Host findings; Tags had no new finding.

- Definitive 4xx rejection (excluding timeout and rate-limit responses) retires
  dispatched and retained failed move intent. Independent sparse successors
  remain eligible; uncertain transport/5xx failures keep guarded replay.
- The generic SDK decoder consumes canonical classification fields once,
  validates array bounds while reading, and rejects duplicate or case-folded
  recognized fields before normalization. Nested group/membership identities
  follow the same rule. Unrelated extension metadata remains non-classifying.
- Regression coverage includes newer WebSocket order plus queued independent
  rename, real touch drag/HTTP rejection/reload, SDK ambiguous and over-limit
  raw arrays, supervised resolver unavailability, and native HTTP/unavailable
  recovery rather than a successful Untagged page.

Verification:

- RED: SDK ambiguous-field cases failed before the decoder change. The journal
  rejected-move cases failed for 400/409; the prior-bundle true-touch test
  reproduced the independent-successor reorder loss.
- GREEN: 51 focused frontend tests, scoped types/ESLint/Prettier, SDK grouping
  race tests and SDK vet, real Service resolver malformed-wire plus healthy,
  authorization and lifecycle controls, affected backend Go lint, public-doc
  validation/tests and specification validation/lint.
- The first resolver command exhausted its 180-second compilation deadline
  without test output. A separate cache-only race compilation completed;
  the compiled resolver regressions passed. This is not a deadlock claim.
- Cache-only `make fmt`, backend/helper/plugin-fixture and SPA builds completed
  in 958.30 seconds. Eleven changed code/test paths byte-match the build mirror.
- Native malformed-wire desktop: 1/1 passed. All three duplicate/alias variants
  return 503 with no entries, reach unavailable/Retry and recover with canonical
  data. The initial five-second scaffold wait was corrected for the normal
  thirty-second recovery budget.
- Touch: three unchanged drag controls passed. The new test first exposed a
  scaffold predicate treating the `watchWs` payload as a frame; after correcting
  that argument, its owning case passed 1/1. Real 400 rejection, independent
  rename, API/DOM order and reload assertions remain intact. The screenshot
  shows the persisted rename, not the entire horizontally clipped order.
- Fresh real Tags 0.20.0 installation on the corrected owned host returned
  HTTP 201 Active. Its unchanged archive SHA-256 is
  `4de7a671be43236b79b99771e62b93229d9dd2ee97dd1611c01038be3c502c1d`.
  The final 214.44-second run produced 88 mixed evidence records and 23 PNGs,
  not 88 independent tests: 103 unique tasks/119 occurrences, repeated trees,
  saved-view mutations, explicit overflow/Tag independence/recovery, and true
  touch emulation at 320/393 px. Inspected desktop grouping, unavailable,
  settled recovery and actual phone Page 2 PNGs.
- The first package-driver attempt stopped at an unpaced keyboard-focus loop
  after a successful install/action proof. Semantic focus-transition waits
  fixed the driver; the successful run used fresh owned state. No product
  change or forced DOM/state mutation was used to make it pass.

Receipts are under `~/.cache/sidebar-multi-tag-build/evidence/r10-*`, including
retained driver/launcher source, owner, results, source and screenshot digests.
Both owned runtimes were stopped; open-file checks were empty before removing
only their state/profile and throwaway executables/config/format copies.
Protected demo 52121, 59390 and primary instances were untouched. Tags source
remains `650581569554e471fca37477453d45572c9ac54d`; no SDK API/type or package
publication change was needed. Historical broad backend/web/SQLite gates remain
unresolved; PostgreSQL/no DSN, physical phone and remote CI remain unproven.
The 0.103.0 host floor remains unreleased. Review gate stays 0/3 pending round 11.

### Opus post-rebase round 11 correction

Opus verified its actual Claude Opus 5.5 assistant-role runtime and reported
one Host defect: an already-committed reorder's lost-response retry conflicted
with its own observed base. Tags had no new finding. The review was centred on
the R10 response and related boundaries, not a fresh line-by-line certification
of the complete feature range. The gate remains 0/3.

The settings CAS now accepts either the observed order or the requested order
already being committed. It still rejects a different stale permutation.
No frontend retry policy, API shape, provider behavior or transport cap changed.

- RED: the real settings-service replay returned validation failure. A native
  touch drag committed its first PATCH through `route.fetch`, lost that response
  through `route.abort`, and received 400 on the automatic retry instead of 200.
- GREEN: the settings-service regression covers identical replay, a stale
  conflicting permutation and an independent successor rename without any WS
  dependency. The affected `TestSidebarWorkspace` service group passed with
  `-race`; affected Go lint, native ESLint and Prettier passed.
- Fresh cache-only backend formatting/build and required helper/plugin-fixture
  builds passed in 156.20 seconds. All three changed code/test files byte-match
  the tested mirror; existing R10 web assets are unchanged.
- Native true-touch suite passed 5/5 in one final run, including actual
  committed/lost-response/200-retry/API/reload proof and the retained 400
  conflicting-order and independent-successor controls.

Receipts: `~/.cache/sidebar-multi-tag-build/evidence/r11-*`. The first native
lint invocation pointed outside the cached ESLint base and was rejected;
the corrected relative-path invocation passed. The formatter was reapplied
with the repository configuration, preserving existing formatting.
Real-package R10 evidence remains historical, not a new R11 installation claim.
No physical-phone, PostgreSQL, remote-CI, release or broad-suite certification.
Protected demo and primary instances were untouched. Next round uses Opus.

### Opus post-rebase round 12 corrections

Two Host findings reproduced; Tags had no new finding. Streak remains 0/3.

- A committed move followed by create/delete was rejected before collection
  mutations. The CAS now compares the base with pre-mutation current IDs, or
  the target with natural post-mutation IDs. Real persistence tests cover both
  successors and a different stale permutation rejected without revision/state
  changes.
- Failed local prerequisites followed by guarded no-op acknowledgements left
  phantom drafts/active views. Terminal acknowledgement now adopts the latest
  confirmed snapshot after a local failure, instead of pretending the dependent
  intent persisted. The failure marker survives intermediate acknowledgements;
  error reporting is retained. This is conservative settlement, not automatic
  replay/healing of failed local intent.

RED: both service collection cases failed; scoped store assertions exposed both
phantom states; a native active-view 500 plus draft no-op 200 visibly retained
the uncommitted view. The first store assertion read the unscoped alias and was
corrected before accepting the behavioral RED.

GREEN: 56 focused frontend tests, including one/two queued no-op successors,
group/active-view failures and existing disclosure/replay controls; scoped
types/ESLint; affected workspace service race tests and Go lint. An initial
max-lines warning was resolved by adopting the confirmed snapshot through the
existing snapshot-assignment convention, without raising limits.

Current cache-only backend/helper/fixture and SPA builds passed in 229.61s;
six code/test paths byte-match the tested mirror. Final native desktop case
passed 1/1 (14.8s), proving UI/API/reload agreement after failure/no-op; the
existing true-touch reorder/retry/conflict suite passed 5/5 separately.
The desktop screenshot was not retained after the subsequent project reset;
the proof is the observed browser assertions and API state, not an absent PNG.

Receipts: `~/.cache/sidebar-multi-tag-build/evidence/r12-*`. No new real Tags
package installation, physical-phone, PostgreSQL, remote-CI or broad-suite
certification. Protected demo and primary instances remain untouched. No release
or deployment. Subsequent full-source review uses Opus.

### Opus post-rebase round 15 correction

Round 15 reported one low-severity unreplayed-transition failure lost behind
successful disclosure compaction. Rounds 13/14 were source-only CLEAN on the
previous snapshot; this confirmed finding resets the streak to 0/3.

Four failing-first store cases reproduce selection and overwrite-save failures
at HTTP 400/500 followed by successful compaction: disclosure and authoritative
state settle, but the failure indication disappears. The journal now tracks
`failedUnreplayedWrite` for local, selection and draft-save writes. It survives
intermediate acknowledgements and suppresses terminal error clearing; replayed
view mutations and self-healing compaction remain separate.

Final proof: 60 focused frontend tests, scoped types/ESLint, and fresh frontend
build (51.07s). Three changed code/test paths byte-match the tested mirror.
Existing native desktop save/selection boundaries passed 4/4; separate true-touch
reorder/retry controls passed 5/5. The exact new race is proven at the store/
transport boundary, not claimed as a new browser-interleaving test.
Complexity and duplicate-fixture-literal warnings were fixed with a private
write classification predicate and shared test actor constant; no lint limits
or assertions were weakened.

Receipts: `~/.cache/sidebar-multi-tag-build/evidence/r15-*`. No backend, SDK,
Tags, API, transport-policy or copy changes. No fresh package reinstall,
physical-phone, PostgreSQL, broad-suite or remote-CI certification. Protected
instances untouched; no release/deploy authorization. Next review uses Opus.

### Opus post-rebase round 16 correction

Confirmed the remaining deferred-error overwrite: a newer WS snapshot captured
before a transition failure retains a null error. When queued compaction is
then retired by its admission fence, terminal restoration previously replaced
the current failure with that stale null.

Expanded the existing selection/save 400/500 matrix to cover both persisted and
skipped compaction, with the latter dispatching a real store WS handler before
failure. Four new cases failed at the retained-error assertion before the fix.
Snapshot restoration now preserves the current unreplayed error while adopting
authoritative settings. Normal settlement and replay/healing policies remain.

Final 64 focused frontend tests, scoped types/ESLint/Prettier passed. Fresh
frontend build plus existing desktop save/selection checks 4/4 and separate
true-touch controls 5/5 completed in 142.82s. The exact race is proved at the
store/WS/transport boundary, not claimed as a new browser interleaving. A helper
name was shortened to meet the existing file limit, without suppressions.

Receipts: `~/.cache/sidebar-multi-tag-build/evidence/r16-*`. No backend, SDK,
Tags, API, retry-policy, copy, release or deploy changes. No new real-package,
full-suite, PostgreSQL, physical-phone or remote-CI certification. Protected
instances untouched. Streak remains 0/3; next review is Opus round 17.

### Opus post-rebase round 17 corrections

Confirmed view failures could lose their notice when not actually replayed:
rename intent discarded by skipped compaction, rejected moves followed by a
successful independent write, and uncertain moves retired after endpoint
deletion. Six new matrix cases and the retired-move control failed first.
Deferred snapshots now always preserve the current local error. Journal
settlement marks actually discarded/retired intent rather than only classifying
write kinds; successful replay remains the explicit healing path.

A further failing positive control caught overly eager retirement on an
intermediate local acknowledgement. Only terminal local discard now marks the
notice unreplayed, so a later view write can still heal retained intent.

Final 73 focused tests, scoped types/ESLint and repository-config formatting
passed. Four changed code/test paths byte-match the final build mirror. Final
fresh frontend plus existing desktop checks 4/4 and separate touch controls 5/5
completed in 139.67s. An intermediate artifact run had desktop 4/4 but was
cancelled during touch after production inputs changed; it is not final proof.
Exact new interleavings are store/WS/transport assertions, not a new browser
background-workspace toast test.

Tags README now names the actual desktop section **Plugin groupings**.
Tags production source/package/SDK are unchanged. Receipts:
`~/.cache/sidebar-multi-tag-build/evidence/r17-*`. No new real-package install,
physical-phone, PostgreSQL, broad-suite, remote-CI, release or deployment claim.
Protected instances untouched. Streak remains 0/3; next round is Opus 18.

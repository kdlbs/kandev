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


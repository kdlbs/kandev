---
created: 2026-10-08
status: done
requirements:
  - REQ-UI-FILE-EDITOR-MUTATION-001
system_design:
  - ../../specs/ui/system-design/file-editor-mutation-ownership.md
legacy_specs: []
---

# Implementation Plan: Preserve Editor Workspace Refresh

## Overview

Prevent an obsolete background workspace read from replacing newer editor
content, a reopened buffer or typing made during hashing. One sequential work
order owns permanent behavioral RED, the bounded helper/caller correction and
targeted GREEN. End the design turn with unstaged files; implementation requires
a later explicit ROOT interrupt in the same primary session and local-heavy
lease. No delegation, additional task/session, model change or merge is granted.

## Evidence and specification reconciliation

Authoritative baseline and local design HEAD:
`1257838968f5c92305a858427cdf723a87b0a882`. ROOT supplied three causal assertion
REDs and two passing controls using the real sync helper, Dockview actions/store,
request normalization and hashing, substituting only `client.request` transport.
Original native 65831 was actually joined at 7630ae; wrapper exit 0 reported
actual Vitest exit 1; duration 6.922s. Owned group 1096520 was empty, temporary
test absent and ROOT clean. Accept this proof without replay.

Protected source: `/tmp/kandev-root-editor-refresh-discovery-20261008/candidate.test.tsx`,
mode 0400, SHA256 `410e6932d9c127f2a1d3649987f59d89fd48bb6afb571b823869b9aa93ab2d85`.
Sibling stdout/stderr/receipt/owner files identify the run. Do not modify,
replay, remove, copy or import that source. Permanent tests must be independent.

Source root cause: `syncOpenFileFromWorkspace` captures no admission-time buffer
incarnation or read ordering. It reads `latest` after fetch but before awaiting
`calculateHash`, then publishes using that obsolete object. Concurrent reads can
regress clean content; a same-key replacement can receive another buffer's reply;
typing during hashing can be erased. Both Git-signature and tab-activation
callers use the helper and must share ordering.

The existing UI editor ownership pair explicitly excluded general resync races.
It already owns reusable editor reply publication, unlike workspace disk-save
authority or Files tree/search state. Extend that pair with minimal missing
`AC-UI-FILE-EDITOR-MUTATION-001.7` through `.9`; retain the original six mutation
criteria and their completed delivery evidence. No duplicate incident spec.
Under autonomous planning, latest eligible admission permanently supersedes
older reads even when latest fails; a future real trigger may recover. The
owning design preserves rationale, so `/record` needs no separate ADR.

Assumption check: desired preservation, release boundary and exclusions are
confirmed by the request; caller/instance/visit behavior is source-verified.
No material product decision remains unresolved. Companion-package inventory:
`file-editor-mutation-ownership`, `file-browser-reply-freshness` and
`task-navigation-responsiveness` keep their completed scopes/results. This
package changes neither tablet mutation handlers nor tree/search coordination.

## Scope

### In scope

- Admission/current-read ownership across both background helper callers and
  multiple readers, with independent repo/file keys.
- Existing buffer incarnation, committed session visit/unmount/StrictMode and
  Dockview host checks; tab-activation subscription retirement.
- Live dirty reconciliation after async hashing, current clean/empty/metadata
  reads and current remote reload/title affordances.
- Real helper/store/request/hash tests and faithful hook/provider/panel evidence.
- Owning requirement/design and one pending work order with exact checks.

### Out of scope

- General framework, new store/schema/API/backend payload, dependency, setting,
  retry, fetch abort, save-versus-read ordering or editor rename redesign.
- Tablet restoration and phone viewer reads, layout/copy/touch/navigation/scroll
  changes, browser/build/E2E/full suites and speculative polish.
- Advancing-main rebase, synthetic merged tests, foreign process/cache/worktree
  changes and weakening assertions, race interleavings or delivery gates.

## Technical approach

`file-editors-sync.ts` captures caller/buffer/host ownership before the first
await and keeps one transient publication token per repo-scoped key, shared
across its callers. Validate before transport and after fetch/hash; remove only
the settling request's current token. An absent token cannot revive an older
request. Re-read live state immediately before reconciliation and recheck before
the panel sink. No content cache or additional state action is needed.

`use-file-editors.ts` forwards its existing committed visit ref to the Git-status
sync hook. `file-editor-panel.tsx` adds a committed lifetime guard to its existing
activation subscription and passes it to the same helper; preserve initial
active refresh, true-only activation and cleanup. Reuse existing instanceId,
repo keys, update actions and panel helper without changing persisted descriptors.

| Surface or caller | Identity and transport | Planned verification |
| --- | --- | --- |
| Git-status synchronization in every `useFileEditors` reader | Committed visit, session, repo/file, real `workspace.file.get` normalization | Real provider/hook and store-driven Git signature, current and retired cases |
| Dockview initial-active and activation sync | Committed subscription, captured portal API, same buffer/read token | Real panel and portal manager, actual registered callback and disposal; positive controls |
| Desktop and compact fine-pointer workbench | Shared Dockview buffer and host | Same state-publication tests; no layout change |
| Coarse-pointer tablet `TaskCenterPanel` | Independent local tabs/restoration request | Source inventory only; no repair or runtime claim |
| Phone `MobileFileViewerPanel` | Independent selected-file/fetch path | Source inventory only; no repair or runtime claim |

No provider-specific capability is added. Missing client, retired caller,
missing/mismatched buffer or failed transport leaves existing content intact.

## Tests

| Criteria | Permanent behavioral evidence |
| --- | --- |
| `.7` | Both two-read completion orders, newest failure versus old success, cross-caller/reader overlap, independent files/repos and live peer after another reader unmounts |
| `.8` | Identical same-key reopen/replacement, host replacement, session A-B-A/null/unmount and applicable StrictMode cleanup, retirement during fetch/hash and no panel publication |
| `.9` | Real async hash with typing; matching/nonmatching dirty branches; real current clean/empty/hash/binary/resolved-path metadata; silent transport failure and real reload/title controls |

The work order names exact test cases, instrumentation restrictions and commands.
New owner evidence must use real production helper/store/request/hash; old mocked
control suites may receive only required incarnation/caller fixture updates.
Their passing verdict alone does not prove ownership.

## Mobile and rendered verification

The `/mobile-parity` pure state/data exception applies explicitly: no rendered
composition, navigation, touch, scrolling, copy or breakpoint behavior changes.
Desktop and compact fine-pointer layouts use this Dockview path. Coarse-pointer
tablet and phone reads do not. Faithful helper/hook/panel tests cover publication;
no new mobile Playwright or ASCII composition is needed. Browser/build/E2E/full
suites are not authorized merely because these files are frontend code.

## Work orders

- [x] [Task 01: Bind workspace refresh to its current editor](task-01-bind-refresh-owner.md)

Dependency order: Task 01 only, executed sequentially in this primary session
after ROOT review and later explicit implementation lease.

## Verification results

Implementation and task-defined checks completed on 2026-10-08 after ROOT's
explicit same-primary release. Three named helper regressions and three corrected
Git-caller lifetime cases failed causally before production edits. Final evidence
covers 96 passing tests across eight affected suites: the original combined run
passed six unchanged control suites (65 tests) and the ownership suite; after
repairing two real panel-provider fixtures, the final affected rerun passed all
31 new behavioral tests with zero skips. The first combined run itself exited 1
and is not claimed as green. Affected ESLint, typecheck, i18n check/ratchet,
docs catalog, specification lint and actual product-diff coverage passed.
See Task 01 Results for commands, original failures and joined process receipts.
Normal active hooks and hosted delivery are tracked separately in the live task
plan; this done status records implementation, not a merge or hosted-gate verdict.
Light design checks passed on 2026-10-08: catalog validated 363 decisions and
1437 specifications; specification lint passed; whitespace passed. Actual local
`validateCoverage` read the four real changed docs paths and returned `ok: true`,
`errors: []`, `status: exempt`, `requiresCoverage: false` because this is a
docs-only design diff. It does not prove future product-diff coverage. A separate
check using the repository frontmatter parser confirmed exactly one pending
sequential work order, declared REQ/AC IDs, design/manifest references and plan
links. Implementation must run actual product-diff coverage after its changes.

## Documentation and delivery

Public-docs audit found Files/editor capabilities in
`docs/public/developer-tools.md`; no operator command, configuration, API, label,
navigation, screenshot or public workflow changes. This is a repair of existing
behavior; owning internal specs/plans suffice. No root/scoped AGENTS convention
changes are required. Actual-diff documentation coverage must validate the real
work order, declared requirement and referenced design, including untracked files.

After release, normal active hooks and task checks precede ready PR publication.
ROOT owns serial local-heavy/merge grants. Preserve canonical task PR association
and all five automation flags false, bot-region-safe author-body source/readback,
frozen-head required contexts and successful Backend/Frontend/E2E parents,
substantive current-head/all-files CodeRabbit App 347564 review and complete
finding disposition. Use one original bounded all-terminal `scripts/pr-await`
and actually join it. At most one necessary full review request after inspecting
automatic coverage. No merge before a separate ROOT grant; normal expected-head
squash, actual tree/blob/remote checks and only owned cleanup follow that grant.
The live task plan preserves exact completion gates and process receipts.

## Risks

- Per-caller request maps would miss cross-caller races; global counters would
  incorrectly supersede unrelated files. The shared per-key pending map is local
  to this helper's single Dockview result owner.
- Cleanup must not remove a newer token or reauthorize an older one after failure.
- Whole-object guards reject valid typing; hash/content equality misses reopen.
- Passive-only retirement can leave stale subscriptions writable until cleanup.
- Existing mocked fixtures lack genuine lifetime evidence and need precise
  compatibility updates; they must not become the only regression checks.

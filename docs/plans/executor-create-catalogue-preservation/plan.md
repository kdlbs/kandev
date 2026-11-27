---
created: 2026-10-10
status: implemented
requirements:
  - REQ-EXECUTORS-CREATE-CATALOGUE-001
system_design:
  - ../../specs/executors/system-design/executor-create-catalogue-publication.md
legacy_specs: []
---

# Implementation plan: Preserve live choices during executor creation

## Overview

Publish the accepted executor over the current owning catalogue, preserving
independently received choices. One sequential work order independently proves
the real-path regression, makes the narrow acknowledgement correction and
checks its transport/routing compatibility. ROOT completed the full actual-file
design review and later explicitly released implementation in the same primary
with its exclusive local-heavy lease. Hosted review and merge remain separate
external checkpoints.

## Scope

### In scope

- Accepted owner creation from `/settings/executor/new` with HTTP and WS.
- Every unrelated current owner/profile membership, relative order and value,
  including mixed live/deleted inventory and survivor updates during creation.
- Accepted descriptor exactly once at the existing final creation position,
  ordinary payloads, pending state, real return route and final cleanup.
- Independent integration tests through the real route, provider, transport
  adapters, events, router, owner fallback, options hook and selector.

### Out of scope

- Backend, registry, providers, global revisions, selection and launch changes.
- Speculative preservation of same-created-owner profiles before ACK;
  timestamps, request-lifecycle frameworks and notification-writer redesign.
- Catch/toast/retry changes or a claimed passing rendered rejection control.
- Markup, copy, layout, touch, scrolling, breakpoint or navigation redesign.
- Other owner/profile creation flows, new dependencies, full builds or suites.

## Technical approach

Qualified baseline: clean `ec1bdd573197cb8620364d3a1b638b2973b2812e`.
`ExecutorCreatePageContent.handleCreate` captures an executor array before its
creation await. The captured filter-and-append publication can discard new
choices, restore deleted owners and overwrite survivor values. The route is
reserved by real `renderSettingsRoute`, not a dead helper.

Change only `apps/web/app/settings/executor/new/page.tsx` to acquire
`useAppStoreApi`, read the current owning store after successful transport and
synchronously publish the current array filtered by accepted ID, then the
accepted descriptor last. Remove the captured catalogue/setter subscriptions.
Keep `buildExecutorConfig`, payload, connection selection, action, `try/finally`
and real `/settings/executors` push intact. No await in the read/write span.

| Transport | Real boundary | Accepted result | Verification |
| --- | --- | --- | --- |
| No WS client | `createExecutorAction`, POST `/api/v1/executors`, JSON, no-store | HTTP 200 bare owner DTO | Ordinary and held external-fetch cases |
| Existing WS client | `request<Executor>("executor.create", payload)` | Same owner DTO | Ordinary and held request cases |
| Owner event before ACK | Registered `executor.created` handler, canonical typed payload | Current created-ID row replaced with accepted descriptor once, last | Independent notification control per transport |

The existing page supports `local_docker` and `remote_docker`; keep both types'
current defaults and configuration semantics. Do not imply support for another
provider/shape. Backend creation emits owner data and does not automatically
create profiles. No same-created-owner profile merge is proposed.

## Tests

Own `apps/web/app/settings/executor/new/executor-create-catalogue-publication.test.tsx`
and independently authored `executor-create-catalogue-publication.test-helpers.tsx`.
The intended bounded matrix has six rendered cases, each transport paired:

| Test name | Criteria | Required observations |
| --- | --- | --- |
| `ordinary accepted creation retains payload and choices` (HTTP, WS) | .2, .3, .4 | Exact action/payload, request admitted, disabled pending Create, unchanged choices, accepted descriptor once, real listing route |
| `accepted creation preserves mixed live catalogue changes` (HTTP, WS) | .1, .3, .4 | New unrelated owner/profile, removed owner, survivor metadata/profile update and profile removal; current inventory visible while held; exact current values/order and actual selector choices retained after ACK |
| `owner notification before acceptance keeps the accepted descriptor once and last` (HTTP, WS) | .2, .4 | Registered accepted-owner event before ACK, accepted DTO normalization, one owner row at the existing final position, unrelated choices unchanged; no fabricated automatic profile |

Preserve `is_system`, valid canonical types, required profile scripts and
timestamps in typed fixtures. Assertions must reach both catalogue and actual
rendered options with owner metadata fallback and eligibility intact. The
ordinary and uniqueness cases are controls, not assumed causal failures.
Independently obtain the mixed-inventory RED on unmodified production source
for the precise live-choice loss/deleted-choice resurrection reason; fixture
exceptions or unhandled errors do not qualify. Then GREEN the same matrix.

The existing click-rejection promise is unhandled because there is no catch.
Do not swallow it to fabricate a rendered failure PASS. Any explicitly awaited
real adapter rejection is transport-only, optional evidence; it cannot prove
rendered error/finally behavior. Existing error policy is preserved by the
unchanged control flow, with no new rejection UI claim.

## Rendered and mobile evidence

Mount the actual `renderSettingsRoute` result with a pathname-subscribing test
host, `StateProvider`, real registered event handlers, adapter/connection,
router and downstream `ExecutorProfileSelector`. Hold only external fetch or
the selected WS request. Observe real successful SPA return, current catalogue
and open actual selector choices. Settle held successful work, restore
connection/history/guards, unmount portals/providers and drain owned timers.

This is state/data normalization inside an existing surface. Mobile-parity's
explicit exception applies: desktop combobox and existing touch drawer consume
the same preserved data; no presentation, interaction or responsive change is
planned. Targeted rendered integration tests provide the end-to-end evidence
for this slice. No new Playwright project/spec, UI sketch or build is required.
Reassess the exception if implementation changes those assumptions.

## Work orders

- [x] [Task 01: Publish accepted creation over the current catalogue](task-01-publish-current-catalogue.md)

Dependencies: none. Execution is sequential in the SAME primary session.

## Evidence and resource checkpoint

Allowed receipt: `/tmp/kandev-root-executor-create-discovery-20261010/qualified-proof.json`.
It records one causal failure and two passing HTTP/WS controls through the real
route, POST/WS, events, store, router and selector. Original native handle
`66070` actually joined as `960f80`, exit 1. ROOT reported wrapper `3109221` /
group `3109222` absent, current owned process absence, clean root and checksum-only
scratch removal, with global-heavy returned. These are supplied qualification
receipts, not a test run by this primary. Never read/copy/delete the protected
`candidate.test.tsx` beside that receipt. No duplicate command or historical
process audit. This design starts no owned heavy process or hosted observer.

## Execution and delivery gates

The later explicit ROOT implementation interrupt and exclusive global-heavy
grant are required before installation, product tests, lint, typecheck, builds
or hooks. Remain in task `56b628d0-84d4-41b6-aa30-99a4f02cdebe`, session
`d0a42d0d-09fb-481c-9d15-78464a6f1038`. No native delegates, recursive tasks,
new sessions/tabs or model changes. Preserve others' edits, managed worktrees,
dependencies, shared caches, foreign processes/refs and protected resources.
Retain original native handles and actually join each to terminal before
current owned-process cleanup and global-heavy RETURN. No blind retry for
unknown resource/auth/transport/repeated failures; checkpoint ROOT. Critical
parent questions end the turn immediately. Callback delivery is optional.

After later release and passed local checks, ordinary commit/push/ready PR is
already authorized in the same implementation turn. Keep normal hooks; no
bypass, rebase for drift or optional-polish pushes. Verify template/live body,
canonical repository `16026b06-bd79-47c0-aed1-dc7ca95f63d9`, task association and
complete readback. Do not mutate the five automation switches already FALSE:
`auto_fix_enabled`, `auto_merge_enabled`, `prompt_on_closed`, `prompt_on_merged`,
`prompt_on_review_requested`. Link the original PR once only if needed.

After actual local joins/cleanup and heavy RETURN, retain one original GNU
91-minute TERM / 10-second kill observer running `scripts/pr-await` for
90 minutes, all-terminal, cadence 60 seconds. Preserve its native handle and
clock across interrupts/pushes/main advance; no duplicate ROOT timer. At normal
deadline only one conditional healthy-pending renewal is authorized, after
actual join/current cleanup. Per-job-name retry budgets persist across heads.
Read automatic substantive reviews while CI runs; no manual full bot request
per addition/push/SHA. Use focused re-review only for a material concrete
correctness/security/architecture gap. Batch actual valid corrections before
one push; optional style/docstrings/optimization/docs polish cannot gate.

READY requires fresh complete error-free current-head evidence: all six
required contexts and actual Backend/Frontend/E2E parents SUCCESS, adequate
substantive review, zero actionable visible/hidden/human findings and clean
head. ROOT's one static current-main/head compatibility check precedes a
separate serial expected-head normal-squash grant. No merge before that grant,
no admin/bypass or explicit branch deletion. Use exact remote
`refs/heads/main` and exact candidate branch, not suffix-matching short refs.

Keep paused task `a6032d95`, primary `7974d087`, its serialize-workspace
worktree and anonymous volume
`2c48e791f0a8b8e64e6ecd30db0ede17388b572d4a303d39e2e0ee3fa7573ea7`
untouched; never read/delete/prune the volume. ROOT's proof release requires
actual merge, independent fast-forward, archive and board absence. Another
child's original hosted handle `71386` is not owned or adopted here.

## Verification results

### Implementation

The implementation matches the reviewed current-store publication and all four
criteria. The requirement/design are active/current and the sole order is done.
Only the creation page, independently authored test/helper and four package
documents changed. No protected proof access, backend/provider/event-writer/
selection/launch change, new-owner profile claim, rejection UI redesign,
delegate, new task/session or model switch.

The conditional frozen install ran once from `apps/` and passed. Its wrapper
actually waited the original child, recording exit 0 and owned-group absence;
the native session ID was omitted from projection. ROOT independently accepted
that original wait/terminal evidence. No native ID or native join is invented,
and no duplicate install or recovery audit was run.

Original-source ordinary accepted HTTP/WS controls passed 2/2. The first full
six-case run passed four controls but had two pre-ACK fixture failures from an
owner-update payload incorrectly containing profiles. It is unqualified RED.
After canonical owner-payload correction, only the two mixed cases reran on
unchanged source: both failed after ACK for the expected current-store and real
selector loss/value reversion/deleted-choice resurrection, with no unhandled
errors. This is qualified causal RED. Minimal publication correction and scoped
GREEN passed all six cases, including accepted-owner-event-before-response
controls. Production code did not change after that GREEN.

Formatting, scoped three-file ESLint, typecheck and `i18n:check` passed. Owned
source/new tests/helper were staged before the existing i18n ratchet. Its first
run caught two helper fixture strings. The test's persisted submitted name
received a reasoned exemption and its selector placeholder reused an existing
translation; affected helper formatting/ESLint and final staged ratchet passed.
No extra gate or broad passing replay was added for that copy-only correction.

Original full native results, actual terminal joins, argv/cwd/clocks and
current-owned-group absence receipts are preserved in
`/tmp/kandev-child109-20261010/`. All yielded local handles through validation
were actually joined; direct terminal commands have no live native session.
Commit/publication, heavy RETURN, sole hosted observer and final CI/review/
ROOT static/merge receipts remain in the external task plan, without a
post-publication documentation-only push. No merge authority is implied.

Final lightweight documentation gates and actual seven-path coverage are
recorded in the work order. Public documentation and mobile/browser/screenshot
changes are unnecessary under the reviewed state/data exception.

### Historical design checkpoint

DESIGN_READY at baseline `ec1bdd573197cb8620364d3a1b638b2973b2812e`.
At that checkpoint all four files were unstaged/uncommitted;
requirement/design/manifest draft and the order pending/unimplemented.

- `python3 scripts/list-docs.py validate`: PASS, 368 decisions / 1507 specifications.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- Unfiltered tracked/staged/untracked inventory and exported `validateCoverage`
  preflight using existing
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`: PASS,
  `status: exempt`, `ok: true`, `errors: []`. No environment mutation or
  network/status publication. Actual inventory contains exactly the four
  design files, with no unexpected paths. Independent preflight assertions
  confirm exactly one changed work order and complete REQ/four-AC/requirement/
  design/plan references even though documentation coverage is exempt.
  This is not production coverage evidence.
- Independent lightweight document inspection: PASS, four local-link sets,
  REQ/AC/design traceability, exactly one work order, valid draft/pending
  statuses and whitespace including the untracked documents.
- Owning catalogue discovery includes both new spec paths; final git status
  contains only the four new design documents. Documentation diff check passed.

No production or permanent test edits; no installation, product tests,
product lint/typecheck/build/hooks, commit, push, PR, observer or merge performed
in this phase. No owned asynchronous handles or processes were started.
Public documentation needs no edit for the proposed internal publication fix.

## Risks

- Reading before acceptance or awaiting between current read and write restores
  the original loss. Replace-in-place upsert would alter existing creation order.
- Fixtures bypassing routes, canonical events, adapters, fallback metadata or
  the real selector can miss the observed defect. Keep meaningful RED/controls.
- Same-created-owner profiles before ACK are unproved adjacent scope; require
  independent causal evidence before any bounded design/implementation change.
- A rendered rejection control cannot pass cleanly under the unchanged error
  policy. Report transport-only evidence honestly and add no error redesign.
- Main/source can move before release. Reconcile current contracts in place;
  preserve unrelated work and do not widen the fix to other writers.

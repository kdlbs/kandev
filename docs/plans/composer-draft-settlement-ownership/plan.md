---
created: 2026-10-07
status: completed
requirements:
  - REQ-UI-SESSION-REFRESH-EFFICIENCY-004
system_design:
  - ../../specs/ui/system-design/session-refresh-efficiency.md
legacy_specs: []
---

# Implementation Plan: Composer Draft Settlement Ownership

## Overview

An accepted send from conversation A can erase conversation B's pre-existing
identical draft from the live editor and browser-tab storage after the operator
switches sessions. Admit clearing only for the originating committed composer
visit. Deliver the regression and local hook correction in one sequential work
order. ROOT reviewed the sealed package and released implementation in this
same primary on 2026-10-07 with exclusive local-heavy capacity.

## Ownership and settled assumptions

UI owns the existing reusable composer reply/draft lifecycle, defined by
`REQ-UI-SESSION-REFRESH-EFFICIENCY-004` and its paired design. Tasks retains
message delivery and claimed attachments. Clarify the existing `.3` draft
preservation outcome with `.4`–`.6`, rather than introducing an incident
requirement or a parallel task/UI contract. The earlier
[session refresh plan](../session-refresh-efficiency/plan.md) remains the owner
of performance work; its Task 05 remains incomplete.

Confirmed by ROOT: preserve current-owner successful clear, failed submission
preservation, generated payload/snapshot semantics and browser-local ownership.
Verified in source: both session switching and keyed composer replacement can
retire a visit; `clearAcceptedPayload` reaches the same finalizer. No unresolved
product choice blocks this package. The local visit-admission implementation is
a routine enforcement of existing ownership and requires no new ADR.

## Evidence and root cause

ROOT's independent reproduction used clean main
`aad793bfa93e637e74d5d74d4cf194bc3d02f23b`, matching this worktree's design base.
Both sessions had saved `Continue`; submit A, commit B in the same hook/editor
instance, then settle A's callback as accepted. B became blank in hook state,
the actual editor, saved text and rich content, and remained blank on remount.
One causal test failed and current-owner acceptance/rejected-retired-send
controls passed (three cases, 141 ms, 4.42 seconds overall).

`clearSubmittedInput` compares captured trimmed text against mutable current
text without checking visit ownership. It then dereferences the current editor
and state refs. The editor's `clear()` calls B's current `onChange`, persisting
blank text to B, while the captured storage clear also addresses A. Equal text
is a valid user action and cannot identify the owner.

ROOT archive `/tmp/kandev-root-chat-draft-owner-next-candidate.test.tsx`, SHA256
`7618af2d13c8886e85aa760c630f8450d10de02881c71224a9b2c6b2d856d9f9`, was inspected
read-only. Never replay, copy, import into permanent tests, mutate or delete it.
Parent receipt/classification/log are `/tmp/kandev-root-chat-draft-owner-proof-`
with suffixes `receipt.json`, `classification.json`, and `log`. Native session
44836, start chunk 3e01f9 and terminal e43de5 actually joined; wrapper exit 0
and actual Vitest exit 1 differ. Wrapper 727420 and command PID/PGID 727497 were
proven gone by ROOT. CHILD65 did not execute that proof. Its scope is the actual
production hook/editor/providers and deferred acceptance callback, not the
default message handler, WebSocket, browser, or attachment path.

## Scope

### In scope

- Local committed-visit admission for `handleSubmit`, asynchronous accepted
  clearing and retained `clearAcceptedPayload` callbacks.
- Real production hook/editor/store/provider regression plus rendered shared
  composer entry-point coverage; browser-tab storage and remount assertions.
- Both session directions, A-B-A, task/session replacement, unmount/cleanup,
  StrictMode, current-owner controls and independent different-session inputs.
- Existing attachment, structured payload, context and readiness controls;
  owning requirement/design and delivery-record reconciliation.

### Out of scope

- Transport cancellation/replay, default handler or backend/WS changes, global
  ownership service, storage-schema migration, new flag or dependency.
- Cache, context-node, broad attachment lifecycle, send-busy/toast, raw public
  `clear()` API or dynamic-ref caller redesign. No inferred additional defect.
- Editor CPU/mount optimization or closure of the earlier performance task.
- Layout, navigation, copy, localization changes or a new browser fixture.

## Technical approach

Keep production changes in the shared
`apps/web/components/task/chat/use-chat-input-state.ts` and its small local
`use-chat-draft-visit.ts` helper. Use one per-instance
rendered owner identity for `(taskId, sessionId)` with layout-effect committed
admission and an opaque active visit token. Typing, `isSending`, workspace
resolution and ordinary rerenders do not retire a visit. A committed owner
change or cleanup does. A new visit to A must not revive A's old token.

Before reading mutable refs or calling `onSubmit`, reject a callback whose
rendered owner is not the active committed owner. Capture its active token
before submit, then check that token again at accepted settlement before any
clear/state/history/height/storage side effect. Invalidate outstanding work on
unmount and StrictMode cleanup, without exposing speculative renders through
render-time ref mutation. Apply the same check to accepted-payload clearing;
stale callbacks return `false`. Preserve synchronous success behavior, including
a submitter that commits replacement before returning acceptance.

After admission, retain existing trimmed-text and ordered
`id:deliveryMode` attachment comparisons, accepted-payload descriptor matching,
newer-text preservation and text-only clear when attachments changed. Keep
current attachment upload/restore/delete, context capture and payload building
unchanged. The local helper also holds the unchanged attachment snapshot and
text-storage clear functions to keep the existing hook within its file limit.
Do not use document equality or revision tracking to alter the
existing same-visit matching policy.

The owner must distinguish the callback's rendered identity from the token
captured when work starts. A single mutable ref read by an old callback at
invocation would let that callback adopt B's token. A session-ID-only check
would revive A's pending completion after A-B-A. Cleanup/setup must create new
admission instead of reviving a token captured before cleanup.

## Consumer and readiness audit

| Consumer | Source boundary | Consequence for verification |
| --- | --- | --- |
| Phone | `mobile/session-mobile-layout.tsx` mounts unkeyed `TaskChatPanel` | Session props can change within the instance |
| Tablet | `SessionTabletLayout` → `TaskCenterPanel` → unkeyed `TaskChatPanel` | Same hook correction; no touch geometry change |
| Shared chat | `ChatInputArea` → `ChatInputContainer`, keyed by clarification counter | Cover session reuse and actual replacement |
| Desktop/preview/Threads/run/Quick Chat | Existing shared composer consumers | Cover local instances without expanding their workflows |
| Opening-prompt recovery | `clearAcceptedPayload` exposed via container handle | Retained old callback rejected; current match still clears |

`useChatInputContainer` retains current editable/startup/submittable gates;
`submitDraft` retains `isSending`, content and ready-attachment checks. TipTap's
handle changes with current `onChange`; clearing therefore requires admission
before the actual handle call. Draft persistence remains sessionStorage per
session, with rich JSON separate from text and previews reconstructed on load.
Different app stores share browser-tab storage by design; isolation tests use
different session IDs, not a fabricated per-store storage namespace.

## Tests

Create `apps/web/components/task/chat/chat-input-draft-ownership.test.tsx` from
the behavioral requirements and existing test conventions, independently of
ROOT's protected archive. Use production `useChatInputState`, real `TipTapInput`,
`StateProvider/createAppStore`, `ToastProvider`, and actual sessionStorage.
Use scoped per-composer handles. Defer only submission acceptance. Do not mock
the owner mechanism, editor, persistence or providers.

| Acceptance | Meaningful RED/GREEN or control |
| --- | --- |
| `.3`, `.4` | `preserves an identical destination draft on retired acceptance`, parameterized A→B and B→A; assert live value, rendered editor, saved text/rich content/attachments for both IDs and destination remount |
| `.4` | `returning to A does not revive the old acceptance`; `replacement for the same session retires pending acceptance`; `retained submit and accepted-payload callbacks cannot adopt a successor visit` |
| `.4` | `unmount preserves stored drafts on late acceptance`; StrictMode current-success control plus an outstanding submission retired by effect cleanup before setup |
| `.5` | Unchanged current success clears once; false and rejected promise preserve; same-visit newer typed text persists; synchronous success and synchronous owner replacement before acceptance |
| `.5` | Same-visit attachment change keeps new descriptors while matching text clears; unchanged snapshot clears both; outgoing review/inline/entity fields stay equal to captured payload; existing accepted-payload matching controls |
| `.6` | Two independent composers and separate store providers for different sessions: accepting one changes only that composer/session; an ended visit never replays transport |
| `.4`, `.5` | Real `ChatInputContainer` body/hook/editor submission control: commit session-prop switch without key change, accept old send, assert destination persistence/remount; current-visit entry-point success still clears |

Seed ready attachment descriptors; upload requests are not needed to test this
finalizer. Use real editor commands/events for newer typing and rich content,
and read `getChatDraftText/Content/Attachments` before and after settlement.
Preserve source rich JSON exactly for the retired visit tests; avoid a test
whose initial rich content was already null. Explicitly settle/reject all
deferred promises, unmount owned trees and release timers; no external HTTP or
WS request should escape the declared fixture boundary.

Existing `use-chat-input-state.test.ts` and accepted-payload tests provide
attachment/readiness/immediate-insertion controls, but their mocked handles do
not substitute for the real editor regression. Existing
`chat-input-area-context-submission.test.tsx` preserves PR4281's actual context
snapshot behavior without modifying its implementation.

## End-to-end and mobile assessment

The new rendered component entry-point test executes the actual shared composer
through its submit control, real hook/editor/store/provider and persistence,
holding only acceptance. Run it for desktop, phone and coarse-pointer tablet
conditions if those select distinct composer-body branches. That is component
integration evidence, not default transport or full-browser evidence.

No new Playwright is required under mobile-parity's pure state/data exception:
the correction does not change rendered markup, copy, controls, touch behavior,
scrolling, navigation or breakpoints. The nearest shipped exemplar is the
existing `SessionMobileLayout` chat surface. Its inline composer, session picker,
single transcript scroller and safe-area behavior remain. No ASCII preview is
needed because this package changes admission only. Visual/browser verification
was not run at the design checkpoint. Implementation uses the actual component
entry point at desktop, phone and coarse-pointer tablet conditions; it does not
claim a default transport or browser run.

## Public documentation audit

Reviewed `docs/public/developer-tools.md` Quick Chat and reference-draft guidance,
the public mobile guide, root README and screenshot catalog. This repairs an
existing preservation guarantee without changing workflows, terminology,
commands, configuration or screenshots. No public-doc edit is planned. Internal
requirements/design/plan records are updated; do not publish an incident page.

## Work orders

- [x] [Task 01: Admit draft clearing by committed visit](task-01-admit-draft-clearing-by-visit.md)

Dependencies: none. Execute sequentially in this same primary conversation;
no delegation is authorized. There is exactly one new work order.

## Verification results

Design checkpoint, 2026-10-07:

- `python3 scripts/list-docs.py validate`: pass, 357 decisions and 1416 specs.
- `python3 scripts/lint-spec-files.test.py`: pass, 36 standalone document tests.
- `python3 scripts/lint-spec-files.py --all`: all specification files pass.
- Standalone `pr-docs.cjs` reference preflight: `covered`, `ok: true`,
  `errors: []` for the one new work order and representative future production
  hook path. This validates document wiring, not implemented source/tests.
- `git diff --check`: pass. Separate lightweight checks include untracked
  Markdown whitespace/relative links, AC references and exactly one new work
  order. Protected ROOT archive SHA256 still matches.
- `git status --short`: four modified existing documentation files plus the
  new two-file plan directory. Nothing staged; no `apps/` changes.

Historical design handoff: all standalone design checks returned terminal exit 0.
At that checkpoint, production implementation,
permanent RED/GREEN, install, package checks and rendered verification were not
run. Dependencies were absent and CHILD65 had no local-heavy lease. The exact
later commands and process-accounting rules are in Task 01. DESIGN is ready for
ROOT review; stop here until its later explicit implementation instruction and
exclusive heavy grant. No approval prompt or automatic continuation.

### Implementation checkpoint, 2026-10-07

ROOT's later release admitted the same one sequential work order. The local
committed-visit correction and independently authored real-editor regressions
are complete. Final affected tests pass (116 tests in six files, including 27
new behavior cases); scoped ESLint with zero warnings, typecheck, i18n checks
and ratchet pass. Catalog/spec lint and actual ten-file documentation coverage
pass; both changed work-order references are accepted with no errors.
See [Task 01 Results](task-01-admit-draft-clearing-by-visit.md#results) for
causal RED, retained receipts, fixture/lint corrections and scope limits.
The earlier performance investigation remains incomplete. Publication, hosted
review/CI and any later serial merge are separate platform delivery gates.

## Risks

- Render-time ownership publication can invalidate live work for an abandoned
  render; passive-effect admission can leave a committed replacement exposed.
- Same-ID or reusable boolean admission can revive old callbacks during A-B-A
  or StrictMode replay. Capture a token per admitted operation.
- Existing large hook size may require a small local helper; do not use lint
  pressure to introduce a framework or refactor unrelated attachment logic.
- Overbroad ownership gating could suppress legitimate current-success clear.
  Preserve and prove current-owner and generated-payload controls.
- ROOT owns local-heavy scheduling. Missing dependencies need one pinned frozen
  install later; unknown/resource/timeout outcomes are checkpoints, not passes.

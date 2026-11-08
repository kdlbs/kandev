---
created: 2026-10-27
status: implemented
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-004
system_design:
  - ../../specs/agents/system-design/background-work.md
legacy_specs: []
---

# Implementation Plan: Background work clear window

## Overview

Restore new command output after a local clear when the browser's bounded rolling
window is full. One sequential work order owns the rendered regression and the
client-side correction. The agents system owns workload identity and the viewer's
output contract; its existing requirement and design are the authoritative pair.

The qualification receipt at base `04c4d5d022911e4416f0dfa6a6c67d894268ec16`
reports a real panel, provider, hook, and rolling store producer in HappyDOM:
after Clear view and a fresh append the store retained the marker at the full
200 * 1024 window, but the rendered viewer stayed unavailable. The identical
no-clear control passed. This is not Chromium evidence. The independently authored
work-order regression must reproduce the causal assertion and control itself.

## Scope

### In scope

- Local clear position across browser window rollovers, repeated clears, search,
  auto-scroll, truncation indication, and selected identity isolation.
- Focused rendered desktop regression and the existing phone output drawer check.

### Out of scope

- Producer cap or protocol, backend, feature flag, settings, stale-event or save
  behavior, UTF-8 truncation changes, and new copy, layout, or phone controls.
- Broader reconciliation or snapshot/lifecycle hardening.

## Technical approach

`session-runtime-slice.ts` appends the chunk and slices the retained string at
200 * 1024 JavaScript UTF-16 code units, flagging truncation. A full window stays
the same length on later appends. `WorkloadDetailView` currently captures that
length in `clearedOffset` and always slices at it, hiding all later output.
The client copies `chunk.offset` directly; backend merge uses Go byte lengths,
so the transport value is not a JS string index.

The runtime store owns a per-session/workload client-only sidecar with run,
generation token, and evicted JS code-unit count. A store-local scalar allocates
fresh tokens. Each view owns an anchor for identity, generation and the current
`evicted + output.length` at Clear; it slices at the anchor end minus current
evictions, clamped to the retained length. Repeated clear replaces that anchor.
Append increases evictions by the actual sliced code-unit count atomically.
Identical accepted list output and no-output lifecycle updates preserve metadata
and anchors; rejected updates do not change them. A different accepted output
replacement or run creates a new generation with zero evictions and the old
anchor no longer applies, displaying the replacement. A removed workload or
cleared/purged session loses its sidecar; re-addition gets a fresh token. A
session/workload selection change cannot reuse an anchor. The
[design](../../specs/agents/system-design/background-work.md#content-and-accounting)
defines the transitions. Do not infer overlap or use transport offsets as JS
indices, retain historical copies, or expand the public DTO, wire, or schema.
Preserve search, truncation and auto-scroll; each view clears independently.

The store-subscribing clear-output hook lives in
`hooks/domains/session/use-cleared-background-output.ts`, following the scoped
frontend hook ownership rule. The panel imports the same selection and anchor
behavior; the work order records focused correction checks.

## ASCII UI preview

UI-01: Desktop detail after clear, then a fresh append. Entry: select a workload
from the existing desktop background-work summary. AC-004.1, AC-004.2,
AC-004.5. Before correction, the output region continues to show unavailable;
after correction it shows the fresh marker while the truncation notice persists.

```text
Current after rollover             Intended after rollover
+---------------------------+      +---------------------------+
| Search | Pause | Clear    |      | Search | Pause | Clear    |
| [earlier output truncated]|      | [earlier output truncated]|
| Live output unavailable   |      | fresh marker              |
+---------------------------+      +---------------------------+
```

Toolbar is fixed above the output scroller; labels are illustrative and actual
copy remains localized. Repeated clear returns the output area to its existing
empty state until new output arrives. UI-02: phone entry is the existing chip,
then summary row and output drawer with Back; the live implementation has no
Clear view control there. It displays the shared retained output and has one
output scroller; no phone layout or control is added by this repair. The nearest
shipped exemplar is the existing background-work drawer and its inset picker
pattern (`mobile-picker-sheet.tsx`). This differs from the aspirational
full-height phone design in the owning draft; closing that existing gap is out
of scope. Check the real drawer at phone width and do not claim phone clear.

```text
+-----------------------+
| Back   Build watcher  |
|                       |
| fresh marker          |  <- output scroll region
+-----------------------+
```

## Tests

`components/task/chat/background-work/background-work-clear-window.test.tsx`
will own AC-004.1, AC-004.2 and AC-004.5: real `BackgroundWorkPanel`,
`StateProvider`, `useBackgroundWork`, and store action; mock only the HTTP
workload list. Establish independent pre-fix causal RED and no-clear control,
then GREEN for pre-cap, rollover including eviction beyond the boundary, repeated
clear, search, truncation, identity, and auto-scroll behavior. Include multibyte
and supplementary characters to distinguish JS code units from wire bytes.
Add a focused phone-width rendered check of the actual chip drawer and its output
with the real store; it must not imply a phone clear control.
The new unreviewed suite belongs to Vitest's `browser-locales` project
(HappyDOM). Confirm nonzero collected tests; `passWithNoTests` is false.

`lib/state/slices/session-runtime/background-work-clear-window.test.ts` directly
exercises the owning real store actions: pre-cap and rolling append including a
large chunk and non-ASCII, identical snapshot, omitted-output lifecycle update,
different accepted replacement, run change, rejected update, list removal,
clear and session purge, isolation and re-addition. It is a new unreviewed suite
in `browser-locales`; the existing `purge-session.test.ts` belongs to `browser`.
Assert actual state and sidecar lifecycle, not just helper calls. Confirm
nonzero discovery in each project.

## E2E tests

The change is limited to state bookkeeping inside the existing desktop detail
view; it changes no layout, touch, scrolling, navigation, or viewport-specific
interaction. The real-producer rendered integration is the primary user-action
evidence; no new mobile Playwright flow is prescribed because the existing phone
drawer does not offer Clear view. A phone-width rendered check verifies its
existing available output flow, but is not a substitute for Chromium or proof of
mobile clear. The missing mobile clear capability is an existing draft-design gap
outside this bounded repair.

## Work orders

- [x] [Task 01: Restore output after local clear](task-01-clear-window.md)

## Verification results

Independent rendered RED: 1 causal failure and 1 control pass, actual exit 1.
Focused GREEN: 10 `browser-locales` tests and 7 `browser` tests passed.
Scoped ESLint and canonical web typecheck (6 GiB heap) passed. The actual phone
drawer was exercised in HappyDOM; real Chromium geometry was not checked.
Staged `pnpm run i18n:ratchet` and `pnpm run e2e:sleep-ratchet` passed;
the staged PR-documentation evaluator reported `covered`. Catalog validation,
specification lint, and diff checks passed. The owning background-work pair
remains draft because it also describes broader unfinished work.

## Risks

- Snapshot replacement and run changes are distinct from appends; do not let an
  old clear anchor leak to another identity or mistake backend byte offsets for
  client positions. Broader snapshot ordering is not part of this repair.
- HappyDOM validates rendered behavior, not real browser geometry or mobile clear.
- Public docs search found workload cleanup, but no documented clear-view behavior
  requiring correction. No public documentation change is planned.

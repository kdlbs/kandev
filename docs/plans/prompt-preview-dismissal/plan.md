---
created: 2026-10-22
status: done
requirements:
  - REQ-UI-PROMPT-ALIAS-001
  - REQ-UI-PROMPT-ALIAS-002
system_design:
  - ../../specs/ui/system-design/prompt-alias-rendering.md
legacy_specs: []
---

# Implementation Plan: Prompt preview dismissal

## Overview

Keep an explicitly dismissed host prompt preview closed when an older Radix
hover/focus timer fires. One sequential work order establishes permanent causal
RED, applies the local reconciliation, and verifies desktop and native phone paths.
The user authorized bounded planning and implementation in this session.

## Scope

Host prompt hover preview, focused regression coverage, existing owning UI pair,
and this plan/work order. No shared primitive, parser, task mention, plugin,
backend, API, permission, or persistence changes.

## Technical approach

The supplied probe uses actual StateProvider, PromptMentionChip, HoverCard and
installed Radix: pointer entry schedules an open; click opens immediately;
Escape closes; the old callback reopens. Ordinary hover and click/Escape without
a pending hover are passing controls. Independently prove this before editing
production. Reconcile delayed opens with a local ref revoked on close and
rearmed by fresh trigger entry/focus. Preserve immediate toggles and Radix
pointer/content transit. Exclude the trigger from outside pointer-down dismissal
so a second click toggles closed once. Keep native Drawer behavior unchanged.

## ASCII UI preview

### UI-01: Desktop prompt inspection

Entry: a saved-prompt alias in a transcript or Host prompt row.

```text
[@daily] -> hover/focus delay or activation -> [saved definition]
Escape / outside / toggle close -> [@daily] (closed)
Older open callback -> [@daily] (still closed)
New pointer/focus entry or activation -> [saved definition]
```

### UI-02: Phone prompt inspection

Entry: tap the existing 44px alias trigger.

```text
Transcript [@daily] -> tap -> bottom Drawer
+--------------------------+
| Custom prompt: daily     | fixed existing header
| Saved definition         | internal scroll body
+--------------------------+
Dismiss -> transcript [@daily]; tap again -> Drawer
```

Structure and behavior are required; spacing is illustrative. The existing
Drawer is the nearest shipped mobile exemplar, appropriate for temporary
inspection. It retains its sole content scroller, dynamic viewport bound,
safe-area padding and touch targets. No composition or copy changes.
Both views map to AC-UI-PROMPT-ALIAS-001.1 and 002.4.

## Tests

`prompt-mention-preview.test.tsx` exercises real store/component/primitive behavior
with a controlled clock, mocking only viewport selection. Primary protection:
AC-UI-PROMPT-ALIAS-001.1 and 002.4. Include pending hover/focus then explicit
close, ordinary hover, click-only, new intent, content transit and unmount.
Existing parser/presentation tests remain compatibility controls.

## E2E tests

Extend `chat/chat-prompt-mention-hover.spec.ts` for desktop activation, keyboard,
dismissal, and fresh hover through shipped controls. Existing
`plugins/mobile-prompt-history-plugin.spec.ts` proves the Host wrapper's native
Drawer tap/dismissal path in a disposable managed fixture; extend only as needed
for repeat entry. Local reproduction uses retries zero, one shard/worker.

## Work orders

- [x] [Task 01: Preserve dismissal intent](task-01-preserve-dismissal.md)

## Verification results

[Task 01](task-01-preserve-dismissal.md#results) records independent causal RED
and final checks: 40 focused tests, two desktop cases and one native phone case
passed with retries zero; the phone capture rerun also passed and its settled
Drawer was inspected against UI-02. Scoped ESLint/Prettier, 8 GiB Node 24
typecheck under the shared guard, i18n, catalog/spec lint and documentation
coverage passed. The default 4 GiB typecheck attempt exhausted its heap and is
recorded separately. Both normal hooks remain required for the commit receipt.
The correction is ready for independent parent validation; no push or PR is
authorized by this package alone.

## Risks

Fresh pointer/focus must rearm opening; content transit must remain usable.
Controlled happy-dom timing proof alone is not a real-browser incident claim.

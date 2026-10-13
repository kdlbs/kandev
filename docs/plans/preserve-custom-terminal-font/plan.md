---
created: 2026-10-11
status: completed
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
legacy_specs: []
---

# Implementation Plan: Preserve Custom Terminal-Font Drafts

## Overview

Register raw custom-font input with the terminal contributor so an older successful
save cannot make a newer unblurred input clean. Deliver the local editor repair
and independently authored regression coverage in one sequential work order.
The active [requirement](../../specs/ui/requirements/settings-manual-save.md)
remains unchanged; AC-UI-SETTINGS-MANUAL-SAVE-001.1/.2/.4 already define the result.

## Evidence and assumptions

ROOT supplied a causal real-component reproduction requalified on authoritative
main `7229dbe15fb5f2aec37c9cf0c0f64fdd434e45ad`: real TerminalSettings, StateProvider,
ToastProvider, SettingsSaveProvider, and updateUserSettings client; only the
external PATCH fetch deferred. Starting with a custom font, edit and blur
SubmittedFont, start Save, type NewerFont without blur, then resolve PATCH 200.
The store accepts SubmittedFont and raw NewerFont remains visible, but the actual
coordinator reports `hasDirty=false`. Ordinary custom-font save and newer
parent-managed fontSize edits pass as controls. The Save availability consequence
is inferred from that clean registry and actual Saved action state; this package
requires independent rendered action assertions and a second explicit save.
This design turn did not rerun ROOT's probe or inspect its protected source.

Source inspection confirms `TerminalFontCard.onChange` only updates child-local
`customValue`; the parent family updates on blur/Enter. ACK sees an unchanged
parent draft and marks it saved. Updating the family on every keystroke alone is
insufficient: its current effect resets Custom mode and text for blank/preset
values. The UI catalog and adjacent designs provide no dedicated manual-save
system design; ADR 0046 is the existing coordination authority. Add the paired
vertical design and a UI boundary link instead of a separate repair specification.

Confirmed: bounded local ownership, no global redesign, existing ACs, one work
order, independent tests after later release, desktop/phone shared-save parity.
Conservative design choice: blank custom text retains the last accepted candidate
until canonical settlement or Reset; explicit Default is the clearing operation.
This preserves the existing nonempty blur acceptance policy.

## Scope

### In scope

- Parent-owned raw editor registration, accepted-boundary normalization, and
  exact captured-revision reconciliation in TerminalSettings/TerminalFontCard.
- Blank/custom/preset transitions, explicit Default, trimming, Reset, failure
  retry, and unchanged ordinary saves.
- Real-provider integration regressions and focused desktop/phone browser flows.
- Owning manual-save system design/index and this one-order delivery record.

### Out of scope

- Font libraries, xterm rendering, layout/markup redesign, APIs/backend, global
  save coordinator changes, unrelated settings publication/concurrency repairs.
- Requirements rewrites, new ACs, new flags, dependencies, or standalone repair spec.
- Reading/copying/deleting ROOT's protected candidate, old-source reconstruction,
  delegation, additional tasks/sessions/model switches, or foreign resource cleanup.

## Technical approach

Follow [Contributor registration and Submission and reconciliation](../../specs/ui/system-design/settings-manual-save.md#contributor-registration).
Keep raw text, mode, and the accepted family distinct in the parent editor. Include
raw edits in contributor revision immediately; mode-only entry does not dirty a
setting. Remove raw-character-driven mode reset. Preserve the complete snapshot
on a newer edit when ACK advances the baseline; the store gets the submitted
family. Normalize only at blur/Enter or immutable Save capture. Reset rebuilds
canonical mode and raw text from the latest accepted baseline even if the family
string is unchanged. Small helpers may stay file-local; do not introduce a reusable
framework or alter the coordinator. Keep other terminal draft fields and payloads.

## ASCII UI preview

UI-01: Terminal & Editors, after ACK of SubmittedFont with newer input still focused.
Shared card/control order on desktop and phone; illustrative spacing only.

```text
Before (source/probe):           After (required):
Terminal font                   Terminal font
[Custom... v]                   [Custom... v]
[NewerFont  ]                   [NewerFont  ]  dirty
Store: SubmittedFont            Store: SubmittedFont
Coordinator: clean              Coordinator: dirty
[Saved (disabled)]              [Reset] [Save changes]
```

The observed probe establishes clean coordinator and retained raw input; the
before action consequence follows source and must be independently tested.
While PATCH is pending, input stays editable and Save is disabled as today.
After the second Save of NewerFont, the contributor becomes clean. Structural
requirements: retained Custom input and shared action, not a new local button.

Phone entry: Settings index -> Terminal & Editors dedicated preferences page;
reuse its focused vertical cards, single existing content scroller, and fixed
safe-area-aware save surface. Desktop retains its surrounding settings navigation.
No composition, sizing, or scroll changes are planned. Phone tap and desktop click
prove the same state invariant; inspect the phone rendering during scoped E2E.
Preview maps to AC-UI-SETTINGS-MANUAL-SAVE-001.1/.2/.4.

## Tests

Author `apps/web/components/settings/terminal-settings-save.test.tsx` independently,
using real StateProvider, ToastProvider, SettingsSaveProvider, TerminalSettings,
and API client. A read-only coordinator/store observer may expose actual state;
it must not recreate save logic. Stub only external fetch; reject unexpected calls.

| Evidence | Test name | AC |
| --- | --- | --- |
| Causal RED, then GREEN | keeps newer unblurred custom font dirty and saves it explicitly | .1/.2/.4 |
| Control 1, passes before fix | saves an ordinary custom font without newer edits | .1/.2 |
| Control 2, passes before fix | keeps newer parent-managed font size dirty after font ACK | .4 |
| Affected GREEN | custom editor transition and normalization matrix | .1/.2/.4 |

The matrix covers empty/whitespace custom text, a full preset stack typed as raw
text, explicit preset/Default selection, leading/trailing spaces, Enter/blur,
Reset after ACK (including unchanged family), reverting raw text to baseline,
and failed first Save followed by explicit retry. Assert mode, raw value, dirty
registration, payload, and accepted baseline as appropriate; no helper-only proxy.

## E2E tests

Add `tests/settings/terminal-custom-font-save.spec.ts` (chromium) and
`tests/settings/mobile-terminal-custom-font-save.spec.ts` (mobile-chrome), sharing
a narrowly scoped `terminal-custom-font-save-helpers.ts` if needed. Test name in
both: `keeps newer custom font available for a second explicit Save`.
Seed a custom font through the API, intercept only the first settings PATCH,
await its actual admission, defer response delivery, type NewerFont without blur,
and release the response. Before any blur or second click/tap, assert raw value,
dirty marker, and enabled actual Save. Click/tap it; assert second PATCH family,
API saved value and reload persistence. Restore only test-owned settings in
finally, release pending routes on failure, and avoid sleeps. The mounted
font-size control covers normal sibling behavior; no broad existing E2E replay
is needed for unchanged markup.

## Work orders

- [x] [Task 01: Register and reconcile custom-font drafts](task-01-register-custom-font-drafts.md)

Sequential; no prerequisite work order. ROOT completed full design review and
issued the later explicit implementation interrupt/lease113 in this same primary
session on 2026-10-11. The once-only frozen-lock install from apps passed; exact RED,
controls, GREEN, normal scoped checks and staged i18n ratchet live in Task 01.
Retain every native handle to actual exit and respect ROOT's heavy-work lease.

## Verification results

ROOT approved the full design and released implementation on 2026-10-11.
The requirement remains unchanged. Independently authored mounted regressions
proved one causal RED and two passing controls before production edits.
Final affected GREEN passed 43 tests across four files; the final new-file run
passed all 14 transition/concurrency cases. Scoped ESLint and web typecheck
passed. Exact managed desktop and phone E2E each passed one test, one worker,
zero retries; phone reused unchanged backend and web artifact hashes. Both
captures were inspected. See Task 01 for commands and native exit receipts.

Specification catalog/lint, documentation coverage, staged i18n, and normal
commit-hook outcomes are recorded in Task 01. Hosted CI/review and later merge
permission remain external delivery work under ROOT supervision. No merge is
performed by this work order. All local validation handles were actually joined;
no owned validation command remains active.

## Risks

- Raw every-keystroke family updates can close Custom mode for blank/preset text.
- ACK and Reset must restore mode/text even when accepted family is unchanged.
- Browser click blur can hide the defect; assertions must precede second click.
- Additional sibling edits must survive the same ACK; no broad reconciliation fix.
- Public docs: the repair restores existing
  manual-save behavior with no labels/routes/API changes; `docs/public/developer-tools.md` remains accurate.

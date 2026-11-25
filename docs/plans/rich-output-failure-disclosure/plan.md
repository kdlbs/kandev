---
created: 2026-10-26
status: done
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-003
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/agents/system-design/agent-rich-output-failures.md
legacy_specs: []
---

# Implementation Plan: Rich-output failure disclosure

## Overview

Improve StatusList discovery and let users inspect failed visual-output calls
without adding persistent transcript text. Execute two work orders sequentially:
discovery first, then the desktop/phone disclosure slice.
The user selected the red cross with hover details and a click/tap popup.
No material product question remains open.

## Evidence and root cause

Read-only inspection on 2026-10-26 correlated the task
`e3b5c193-e6bb-431d-9226-f72a76ad22ec`, Kandev session
`0ae0bd41-5843-4e6d-922f-0832416bc3c3`, and native ACP session
`01a178dd-4a65-77e3-8edc-dbf2a10faa43`.

- At 15:43:51 UTC, call `exec-b616ef44-c2bf-44b3-9332-40216b8cccac`
  used StatusList status `success`. Raw ACP recorded `failed`; normalized
  ACP and SQLite recorded `error` with an enum-validation diagnostic.
- At 15:44:03 UTC, call `exec-96043331-ca3c-43bc-8278-4b9dc5842478`
  used `passed`. Raw ACP recorded `completed`; SQLite recorded `complete`
  with “Rich output accepted.”
- Evidence was read from `task_session_messages` in the instance database and
  the matching `raw-acp-codex-acp-<native-session>.jsonl` lines 427-431,
  plus normalized frames. These retained local logs are diagnostic evidence,
  not permanent test dependencies.

There is no observed status persistence or transport mismatch.
`RichOutputRenderer` receives the parsed diagnostic but returns a
non-expandable `KandevRow` for errors. `KandevStatusIcon` is a plain cross.
The generated description advertises `items:{label,status,detail?}[]` without
the status enum. That omission can encourage guessing; it does not prove the
model's internal reason for choosing an invalid value.

The existing AC 001.13 requires normal tool errors, but did not specify this
disclosure. New criteria 001.21-.23 define the user-selected interaction.
Criterion 003.14 makes the short-description enum guarantee explicit.

## Scope

### In scope

- Schema-derived StatusList enum guidance and a valid existing example.
- Rich-output-only failed-call tooltip and accessible error popup/drawer.
- Missing-detail fallback, plain-text rendering, replay and retry separation.
- Seven-language host copy and focused desktop/mobile regressions.

### Out of scope

- Accepting aliases, changing validation/redaction, automatic retry or recovery inference.
- Changing successful cards, grouping, badges, transport or stored statuses.
- Generalizing disclosure to every tool, database migrations or runtime flags.
- The independent debug-skill edits already present in this worktree.

## Technical approach

Use the existing catalog/guidance generator and the renderer's parsed result.
Add only a narrow optional status-control slot to `KandevRow`, a rich-output
error-disclosure component, and a bounded error-selection helper. Preserve
permission-indicator precedence. The detailed design defines accepted result
shapes, fallback, popup state, responsive branching and localization.

| Input evidence | Outcome | Verification | Unsupported fallback |
| --- | --- | --- | --- |
| Canonical MCP text result, including null structured content | Retained diagnostic | Component fixture through KandevToolMessage | Localized missing-details text |
| Existing recognized historic Kandev identity | Same disclosure on replay | Component and reload E2E | Existing generic route for unrecognized identity |
| Foreign or incomplete identity | Existing generic tool rendering | Negative matcher/control tests | No new provider inference |

Fixture coverage is not a claim of a live provider run.
Agents owns the vertical rich-output contract; no parallel UI requirement is created.
Existing native-output and json-render ADRs remain applicable.

## ASCII UI preview

UI-01: Failed call, desktop resting state (AC 001.21).
The visible row stays compact:

```text
[chart] Kandev: visual output  [x]
[successful report card stays unchanged]
```

UI-02: Desktop cross hovered/focused, then activated (AC 001.21).
The tooltip shows a short error. Click or Enter/Space opens the popup:

```text
[chart] Kandev: visual output  [x]
                              +---------------------------+
                              | Visual output error [Close]|
                              | Retained validation error |
                              | (wrap; scroll if long)    |
                              +---------------------------+
```

UI-03: Phone cross tapped (AC 001.22).

```text
+--------------------------------+
| Conversation behind backdrop   |
|                                |
| +----------------------------+ |
| | Visual output error [Close]| |
| | Retained validation error  | |
| | One wrapping scroll region | |
| |                            | |
| +------ safe-area space -----+ |
+--------------------------------+
```

The title/close row stays fixed. Only the diagnostic body scrolls.
Closed rows have no inline error copy. Cross and close targets measure at least
44px on phones/coarse pointers; desktop retains compact sizing.
Popover versus inset Drawer, error ownership and content order are required.
Spacing and illustrative English labels are not pixel/copy specifications.
Use localized host labels and existing primitives.


## Tests

- Task 01: `guidance.test.ts` test “includes schema-derived StatusList statuses”
  fails before the change. Mutation of a schema enum proves derivation.
  Generator and Go registration tests verify the advertised artifact and budget.
  Existing schema acceptance/rejection preserves AC 003.6, .10, .13.
- Task 02: `kandev-tool-message-rich-output-errors.test.tsx` test
  “opens the retained failed-call error without changing a successful retry”
  fails before the change. Add missing/structured/text/long/plain-text,
  permission-precedence and status-negative controls. Covers AC 001.13, .21-.23.

## E2E tests

New `chat/rich-output-errors.spec.ts` (chromium) and
`chat/mobile-rich-output-errors.spec.ts` (mobile-chrome) seed distinct failed
and successful tool messages using existing rich-output fixture patterns.
Desktop proves hover, focus, click/keyboard activation, Escape, close and focus
return. Phone proves actual Drawer composition, tap, dismissal and minimum
44px targets. Both prove reload and retained success-card state. Long errors
remain inside the viewport with one internal scroll owner. Include phone
fine-pointer width and 767/768px boundary checks. Save screenshots for UI-02/03.
No live user task is mutated.

## Work orders

- [x] [Task 01: Clarify StatusList discovery](task-01-status-discovery.md)
- [x] [Task 02: Disclose failed rich-output errors](task-02-error-disclosure.md)

## Verification results

Design and implementation validation on 2026-10-26:

- `python3 scripts/list-docs.py validate`: passed (403 decisions, 1741 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `validateCoverage` from `.github/scripts/pr-docs.cjs`, supplied both work
  orders, their linked documents and planned production paths: covered, no errors.
- Task 01 targeted web and backend tests, generated-artifact checks, targeted ESLint,
  changed-Go lint (`0 issues`), and `git diff --check`: passed.
- Task 02 component/helper tests: 56 passed. I18n conversion, pseudo generation,
  completeness checks, new-code ratchet, targeted ESLint and Prettier passed.
- Web typecheck passed with an 8 GB Node heap. The default 4 GB heap exhausted
  memory during the first attempt.
- Chromium rich-output error E2E passed. Mobile Chromium passed both disclosure
  tests. Managed E2E builds completed successfully.
- PR review remediation on 2026-10-26: a red unit regression reproduced the
  open-popover toggle defect, then all 7 targeted component tests passed with
  `PopoverTrigger` in place. The 8 GB web typecheck, targeted lint/format checks,
  and fresh-build desktop (1 test) and mobile (2 tests) E2E runs passed. The
  desktop and phone screenshots were recaptured from the fixed build.
- Public-doc tests passed (63 tests), public-doc validation passed (54 pages),
  specification validation and lint passed, and `git diff --check` passed.

## Documentation impact

Task 02 added one short error-inspection instruction to the Native rich output
section in `docs/public/automation-and-mcp.md`.
Existing companion packages (replacement, evidence, authoring recovery,
identity and file-preview fixes) retain their completed scope and results;
this additive package does not reopen their work orders.

## Risks

- Guidance must stay within 650 runes without losing existing routing/shape facts.
- Shared row changes must preserve permission precedence and neighboring tool behavior.
- Plain-text diagnostics can be long; never dump arguments or arbitrary result objects.
- A tooltip alone is not mobile support. The Drawer and target geometry need rendered proof.

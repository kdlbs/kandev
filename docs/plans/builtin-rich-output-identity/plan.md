---
created: 2026-11-05
status: done
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
  - REQ-UI-KANDEV-MCP-TOOL-RESULTS-001
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
legacy_specs: []
---

# Implementation Plan: Built-in rich-output identity

## Overview

Restore native transcript rendering after the built-in MCP package split.
One sequential work order corrects shared identity recognition and proves
dispatch, grouping, and replay. Existing requirements define the intended
behavior. No new requirement, persistence change, or ADR is needed.

## Confirmed root cause and evidence

The user reported missing file previews in task
`01936404-ebc5-4c66-9090-9de39d1c3e78`, "Add MCP selection to automations".
Read-only SQLite inspection found session
`a0c21b5e-c960-49f1-9c5a-776af07e63f1`, using `codex-acp`.
Message `e66a3b94-56cf-4dc8-9be5-3921aa70fb3b`, recorded on
2026-11-04 at 20:12:21 UTC, has status `complete` and result
`Rich output accepted.` Its normalized name is
`kandev-rich-output/show_rich_output_kandev`.

The stored composition has a vertical Stack and two FilePreview elements.
Both referenced PNG files exist in the task workspace. Its title is
"Automation MCP UI previews". The saved input passes `normalizeRichOutput`.
`kandevToolStemOf` returns null because `extractKandevStem` recognizes only
the monolithic server namespace. Renderer dispatch and standalone transcript
grouping therefore miss the accepted presentation.

A temporary test against the saved message failed at that exact boundary.
A control changed only the normalized name to
`kandev/show_rich_output_kandev` and passed. Earlier calls in the same session
used that old name. Later accepted StatusList and FilePreview calls use the
new package name. This is persisted-message evidence, not a raw ACP capture.
No matching raw ACP files were located in the inspected default log directory.

The running instance reports `v0.98.0-207-g5d6adcbca`. The package split
entered in `a1013e89f` / PR #4762, after the `v0.98.0` tag. This confirmed
local defect does not establish the cause of the separate original 0.98.0
table report. That report's copied title exceeds the 120-character bound.

## Scope

Recognize exact built-in package names through the existing shared parser.
Preserve monolithic history, bare names, foreign-provider rejection, strict
payload validation, and existing desktop/phone rendering. Cover other built-in
package names at the shared parser boundary because they use the same helper.
Do not change agent adapters, tool execution, payload limits, stored records,
file authorization, or visual components. Do not broaden third-party trust.

## Technical approach

Update `messages/kandev/parse.ts`. Keep the historical provider fallback guard
in chat `types.ts` on its original `kandev`-only rule and apply it to both
explicit server fields: legacy imported external definitions can retain new
reserved names. Current calls use normalized qualified identities. The
canonical server inventory is `apps/backend/internal/mcp/profile/builtin_descriptors.go`.
Do not reuse the MCP explorer's broader reserved-name policy: reserving a name
does not establish a native renderer identity.

| Serialized identity | Intended result | Evidence and limit |
| --- | --- | --- |
| `kandev-rich-output/show_rich_output_kandev` | Native report | Observed persisted Codex call |
| Exact built-in names in existing dotted or double-underscore formats | Registered renderer | Parser fixtures, not new live-provider claims |
| Exact built-in names in colon-space or hyphenated titles | Registered renderer with existing foreign guard | Historical-format fixtures |
| Monolithic `kandev` and accepted bare names | Existing behavior | Compatibility controls |
| Foreign, lookalike, malformed, or unknown renderer identities | Generic activity | Negative parser and full-consumer controls |

Longer package prefixes must match before the legacy `kandev-` prefix.
Keep normalized foreign identity authoritative over a trusted-looking title.
Do not infer identity from the `_kandev` suffix alone for qualified names.

## UI outcome

Entry point: completed task or Office transcript call, live or after reload.
The existing report appears as a standalone item between surrounding activity
groups. Its title, description, and ordered preview cards remain unchanged.
Desktop and phone reuse the current native rich-output components, the shared
transcript scroll owner, and existing file actions. This is an identity-routing
repair with no new layout, controls, motion, or preview design.

## Tests

- Parser and message predicates: exact package identities, legacy controls,
  lookalikes, explicit foreign provider, unknown stem, and separator ambiguity.
- Full consumer: valid FilePreview, Table, and StatusList inputs with accepted
  MCP results select their existing renderer. Payload validation remains strict.
- Grouping: completed reports remain standalone between ordinary tool groups.
  Pending and failed calls retain their existing behavior.

These cover AC-AGENTS-AGENT-RICH-OUTPUT-001.1, .13, .17, .18,
AC-AGENTS-AGENT-RICH-OUTPUT-002.6, .9, and
AC-UI-KANDEV-MCP-TOOL-RESULTS-001.2, .3.

## E2E tests

Add desktop and phone fixtures with the observed normalized package name.
Do not substitute an unqualified tool title. Verify a visible standalone
report, two valid workspace image previews, a table, surrounding ordinary tool
activity, and reload. Verify a foreign-name control stays generic. Use generated
fixture data, not a developer's live task or database.

Files: `tests/chat/builtin-rich-output.spec.ts` and
`tests/chat/mobile-builtin-rich-output.spec.ts`, with a shared helper.
Projects: `chromium` and `mobile-chrome`, respectively. These prove the stored
message rendering path, not live provider wire behavior.

## Work orders

- [x] [Task 01: Restore built-in package rendering](task-01-restore-package-rendering.md)

## Verification results

Diagnostic test: one expected failure for the exact persisted package identity,
one passing legacy-name control. Temporary reproduction files are removed.
Implementation and desktop/phone browser verification passed. Public documentation
needs no change because this repair restores already documented behavior.
Existing completed companion plans retain their historical results and status.

Design checks passed before implementation: `list-docs.py validate`, `lint-spec-files.py --all`,
`git diff --check`, and prospective implementation coverage through
`.github/scripts/pr-docs.cjs` (`covered`, no errors). The coverage check used
the planned parser edit as an input without modifying production code.

## Risks

- A broad prefix match could misclassify a foreign server as native.
- Legacy `kandev-` matching could consume a longer package prefix.
- Tests with bare names can pass while real qualified names still fail.
- File contents remain workspace-owned. Routing recovery cannot restore deleted files.

## Implementation outcome

The shared parser now recognizes the five exact built-in server names. The
historical provider fallback guard still accepts only `kandev`. Existing payload validation,
foreign-provider rejection, and native components are unchanged. Saved valid
reports can render after loading the patched frontend. The running user instance
was not restarted or modified.

The initial four focused unit suites passed (83 tests). Review remediation
adds eight provider-collision and Codex fallback cases (91 tests total); see
the work-order results. Desktop Chromium and mobile
Chrome each passed the live-and-reload scenario with two loaded image previews,
a table, neighboring activity groups, and a foreign-name control. Type checking,
focused lint, specification checks, and documentation coverage passed. See the
[work-order results](task-01-restore-package-rendering.md#results) for commands
and test-environment corrections.

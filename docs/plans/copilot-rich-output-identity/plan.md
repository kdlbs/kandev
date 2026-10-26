---
created: 2026-10-26
status: done
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
  - REQ-AGENTS-AGENT-RICH-OUTPUT-002
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
legacy_specs: []
---

# Implementation Plan: Copilot rich-output identity

## Overview

Restore native rich-output rendering for the Copilot ACP message shape in
[issue #4800](https://github.com/kdlbs/kandev/issues/4800). One sequential work
order repairs the shared identity parser and proves dispatch, grouping, and replay.

The issue violates existing criteria; no new product requirement is needed.
Use AC-AGENTS-AGENT-RICH-OUTPUT-001.1, .13, .17, .18 and
AC-AGENTS-AGENT-RICH-OUTPUT-002.6, .9. The owning requirements remain active.
The shared UI design now specifies the missing title compatibility path.

## Confirmed root cause

Source baseline: `b4e9cf5ede3`. The issue has no image attachments.
`extractKandevStem("kandev-show_rich_output_kandev")` returns
`kandev-show_rich_output`; the registry contains `show_rich_output`.
`kandevToolStemOf` falls back from `generic.name: "other"` to this title.
Both `hasKandevRenderer` and `isRichOutputMessage` therefore miss the report.
`buildGroupedRenderItems` consumes it as ordinary activity.

The smallest reproduction imports `parse.ts` with Node TypeScript stripping
and compares the hyphenated title with `kandev/show_rich_output_kandev`.
The latter returns `show_rich_output`. Direct `input.raw_input` already unwraps
correctly, so argument parsing needs regression coverage rather than new logic.

An in-memory candidate added only the exact `kandev-` prefix. Six supported
identities and nine negative identities passed assertions. Direct report
arguments retained object identity. No production or permanent test file changed.
This is parser evidence, not a live Copilot or browser reproduction.

## Scope

### In scope

- Recognize the exact observed title through the shared identity parser.
- Render both new messages and stored history without rewriting metadata.
- Keep reports standalone and ordinary tools grouped in chronological order.
- Preserve foreign-provider guards and every existing supported name format.
- Prove the exact version 2 Callout fixture on desktop and phone.

### Out of scope

- A Copilot adapter dialect inferred from unverified raw wire frames.
- Database migrations, transcript rewrites, or repeated MCP calls.
- Rich-output schemas, new copy, responsive layout, or interaction changes.
- The separate text-delivery issue #4788.

## Technical approach

Add the exact prefix to `KANDEV_NAMESPACES` in
`apps/web/components/task/chat/messages/kandev/parse.ts`.
Retain non-empty stem, suffix, and namespace-separator validation.
Do not split arbitrary hyphens or accept names by substring.

Keep `kandevToolStemOf` in `components/task/chat/types.ts` as the single scanner.
Registry dispatch and grouping must continue to consume its result. Preserve
normalized foreign identity precedence and the existing raw provider guard.
Unknown stems can remain unregistered; they must never select rich output.

The saved fixture uses title/content `kandev-show_rich_output_kandev`, status
`complete`, generic name `other`, input `{kind: "other", raw_input: args}`,
and output `{content: "Rich output accepted."}`. Use the issue's version 2,
catalog 1 Callout arguments with title `Synthetic report` and text
`Synthetic check completed.`. Do not convert the fixture to canonical metadata.

| Provider / transport shape | Intended result | Evidence / fallback |
| --- | --- | --- |
| Copilot ACP observed title, `other`, direct `raw_input` | Native standalone card | New exact stored-shape tests; other wire shapes unclaimed |
| Cursor/Grok ACP envelope or `kandev: ` history | Existing native card | Existing tests remain green |
| Codex slash/dotted and Claude double-underscore names | Existing native card | Parser and renderer controls remain green |
| Bare suffixed Kandev names | Existing behavior | Parser controls remain green |
| Foreign provider, malformed prefix, unknown stem | No native rich-output card | Negative scanner and dispatch controls |

Existing linked plans remain historical delivery records. The completed
`native-kandev-mcp-rendering` package covers Cursor/Grok, not this Copilot shape.
The json-render and structured-evidence packages own the current renderer and
Callout contract. Their completed scope and results do not change.

## ASCII UI preview

### UI-01: Completed report in task chat

Entry point: task conversation, live completion or history reload.
Applies to AC-AGENTS-AGENT-RICH-OUTPUT-001.1, .13, .17, .18 and
AC-AGENTS-AGENT-RICH-OUTPUT-002.6.

```text
Before (source-traced):
[Tool activity >]
  kandev-show_rich_output_kandev -> generic JSON

After (desktop and phone):
[Tool activity >] ordinary tools before
+ Synthetic report ----------------------+
| Info: Result                           |
| Synthetic check completed.             |
+----------------------------------------+
[Tool activity >] ordinary tools after
```

The report is inline, standalone, and chronological. These are structural
requirements; spacing is illustrative. Reuse the shipped rich-output Callout
and phone transcript. Text wraps within the viewport; the transcript remains
the primary vertical scroll owner. No new controls or phone navigation exist.
Pending and failed calls retain their existing compact activity/error surfaces.

## Tests

- `messages/kandev/parse.test.ts`: `recognizes the Copilot Kandev prefix` and
  `preserves direct Copilot raw_input arguments`; supported and negative names.
- `components/task/chat/types.test.ts`: `recognizes Copilot title and content`
  and `rejects foreign identity despite Copilot title`; covers criterion 002.9.
- `messages/kandev-tool-message.test.tsx`: registry dispatch and unknown stems.
- `messages/kandev-tool-message-rich-output.test.tsx`: `renders the saved Copilot
  Callout`; assert report, title, and text with no unavailable state (001.1, 002.6).
- `hooks/use-processed-messages-rich-output.test.ts`: `keeps Copilot output
  between ordinary activity groups`; assert ordered group/report/group items
  and both neighbors (001.13). Cover pending/error without changing their UI.

Message paths above are relative to `apps/web/components/task/chat`;
`components/` and `hooks/` paths are relative to `apps/web`.

## E2E tests

Add `tests/chat/copilot-rich-output.spec.ts` for `chromium` and
`tests/chat/mobile-copilot-rich-output.spec.ts` for `mobile-chrome`.
Share fixture setup in `tests/chat/copilot-rich-output-helpers.ts`.
Use the existing isolated backend and `ApiClient.seedSessionMessage` patterns.
Seed the exact completed Copilot metadata while chat is open, then assert the
native Callout without reloading. Reload and assert the same report once,
with the same values and chronological placement. Include ordinary tools
before and after the report and prove they remain grouped. The phone test
also asserts viewport containment and no document horizontal overflow.
These cover 001.1, 001.13, 001.17, 001.18, 002.6, and 002.9 through unit controls.
A seeded live message proves UI delivery, not a real Copilot ACP execution.

## Work orders

- [x] [Task 01: Recognize Copilot rich-output identity](task-01-recognize-copilot-identity.md)

## Verification results

Implementation evidence: the shared parser recognizes the exact `kandev-`
prefix and retains the existing namespace, suffix, registry, and provider
guards. The focused unit suite passes 91 tests across five files. Web typecheck
passes with an 8 GiB Node heap; targeted ESLint passes. Desktop and phone E2E
both pass against rebuilt production assets, covering mounted delivery,
chronological activity grouping, reload replay, Callout content, phone
containment, and horizontal overflow. These browser cases use synthetic
Copilot metadata and do not claim a live Copilot ACP execution.

Specification validation passes for 403 decisions and 1740 specifications;
`lint-spec-files.py --all`, `git diff --check`, and the PR documentation
coverage preflight pass. The changed work order reports `covered` against the
linked plan, requirements, criteria, and designs. The planning checkpoint
preceded implementation and commit; later verification is recorded below.

Subsequent pull request fixup: both browser scenarios now arm a
`session.message.added` wait before seeding, filtered by session, `tool_call`
type, and the exact report title. After that delivery event, the report
assertion uses Playwright's default timeout. Desktop and phone scenarios pass
with retries disabled; web typecheck and targeted ESLint pass. This only
changes E2E synchronization and does not change product behavior. The initial
pull request CI failures in unrelated mention-refresh and session-tab-restart
scenarios passed retry-free local reproductions; their initial CI result is
superseded by verification on the updated pull request head. The initial
walkthrough generation reached its workflow deadline and was superseded by a
successful updated-head run. After the failed shard jobs were rerun, `pr-await`
reported 63 passed checks, 18 skipped, and zero failed or pending checks. The
three rerun shard reports contained 440 passes and four skips with no retries.

## Risks

- Broad hyphen parsing could select foreign tools. Match only the exact prefix
  and retain existing namespace guards and registry membership checks.
- A parser-only test could miss grouping or argument loss. Exercise the saved
  message through the renderer and real grouping function, then browser replay.
- The issue contains serialized-message evidence, not a verified Copilot wire
  envelope. Do not expand backend scope without new evidence and a plan update.

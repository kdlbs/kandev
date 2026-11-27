---
created: 2026-09-25
status: complete
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
  - ../../specs/ui/system-design/kandev-mcp-tool-results.md
legacy_specs: []
---

# Implementation Plan: Native Kandev MCP Rendering

## Overview

Normalize Cursor-style MCP identity and arguments at the ACP adapter boundary
for Cursor and Grok, then keep the frontend tolerant of already-persisted
envelopes. The
adapter correction comes first so new messages follow the existing
provider-neutral contract; shared replay parsing follows so stored history
renders without a migration or repeat tool call.

## Confirmed root cause

Cursor emits a broad ACP `other` category while the actual MCP provider, tool,
and arguments live in `rawInput.providerIdentifier`, `rawInput.toolName`, and
`rawInput.args`. The Cursor dialect does not recognize that envelope, so the
adapter persists `generic.name: "other"` and wrapped arguments.

For historic messages, the frontend does not recognize the observed
`kandev: show_rich_output_kandev` title and does not unwrap `raw_input.args`.
The renderer registry therefore misses, transcript grouping consumes the call
as generic activity, and the rich-output parser would reject the wrapper even
if dispatch succeeded.

The smallest reliable reproduction is a completed `tool_call` fixture with
title/content `kandev: show_rich_output_kandev`, no `metadata.tool_name`,
`generic.name: "other"`, and a version 1 presentation under
`generic.input.raw_input.args`.

## Scope

### In scope

- Recognize the complete Cursor MCP envelope in the Cursor ACP dialect.
- Persist new Cursor calls with provider-qualified tool identity and unwrapped
  arguments through the existing generic MCP payload.
- Recognize and unwrap the exact historic Cursor shape in shared frontend
  Kandev tool parsing.
- Keep every registered native Kandev MCP renderer on the same identity path.
- Prove rich output remains standalone and renders from replayed metadata.

### Out of scope

- Rich-output schema, blocks, charts, settings, or unavailable-state changes.
- Database migration or stored-message rewrite.
- Generic rendering for foreign MCP tools.
- New copy, navigation, layout, touch, scrolling, or breakpoint behavior.
- Requiring the original agent to call the tool again.

## Technical approach

### ACP adapter normalization

Add an MCP call parser to the Cursor and Grok dialects. The parser accepts only
a complete map containing non-empty
`providerIdentifier`, non-empty `toolName`, and object-valued `args`. It returns
the existing `<provider>/<tool>` identity and unwrapped arguments through
`streams.NewMCPTool`.

This preserves provider identity, keeps ordinary `other` frames generic, and
prevents a foreign provider from selecting Kandev's native renderer.

### Historic frontend replay

Extend `extractKandevStem` with the observed `kandev: ` namespace. The prefix is
accepted only through the existing suffix and separator validation, so
`kandev: Edit`, `mcp__github__...`, and foreign provider identities remain
unrecognized.

Extend `extractKandevArgs` precedence to use `raw_input.arguments`, then
`raw_input.args`, then the existing raw/direct fallbacks. `kandevToolStemOf`,
`hasKandevRenderer`, `isRichOutputMessage`, grouping, and rendering continue to
share these helpers rather than adding a rich-output-only branch.

## ASCII UI preview

### UI-01: Completed native result in transcript

Entry point: task or Office transcript after replay.
Applies to `AC-AGENTS-AGENT-RICH-OUTPUT-001.1`.

Before:

```text
┌ Tool activity (collapsed)                              ┐
│ kandev: show_rich_output_kandev                       │
│   expanded: raw JSON                                  │
└────────────────────────────────────────────────────────┘
```

After:

```text
┌ Build health                                          ┐
│ [native chart and metric blocks from persisted args]  │
└────────────────────────────────────────────────────────┘
```

The native card remains one standalone chronological transcript item. Desktop
and phone keep the existing shared inline composition; the transcript remains
the sole scroll owner. Spacing is illustrative, and no responsive geometry or
interaction changes.

## Tests

- `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_test.go`
  reproduces the complete Cursor-style envelope for Cursor and Grok and proves
  canonical provider/tool identity plus unwrapped arguments. Negative cases
  retain foreign identity and leave malformed or ordinary `other` frames
  generic.
- `apps/web/components/task/chat/messages/kandev/parse.test.ts` proves
  `raw_input.args` unwrapping and guarded `kandev: ` stem extraction while the
  Codex `arguments` case remains green.
- `apps/web/components/task/chat/types.test.ts` proves the stored message maps
  to `show_rich_output` and is a rich-output message; foreign and unrelated
  tools remain rejected.
- `apps/web/components/task/chat/messages/kandev-tool-message.test.tsx` proves
  registry recognition and rendered HTML containing
  `data-testid="rich-output"` and the persisted presentation title.
- `apps/web/hooks/use-processed-messages-rich-output.test.ts` proves the exact
  stored message stays outside generic activity grouping.

## E2E tests

Existing `apps/web/e2e/tests/chat/rich-output.spec.ts` and
`apps/web/e2e/tests/chat/mobile-rich-output.spec.ts` already prove the shared
native card, replay, and desktop/mobile user outcome. This repair changes only
serialized identity and argument normalization, so the exact persisted unit
fixture provides the missing regression evidence while those existing flows
remain the rendered surface checks.

## Work orders

- [x] [Task 01: Normalize Cursor Kandev MCP calls](task-01-normalize-cursor-kandev-mcp.md)

## Verification results

Focused Go and Vitest regressions pass, including adapter normalization,
historic replay recognition, standalone transcript grouping, and native-card
rendering. Formatting, type checking, lint, specification validation, and
whitespace checks pass. Existing desktop and mobile rich-output E2E suites
pass.

The repository-wide backend test run reached completion with two unrelated,
reproducible environment failures: the installed npm version emits a project
configuration warning into a registry assertion, and an LSP WebSocket test
does not receive its expected close error. All changed backend package tests
pass.

## Risks

- Broad title-prefix matching could misclassify unrelated tools; suffix and
  namespace validation must remain fail-closed.
- Treating every Cursor `other` frame as MCP could corrupt shell or generic
  activity; recognition requires the complete provider envelope.
- Fixing only the adapter would not repair persisted history; fixing only the
  frontend would keep producing non-canonical new messages.
- Adapter updates must retain trusted in-memory MCP provenance so terminal
  updates preserve the canonical input.
